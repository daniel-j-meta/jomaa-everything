package matter

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/jomaa-daniel/jomaa-everything/jomaa-home-hub/internal/device"
)

// Cluster ids and attribute ids from the Matter spec.
const (
	clusterOnOff        = 6
	clusterLevelControl = 8
	clusterColorControl = 0x0300

	attrOnOff             = 0      // OnOff.OnOff
	attrCurrentLevel      = 0      // LevelControl.CurrentLevel
	attrCurrentHue        = 0      // ColorControl.CurrentHue
	attrColorTempMireds   = 7      // ColorControl.ColorTemperatureMireds
	attrColorTempMinMired = 0x400B // ColorControl.ColorTempPhysicalMinMireds
	attrColorTempMaxMired = 0x400C // ColorControl.ColorTempPhysicalMaxMireds

	maxLevel = 254
)

// Light is a Matter light (or plug) on one endpoint of a commissioned node.
// Commands go out through the Matter server; state comes from the attribute
// reports the server streams back.
type Light struct {
	client     *Client
	id, name   string
	nodeID     uint64
	endpoint   int
	minK, maxK int // fallback when the device doesn't report its range
}

type LightConfig struct {
	ID, Name  string
	NodeID    uint64
	Endpoint  int
	MinKelvin int
	MaxKelvin int
}

func NewLight(c *Client, cfg LightConfig) *Light {
	if cfg.Endpoint == 0 {
		cfg.Endpoint = 1
	}
	if cfg.MinKelvin == 0 {
		cfg.MinKelvin = 2700
	}
	if cfg.MaxKelvin == 0 {
		cfg.MaxKelvin = 6500
	}
	return &Light{client: c, id: cfg.ID, name: cfg.Name, nodeID: cfg.NodeID, endpoint: cfg.Endpoint, minK: cfg.MinKelvin, maxK: cfg.MaxKelvin}
}

func (l *Light) ID() string   { return l.id }
func (l *Light) Name() string { return l.name }

func (l *Light) path(cluster, attr int) string {
	return fmt.Sprintf("%d/%d/%d", l.endpoint, cluster, attr)
}

func (l *Light) node() (Node, error) {
	n, ok := l.client.Node(l.nodeID)
	if !ok {
		if !l.client.Connected() {
			return Node{}, ErrNotConnected
		}
		return Node{}, fmt.Errorf("matter node %d is not commissioned", l.nodeID)
	}
	if !n.Available {
		return Node{}, fmt.Errorf("matter node %d is unreachable", l.nodeID)
	}
	return n, nil
}

// Features reads what the endpoint supports from which clusters it exposes.
func (l *Light) Features() device.Features {
	n, ok := l.client.Node(l.nodeID)
	if !ok {
		return device.Features{}
	}
	_, level := n.Attributes[l.path(clusterLevelControl, attrCurrentLevel)]
	_, temp := n.Attributes[l.path(clusterColorControl, attrColorTempMireds)]
	_, hue := n.Attributes[l.path(clusterColorControl, attrCurrentHue)]
	return device.Features{Brightness: level, ColorTemperature: temp, Color: hue}
}

// kelvinRange prefers the range the device reports over the configured one.
func (l *Light) kelvinRange(n Node) (minK, maxK int) {
	var minM, maxM int
	_ = json.Unmarshal(n.Attributes[l.path(clusterColorControl, attrColorTempMinMired)], &minM)
	_ = json.Unmarshal(n.Attributes[l.path(clusterColorControl, attrColorTempMaxMired)], &maxM)
	if minM > 0 && maxM > minM {
		return miredsToKelvin(maxM), miredsToKelvin(minM)
	}
	return l.minK, l.maxK
}

// Apply sends power first, then level and color. Fields the device doesn't
// support are skipped, so a room can mix bulbs and plugs.
func (l *Light) Apply(ctx context.Context, cmd device.Command) error {
	n, err := l.node()
	if err != nil {
		return err
	}
	f := l.Features()
	send := func(cluster int, name string, payload map[string]any) error {
		return l.client.DeviceCommand(ctx, l.nodeID, l.endpoint, cluster, name, payload)
	}
	opts := func(p map[string]any) map[string]any {
		p["transitionTime"] = 0
		p["optionsMask"] = 0
		p["optionsOverride"] = 0
		return p
	}

	if cmd.On != nil {
		name := "Off"
		if *cmd.On {
			name = "On"
		}
		if err := send(clusterOnOff, name, nil); err != nil {
			return err
		}
	}
	if cmd.TurnsOff() {
		return nil
	}
	if cmd.Brightness != nil && f.Brightness {
		if err := send(clusterLevelControl, "MoveToLevelWithOnOff", opts(map[string]any{"level": pctToLevel(*cmd.Brightness)})); err != nil {
			return err
		}
	}
	if cmd.ColorTemperaturePct != nil && f.ColorTemperature {
		minK, maxK := l.kelvinRange(n)
		mireds := kelvinToMireds(device.KelvinAtPct(minK, maxK, *cmd.ColorTemperaturePct))
		if err := send(clusterColorControl, "MoveToColorTemperature", opts(map[string]any{"colorTemperatureMireds": mireds})); err != nil {
			return err
		}
	}
	if cmd.Color != nil && f.Color {
		hue, sat := rgbToHueSat(*cmd.Color)
		if err := send(clusterColorControl, "MoveToHueAndSaturation", opts(map[string]any{"hue": hue, "saturation": sat})); err != nil {
			return err
		}
	}
	return nil
}

func (l *Light) State(ctx context.Context) (device.State, error) {
	n, err := l.node()
	if err != nil {
		return device.State{}, err
	}
	var s device.State
	if err := json.Unmarshal(n.Attributes[l.path(clusterOnOff, attrOnOff)], &s.On); err != nil {
		return device.State{}, fmt.Errorf("matter node %d has no OnOff on endpoint %d", l.nodeID, l.endpoint)
	}
	// CurrentLevel and ColorTemperatureMireds can be null; leave zero then.
	var level *int
	if json.Unmarshal(n.Attributes[l.path(clusterLevelControl, attrCurrentLevel)], &level) == nil && level != nil {
		s.Brightness = levelToPct(*level)
	}
	var mireds *int
	if json.Unmarshal(n.Attributes[l.path(clusterColorControl, attrColorTempMireds)], &mireds) == nil && mireds != nil && *mireds > 0 {
		minK, maxK := l.kelvinRange(n)
		s.ColorTemperaturePct = device.PctFromKelvin(minK, maxK, miredsToKelvin(*mireds))
	}
	return s, nil
}

func pctToLevel(pct int) int {
	return max(1, int(math.Round(float64(pct)*maxLevel/100)))
}

func levelToPct(level int) int {
	return int(math.Round(float64(level) * 100 / maxLevel))
}

func kelvinToMireds(k int) int { return int(math.Round(1e6 / float64(k))) }
func miredsToKelvin(m int) int { return int(math.Round(1e6 / float64(m))) }

// rgbToHueSat converts to Matter's 0-254 hue and saturation scales.
func rgbToHueSat(c device.RGB) (hue, sat int) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	hi, lo := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	d := hi - lo
	var h float64
	switch {
	case d == 0:
		h = 0
	case hi == r:
		h = math.Mod((g-b)/d, 6)
	case hi == g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	s := 0.0
	if hi > 0 {
		s = d / hi
	}
	return int(math.Round(h / 360 * 254)), int(math.Round(s * 254))
}
