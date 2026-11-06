// Package matter talks to a Matter controller server (the Open Home Foundation
// Matter Server, https://github.com/matter-js/matterjs-server) over its
// WebSocket API. That server owns the Matter fabric: pairing, Thread and Wi-Fi
// credentials, and device sessions. This package keeps a live copy of every
// node's attributes and sends cluster commands on the hub's behalf.
package matter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// ErrNotConnected means the hub has no live session with the Matter server.
var ErrNotConnected = errors.New("not connected to the Matter server")

// Node is one commissioned Matter device. Attributes are keyed by
// "endpoint/cluster/attribute", e.g. "1/6/0" is OnOff on endpoint 1.
type Node struct {
	NodeID     uint64                     `json:"node_id"`
	Available  bool                       `json:"available"`
	IsBridge   bool                       `json:"is_bridge"`
	Attributes map[string]json.RawMessage `json:"attributes"`
}

// ServerError is an error the Matter server returned for a command.
type ServerError struct {
	Code    int
	Details string
}

func (e *ServerError) Error() string {
	return fmt.Sprintf("matter server error %d: %s", e.Code, e.Details)
}

type message struct {
	MessageID string          `json:"message_id,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	ErrorCode *int            `json:"error_code,omitempty"`
	Details   string          `json:"details,omitempty"`
	Event     string          `json:"event,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
}

type command struct {
	MessageID string `json:"message_id"`
	Command   string `json:"command"`
	Args      any    `json:"args,omitempty"`
}

// Client is a self-reconnecting connection to the Matter server. Start it with
// Run; until it connects, calls fail with ErrNotConnected.
type Client struct {
	url string
	log *slog.Logger

	nextID    atomic.Uint64
	connected atomic.Bool

	mu       sync.Mutex
	conn     *websocket.Conn
	pending  map[string]chan message
	listenID string
	nodes    map[uint64]*Node
}

// NewClient returns a client for the Matter server at url. Call Run to connect.
func NewClient(url string, log *slog.Logger) *Client {
	return &Client{url: url, log: log, pending: map[string]chan message{}, nodes: map[uint64]*Node{}}
}

// Connected reports whether the client is connected and has loaded the nodes.
func (c *Client) Connected() bool { return c.connected.Load() }

// Run connects and stays connected until ctx is cancelled, backing off between
// attempts.
func (c *Client) Run(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		started := time.Now()
		err := c.session(ctx)
		if ctx.Err() != nil {
			return
		}
		if time.Since(started) > time.Minute {
			backoff = time.Second
		}
		c.log.Warn("matter server connection lost", "err", err, "retry_in", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}

func (c *Client) session(ctx context.Context) error {
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	conn, resp, err := websocket.Dial(dialCtx, c.url, nil)
	cancel()
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return err
	}
	// Node dumps from start_listening can be large.
	conn.SetReadLimit(64 << 20)
	defer func() { _ = conn.CloseNow() }()

	// The server greets every connection with its server_info.
	var info json.RawMessage
	if err = wsjson.Read(ctx, conn, &info); err != nil {
		return fmt.Errorf("reading server info: %w", err)
	}
	c.log.Info("connected to matter server", "url", c.url, "info", string(info))

	listenID := c.newID()
	c.mu.Lock()
	c.conn = conn
	c.listenID = listenID
	c.mu.Unlock()

	readErr := make(chan error, 1)
	go func() { readErr <- c.readLoop(ctx, conn) }()

	// start_listening returns every node, then streams changes as events.
	if err = wsjson.Write(ctx, conn, command{MessageID: listenID, Command: "start_listening"}); err != nil {
		c.disconnect(conn)
		return err
	}
	err = <-readErr
	c.disconnect(conn)
	return err
}

func (c *Client) disconnect(conn *websocket.Conn) {
	c.connected.Store(false)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == conn {
		c.conn = nil
	}
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
}

func (c *Client) readLoop(ctx context.Context, conn *websocket.Conn) error {
	for {
		var msg message
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			return err
		}
		switch {
		case msg.Event != "":
			c.handleEvent(msg)
		case msg.MessageID != "":
			c.handleResult(msg)
		}
	}
}

func (c *Client) handleResult(msg message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if msg.MessageID == c.listenID {
		if msg.ErrorCode != nil {
			c.log.Error("start_listening failed", "code", *msg.ErrorCode, "details", msg.Details)
			return
		}
		var nodes []*Node
		if err := json.Unmarshal(msg.Result, &nodes); err != nil {
			c.log.Error("bad node list from matter server", "err", err)
			return
		}
		c.nodes = make(map[uint64]*Node, len(nodes))
		for _, n := range nodes {
			c.nodes[n.NodeID] = n
		}
		c.connected.Store(true)
		c.log.Info("matter nodes loaded", "count", len(nodes))
		return
	}
	if ch, ok := c.pending[msg.MessageID]; ok {
		ch <- msg
		delete(c.pending, msg.MessageID)
	}
}

