package config

import (
	"os"
	"strings"
	"testing"
)

func TestExampleConfigIsValid(t *testing.T) {
	c, err := Load("../../config.example.json")
	if err != nil {
		t.Fatal(err)
	}
	if !c.UsesDriver(DriverGovee) || !c.UsesDriver(DriverMatter) {
		t.Fatal("example should show both drivers")
	}
}

func TestDefaults(t *testing.T) {
	c, err := Parse([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != ":8080" || c.MatterServerURL != "ws://127.0.0.1:5580/ws" {
		t.Fatalf("defaults = %+v", c)
	}
}

func TestRejects(t *testing.T) {
	cases := map[string]string{
		"duplicate id":         `{"devices":[{"id":"a","driver":"matter","nodeId":1},{"id":"a","driver":"matter","nodeId":2}]}`,
		"bad id":               `{"devices":[{"id":"Living Room","driver":"matter","nodeId":1}]}`,
		"unknown driver":       `{"devices":[{"id":"a","driver":"hue"}]}`,
		"govee no sku":         `{"devices":[{"id":"a","driver":"govee","device":"x","minKelvin":1,"maxKelvin":2}]}`,
		"govee no range":       `{"devices":[{"id":"a","driver":"govee","sku":"s","device":"x"}]}`,
		"matter no node":       `{"devices":[{"id":"a","driver":"matter"}]}`,
		"unknown member":       `{"rooms":[{"id":"r","members":["ghost"]}]}`,
		"room contains itself": `{"rooms":[{"id":"r","members":["r"]}]}`,
		"empty room":           `{"rooms":[{"id":"r","members":[]}]}`,
		"not json":             `{`,
	}
	for name, raw := range cases {
		if _, err := Parse([]byte(raw)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestMissingFile(t *testing.T) {
	_, err := Load("does-not-exist.json")
	if !os.IsNotExist(err) && (err == nil || !strings.Contains(err.Error(), "no such file")) {
		t.Fatalf("err = %v", err)
	}
}
