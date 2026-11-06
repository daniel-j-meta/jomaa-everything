// Package config loads the hub's device and room layout from a JSON file.
// Secrets (the hub password, the Govee API key) come from the environment,
// never from this file.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
)

type Config struct {
	Listen          string   `json:"listen"`
	MatterServerURL string   `json:"matterServerUrl"`
	Devices         []Device `json:"devices"`
	Rooms           []Room   `json:"rooms"`
}

// Device is one light. Driver picks which fields apply:
// "govee" uses SKU and Device; "matter" uses NodeID and Endpoint.
type Device struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Driver    string `json:"driver"`
	SKU       string `json:"sku,omitempty"`
	Device    string `json:"device,omitempty"`
	NodeID    uint64 `json:"nodeId,omitempty"`
	Endpoint  int    `json:"endpoint,omitempty"`
	MinKelvin int    `json:"minKelvin,omitempty"`
	MaxKelvin int    `json:"maxKelvin,omitempty"`
}

// Room groups devices. Members may name devices or rooms listed earlier in the
// file, so a "downstairs" room can contain "living-room" and "kitchen".
type Room struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Members []string `json:"members"`
}

const (
	DriverGovee  = "govee"
	DriverMatter = "matter"
)

var validID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(raw)
}

func Parse(raw []byte) (*Config, error) {
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if c.Listen == "" {
		c.Listen = ":8080"
	}
	if c.MatterServerURL == "" {
		c.MatterServerURL = "ws://127.0.0.1:5580/ws"
	}
	return &c, c.Validate()
}

func (c *Config) Validate() error {
	seen := map[string]bool{}
	claim := func(id string) error {
		if !validID.MatchString(id) {
			return fmt.Errorf("config: id %q must be lowercase letters, digits and dashes", id)
		}
		if seen[id] {
			return fmt.Errorf("config: id %q is used twice", id)
		}
		seen[id] = true
		return nil
	}
	for _, d := range c.Devices {
		if err := claim(d.ID); err != nil {
			return err
		}
		switch d.Driver {
		case DriverGovee:
			if d.SKU == "" || d.Device == "" {
				return fmt.Errorf("config: govee device %q needs sku and device", d.ID)
			}
			if d.MinKelvin <= 0 || d.MaxKelvin <= d.MinKelvin {
				return fmt.Errorf("config: govee device %q needs minKelvin < maxKelvin", d.ID)
			}
		case DriverMatter:
			if d.NodeID == 0 {
				return fmt.Errorf("config: matter device %q needs nodeId", d.ID)
			}
		default:
			return fmt.Errorf("config: device %q has unknown driver %q", d.ID, d.Driver)
		}
	}
	for _, r := range c.Rooms {
		// Check members before claiming the room's own id, so a room can't
		// contain itself and every reference points backwards (no cycles).
		if len(r.Members) == 0 {
			return fmt.Errorf("config: room %q has no members", r.ID)
		}
		for _, m := range r.Members {
			if !seen[m] {
				return fmt.Errorf("config: room %q lists %q, which isn't a device or an earlier room", r.ID, m)
			}
		}
		if err := claim(r.ID); err != nil {
			return err
		}
	}
	return nil
}

// UsesDriver reports whether any device needs the given driver.
func (c *Config) UsesDriver(driver string) bool {
	for _, d := range c.Devices {
		if d.Driver == driver {
			return true
		}
	}
	return false
}
