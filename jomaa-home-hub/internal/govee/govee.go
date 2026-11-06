// Package govee drives Govee lights through Govee's cloud API
// (https://developer.govee.com). It is the Go port of device-panel's GoveeLight.
package govee

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jomaa-daniel/jomaa-everything/jomaa-home-hub/internal/device"
)

// DefaultBaseURL is Govee's public API.
const DefaultBaseURL = "https://openapi.api.govee.com"

const (
	capOnOff = "devices.capabilities.on_off"
	capRange = "devices.capabilities.range"
	capColor = "devices.capabilities.color_setting"
)

// Client holds the API key and HTTP settings shared by every Govee light.
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

// NewClient returns a client for Govee's cloud API.
func NewClient(apiKey string) *Client {
	return &Client{BaseURL: DefaultBaseURL, APIKey: apiKey, HTTP: &http.Client{Timeout: 10 * time.Second}}
}

type capability struct {
	Type     string `json:"type"`
	Instance string `json:"instance"`
	Value    any    `json:"value,omitempty"`
	State    *struct {
		Value json.RawMessage `json:"value"`
	} `json:"state,omitempty"`
}

type payload struct {
	SKU          string       `json:"sku"`
	Device       string       `json:"device"`
	Capability   *capability  `json:"capability,omitempty"`
	Capabilities []capability `json:"capabilities,omitempty"`
}

type envelope struct {
	RequestID string  `json:"requestId"`
	Payload   payload `json:"payload"`
}

type response struct {
	Code    int     `json:"code"`
	Message string  `json:"msg"`
	Payload payload `json:"payload"`
}

func (c *Client) post(ctx context.Context, path string, p payload) (*response, error) {
	body, err := json.Marshal(envelope{RequestID: requestID(), Payload: p})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Govee-API-Key", c.APIKey)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("govee %s: %w", path, err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("govee %s: HTTP %d: %s", path, res.StatusCode, bytes.TrimSpace(raw))
	}
	var out response
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("govee %s: bad response: %w", path, err)
	}
	// Govee reports some failures as HTTP 200 with a non-200 code in the body.
	if out.Code != 0 && out.Code != http.StatusOK {
		return nil, fmt.Errorf("govee %s: code %d: %s", path, out.Code, out.Message)
	}
	return &out, nil
}

// Light is one Govee bulb or lamp.
type Light struct {
	client     *Client
	id, name   string
	sku, mac   string
	minK, maxK int
}

// LightConfig mirrors one entry from device-panel's light list.
type LightConfig struct {
	ID, Name  string
	SKU       string
	Device    string // the colon-separated device id Govee assigns
	MinKelvin int
	MaxKelvin int
}

// NewLight returns a light that sends its commands through c.
func NewLight(c *Client, cfg LightConfig) *Light {
	return &Light{client: c, id: cfg.ID, name: cfg.Name, sku: cfg.SKU, mac: cfg.Device, minK: cfg.MinKelvin, maxK: cfg.MaxKelvin}
}

// ID returns the hub id of the light.
func (l *Light) ID() string { return l.id }

// Name returns the display name.
func (l *Light) Name() string { return l.name }

func (l *Light) control(ctx context.Context, typ, instance string, value any) error {
	_, err := l.client.post(ctx, "/router/api/v1/device/control", payload{
		SKU: l.sku, Device: l.mac,
		Capability: &capability{Type: typ, Instance: instance, Value: value},
	})
	return err
}

// Apply sends each changed field as its own Govee call, power first. Govee's
// API takes one capability per request.
func (l *Light) Apply(ctx context.Context, cmd device.Command) error {
	if cmd.On != nil {
		v := 0
		if *cmd.On {
			v = 1
		}
		if err := l.control(ctx, capOnOff, "powerSwitch", v); err != nil {
			return err
		}
	}
	if cmd.TurnsOff() {
		return nil
	}
	if cmd.Brightness != nil {
		if err := l.control(ctx, capRange, "brightness", *cmd.Brightness); err != nil {
			return err
		}
	}
	if cmd.ColorTemperaturePct != nil {
		k := device.KelvinAtPct(l.minK, l.maxK, *cmd.ColorTemperaturePct)
		if err := l.control(ctx, capColor, "colorTemperatureK", k); err != nil {
			return err
		}
	}
	if cmd.Color != nil {
		if err := l.control(ctx, capColor, "colorRgb", cmd.Color.Int()); err != nil {
			return err
		}
	}
	return nil
}

// State asks Govee for the light's current state.
func (l *Light) State(ctx context.Context) (device.State, error) {
	res, err := l.client.post(ctx, "/router/api/v1/device/state", payload{SKU: l.sku, Device: l.mac})
	if err != nil {
		return device.State{}, err
	}
	values := map[string]float64{}
	for _, c := range res.Payload.Capabilities {
		if c.State == nil {
			continue
		}
		var v float64
		if json.Unmarshal(c.State.Value, &v) == nil {
			values[c.Instance] = v
		}
	}
	power, ok := values["powerSwitch"]
	if !ok {
		return device.State{}, fmt.Errorf("govee state for %s: no powerSwitch value", l.id)
	}
	brightness, ok := values["brightness"]
	if !ok {
		return device.State{}, fmt.Errorf("govee state for %s: no brightness value", l.id)
	}
	s := device.State{On: power == 1, Brightness: int(brightness)}
	if k, ok := values["colorTemperatureK"]; ok && k > 0 {
		s.ColorTemperaturePct = device.PctFromKelvin(l.minK, l.maxK, int(k))
	}
	return s, nil
}

func requestID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
