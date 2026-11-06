// Package hub holds every device and room by id and runs commands and
// reconcile passes against them. It doesn't care which brand a device is.
package hub

import (
	"context"
	"fmt"
	"sync"

	"github.com/jomaa-daniel/jomaa-everything/jomaa-home-hub/internal/config"
	"github.com/jomaa-daniel/jomaa-everything/jomaa-home-hub/internal/device"
	"github.com/jomaa-daniel/jomaa-everything/jomaa-home-hub/internal/govee"
	"github.com/jomaa-daniel/jomaa-everything/jomaa-home-hub/internal/matter"
)

// Target is a device or room as the API lists it.
type Target struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Kind     string          `json:"kind"` // "room", "govee" or "matter"
	Members  []string        `json:"members,omitempty"`
	Features device.Features `json:"features"`
}

// Hub holds every device and room by id.
type Hub struct {
	order   []string
	devices map[string]device.Device
	kinds   map[string]string
	revs    *revisions
}

// New returns an empty hub.
func New() *Hub {
	return &Hub{devices: map[string]device.Device{}, kinds: map[string]string{}, revs: newRevisions()}
}

// Add registers a device or room. kind is "room" for groups, else the driver.
func (h *Hub) Add(kind string, d device.Device) error {
	if _, dup := h.devices[d.ID()]; dup {
		return fmt.Errorf("duplicate id %q", d.ID())
	}
	h.order = append(h.order, d.ID())
	h.devices[d.ID()] = d
	h.kinds[d.ID()] = kind
	return nil
}

// Build creates the hub from a validated config.
func Build(cfg *config.Config, gc *govee.Client, mc *matter.Client) (*Hub, error) {
	h := New()
	for _, d := range cfg.Devices {
		var dev device.Device
		switch d.Driver {
		case config.DriverGovee:
			dev = govee.NewLight(gc, govee.LightConfig{
				ID: d.ID, Name: d.Name, SKU: d.SKU, Device: d.Device,
				MinKelvin: d.MinKelvin, MaxKelvin: d.MaxKelvin,
			})
		case config.DriverMatter:
			dev = matter.NewLight(mc, matter.LightConfig{
				ID: d.ID, Name: d.Name, NodeID: d.NodeID, Endpoint: d.Endpoint,
				MinKelvin: d.MinKelvin, MaxKelvin: d.MaxKelvin,
			})
		}
		if err := h.Add(d.Driver, dev); err != nil {
			return nil, err
		}
	}
	for _, r := range cfg.Rooms {
		members := make([]device.Device, len(r.Members))
		for i, m := range r.Members {
			members[i] = h.devices[m]
		}
		if err := h.Add("room", device.NewGroup(r.ID, r.Name, members)); err != nil {
			return nil, err
		}
	}
	return h, nil
}

// Get looks up a device or room.
func (h *Hub) Get(id string) (device.Device, bool) {
	d, ok := h.devices[id]
	return d, ok
}

// List returns every device and room in config order.
func (h *Hub) List() []Target {
	out := make([]Target, 0, len(h.order))
	for _, id := range h.order {
		d := h.devices[id]
		t := Target{ID: id, Name: d.Name(), Kind: h.kinds[id], Features: h.features(d)}
		if g, ok := d.(*device.Group); ok {
			for _, m := range g.Members() {
				t.Members = append(t.Members, m.ID())
			}
		}
		out = append(out, t)
	}
	return out
}

// features of a room is what any of its devices can do.
func (h *Hub) features(d device.Device) device.Features {
	var f device.Features
	for _, l := range device.Leaves(d) {
		lf := device.FeaturesOf(l)
		f.Brightness = f.Brightness || lf.Brightness
		f.ColorTemperature = f.ColorTemperature || lf.ColorTemperature
		f.Color = f.Color || lf.Color
	}
	return f
}

// StateResult is a target's summary state plus each device's own state.
type StateResult struct {
	State   *device.State           `json:"state,omitempty"`
	Devices map[string]device.State `json:"devices"`
	Failed  map[string]string       `json:"failed,omitempty"`
}

// State reads the target and every device under it.
func (h *Hub) State(ctx context.Context, id string) (StateResult, error) {
	d, ok := h.devices[id]
	if !ok {
		return StateResult{}, ErrNotFound
	}
	states, failed := observe(ctx, device.Leaves(d))
	res := StateResult{Devices: states, Failed: failed}
	if s, err := d.State(ctx); err == nil {
		res.State = &s
	}
	return res, nil
}

// Command validates cmd and sends it to the target, recording the client's
// revision so older reconcile passes from that client stand down.
func (h *Hub) Command(ctx context.Context, id string, cmd device.Command, clientID string, revision *int64) (map[string]string, error) {
	d, ok := h.devices[id]
	if !ok {
		return nil, ErrNotFound
	}
	if err := cmd.Validate(); err != nil {
		return nil, err
	}
	h.revs.note(clientID, revision)
	return device.Failures(id, d.Apply(ctx, cmd)), nil
}

func observe(ctx context.Context, leaves []device.Device) (map[string]device.State, map[string]string) {
	var mu sync.Mutex
	var wg sync.WaitGroup
	states := map[string]device.State{}
	failed := map[string]string{}
	for _, l := range leaves {
		wg.Add(1)
		go func(l device.Device) {
			defer wg.Done()
			s, err := l.State(ctx)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed[l.ID()] = err.Error()
			} else {
				states[l.ID()] = s
			}
		}(l)
	}
	wg.Wait()
	return states, failed
}