func (c *Client) handleEvent(msg message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch msg.Event {
	case "node_added", "node_updated":
		var n Node
		if err := json.Unmarshal(msg.Data, &n); err == nil {
			c.nodes[n.NodeID] = &n
		}
	case "node_removed":
		var id uint64
		if err := json.Unmarshal(msg.Data, &id); err == nil {
			delete(c.nodes, id)
		}
	case "attribute_updated":
		// data is [node_id, "endpoint/cluster/attribute", value]
		var data []json.RawMessage
		if err := json.Unmarshal(msg.Data, &data); err != nil || len(data) != 3 {
			return
		}
		var id uint64
		var path string
		if json.Unmarshal(data[0], &id) != nil || json.Unmarshal(data[1], &path) != nil {
			return
		}
		if n, ok := c.nodes[id]; ok {
			if n.Attributes == nil {
				n.Attributes = map[string]json.RawMessage{}
			}
			n.Attributes[path] = data[2]
		}
	}
}

func (c *Client) newID() string { return strconv.FormatUint(c.nextID.Add(1), 10) }

// Call sends a raw command and waits for its result.
func (c *Client) Call(ctx context.Context, name string, args any) (json.RawMessage, error) {
	id := c.newID()
	ch := make(chan message, 1)
	c.mu.Lock()
	conn := c.conn
	if conn == nil || !c.connected.Load() {
		c.mu.Unlock()
		return nil, ErrNotConnected
	}
	c.pending[id] = ch
	c.mu.Unlock()

	if err := wsjson.Write(ctx, conn, command{MessageID: id, Command: name, Args: args}); err != nil {
		c.forget(id)
		return nil, err
	}
	select {
	case <-ctx.Done():
		c.forget(id)
		return nil, ctx.Err()
	case msg, ok := <-ch:
		if !ok {
			return nil, ErrNotConnected
		}
		if msg.ErrorCode != nil {
			return nil, &ServerError{Code: *msg.ErrorCode, Details: msg.Details}
		}
		return msg.Result, nil
	}
}

func (c *Client) forget(id string) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

// Node returns a copy of a node's cached data.
func (c *Client) Node(id uint64) (Node, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n, ok := c.nodes[id]
	if !ok {
		return Node{}, false
	}
	cp := *n
	cp.Attributes = make(map[string]json.RawMessage, len(n.Attributes))
	for k, v := range n.Attributes {
		cp.Attributes[k] = v
	}
	return cp, true
}

// NodeSummary is the short form of a node, for listing.
type NodeSummary struct {
	NodeID    uint64 `json:"nodeId"`
	Available bool   `json:"available"`
	IsBridge  bool   `json:"isBridge"`
	Vendor    string `json:"vendor,omitempty"`
	Product   string `json:"product,omitempty"`
	Label     string `json:"label,omitempty"`
}

// Basic Information cluster (0x28) attributes on endpoint 0.
const (
	attrVendorName  = "0/40/1"
	attrProductName = "0/40/3"
	attrNodeLabel   = "0/40/5"
)

// Nodes lists every commissioned node, sorted by id.
func (c *Client) Nodes() []NodeSummary {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]NodeSummary, 0, len(c.nodes))
	for _, n := range c.nodes {
		s := NodeSummary{NodeID: n.NodeID, Available: n.Available, IsBridge: n.IsBridge}
		_ = json.Unmarshal(n.Attributes[attrVendorName], &s.Vendor)
		_ = json.Unmarshal(n.Attributes[attrProductName], &s.Product)
		_ = json.Unmarshal(n.Attributes[attrNodeLabel], &s.Label)
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NodeID < out[j].NodeID })
	return out
}

// DeviceCommand invokes a cluster command on one endpoint of a node.
func (c *Client) DeviceCommand(ctx context.Context, nodeID uint64, endpoint, cluster int, name string, payload map[string]any) error {
	if payload == nil {
		payload = map[string]any{}
	}
	_, err := c.Call(ctx, "device_command", map[string]any{
		"node_id": nodeID, "endpoint_id": endpoint, "cluster_id": cluster,
		"command_name": name, "payload": payload,
	})
	return err
}

// Commission pairs a new device using its QR code ("MT:...") or manual
// pairing code. Set networkOnly for devices already on the network, so the
// server doesn't try Bluetooth. Wi-Fi and Thread devices need credentials set
// first.
func (c *Client) Commission(ctx context.Context, code string, networkOnly bool) (json.RawMessage, error) {
	return c.Call(ctx, "commission_with_code", map[string]any{"code": code, "network_only": networkOnly})
}

// SetWiFiCredentials sets the Wi-Fi network handed to devices during pairing.
func (c *Client) SetWiFiCredentials(ctx context.Context, ssid, password string) error {
	_, err := c.Call(ctx, "set_wifi_credentials", map[string]any{"ssid": ssid, "credentials": password})
	return err
}

// SetThreadDataset sets the Thread network handed to devices during pairing.
func (c *Client) SetThreadDataset(ctx context.Context, dataset string) error {
	_, err := c.Call(ctx, "set_thread_dataset", map[string]any{"dataset": dataset})
	return err
}

// RemoveNode unpairs a node from the hub's Matter fabric.
func (c *Client) RemoveNode(ctx context.Context, nodeID uint64) error {
	_, err := c.Call(ctx, "remove_node", map[string]any{"node_id": nodeID})
	return err
}
