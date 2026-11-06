package api

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLimiterBlocksAClientAfterFiveFailures(t *testing.T) {
	l := newLoginLimiter()
	now := time.Unix(1_000_000, 0)
	l.now = func() time.Time { return now }

	for i := range 5 {
		if ok, _ := l.allow("1.2.3.4"); !ok {
			t.Fatalf("blocked after %d failures", i)
		}
		l.fail("1.2.3.4")
	}
	ok, wait := l.allow("1.2.3.4")
	if ok || wait != 15*time.Minute {
		t.Fatalf("allow = %v, wait %v", ok, wait)
	}
	if ok, _ := l.allow("5.6.7.8"); !ok {
		t.Fatal("another client was blocked")
	}

	now = now.Add(15*time.Minute + time.Second)
	if ok, _ := l.allow("1.2.3.4"); !ok {
		t.Fatal("still blocked after the window passed")
	}
}

func TestLimiterCapsFailuresAcrossAllClients(t *testing.T) {
	l := newLoginLimiter()
	for i := range 50 {
		l.fail(string(rune('a'+i%26)) + string(rune('a'+i/26)))
	}
	if ok, _ := l.allow("fresh-client"); ok {
		t.Fatal("global budget not enforced")
	}
}

func TestSuccessClearsAClientsFailures(t *testing.T) {
	l := newLoginLimiter()
	for range 4 {
		l.fail("me")
	}
	l.succeed("me")
	for range 4 {
		l.fail("me")
	}
	if ok, _ := l.allow("me"); !ok {
		t.Fatal("failures before a successful sign-in still counted")
	}
}

func TestForwardedHeadersOnlyTrustedFromLocalProxy(t *testing.T) {
	local := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/login", http.NoBody)
	local.RemoteAddr = "127.0.0.1:50000"
	local.Header.Set("CF-Connecting-IP", "203.0.113.9")
	local.Header.Set("X-Forwarded-Proto", "https")
	if got := clientIP(local); got != "203.0.113.9" {
		t.Fatalf("local proxy client = %q", got)
	}
	if !isHTTPS(local) {
		t.Fatal("tunnel request not seen as HTTPS")
	}

	remote := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/login", http.NoBody)
	remote.RemoteAddr = "192.168.1.50:50000"
	remote.Header.Set("CF-Connecting-IP", "203.0.113.9")
	remote.Header.Set("X-Forwarded-Proto", "https")
	if got := clientIP(remote); got != "192.168.1.50" {
		t.Fatalf("forged header trusted: %q", got)
	}
	if isHTTPS(remote) {
		t.Fatal("forged X-Forwarded-Proto trusted")
	}

	direct := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/login", http.NoBody)
	direct.RemoteAddr = "192.168.1.50:50000"
	direct.TLS = &tls.ConnectionState{}
	if !isHTTPS(direct) {
		t.Fatal("direct TLS not seen as HTTPS")
	}
}
