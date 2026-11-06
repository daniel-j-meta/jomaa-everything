package hub

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"sync"
	"testing"

	"github.com/jomaa-daniel/jomaa-everything/jomaa-home-hub/internal/config"
	"github.com/jomaa-daniel/jomaa-everything/jomaa-home-hub/internal/device"
	"github.com/jomaa-daniel/jomaa-everything/jomaa-home-hub/internal/govee"
	"github.com/jomaa-daniel/jomaa-everything/jomaa-home-hub/internal/matter"
)

// bulb is an in-memory light that changes state when commanded.
type bulb struct {
	id       string
	features *device.Features
	stateErr error
	applyErr error
	onApply  func()

	mu    sync.Mutex
	state device.State
	got   []device.Command
}

func (b *bulb) ID() string   { return b.id }
func (b *bulb) Name() string { return b.id }
func (b *bulb) Apply(ctx context.Context, c device.Command) error {
	if b.onApply != nil {
		b.onApply()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.got = append(b.got, c)
	if b.applyErr != nil {
		return b.applyErr
	}
	if c.On != nil {
		b.state.On = *c.On
	}
	if c.Brightness != nil {
		b.state.Brightness = *c.Brightness
	}
	if c.ColorTemperaturePct != nil {
		b.state.ColorTemperaturePct = *c.ColorTemperaturePct
	}
	return nil
}
func (b *bulb) State(ctx context.Context) (device.State, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state, b.stateErr
}
func (b *bulb) Features() device.Features {
	if b.features != nil {
		return *b.features
	}
	return device.Features{Brightness: true, ColorTemperature: true, Color: true}
}
func (b *bulb) commands() []device.Command {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]device.Command(nil), b.got...)
}

