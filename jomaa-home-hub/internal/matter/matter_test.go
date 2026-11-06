package matter

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
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/jomaa-daniel/jomaa-everything/jomaa-home-hub/internal/device"
)

// fakeServer speaks enough of the Matter server's WebSocket API to test
// against: it greets, dumps nodes on start_listening, records device commands,
// and lets a test push attribute reports.
type fakeServer struct {
	t     *testing.T
	nodes []map[string]any
	fail  map[string]bool // command_name -> answer with an error

	mu       sync.Mutex
	commands []map[string]any
	conn     *websocket.Conn
	srv      *httptest.Server
}

func newFakeServer(t *testing.T, nodes ...map[string]any) *fakeServer {
	f := &fakeServer{t: t, nodes: nodes, fail: map[string]bool{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeServer) url() string { return "ws" + strings.TrimPrefix(f.srv.URL, "http") + "/ws" }

func (f *fakeServer) serve(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	ctx := r.Context()
	f.mu.Lock()
	f.conn = conn
	f.mu.Unlock()
	_ = wsjson.Write(ctx, conn, map[string]any{"fabric_id": 1, "schema_version": 11, "sdk_version": "fake"})
	for {
		var cmd struct {
			MessageID string         `json:"message_id"`
			Command   string         `json:"command"`
			Args      map[string]any `json:"args"`
		}
		if err := wsjson.Read(ctx, conn, &cmd); err != nil {
			return
		}
		switch cmd.Command {
		case "start_listening":
			_ = wsjson.Write(ctx, conn, map[string]any{"message_id": cmd.MessageID, "result": f.nodes})
		case "device_command":
			f.mu.Lock()
			f.commands = append(f.commands, cmd.Args)
			f.mu.Unlock()
			if f.fail[cmd.Args["command_name"].(string)] {
				_ = wsjson.Write(ctx, conn, map[string]any{"message_id": cmd.MessageID, "error_code": 0, "details": "device did not respond"})
				continue
			}
			_ = wsjson.Write(ctx, conn, map[string]any{"message_id": cmd.MessageID, "result": nil})
		case "commission_with_code":
			_ = wsjson.Write(ctx, conn, map[string]any{"message_id": cmd.MessageID, "result": map[string]any{"node_id": 7, "available": true}})
		default:
			_ = wsjson.Write(ctx, conn, map[string]any{"message_id": cmd.MessageID, "error_code": 9, "details": "unknown command"})
		}
	}
}

func (f *fakeServer) push(event string, data any) {
	f.mu.Lock()
	conn := f.conn
	f.mu.Unlock()
	_ = wsjson.Write(context.Background(), conn, map[string]any{"event": event, "data": data})
}

func (f *fakeServer) sent() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any(nil), f.commands...)
}

func colorBulb() map[string]any {
	return map[string]any{
		"node_id": 1, "available": true,
		"attributes": map[string]any{
			"0/40/1": "Acme", "0/40/3": "Bulb",
			"1/6/0": true, "1/8/0": 127,
			"1/768/0": 0, "1/768/7": 250, "1/768/16395": 153, "1/768/16396": 500,
		},
	}
}

func plug() map[string]any {
	return map[string]any{"node_id": 2, "available": true, "attributes": map[string]any{"1/6/0": false}}
}

