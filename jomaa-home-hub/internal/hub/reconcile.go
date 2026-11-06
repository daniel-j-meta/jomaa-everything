package hub

import (
	"context"
	"errors"
	"sync"

	"github.com/jomaa-daniel/jomaa-everything/jomaa-home-hub/internal/device"
)

// ErrNotFound means the id names no device or room.
var ErrNotFound = errors.New("no device or room with that id")

// Desired is the state a client last asked for. Like device-panel, the client
// owns it and sends it with each sync; the hub only tracks revisions.
type Desired struct {
	ClientID            string `json:"clientId"`
	Revision            *int64 `json:"revision"`
	On                  *bool  `json:"on"`
	Brightness          *int   `json:"brightness"`
	ColorTemperaturePct *int   `json:"colorTemperaturePct"`
}

// SyncResult reports what a sync pass found and fixed, by device id.
type SyncResult struct {
	Synchronized bool                    `json:"synchronized"`
	Stale        bool                    `json:"stale"`
	Corrected    map[string][]string     `json:"corrected"`
	Failed       map[string]string       `json:"failed"`
	Observed     map[string]device.State `json:"observed"`
}

// Values within this many points count as matching. Converting between
// percent, kelvin, mireds and Matter's 0-254 levels can be off by one.
const tolerance = 1

// Sync checks every device under the target against desired and corrects
// only the devices and fields that drifted. Unlike device-panel, which checked
// one representative bulb, each bulb is checked on its own.
func (h *Hub) Sync(ctx context.Context, id string, want Desired) (SyncResult, error) {
	d, ok := h.devices[id]
	if !ok {
		return SyncResult{}, ErrNotFound
	}
	res := SyncResult{Corrected: map[string][]string{}, Failed: map[string]string{}, Observed: map[string]device.State{}}
	clientID := h.revs.note(want.ClientID, want.Revision)
	if h.revs.stale(clientID, want.Revision) {
		res.Stale = true
		return res, nil
	}

	leaves := device.Leaves(d)
	res.Observed, res.Failed = observe(ctx, leaves)

	fixes := map[string]device.Command{}
	for _, l := range leaves {
		s, ok := res.Observed[l.ID()]
		if !ok {
			continue
		}
		cmd, fields := drift(want, s, device.FeaturesOf(l))
		if len(fields) > 0 {
			fixes[l.ID()] = cmd
			res.Corrected[l.ID()] = fields
		}
	}

	// A newer command may have landed while we were reading state.
	if len(fixes) > 0 && h.revs.stale(clientID, want.Revision) {
		return SyncResult{Stale: true, Corrected: map[string][]string{}, Failed: map[string]string{}, Observed: res.Observed}, nil
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, l := range leaves {
		cmd, ok := fixes[l.ID()]
		if !ok {
			continue
		}
		wg.Add(1)
		go func(l device.Device) {
			defer wg.Done()
			if err := l.Apply(ctx, cmd); err != nil {
				mu.Lock()
				res.Failed[l.ID()] = err.Error()
				mu.Unlock()
			}
		}(l)
	}
	wg.Wait()

	res.Synchronized = len(res.Corrected) == 0 && len(res.Failed) == 0
	return res, nil
}

// drift returns the command that moves s to want, and the fields it touches.
func drift(want Desired, s device.State, f device.Features) (device.Command, []string) {
	var cmd device.Command
	var fields []string
	if want.On != nil && s.On != *want.On {
		cmd.On = device.Bool(*want.On)
		fields = append(fields, "on")
	}
	// Only fix levels when the light should be on, or a correction could
	// switch an off light back on.
	if want.On != nil && !*want.On {
		return cmd, fields
	}
	if f.Brightness && want.Brightness != nil && abs(s.Brightness-*want.Brightness) > tolerance {
		cmd.Brightness = device.Int(*want.Brightness)
		fields = append(fields, "brightness")
	}
	if f.ColorTemperature && want.ColorTemperaturePct != nil && abs(s.ColorTemperaturePct-*want.ColorTemperaturePct) > tolerance {
		cmd.ColorTemperaturePct = device.Int(*want.ColorTemperaturePct)
		fields = append(fields, "colorTemperaturePct")
	}
	return cmd, fields
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

const defaultClientID = "default"

// revisions remembers the newest revision each client has sent, so a slow sync
// can't undo a command the same client sent after it. Like device-panel, a
// client uses one counter for everything it controls: a newer command to a
// single lamp also retires an older sync of the room that lamp is in.
type revisions struct {
	mu     sync.Mutex
	latest map[string]int64
}

func newRevisions() *revisions { return &revisions{latest: map[string]int64{}} }

func (r *revisions) note(clientID string, rev *int64) string {
	if clientID == "" {
		clientID = defaultClientID
	}
	if rev == nil || *rev < 0 {
		return clientID
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if *rev > r.latest[clientID] {
		r.latest[clientID] = *rev
	}
	return clientID
}

func (r *revisions) stale(clientID string, rev *int64) bool {
	if rev == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return *rev < r.latest[clientID]
}