func room(t *testing.T, bulbs ...*bulb) *Hub {
	t.Helper()
	h := New()
	members := make([]device.Device, len(bulbs))
	for i, b := range bulbs {
		members[i] = b
		if err := h.Add("test", b); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.Add("room", device.NewGroup("living-room", "Living Room", members)); err != nil {
		t.Fatal(err)
	}
	return h
}

func rev(v int64) *int64 { return &v }

func TestSyncFixesOnlyTheBulbsThatDrifted(t *testing.T) {
	good := &bulb{id: "good", state: device.State{On: true, Brightness: 50, ColorTemperaturePct: 30}}
	dim := &bulb{id: "dim", state: device.State{On: true, Brightness: 10, ColorTemperaturePct: 31}}
	off := &bulb{id: "off", state: device.State{On: false, Brightness: 50, ColorTemperaturePct: 30}}
	h := room(t, good, dim, off)

	want := Desired{On: device.Bool(true), Brightness: device.Int(50), ColorTemperaturePct: device.Int(30)}
	res, err := h.Sync(context.Background(), "living-room", want)
	if err != nil {
		t.Fatal(err)
	}
	if res.Synchronized {
		t.Fatal("reported synchronized while bulbs had drifted")
	}
	wantFixed := map[string][]string{"dim": {"brightness"}, "off": {"on"}}
	if !reflect.DeepEqual(res.Corrected, wantFixed) {
		t.Fatalf("corrected = %v, want %v", res.Corrected, wantFixed)
	}
	if len(good.commands()) != 0 {
		t.Fatal("touched a bulb that was already right")
	}

	// The next pass sees everything in place.
	res, _ = h.Sync(context.Background(), "living-room", want)
	if !res.Synchronized || len(res.Corrected) != 0 {
		t.Fatalf("second pass = %+v", res)
	}
}

func TestSyncNeverRelightsAnOffRoom(t *testing.T) {
	b := &bulb{id: "b", state: device.State{On: false, Brightness: 5}}
	h := room(t, b)
	res, _ := h.Sync(context.Background(), "living-room", Desired{On: device.Bool(false), Brightness: device.Int(80)})
	if !res.Synchronized || len(b.commands()) != 0 {
		t.Fatalf("res = %+v, commands = %v", res, b.commands())
	}
}

func TestSyncSkipsFieldsADeviceCantChange(t *testing.T) {
	p := &bulb{id: "plug", features: &device.Features{}, state: device.State{On: true}}
	h := room(t, p)
	res, _ := h.Sync(context.Background(), "living-room", Desired{On: device.Bool(true), Brightness: device.Int(80), ColorTemperaturePct: device.Int(20)})
	if !res.Synchronized {
		t.Fatalf("plug can't dim, but sync wanted to fix it: %+v", res)
	}
}

func TestOlderSyncStandsDownAfterANewerCommand(t *testing.T) {
	b := &bulb{id: "b", state: device.State{On: true, Brightness: 50}}
	h := room(t, b)
	ctx := context.Background()

	if _, err := h.Command(ctx, "b", device.Command{Brightness: device.Int(90)}, "phone", rev(5)); err != nil {
		t.Fatal(err)
	}
	res, _ := h.Sync(ctx, "living-room", Desired{ClientID: "phone", Revision: rev(4), On: device.Bool(true), Brightness: device.Int(50)})
	if !res.Stale || len(b.commands()) != 1 {
		t.Fatalf("stale sync changed things: %+v, commands %v", res, b.commands())
	}

	// Another client's revisions are tracked separately.
	res, _ = h.Sync(ctx, "living-room", Desired{ClientID: "tablet", Revision: rev(1), On: device.Bool(true), Brightness: device.Int(90)})
	if res.Stale || !res.Synchronized {
		t.Fatalf("tablet sync = %+v", res)
	}
}

func TestSyncStandsDownIfACommandArrivesMidway(t *testing.T) {
	var h *Hub
	b := &bulb{id: "b", state: device.State{On: true, Brightness: 10}}
	slow := &slowState{bulb: b, during: func() {
		_, _ = h.Command(context.Background(), "b", device.Command{Brightness: device.Int(70)}, "phone", rev(2))
	}}
	h = New()
	_ = h.Add("test", slow)
	_ = h.Add("room", device.NewGroup("r", "R", []device.Device{slow}))

	res, _ := h.Sync(context.Background(), "r", Desired{ClientID: "phone", Revision: rev(1), On: device.Bool(true), Brightness: device.Int(40)})
	if !res.Stale {
		t.Fatalf("res = %+v", res)
	}
	if s, _ := b.State(context.Background()); s.Brightness != 70 {
		t.Fatalf("sync overwrote the newer command: brightness %d", s.Brightness)
	}
}

// slowState runs a hook the first time state is read, to simulate a command
// landing while a sync is checking devices.
type slowState struct {
	*bulb
	during func()
	once   sync.Once
}

func (s *slowState) State(ctx context.Context) (device.State, error) {
	st, err := s.bulb.State(ctx)
	s.once.Do(s.during)
	return st, err
}

func TestSyncReportsUnreachableDevices(t *testing.T) {
	ok := &bulb{id: "ok", state: device.State{On: true}}
	gone := &bulb{id: "gone", stateErr: errors.New("timeout")}
	h := room(t, ok, gone)
	res, _ := h.Sync(context.Background(), "living-room", Desired{On: device.Bool(true)})
	if res.Synchronized || res.Failed["gone"] != "timeout" {
		t.Fatalf("res = %+v", res)
	}
}

func TestCommandReportsPerDeviceFailures(t *testing.T) {
	ok := &bulb{id: "ok"}
	bad := &bulb{id: "bad", applyErr: errors.New("401")}
	h := room(t, ok, bad)
	failed, err := h.Command(context.Background(), "living-room", device.Command{On: device.Bool(true)}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(failed, map[string]string{"bad": "401"}) || len(ok.commands()) != 1 {
		t.Fatalf("failed = %v", failed)
	}
}

func TestCommandRejectsBadInputAndUnknownTargets(t *testing.T) {
	h := room(t, &bulb{id: "b"})
	if _, err := h.Command(context.Background(), "nope", device.Command{On: device.Bool(true)}, "", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if _, err := h.Command(context.Background(), "b", device.Command{}, "", nil); !errors.Is(err, device.ErrInvalidCommand) {
		t.Fatalf("err = %v", err)
	}
}

func TestBuildFromConfig(t *testing.T) {
	cfg, err := config.Parse([]byte(`{
		"devices": [
			{"id":"lamp","name":"Lamp","driver":"govee","sku":"H607C","device":"AA:BB","minKelvin":2200,"maxKelvin":6500},
			{"id":"bulb","name":"Bulb","driver":"matter","nodeId":3}
		],
		"rooms": [
			{"id":"living-room","name":"Living Room","members":["lamp","bulb"]},
			{"id":"house","name":"House","members":["living-room"]}
		]}`))
	if err != nil {
		t.Fatal(err)
	}
	h, err := Build(cfg, govee.NewClient("k"), matter.NewClient("ws://127.0.0.1:1/ws", slog.New(slog.NewTextHandler(io.Discard, nil))))
	if err != nil {
		t.Fatal(err)
	}
	list := h.List()
	if len(list) != 4 || list[2].Kind != "room" || !reflect.DeepEqual(list[3].Members, []string{"living-room"}) {
		t.Fatalf("list = %+v", list)
	}
	house, _ := h.Get("house")
	if n := len(device.Leaves(house)); n != 2 {
		t.Fatalf("house has %d bulbs", n)
	}
}
