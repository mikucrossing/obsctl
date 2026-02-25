package btsync

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

const (
	ProtocolVersion = 1

	MsgPairRequest  = "pair_request"
	MsgPairAccept   = "pair_accept"
	MsgSceneCommand = "scene_command"
	MsgSceneAck     = "scene_ack"
	MsgHeartbeat    = "heartbeat"
)

type Role string

const (
	RoleOff    Role = "off"
	RoleParent Role = "parent"
	RoleChild  Role = "child"
)

type Source string

const (
	SourceGUI  Source = "gui"
	SourceMIDI Source = "midi"
)

type AckStatus string

const (
	AckOK       AckStatus = "ok"
	AckNotFound AckStatus = "not_found"
	AckError    AckStatus = "error"
	AckLateDrop AckStatus = "late_drop"
)

type TrustedPeer struct {
	PeerID   string `json:"peer_id"`
	Name     string `json:"name"`
	Secret   string `json:"secret"`
	LastSeen string `json:"last_seen"`
	Platform string `json:"platform"`
}

type Config struct {
	Enabled           bool          `json:"enabled"`
	Role              Role          `json:"role"`
	DeviceName        string        `json:"device_name"`
	LeadTimeMs        int           `json:"lead_time_ms"`
	PairingCodeTTLSec int           `json:"pairing_code_ttl_sec"`
	AcceptLateMs      int           `json:"accept_late_ms"`
	MaxNodes          int           `json:"max_nodes"`
	AutoReconnect     bool          `json:"auto_reconnect"`
	DropMissedEvents  bool          `json:"drop_missed_events"`
	TrustedPeers      []TrustedPeer `json:"trusted_peers"`
}

func (c Config) Normalize() Config {
	out := c
	switch out.Role {
	case RoleOff, RoleParent, RoleChild:
	default:
		out.Role = RoleOff
	}
	if out.LeadTimeMs <= 0 {
		out.LeadTimeMs = 300
	}
	if out.PairingCodeTTLSec <= 0 {
		out.PairingCodeTTLSec = 60
	}
	if out.AcceptLateMs <= 0 {
		out.AcceptLateMs = 500
	}
	if out.MaxNodes <= 0 {
		out.MaxNodes = 4
	}
	legacyUnset := !out.Enabled &&
		out.Role == RoleOff &&
		strings.TrimSpace(out.DeviceName) == "" &&
		out.LeadTimeMs == 300 &&
		out.PairingCodeTTLSec == 60 &&
		out.AcceptLateMs == 500 &&
		out.MaxNodes == 4 &&
		!out.AutoReconnect &&
		!out.DropMissedEvents &&
		len(out.TrustedPeers) == 0
	if strings.TrimSpace(out.DeviceName) == "" {
		out.DeviceName = defaultDeviceName()
	}
	if legacyUnset {
		out.AutoReconnect = true
		out.DropMissedEvents = true
	}
	if out.TrustedPeers == nil {
		out.TrustedPeers = []TrustedPeer{}
	}
	return out
}

func defaultDeviceName() string {
	host := strings.TrimSpace(runtime.GOOS)
	if n, err := os.Hostname(); err == nil && strings.TrimSpace(n) != "" {
		host = strings.TrimSpace(n)
	}
	host = strings.ReplaceAll(host, " ", "-")
	return fmt.Sprintf("obsctl-%s", host)
}

type Status struct {
	Supported                bool         `json:"supported"`
	SupportedRoleParent      bool         `json:"supported_role_parent"`
	SupportedRoleChild       bool         `json:"supported_role_child"`
	UnsupportedReason        string       `json:"unsupported_reason,omitempty"`
	Running                  bool         `json:"running"`
	Enabled                  bool         `json:"enabled"`
	Role                     Role         `json:"role"`
	DeviceName               string       `json:"device_name"`
	ConnectedPeers           int          `json:"connected_peers"`
	ParentConnected          bool         `json:"parent_connected"`
	PairingCodeActive        bool         `json:"pairing_code_active"`
	PairingCodeExpiresUnixMs int64        `json:"pairing_code_expires_unix_ms,omitempty"`
	LastError                string       `json:"last_error,omitempty"`
	Peers                    []PeerStatus `json:"peers"`
}

type PeerStatus struct {
	PeerID         string `json:"peer_id"`
	Name           string `json:"name"`
	Platform       string `json:"platform"`
	Connected      bool   `json:"connected"`
	LastSeenUnixMs int64  `json:"last_seen_unix_ms,omitempty"`
	LastAckUnixMs  int64  `json:"last_ack_unix_ms,omitempty"`
	LastAckStatus  string `json:"last_ack_status,omitempty"`
	LastLatencyMs  int64  `json:"last_latency_ms,omitempty"`
}

type Message struct {
	Type            string `json:"type"`
	ProtocolVersion int    `json:"protocol_version,omitempty"`

	EventID      string `json:"event_id,omitempty"`
	SceneName    string `json:"scene_name,omitempty"`
	Source       string `json:"source,omitempty"`
	FireAtUnixMs int64  `json:"fire_at_unix_ms,omitempty"`
	SentAtUnixMs int64  `json:"sent_at_unix_ms,omitempty"`
	HMAC         string `json:"hmac,omitempty"`

	PairingCode string `json:"pairing_code,omitempty"`
	PeerID      string `json:"peer_id,omitempty"`
	PeerName    string `json:"peer_name,omitempty"`
	Platform    string `json:"platform,omitempty"`
	Secret      string `json:"secret,omitempty"`

	Status    string `json:"status,omitempty"`
	Error     string `json:"error,omitempty"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
}

func (m Message) Marshal() ([]byte, error) {
	return json.Marshal(m)
}

func UnmarshalMessage(payload []byte) (Message, error) {
	var m Message
	err := json.Unmarshal(payload, &m)
	return m, err
}

func sceneHMAC(secret string, m Message) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(fmt.Sprintf("%d|%s|%s|%s|%d|%d", ProtocolVersion, m.EventID, m.SceneName, m.Source, m.FireAtUnixMs, m.SentAtUnixMs)))
	return hex.EncodeToString(mac.Sum(nil))
}

func verifySceneHMAC(secret string, m Message) bool {
	got := sceneHMAC(secret, m)
	return hmac.Equal([]byte(strings.ToLower(got)), []byte(strings.ToLower(m.HMAC)))
}

type PeerRef struct {
	PeerID   string
	Name     string
	Platform string
}

type PeerEvent struct {
	Peer      PeerRef
	Connected bool
	At        time.Time
}

type Transport interface {
	Start(ctx Context, role Role, deviceName string, onMessage func(from PeerRef, payload []byte), onPeerEvent func(PeerEvent)) error
	Stop() error
	Send(peerID string, payload []byte) error
	Broadcast(payload []byte) error
	SupportsRole(role Role) error
	LocalPeer() PeerRef
}

type Context interface {
	Done() <-chan struct{}
}
