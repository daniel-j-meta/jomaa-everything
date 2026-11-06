package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jomaa-daniel/jomaa-everything/jomaa-home-hub/internal/device"
	"github.com/jomaa-daniel/jomaa-everything/jomaa-home-hub/internal/hub"
	"github.com/jomaa-daniel/jomaa-everything/jomaa-home-hub/internal/matter"
)

type light struct {
	id  string
	err error
	mu  sync.Mutex
	s   device.State
}

func (l *light) ID() string   { return l.id }
func (l *light) Name() string { return l.id }
func (l *light) Apply(_ context.Context, c device.Command) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err != nil {
		return l.err
	}
	if c.On != nil {
		l.s.On = *c.On
	}
	if c.Brightness != nil {
		l.s.Brightness = *c.Brightness
	}
	return nil
}
func (l *light) State(_ context.Context) (device.State, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.s, l.err
}

type fakeMatter struct {
	connected bool
	code      string
	err       error
}

func (m *fakeMatter) Connected() bool { return m.connected }
func (m *fakeMatter) Nodes() []matter.NodeSummary {
	return []matter.NodeSummary{{NodeID: 1, Available: true}}
}
func (m *fakeMatter) Commission(_ context.Context, code string, _ bool) (json.RawMessage, error) {
	m.code = code
	return json.RawMessage(`{"node_id":5}`), m.err
}
func (m *fakeMatter) SetWiFiCredentials(_ context.Context, _, _ string) error {
	return m.err
}
func (m *fakeMatter) SetThreadDataset(_ context.Context, _ string) error { return m.err }
func (m *fakeMatter) RemoveNode(_ context.Context, _ uint64) error       { return m.err }

type env struct {
	srv   *httptest.Server
	token string
	a, b  *light
	m     *fakeMatter
}

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{a: &light{id: "a"}, b: &light{id: "b"}, m: &fakeMatter{connected: true}}
	h := hub.New()
	_ = h.Add("test", e.a)
	_ = h.Add("test", e.b)
	_ = h.Add("room", device.NewGroup("room", "Room", []device.Device{e.a, e.b}))
	e.srv = httptest.NewServer(New(h, e.m, "hunter2", slog.New(slog.NewTextHandler(io.Discard, nil))).Handler())
	t.Cleanup(e.srv.Close)

	status, body := e.do(t, "POST", "/api/login", `{"password":"hunter2"}`, "")
	if status != 200 {
		t.Fatalf("login: %d %v", status, body)
	}
	e.token = body["token"].(string)
	return e
}

func (e *env) do(t *testing.T, method, path, body, token string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), method, e.srv.URL+path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func TestAuth(t *testing.T) {
	e := setup(t)
	if s, _ := e.do(t, "GET", "/api/targets", "", ""); s != 401 {
		t.Fatalf("no token: %d", s)
	}
	if s, _ := e.do(t, "GET", "/api/targets", "", "hunter2"); s != 401 {
		t.Fatalf("raw password accepted as a token: %d", s)
	}
	if s, _ := e.do(t, "POST", "/api/login", `{"password":"wrong"}`, ""); s != 401 {
		t.Fatalf("wrong password: %d", s)
	}
	if s, _ := e.do(t, "GET", "/healthz", "", ""); s != 200 {
		t.Fatalf("health needs no auth: %d", s)
	}
}

func TestLoginCookieWorks(t *testing.T) {
	e := setup(t)
	login, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, e.srv.URL+"/api/login", strings.NewReader(`{"password":"hunter2"}`))
	res, err := http.DefaultClient.Do(login)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, e.srv.URL+"/api/targets", http.NoBody)
	for _, c := range res.Cookies() {
		req.AddCookie(c)
	}
	got, _ := http.DefaultClient.Do(req)
	got.Body.Close()
	if got.StatusCode != http.StatusOK {
		t.Fatalf("cookie auth: %d", got.StatusCode)
	}
}

