package device

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fake struct {
	id    string
	state State
	err   error
	delay time.Duration

	mu  sync.Mutex
	got []Command
}

func (f *fake) ID() string   { return f.id }
func (f *fake) Name() string { return f.id }
func (f *fake) Apply(ctx context.Context, c Command) error {
	time.Sleep(f.delay)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got = append(f.got, c)
	return f.err
}
func (f *fake) State(ctx context.Context) (State, error) { return f.state, f.err }

func TestGroupSendsToEveryMemberAtOnce(t *testing.T) {
	a := &fake{id: "a", delay: 100 * time.Millisecond}
	b := &fake{id: "b", delay: 100 * time.Millisecond}
	c := &fake{id: "c", delay: 100 * time.Millisecond}
	g := NewGroup("room", "Room", []Device{a, b, c})

	start := time.Now()
	if err := g.Apply(context.Background(), Command{On: Bool(true)}); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 250*time.Millisecond {
		t.Fatalf("members ran one after another (%v)", took)
	}
	for _, f := range []*fake{a, b, c} {
		if len(f.got) != 1 || !*f.got[0].On {
			t.Fatalf("%s got %+v", f.id, f.got)
		}
	}
}

func TestGroupKeepsGoingWhenOneMemberFails(t *testing.T) {
	ok := &fake{id: "ok"}
	bad := &fake{id: "bad", err: errors.New("offline")}
	inner := NewGroup("inner", "Inner", []Device{bad})
	g := NewGroup("room", "Room", []Device{ok, inner})

	err := g.Apply(context.Background(), Command{On: Bool(false)})
	if len(ok.got) != 1 {
		t.Fatal("healthy member was skipped")
	}
	failed := Failures("room", err)
	if len(failed) != 1 || failed["bad"] != "offline" {
		t.Fatalf("failures = %v, want just the nested bulb", failed)
	}
}

func TestGroupStatePrefersAMemberThatIsOn(t *testing.T) {
	g := NewGroup("room", "Room", []Device{
		&fake{id: "a", state: State{On: false, Brightness: 10}},
		&fake{id: "b", state: State{On: true, Brightness: 80}},
	})
	s, err := g.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !s.On || s.Brightness != 80 {
		t.Fatalf("state = %+v", s)
	}
}

func TestLeavesExpandsNestedRooms(t *testing.T) {
	a, b, c := &fake{id: "a"}, &fake{id: "b"}, &fake{id: "c"}
	g := NewGroup("all", "All", []Device{a, NewGroup("r", "R", []Device{b, c})})
	got := Leaves(g)
	if len(got) != 3 || got[0] != a || got[1] != b || got[2] != c {
		t.Fatalf("leaves = %v", got)
	}
}

func TestValidate(t *testing.T) {
	cases := map[string]Command{
		"empty":           {},
		"brightness zero": {Brightness: Int(0)},
		"brightness 101":  {Brightness: Int(101)},
		"temp -1":         {ColorTemperaturePct: Int(-1)},
	}
	for name, c := range cases {
		if err := c.Validate(); !errors.Is(err, ErrInvalidCommand) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if err := (Command{Brightness: Int(100), ColorTemperaturePct: Int(0)}).Validate(); err != nil {
		t.Errorf("valid command rejected: %v", err)
	}
}

func TestRGBJSON(t *testing.T) {
	var c RGB
	if err := c.UnmarshalJSON([]byte(`"#FF8000"`)); err != nil {
		t.Fatal(err)
	}
	if c.Int() != 0xff8000 || c.String() != "#ff8000" {
		t.Fatalf("got %v", c)
	}
	if err := c.UnmarshalJSON([]byte(`"red"`)); err == nil {
		t.Fatal("accepted a color name")
	}
}

func TestKelvinRoundTrip(t *testing.T) {
	for pct := 0; pct <= 100; pct++ {
		k := KelvinAtPct(2200, 6500, pct)
		if back := PctFromKelvin(2200, 6500, k); back != pct {
			t.Fatalf("%d%% -> %dK -> %d%%", pct, k, back)
		}
	}
}
