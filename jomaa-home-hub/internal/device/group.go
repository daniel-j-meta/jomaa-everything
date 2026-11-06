package device

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Group is a room: it implements Device by sending every command to all of its
// members at once. A slow or broken bulb never stops the rest from changing;
// failures come back together as a *GroupError.
type Group struct {
	id, name string
	members  []Device
}

// NewGroup makes a room from its members.
func NewGroup(id, name string, members []Device) *Group {
	return &Group{id: id, name: name, members: members}
}

// ID returns the room id.
func (g *Group) ID() string { return g.id }

// Name returns the room name.
func (g *Group) Name() string { return g.name }

// Members returns the devices and rooms directly in this room.
func (g *Group) Members() []Device { return g.members }

// GroupError lists which members failed and why.
type GroupError struct {
	Failed map[string]error
}

func (e *GroupError) Error() string {
	ids := make([]string, 0, len(e.Failed))
	for id := range e.Failed {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("%s: %v", id, e.Failed[id])
	}
	return fmt.Sprintf("%d device(s) failed: %s", len(ids), strings.Join(parts, "; "))
}

// Failures flattens err into a device-id -> message map. Nested groups are
// expanded so callers see the individual bulbs that failed.
func Failures(id string, err error) map[string]string {
	out := map[string]string{}
	var ge *GroupError
	if !errors.As(err, &ge) {
		if err != nil {
			out[id] = err.Error()
		}
		return out
	}
	for mid, merr := range ge.Failed {
		for k, v := range Failures(mid, merr) {
			out[k] = v
		}
	}
	return out
}

// Apply sends cmd to every member at once.
func (g *Group) Apply(ctx context.Context, cmd Command) error {
	return g.each(func(d Device) error { return d.Apply(ctx, cmd) })
}

// State summarizes the room: it is on if any member is on, and brightness and
// color temperature come from the first member that is on (or the first that
// answered, if all are off). Use MemberStates for the per-device view.
func (g *Group) State(ctx context.Context) (State, error) {
	states, err := g.MemberStates(ctx)
	if len(states) == 0 {
		if err == nil {
			err = errors.New("group has no members")
		}
		return State{}, err
	}
	var summary *State
	for _, m := range g.members {
		s, ok := states[m.ID()]
		if !ok {
			continue
		}
		if summary == nil || (s.On && !summary.On) {
			s := s
			summary = &s
		}
	}
	return *summary, nil
}

// MemberStates queries every member at once. It returns whatever states it got
// plus a *GroupError for members that didn't answer.
func (g *Group) MemberStates(ctx context.Context) (map[string]State, error) {
	var mu sync.Mutex
	states := make(map[string]State, len(g.members))
	err := g.each(func(d Device) error {
		s, err := d.State(ctx)
		if err == nil {
			mu.Lock()
			states[d.ID()] = s
			mu.Unlock()
		}
		return err
	})
	return states, err
}

func (g *Group) each(fn func(Device) error) error {
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		failed = map[string]error{}
	)
	for _, m := range g.members {
		wg.Add(1)
		go func(d Device) {
			defer wg.Done()
			if err := fn(d); err != nil {
				mu.Lock()
				failed[d.ID()] = err
				mu.Unlock()
			}
		}(m)
	}
	wg.Wait()
	if len(failed) > 0 {
		return &GroupError{Failed: failed}
	}
	return nil
}

// Leaves returns the individual devices under d, expanding nested groups.
func Leaves(d Device) []Device {
	g, ok := d.(*Group)
	if !ok {
		return []Device{d}
	}
	var out []Device
	for _, m := range g.members {
		out = append(out, Leaves(m)...)
	}
	return out
}
