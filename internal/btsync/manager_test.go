package btsync

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

type captureTransport struct {
	mu    sync.Mutex
	local PeerRef
	sent  map[string][][]byte
}

func newCaptureTransport(id string) *captureTransport {
	return &captureTransport{local: PeerRef{PeerID: id, Name: id, Platform: "test"}, sent: map[string][][]byte{}}
}

func (t *captureTransport) Start(_ Context, _ Role, _ string, _ func(PeerRef, []byte), _ func(PeerEvent)) error {
	return nil
}
func (t *captureTransport) Stop() error                    { return nil }
func (t *captureTransport) SupportsRole(_ Role) error      { return nil }
func (t *captureTransport) LocalPeer() PeerRef             { return t.local }
func (t *captureTransport) Broadcast(payload []byte) error { return t.Send("broadcast", payload) }
func (t *captureTransport) Send(peerID string, payload []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	cp := make([]byte, len(payload))
	copy(cp, payload)
	t.sent[peerID] = append(t.sent[peerID], cp)
	return nil
}
func (t *captureTransport) last(peerID string) (Message, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	arr := t.sent[peerID]
	if len(arr) == 0 {
		return Message{}, false
	}
	var m Message
	if err := json.Unmarshal(arr[len(arr)-1], &m); err != nil {
		return Message{}, false
	}
	return m, true
}

func setupChildManager(t *testing.T) (*Manager, *captureTransport, *int, *time.Time) {
	t.Helper()

	applyCount := 0
	var appliedAt time.Time
	tr := newCaptureTransport("child")
	mgr := NewManager(tr, ManagerOptions{
		ApplyScene: func(scene string, source Source) error {
			applyCount++
			appliedAt = time.Now()
			return nil
		},
		SceneExists: func(scene string) (bool, error) {
			return scene == "SceneA", nil
		},
	})

	cfg := Config{Enabled: true, Role: RoleChild, AcceptLateMs: 200}.Normalize()

	mgr.mu.Lock()
	mgr.cfg = cfg
	mgr.running = true
	mgr.role = RoleChild
	mgr.peers = map[string]*peerState{
		"parent": {
			TrustedPeer: TrustedPeer{PeerID: "parent", Name: "parent", Secret: "secret"},
			Connected:   true,
		},
	}
	mgr.mu.Unlock()

	return mgr, tr, &applyCount, &appliedAt
}

func TestSceneHMACVerify(t *testing.T) {
	msg := Message{
		Type:            MsgSceneCommand,
		ProtocolVersion: ProtocolVersion,
		EventID:         "evt-1",
		SceneName:       "SceneA",
		Source:          string(SourceGUI),
		FireAtUnixMs:    time.Now().Add(100 * time.Millisecond).UnixMilli(),
		SentAtUnixMs:    time.Now().UnixMilli(),
	}
	msg.HMAC = sceneHMAC("secret", msg)
	if !verifySceneHMAC("secret", msg) {
		t.Fatalf("HMAC should verify")
	}
	msg.SceneName = "tampered"
	if verifySceneHMAC("secret", msg) {
		t.Fatalf("tampered message must fail HMAC verification")
	}
}

func TestChildDropsLateSceneCommand(t *testing.T) {
	mgr, tr, applyCount, _ := setupChildManager(t)

	msg := Message{
		Type:            MsgSceneCommand,
		ProtocolVersion: ProtocolVersion,
		EventID:         "evt-late",
		SceneName:       "SceneA",
		Source:          string(SourceGUI),
		FireAtUnixMs:    time.Now().Add(-2 * time.Second).UnixMilli(),
		SentAtUnixMs:    time.Now().Add(-2 * time.Second).UnixMilli(),
	}
	msg.HMAC = sceneHMAC("secret", msg)

	mgr.handleSceneCommand(PeerRef{PeerID: "parent", Name: "parent", Platform: "test"}, msg)

	if *applyCount != 0 {
		t.Fatalf("late command should not apply scene")
	}
	ack, ok := tr.last("parent")
	if !ok {
		t.Fatalf("expected ACK")
	}
	if ack.Status != string(AckLateDrop) {
		t.Fatalf("expected %s, got %s", AckLateDrop, ack.Status)
	}
}

func TestChildDeduplicatesSceneCommand(t *testing.T) {
	mgr, tr, applyCount, appliedAt := setupChildManager(t)

	fireAt := time.Now().Add(80 * time.Millisecond)
	msg := Message{
		Type:            MsgSceneCommand,
		ProtocolVersion: ProtocolVersion,
		EventID:         "evt-dup",
		SceneName:       "SceneA",
		Source:          string(SourceMIDI),
		FireAtUnixMs:    fireAt.UnixMilli(),
		SentAtUnixMs:    time.Now().UnixMilli(),
	}
	msg.HMAC = sceneHMAC("secret", msg)

	mgr.handleSceneCommand(PeerRef{PeerID: "parent", Name: "parent", Platform: "test"}, msg)
	mgr.handleSceneCommand(PeerRef{PeerID: "parent", Name: "parent", Platform: "test"}, msg)

	time.Sleep(160 * time.Millisecond)

	if *applyCount != 1 {
		t.Fatalf("expected 1 apply, got %d", *applyCount)
	}
	if appliedAt.IsZero() {
		t.Fatalf("expected apply timestamp")
	}
	if appliedAt.Before(fireAt.Add(-20 * time.Millisecond)) {
		t.Fatalf("applied too early: fireAt=%v appliedAt=%v", fireAt, *appliedAt)
	}

	ack, ok := tr.last("parent")
	if !ok {
		t.Fatalf("expected ACK")
	}
	if ack.Status != string(AckOK) {
		t.Fatalf("expected %s, got %s", AckOK, ack.Status)
	}
}

func TestChildRejectsTamperedHMAC(t *testing.T) {
	mgr, tr, applyCount, _ := setupChildManager(t)

	msg := Message{
		Type:            MsgSceneCommand,
		ProtocolVersion: ProtocolVersion,
		EventID:         "evt-bad-hmac",
		SceneName:       "SceneA",
		Source:          string(SourceGUI),
		FireAtUnixMs:    time.Now().Add(50 * time.Millisecond).UnixMilli(),
		SentAtUnixMs:    time.Now().UnixMilli(),
		HMAC:            "deadbeef",
	}

	mgr.handleSceneCommand(PeerRef{PeerID: "parent", Name: "parent", Platform: "test"}, msg)

	if *applyCount != 0 {
		t.Fatalf("tampered command should not apply scene")
	}
	ack, ok := tr.last("parent")
	if !ok {
		t.Fatalf("expected ACK")
	}
	if ack.Status != string(AckError) {
		t.Fatalf("expected %s, got %s", AckError, ack.Status)
	}
}
