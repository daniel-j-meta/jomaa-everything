// Package device defines the one interface every controllable thing in the hub
// implements, whether it is a single Govee bulb, a Matter light, or a room that
// fans commands out to its members.
package device

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

// State is what a light reports back. Brightness and ColorTemperaturePct are
// both 0-100. ColorTemperaturePct runs from the device's warmest white (0) to
// its coolest (100), so the same number means the same thing across brands.
type State struct {
	On                  bool `json:"on"`
	Brightness          int  `json:"brightness"`
	ColorTemperaturePct int  `json:"colorTemperaturePct"`
}

// Command is a partial update. Nil fields are left alone.
type Command struct {
	On                  *bool `json:"on,omitempty"`
	Brightness          *int  `json:"brightness,omitempty"`
	ColorTemperaturePct *int  `json:"colorTemperaturePct,omitempty"`
	Color               *RGB  `json:"color,omitempty"`
}

// Device is anything the hub can send a command to.
type Device interface {
	ID() string
	Name() string
	Apply(ctx context.Context, cmd Command) error
	State(ctx context.Context) (State, error)
}

// ErrInvalidCommand marks a command rejected before anything was sent.
var ErrInvalidCommand = errors.New("invalid command")

// Validate rejects empty commands and out-of-range values.
func (c Command) Validate() error {
	if c.On == nil && c.Brightness == nil && c.ColorTemperaturePct == nil && c.Color == nil {
		return fmt.Errorf("%w: no fields set", ErrInvalidCommand)
	}
	if c.Brightness != nil && (*c.Brightness < 1 || *c.Brightness > 100) {
		return fmt.Errorf("%w: brightness must be 1-100", ErrInvalidCommand)
	}
	if c.ColorTemperaturePct != nil && (*c.ColorTemperaturePct < 0 || *c.ColorTemperaturePct > 100) {
		return fmt.Errorf("%w: colorTemperaturePct must be 0-100", ErrInvalidCommand)
	}
	return nil
}

// TurnsOff reports whether the command switches the light off. Drivers skip
// level and color changes in that case so a correction can't relight a lamp.
func (c Command) TurnsOff() bool { return c.On != nil && !*c.On }

// RGB is a color, written in JSON as "#rrggbb".
type RGB struct{ R, G, B uint8 }

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// ParseRGB parses "#rrggbb".
func ParseRGB(s string) (RGB, error) {
	if !hexColor.MatchString(s) {
		return RGB{}, fmt.Errorf("%w: color must look like #rrggbb", ErrInvalidCommand)
	}
	v, _ := strconv.ParseUint(s[1:], 16, 32)
	return RGB{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v)}, nil
}

// Int packs the color as 0xRRGGBB, the form Govee expects.
func (c RGB) Int() int { return int(c.R)<<16 | int(c.G)<<8 | int(c.B) }

func (c RGB) String() string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

func (c RGB) MarshalJSON() ([]byte, error) { return []byte(strconv.Quote(c.String())), nil }

func (c *RGB) UnmarshalJSON(b []byte) error {
	s, err := strconv.Unquote(string(b))
	if err != nil {
		return fmt.Errorf("%w: color must be a string", ErrInvalidCommand)
	}
	parsed, err := ParseRGB(s)
	if err != nil {
		return err
	}
	*c = parsed
	return nil
}

// Bool and Int return pointers, for building commands in code.
func Bool(v bool) *bool { return &v }
func Int(v int) *int    { return &v }

// KelvinAtPct maps 0-100 onto a device's white range (0 = warmest).
func KelvinAtPct(minK, maxK, pct int) int {
	return minK + int(float64(maxK-minK)*float64(pct)/100+0.5)
}

// PctFromKelvin is the inverse of KelvinAtPct, clamped to 0-100.
func PctFromKelvin(minK, maxK, k int) int {
	if maxK <= minK {
		return 0
	}
	return clamp(int(float64(k-minK)*100/float64(maxK-minK)+0.5), 0, 100)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Features says which fields a device can actually change. A smart plug, say,
// only switches on and off. Devices that don't implement FeatureReporter are
// treated as supporting everything.
type Features struct {
	Brightness       bool `json:"brightness"`
	ColorTemperature bool `json:"colorTemperature"`
	Color            bool `json:"color"`
}

type FeatureReporter interface {
	Features() Features
}

// FeaturesOf returns d's features, defaulting to all of them.
func FeaturesOf(d Device) Features {
	if fr, ok := d.(FeatureReporter); ok {
		return fr.Features()
	}
	return Features{Brightness: true, ColorTemperature: true, Color: true}
}
