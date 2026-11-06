// Package api is the hub's HTTP interface. Every client (the phone apps, a
// web panel, scripts) uses the same endpoints regardless of device brand.
package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/daniel-j-meta/jomaa-everything/jomaa-home-hub/internal/device"
	"github.com/daniel-j-meta/jomaa-everything/jomaa-home-hub/internal/hub"
	"github.com/daniel-j-meta/jomaa-everything/jomaa-home-hub/internal/matter"
)

const (
	authCookie     = "hub_auth"
	commandTimeout = 15 * time.Second
	pairingTimeout = 5 * time.Minute
)

// Matter is the slice of the Matter client the API uses.
type Matter interface {
	Connected() bool
	Nodes() []matter.NodeSummary
	Commission(ctx context.Context, code string, networkOnly bool) (json.RawMessage, error)
	SetWiFiCredentials(ctx context.Context, ssid, password string) error
	SetThreadDataset(ctx context.Context, dataset string) error
	RemoveNode(ctx context.Context, nodeID uint64) error
}

// Server serves the hub API.
type Server struct {
	hub     *hub.Hub
	matter  Matter
	token   string
	log     *slog.Logger
	limiter *loginLimiter
}

// New builds the API. password must be non-empty; clients exchange it for a
// session token at POST /api/login.
func New(h *hub.Hub, m Matter, password string, log *slog.Logger) *Server {
	return &Server{hub: h, matter: m, token: sessionToken(password), log: log, limiter: newLoginLimiter()}
}

// sessionToken derives the token from the password, so the password itself is
// never stored in a cookie and changing it signs everyone out.
func sessionToken(password string) string {
	mac := hmac.New(sha256.New, []byte(password))
	mac.Write([]byte("jomaa-home-hub session v1"))
	return hex.EncodeToString(mac.Sum(nil))
}

// Handler returns the routes, with auth on everything under /api/ except login.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.logout)

	authed := http.NewServeMux()
	authed.HandleFunc("GET /api/targets", s.listTargets)
	authed.HandleFunc("GET /api/targets/{id}/state", s.targetState)
	authed.HandleFunc("POST /api/targets/{id}/command", s.command)
	authed.HandleFunc("POST /api/targets/{id}/sync", s.sync)
	authed.HandleFunc("GET /api/matter/nodes", s.matterNodes)
	authed.HandleFunc("DELETE /api/matter/nodes/{nodeId}", s.matterRemove)
	authed.HandleFunc("POST /api/matter/commission", s.matterCommission)
	authed.HandleFunc("POST /api/matter/wifi", s.matterWiFi)
	authed.HandleFunc("POST /api/matter/thread", s.matterThread)
	mux.Handle("/api/", s.requireAuth(authed))
	return mux
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.authed(r) {
			writeError(w, http.StatusUnauthorized, "sign in first")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authed(r *http.Request) bool {
	got := ""
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		got = strings.TrimPrefix(h, "Bearer ")
	} else if c, err := r.Cookie(authCookie); err == nil {
		got = c.Value
	}
	return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) == 1
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "matterConnected": s.matter.Connected()})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if !decode(w, r, &body) {
		return
	}
	client := clientIP(r)
	if ok, wait := s.limiter.allow(client); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "too many failed sign-ins; try again later")
		return
	}
	if subtle.ConstantTimeCompare([]byte(sessionToken(body.Password)), []byte(s.token)) != 1 {
		s.limiter.fail(client)
		s.log.Warn("failed sign-in", "client", client)
		writeError(w, http.StatusUnauthorized, "wrong password")
		return
	}
	s.limiter.succeed(client)
	// Secure only over HTTPS (directly or through the tunnel): on the home
	// network the hub is often plain HTTP, and a Secure cookie would never be
	// sent back.
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: Secure depends on the request, see above
		Name: authCookie, Value: s.token, Path: "/", HttpOnly: true, Secure: isHTTPS(r),
		SameSite: http.SameSiteLaxMode, MaxAge: 30 * 24 * 60 * 60,
	})
	writeJSON(w, http.StatusOK, map[string]any{"token": s.token})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: Secure depends on the request, as in login
		Name: authCookie, Value: "", Path: "/", HttpOnly: true, Secure: isHTTPS(r),
		SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) listTargets(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"targets": s.hub.List()})
}

