package api

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// loginLimiter slows password guessing once the hub is reachable from the
// internet. Each client gets a few failed sign-ins per window, and all clients
// together get a larger budget, so spreading guesses across many addresses
// doesn't help either.
type loginLimiter struct {
	perClient int
	global    int
	window    time.Duration
	now       func() time.Time

	mu      sync.Mutex
	clients map[string][]time.Time
	all     []time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{
		perClient: 5,
		global:    50,
		window:    15 * time.Minute,
		now:       time.Now,
		clients:   map[string][]time.Time{},
	}
}

// allow reports whether client may try a password now, and if not, how long
// until it may.
func (l *loginLimiter) allow(client string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.all = l.recent(l.all, now)
	mine := l.recent(l.clients[client], now)
	if len(mine) == 0 {
		delete(l.clients, client)
	} else {
		l.clients[client] = mine
	}
	switch {
	case len(mine) >= l.perClient:
		return false, mine[0].Add(l.window).Sub(now)
	case len(l.all) >= l.global:
		return false, l.all[0].Add(l.window).Sub(now)
	}
	return true, 0
}

// fail records a wrong password from client.
func (l *loginLimiter) fail(client string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.clients[client] = append(l.recent(l.clients[client], now), now)
	l.all = append(l.recent(l.all, now), now)
	// Keep memory bounded if many addresses each fail once.
	if len(l.clients) > 10_000 {
		for c, times := range l.clients {
			if len(l.recent(times, now)) == 0 {
				delete(l.clients, c)
			}
		}
	}
}

// succeed clears client's failures after a correct password.
func (l *loginLimiter) succeed(client string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.clients, client)
}

func (l *loginLimiter) recent(times []time.Time, now time.Time) []time.Time {
	cutoff := now.Add(-l.window)
	i := 0
	for i < len(times) && !times[i].After(cutoff) {
		i++
	}
	return times[i:]
}

// fromLocalProxy reports whether the request came from this machine, which is
// where cloudflared runs. Only then are Cloudflare's forwarding headers
// trusted; from anywhere else they could be forged.
func fromLocalProxy(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// clientIP is the caller's address: Cloudflare's CF-Connecting-IP when the
// request came through the local tunnel, otherwise the connection's address.
func clientIP(r *http.Request) string {
	if fromLocalProxy(r) {
		if ip := net.ParseIP(r.Header.Get("CF-Connecting-IP")); ip != nil {
			return ip.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// isHTTPS reports whether the client reached the hub over HTTPS, either
// directly or through the tunnel, which terminates TLS at Cloudflare.
func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || (fromLocalProxy(r) && r.Header.Get("X-Forwarded-Proto") == "https")
}
