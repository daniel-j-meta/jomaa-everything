package govee

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jomaa-daniel/jomaa-everything/jomaa-home-hub/internal/device"
)

type call struct {
	Path string
	Key  string
	Env  envelope
}

func fakeGovee(t *testing.T, handle func(call) (int, string)) (*Client, *[]call) {
	t.Helper()
	var mu sync.Mutex
	calls := &[]call{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env envelope
		_ = json.NewDecoder(r.Body).Decode(&env)
		c := call{Path: r.URL.Path, Key: r.Header.Get("Govee-API-Key"), Env: env}
		mu.Lock()
		*calls = append(*calls, c)
		mu.Unlock()
		status, body := handle(c)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c := NewClient("test-key")
	c.BaseURL = srv.URL
	return c, calls
}

func lamp(c *Client) *Light {
	return NewLight(c, LightConfig{ID: "lamp", Name: "Lamp", SKU: "H607C", Device: "3F:3C", MinKelvin: 2200, MaxKelvin: 6500})
}

func TestApplySendsOneCallPerField(t *testing.T) {
	c, calls := fakeGovee(t, func(call) (int, string) { return 200, `{"code":200,"msg":"success"}` })
	red, _ := device.ParseRGB("#ff0000")
	err := lamp(c).Apply(context.Background(), device.Command{
		On: device.Bool(true), Brightness: device.Int(50), ColorTemperaturePct: device.Int(50), Color: &red,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		instance string
		value    float64
	}{{"powerSwitch", 1}, {"brightness", 50}, {"colorTemperatureK", 4350}, {"colorRgb", 0xff0000}}
	if len(*calls) != len(want) {
		t.Fatalf("got %d calls, want %d", len(*calls), len(want))
	}
	for i, w := range want {
		got := (*calls)[i]
		if got.Path != "/router/api/v1/device/control" || got.Key != "test-key" {
			t.Fatalf("call %d: path %s key %q", i, got.Path, got.Key)
		}
		capab := got.Env.Payload.Capability
		if capab.Instance != w.instance || capab.Value.(float64) != w.value {
			t.Fatalf("call %d: %s=%v, want %s=%v", i, capab.Instance, capab.Value, w.instance, w.value)
		}
		if got.Env.Payload.SKU != "H607C" || got.Env.Payload.Device != "3F:3C" || got.Env.RequestID == "" {
			t.Fatalf("call %d: bad envelope %+v", i, got.Env)
		}
	}
}

func TestTurningOffSkipsLevels(t *testing.T) {
	c, calls := fakeGovee(t, func(call) (int, string) { return 200, `{"code":200}` })
	err := lamp(c).Apply(context.Background(), device.Command{On: device.Bool(false), Brightness: device.Int(80)})
	if err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || (*calls)[0].Env.Payload.Capability.Value.(float64) != 0 {
		t.Fatalf("calls = %+v", *calls)
	}
}

func TestErrorInBodyIsAFailure(t *testing.T) {
	c, _ := fakeGovee(t, func(call) (int, string) { return 200, `{"code":400,"msg":"devices not exist"}` })
	err := lamp(c).Apply(context.Background(), device.Command{On: device.Bool(true)})
	if err == nil || !strings.Contains(err.Error(), "devices not exist") {
		t.Fatalf("err = %v", err)
	}
}

func TestHTTPErrorIsAFailure(t *testing.T) {
	c, _ := fakeGovee(t, func(call) (int, string) { return 429, `rate limited` })
	if err := lamp(c).Apply(context.Background(), device.Command{On: device.Bool(true)}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestState(t *testing.T) {
	c, calls := fakeGovee(t, func(call) (int, string) {
		return 200, `{"code":200,"payload":{"sku":"H607C","device":"3F:3C","capabilities":[
			{"type":"devices.capabilities.online","instance":"online","state":{"value":true}},
			{"type":"devices.capabilities.on_off","instance":"powerSwitch","state":{"value":1}},
			{"type":"devices.capabilities.range","instance":"brightness","state":{"value":75}},
			{"type":"devices.capabilities.color_setting","instance":"colorTemperatureK","state":{"value":4350}}]}}`
	})
	s, err := lamp(c).State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if (*calls)[0].Path != "/router/api/v1/device/state" {
		t.Fatalf("path = %s", (*calls)[0].Path)
	}
	if s != (device.State{On: true, Brightness: 75, ColorTemperaturePct: 50}) {
		t.Fatalf("state = %+v", s)
	}
}

func TestStateWithoutPowerIsAnError(t *testing.T) {
	c, _ := fakeGovee(t, func(call) (int, string) { return 200, `{"code":200,"payload":{"capabilities":[]}}` })
	if _, err := lamp(c).State(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
}