func TestCommandFansOutToTheRoom(t *testing.T) {
	e := setup(t)
	s, body := e.do(t, "POST", "/api/targets/room/command", `{"on":true,"brightness":60,"clientId":"phone","revision":3}`, e.token)
	if s != 200 || body["ok"] != true {
		t.Fatalf("%d %v", s, body)
	}
	for _, l := range []*light{e.a, e.b} {
		if st, _ := l.State(context.Background()); !st.On || st.Brightness != 60 {
			t.Fatalf("%s = %+v", l.id, st)
		}
	}
	s, body = e.do(t, "GET", "/api/targets/room/state", "", e.token)
	if s != 200 || body["state"].(map[string]any)["brightness"] != 60.0 || len(body["devices"].(map[string]any)) != 2 {
		t.Fatalf("state: %d %v", s, body)
	}
}

func TestPartialFailureIs502WithDetails(t *testing.T) {
	e := setup(t)
	e.b.err = errors.New("govee: HTTP 401")
	s, body := e.do(t, "POST", "/api/targets/room/command", `{"on":true}`, e.token)
	if s != 502 || body["failed"].(map[string]any)["b"] != "govee: HTTP 401" {
		t.Fatalf("%d %v", s, body)
	}
}

func TestBadRequests(t *testing.T) {
	e := setup(t)
	cases := []struct {
		path, body string
		want       int
	}{
		{"/api/targets/room/command", `{"brightnes":50}`, 400},
		{"/api/targets/room/command", `{"on":1}`, 400},
		{"/api/targets/room/command", `{"brightness":0}`, 400},
		{"/api/targets/room/command", `{"color":"red"}`, 400},
		{"/api/targets/room/command", `{}`, 400},
		{"/api/targets/nope/command", `{"on":true}`, 404},
		{"/api/matter/commission", `{}`, 400},
	}
	for _, c := range cases {
		if s, body := e.do(t, "POST", c.path, c.body, e.token); s != c.want {
			t.Errorf("%s %s: %d %v, want %d", c.path, c.body, s, body, c.want)
		}
	}
}

func TestSync(t *testing.T) {
	e := setup(t)
	e.a.s = device.State{On: true, Brightness: 40}
	e.b.s = device.State{On: false}
	s, body := e.do(t, "POST", "/api/targets/room/sync", `{"clientId":"phone","revision":1,"on":true,"brightness":40}`, e.token)
	if s != 200 || body["synchronized"] != false {
		t.Fatalf("%d %v", s, body)
	}
	fixed := body["corrected"].(map[string]any)["b"].([]any)
	if len(fixed) != 2 {
		t.Fatalf("corrected b = %v", fixed)
	}
	s, body = e.do(t, "POST", "/api/targets/room/sync", `{"clientId":"phone","revision":1,"on":true,"brightness":40}`, e.token)
	if s != 200 || body["synchronized"] != true {
		t.Fatalf("second pass: %d %v", s, body)
	}
}

func TestMatterEndpoints(t *testing.T) {
	e := setup(t)
	s, body := e.do(t, "POST", "/api/matter/commission", `{"code":"MT:Y.K90AFN00KA0648G00","networkOnly":true}`, e.token)
	if s != 200 || e.m.code != "MT:Y.K90AFN00KA0648G00" || body["node"].(map[string]any)["node_id"] != 5.0 {
		t.Fatalf("commission: %d %v", s, body)
	}
	if s, _ := e.do(t, "GET", "/api/matter/nodes", "", e.token); s != 200 {
		t.Fatalf("nodes: %d", s)
	}
	if s, _ := e.do(t, "DELETE", "/api/matter/nodes/abc", "", e.token); s != 400 {
		t.Fatalf("bad node id: %d", s)
	}
	e.m.err = &matter.ServerError{Code: 1, Details: "pairing failed"}
	if s, _ := e.do(t, "POST", "/api/matter/thread", `{"dataset":"0e08"}`, e.token); s != 502 {
		t.Fatalf("server error: %d", s)
	}
	e.m.connected = false
	if s, _ := e.do(t, "POST", "/api/matter/wifi", `{"ssid":"home","password":"x"}`, e.token); s != 503 {
		t.Fatalf("disconnected: %d", s)
	}
}