func (s *Server) targetState(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), commandTimeout)
	defer cancel()
	res, err := s.hub.State(ctx, r.PathValue("id"))
	if err != nil {
		s.hubError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type commandBody struct {
	device.Command
	ClientID string `json:"clientId"`
	Revision *int64 `json:"revision"`
}

// command answers 200 when every device took the command, and 502 with the
// failures otherwise, so a client can roll back its optimistic update.
func (s *Server) command(w http.ResponseWriter, r *http.Request) {
	var body commandBody
	if !decode(w, r, &body) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), commandTimeout)
	defer cancel()
	failed, err := s.hub.Command(ctx, r.PathValue("id"), body.Command, body.ClientID, body.Revision)
	if err != nil {
		s.hubError(w, err)
		return
	}
	if len(failed) > 0 {
		s.log.Warn("command partly failed", "target", r.PathValue("id"), "failed", failed)
		writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "failed": failed})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) sync(w http.ResponseWriter, r *http.Request) {
	var body hub.Desired
	if !decode(w, r, &body) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), commandTimeout)
	defer cancel()
	res, err := s.hub.Sync(ctx, r.PathValue("id"), body)
	if err != nil {
		s.hubError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) matterNodes(w http.ResponseWriter, _ *http.Request) {
	if !s.matterReady(w) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": s.matter.Nodes()})
}

func (s *Server) matterRemove(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("nodeId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "nodeId must be a number")
		return
	}
	if !s.matterReady(w) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), commandTimeout)
	defer cancel()
	if err := s.matter.RemoveNode(ctx, id); err != nil {
		s.matterError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) matterCommission(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code        string `json:"code"`
		NetworkOnly bool   `json:"networkOnly"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Code == "" {
		writeError(w, http.StatusBadRequest, "code is required")
		return
	}
	if !s.matterReady(w) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), pairingTimeout)
	defer cancel()
	node, err := s.matter.Commission(ctx, body.Code, body.NetworkOnly)
	if err != nil {
		s.matterError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"node": node})
}

func (s *Server) matterWiFi(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SSID     string `json:"ssid"`
		Password string `json:"password"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.SSID == "" {
		writeError(w, http.StatusBadRequest, "ssid is required")
		return
	}
	s.matterCall(w, r, func(ctx context.Context) error {
		return s.matter.SetWiFiCredentials(ctx, body.SSID, body.Password)
	})
}

func (s *Server) matterThread(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Dataset string `json:"dataset"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Dataset == "" {
		writeError(w, http.StatusBadRequest, "dataset is required")
		return
	}
	s.matterCall(w, r, func(ctx context.Context) error {
		return s.matter.SetThreadDataset(ctx, body.Dataset)
	})
}

func (s *Server) matterCall(w http.ResponseWriter, r *http.Request, fn func(context.Context) error) {
	if !s.matterReady(w) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), commandTimeout)
	defer cancel()
	if err := fn(ctx); err != nil {
		s.matterError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) matterReady(w http.ResponseWriter) bool {
	if !s.matter.Connected() {
		writeError(w, http.StatusServiceUnavailable, matter.ErrNotConnected.Error())
		return false
	}
	return true
}

func (s *Server) matterError(w http.ResponseWriter, err error) {
	var se *matter.ServerError
	switch {
	case errors.Is(err, matter.ErrNotConnected):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	case errors.As(err, &se):
		writeError(w, http.StatusBadGateway, err.Error())
	default:
		writeError(w, http.StatusGatewayTimeout, err.Error())
	}
}

func (s *Server) hubError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, hub.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, device.ErrInvalidCommand):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

// decode reads a JSON body, rejecting unknown fields so a typo like
// "brightnes" fails loudly instead of doing nothing.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "bad request body: "+err.Error())
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}