func connect(t *testing.T, f *fakeServer) *Client {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	c := NewClient(f.url(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	go c.Run(ctx)
	waitFor(t, c.Connected)
	return c
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestStateComesFromTheNodeDump(t *testing.T) {
	f := newFakeServer(t, colorBulb())
	c := connect(t, f)
	l := NewLight(c, LightConfig{ID: "bulb", NodeID: 1})

	s, err := l.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// 127/254 = 50%; 250 mireds = 4000K in a 2000-6536K range = 44%.
	if s != (device.State{On: true, Brightness: 50, ColorTemperaturePct: 44}) {
		t.Fatalf("state = %+v", s)
	}
	if got := l.Features(); got != (device.Features{Brightness: true, ColorTemperature: true, Color: true}) {
		t.Fatalf("features = %+v", got)
	}
}

func TestAttributeReportsUpdateState(t *testing.T) {
	f := newFakeServer(t, colorBulb())
	c := connect(t, f)
	l := NewLight(c, LightConfig{ID: "bulb", NodeID: 1})

	f.push("attribute_updated", []any{1, "1/6/0", false})
	waitFor(t, func() bool { s, _ := l.State(context.Background()); return !s.On })

	f.push("node_updated", map[string]any{"node_id": 1, "available": false, "attributes": map[string]any{}})
	waitFor(t, func() bool { _, err := l.State(context.Background()); return err != nil })
}

func TestApplySendsClusterCommands(t *testing.T) {
	f := newFakeServer(t, colorBulb())
	c := connect(t, f)
	l := NewLight(c, LightConfig{ID: "bulb", NodeID: 1})
	red, _ := device.ParseRGB("#ff0000")

	err := l.Apply(context.Background(), device.Command{
		On: device.Bool(true), Brightness: device.Int(100), ColorTemperaturePct: device.Int(0), Color: &red,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := f.sent()
	want := []struct {
		cluster float64
		name    string
		field   string
		value   float64
	}{
		{6, "On", "", 0},
		{8, "MoveToLevelWithOnOff", "level", 254},
		{768, "MoveToColorTemperature", "colorTemperatureMireds", 500}, // warmest
		{768, "MoveToHueAndSaturation", "saturation", 254},
	}
	if len(got) != len(want) {
		t.Fatalf("sent %d commands: %v", len(got), got)
	}
	for i, w := range want {
		if got[i]["cluster_id"] != w.cluster || got[i]["command_name"] != w.name || got[i]["node_id"] != 1.0 || got[i]["endpoint_id"] != 1.0 {
			t.Fatalf("command %d = %v, want cluster %v %s", i, got[i], w.cluster, w.name)
		}
		if w.field != "" && got[i]["payload"].(map[string]any)[w.field] != w.value {
			t.Fatalf("command %d payload = %v, want %s=%v", i, got[i]["payload"], w.field, w.value)
		}
	}
}

func TestPlugIgnoresLevelAndColor(t *testing.T) {
	f := newFakeServer(t, plug())
	c := connect(t, f)
	l := NewLight(c, LightConfig{ID: "plug", NodeID: 2})

	err := l.Apply(context.Background(), device.Command{On: device.Bool(true), Brightness: device.Int(40), ColorTemperaturePct: device.Int(10)})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.sent(); len(got) != 1 || got[0]["command_name"] != "On" {
		t.Fatalf("sent %v", got)
	}
	if got := l.Features(); got != (device.Features{}) {
		t.Fatalf("features = %+v", got)
	}
}

func TestServerErrorsComeBack(t *testing.T) {
	f := newFakeServer(t, colorBulb())
	f.fail["Off"] = true
	c := connect(t, f)
	err := NewLight(c, LightConfig{ID: "bulb", NodeID: 1}).Apply(context.Background(), device.Command{On: device.Bool(false)})
	var se *ServerError
	if !errors.As(err, &se) || se.Details != "device did not respond" {
		t.Fatalf("err = %v", err)
	}
}

func TestUnknownNodeIsAnError(t *testing.T) {
	f := newFakeServer(t)
	c := connect(t, f)
	if _, err := NewLight(c, LightConfig{ID: "x", NodeID: 99}).State(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
}

func TestCallsFailBeforeConnecting(t *testing.T) {
	c := NewClient("ws://127.0.0.1:1/ws", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := c.Call(context.Background(), "get_nodes", nil); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("err = %v", err)
	}
}

func TestReconnectsAfterServerRestart(t *testing.T) {
	f := newFakeServer(t, colorBulb())
	c := connect(t, f)
	f.mu.Lock()
	f.conn.Close(websocket.StatusGoingAway, "restart")
	f.mu.Unlock()
	waitFor(t, func() bool { return !c.Connected() })
	waitFor(t, c.Connected)
	if _, err := c.Commission(context.Background(), "34970112332", true); err != nil {
		t.Fatalf("after reconnect: %v", err)
	}
}

func TestNodesSummary(t *testing.T) {
	f := newFakeServer(t, plug(), colorBulb())
	c := connect(t, f)
	got, _ := json.Marshal(c.Nodes())
	want := `[{"nodeId":1,"available":true,"isBridge":false,"vendor":"Acme","product":"Bulb"},{"nodeId":2,"available":true,"isBridge":false}]`
	if string(got) != want {
		t.Fatalf("nodes = %s", got)
	}
}
