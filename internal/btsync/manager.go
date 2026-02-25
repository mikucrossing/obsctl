package btsync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"awesomeProject/internal/obsws"
)

type ManagerOptions struct {
	ApplyScene          func(scene string, source Source) error
	SceneExists         func(scene string) (bool, error)
	PersistTrustedPeers func(peers []TrustedPeer) error
	Logf                func(level, msg string)
}

type peerState struct {
	TrustedPeer
	Connected     bool
	LastSeenAt    time.Time
	LastAckAt     time.Time
	LastAckStatus string
	LastLatencyMs int64
}

type Manager struct {
	mu        sync.Mutex
	cfg       Config
	transport Transport
	opts      ManagerOptions

	running bool
	role    Role
	cancel  context.CancelFunc

	pairingCode    string
	pairingExpires time.Time

	seenEvents  map[string]time.Time
	peers       map[string]*peerState
	pendingAcks map[string]map[string]time.Time

	lastError string
}

func NewManager(transport Transport, opts ManagerOptions) *Manager {
	m := &Manager{
		transport:   transport,
		opts:        opts,
		cfg:         Config{}.Normalize(),
		seenEvents:  map[string]time.Time{},
		peers:       map[string]*peerState{},
		pendingAcks: map[string]map[string]time.Time{},
	}
	return m
}

func (m *Manager) SetConfig(cfg Config) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = cfg.Normalize()
}

func (m *Manager) Start() error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}
	cfg := m.cfg.Normalize()
	m.cfg = cfg
	m.peers = map[string]*peerState{}
	for _, p := range cfg.TrustedPeers {
		cp := p
		m.peers[p.PeerID] = &peerState{TrustedPeer: cp}
	}
	m.seenEvents = map[string]time.Time{}
	m.pendingAcks = map[string]map[string]time.Time{}

	if !cfg.Enabled || cfg.Role == RoleOff {
		m.mu.Unlock()
		return errors.New("Bluetooth同期は無効です")
	}
	if err := m.transport.SupportsRole(cfg.Role); err != nil {
		m.lastError = err.Error()
		m.mu.Unlock()
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.running = true
	m.role = cfg.Role
	m.lastError = ""
	m.mu.Unlock()

	if err := m.transport.Start(ctx, cfg.Role, cfg.DeviceName, m.onTransportMessage, m.onTransportPeerEvent); err != nil {
		m.mu.Lock()
		m.running = false
		m.role = RoleOff
		m.cancel = nil
		m.lastError = err.Error()
		m.mu.Unlock()
		cancel()
		return err
	}

	m.log("info", fmt.Sprintf("Bluetooth同期を開始しました (role=%s)", cfg.Role))
	go m.houseKeepingLoop(ctx)
	go m.heartbeatLoop(ctx)
	return nil
}

func (m *Manager) Stop() error {
	m.mu.Lock()
	cancel := m.cancel
	wasRunning := m.running
	m.running = false
	m.role = RoleOff
	m.cancel = nil
	m.pendingAcks = map[string]map[string]time.Time{}
	m.pairingCode = ""
	m.pairingExpires = time.Time{}
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	err := m.transport.Stop()
	if wasRunning {
		m.log("info", "Bluetooth同期を停止しました")
	}
	return err
}

func (m *Manager) DispatchScene(scene string, source Source) error {
	scene = strings.TrimSpace(scene)
	if scene == "" {
		return errors.New("シーン名が空です")
	}

	m.mu.Lock()
	cfg := m.cfg.Normalize()
	if !m.running || cfg.Role != RoleParent {
		m.mu.Unlock()
		return errors.New("親機モードで起動中ではありません")
	}

	fireAt := time.Now().Add(time.Duration(cfg.LeadTimeMs) * time.Millisecond)
	evt := Message{
		Type:            MsgSceneCommand,
		ProtocolVersion: ProtocolVersion,
		EventID:         randomHex(16),
		SceneName:       scene,
		Source:          string(source),
		FireAtUnixMs:    fireAt.UnixMilli(),
		SentAtUnixMs:    time.Now().UnixMilli(),
	}

	peers := make([]*peerState, 0, len(m.peers))
	for _, p := range m.peers {
		cp := *p
		peers = append(peers, &cp)
	}

	m.pendingAcks[evt.EventID] = map[string]time.Time{}
	for _, p := range peers {
		if p.Connected {
			m.pendingAcks[evt.EventID][p.PeerID] = time.Now().Add(3 * time.Second)
		}
	}
	m.mu.Unlock()

	for _, p := range peers {
		if !p.Connected || strings.TrimSpace(p.Secret) == "" {
			continue
		}
		msg := evt
		msg.HMAC = sceneHMAC(p.Secret, msg)
		payload, err := msg.Marshal()
		if err != nil {
			m.log("error", fmt.Sprintf("scene_command シリアライズ失敗 [%s]: %v", p.PeerID, err))
			continue
		}
		if err := m.transport.Send(p.PeerID, payload); err != nil {
			m.log("error", fmt.Sprintf("scene_command 送信失敗 [%s]: %v", p.PeerID, err))
			m.dropPendingAck(evt.EventID, p.PeerID)
			continue
		}
	}

	go func(sceneName string, src Source, fire time.Time) {
		obsws.WaitUntil(fire, 2*time.Millisecond)
		if err := m.applyScene(sceneName, src); err != nil {
			m.log("error", fmt.Sprintf("親機ローカル切替失敗: %v", err))
			return
		}
		m.log("info", fmt.Sprintf("親機ローカル切替: %s", sceneName))
	}(scene, source, fireAt)

	return nil
}

func (m *Manager) GeneratePairingCode() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running || m.role != RoleParent {
		return "", errors.New("親機モードで起動中ではありません")
	}
	code := randomDigits(6)
	ttl := time.Duration(m.cfg.PairingCodeTTLSec) * time.Second
	m.pairingCode = code
	m.pairingExpires = time.Now().Add(ttl)
	return code, nil
}

func (m *Manager) JoinByCode(code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return errors.New("ペアリングコードが空です")
	}

	m.mu.Lock()
	running := m.running
	role := m.role
	peer := m.transport.LocalPeer()
	m.mu.Unlock()

	if !running || role != RoleChild {
		return errors.New("子機モードで起動中ではありません")
	}

	msg := Message{
		Type:         MsgPairRequest,
		PairingCode:  code,
		PeerID:       peer.PeerID,
		PeerName:     peer.Name,
		Platform:     peer.Platform,
		SentAtUnixMs: time.Now().UnixMilli(),
	}
	payload, err := msg.Marshal()
	if err != nil {
		return err
	}
	if err := m.transport.Broadcast(payload); err != nil {
		return err
	}
	m.log("info", "親機へペアリング要求を送信しました")
	return nil
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()

	st := Status{
		Supported:           true,
		SupportedRoleParent: m.transport.SupportsRole(RoleParent) == nil,
		SupportedRoleChild:  m.transport.SupportsRole(RoleChild) == nil,
		Running:             m.running,
		Enabled:             m.cfg.Enabled,
		Role:                m.cfg.Role,
		DeviceName:          m.cfg.DeviceName,
		LastError:           m.lastError,
		PairingCodeActive:   m.pairingCode != "" && time.Now().Before(m.pairingExpires),
		Peers:               []PeerStatus{},
	}
	if !st.SupportedRoleParent && !st.SupportedRoleChild {
		st.Supported = false
		st.UnsupportedReason = "このビルドではBluetooth同期が利用できません"
	}
	if st.PairingCodeActive {
		st.PairingCodeExpiresUnixMs = m.pairingExpires.UnixMilli()
	}

	peers := make([]PeerStatus, 0, len(m.peers))
	for _, p := range m.peers {
		ps := PeerStatus{
			PeerID:        p.PeerID,
			Name:          p.Name,
			Platform:      p.Platform,
			Connected:     p.Connected,
			LastAckStatus: p.LastAckStatus,
			LastLatencyMs: p.LastLatencyMs,
		}
		if !p.LastSeenAt.IsZero() {
			ps.LastSeenUnixMs = p.LastSeenAt.UnixMilli()
		}
		if !p.LastAckAt.IsZero() {
			ps.LastAckUnixMs = p.LastAckAt.UnixMilli()
		}
		peers = append(peers, ps)
		if p.Connected {
			st.ConnectedPeers++
		}
	}
	sort.Slice(peers, func(i, j int) bool {
		return strings.ToLower(peers[i].Name+peers[i].PeerID) < strings.ToLower(peers[j].Name+peers[j].PeerID)
	})
	st.Peers = peers

	if m.cfg.Role == RoleChild {
		st.ParentConnected = st.ConnectedPeers > 0
	}

	return st
}

func (m *Manager) onTransportPeerEvent(ev PeerEvent) {
	m.mu.Lock()
	p := m.ensurePeerLocked(ev.Peer)
	p.Connected = ev.Connected
	if !ev.At.IsZero() {
		p.LastSeenAt = ev.At
	} else {
		p.LastSeenAt = time.Now()
	}
	m.mu.Unlock()

	if ev.Connected {
		m.log("info", fmt.Sprintf("BT peer connected: %s (%s)", ev.Peer.Name, ev.Peer.PeerID))
	} else {
		m.log("info", fmt.Sprintf("BT peer disconnected: %s (%s)", ev.Peer.Name, ev.Peer.PeerID))
	}
}

func (m *Manager) onTransportMessage(from PeerRef, payload []byte) {
	msg, err := UnmarshalMessage(payload)
	if err != nil {
		m.log("error", fmt.Sprintf("BT受信メッセージの解析失敗: %v", err))
		return
	}

	m.mu.Lock()
	running := m.running
	role := m.role
	p := m.ensurePeerLocked(from)
	p.LastSeenAt = time.Now()
	m.mu.Unlock()

	if !running {
		return
	}

	switch msg.Type {
	case MsgPairRequest:
		if role == RoleParent {
			m.handlePairRequest(from, msg)
		}
	case MsgPairAccept:
		if role == RoleChild {
			m.handlePairAccept(from, msg)
		}
	case MsgSceneCommand:
		if role == RoleChild {
			m.handleSceneCommand(from, msg)
		}
	case MsgSceneAck:
		if role == RoleParent {
			m.handleSceneAck(from, msg)
		}
	case MsgHeartbeat:
		// peer alive
	default:
		m.log("info", fmt.Sprintf("未定義メッセージ type=%s from=%s", msg.Type, from.PeerID))
	}
}

func (m *Manager) handlePairRequest(from PeerRef, msg Message) {
	m.mu.Lock()
	cfg := m.cfg.Normalize()
	now := time.Now()
	valid := m.pairingCode != "" && now.Before(m.pairingExpires) && subtleEqual(m.pairingCode, strings.TrimSpace(msg.PairingCode))
	if !valid {
		m.mu.Unlock()
		_ = m.sendPairAccept(from.PeerID, "denied", "ペアリングコードが無効です", "")
		return
	}

	if _, ok := m.peers[from.PeerID]; !ok {
		if cfg.MaxNodes > 0 && len(m.peers) >= cfg.MaxNodes-1 {
			m.mu.Unlock()
			_ = m.sendPairAccept(from.PeerID, "denied", "最大接続台数に達しました", "")
			return
		}
	}

	secret := randomHex(32)
	p := m.ensurePeerLocked(from)
	p.Secret = secret
	p.Platform = from.Platform
	p.Name = from.Name
	p.LastSeenAt = now
	p.Connected = true
	peers := m.trustedPeersLocked()
	m.mu.Unlock()

	m.persistTrustedPeers(peers)
	if err := m.sendPairAccept(from.PeerID, "ok", "", secret); err != nil {
		m.log("error", fmt.Sprintf("pair_accept送信失敗: %v", err))
		return
	}
	m.log("info", fmt.Sprintf("ペアリング成功: %s (%s)", from.Name, from.PeerID))
}

func (m *Manager) handlePairAccept(from PeerRef, msg Message) {
	if msg.Status != "ok" {
		if strings.TrimSpace(msg.Error) == "" {
			m.log("error", "ペアリング拒否: unknown")
		} else {
			m.log("error", fmt.Sprintf("ペアリング拒否: %s", msg.Error))
		}
		return
	}
	if strings.TrimSpace(msg.Secret) == "" {
		m.log("error", "pair_accept に secret がありません")
		return
	}

	m.mu.Lock()
	p := m.ensurePeerLocked(from)
	p.Secret = msg.Secret
	p.Connected = true
	p.LastSeenAt = time.Now()
	peers := m.trustedPeersLocked()
	m.mu.Unlock()

	m.persistTrustedPeers(peers)
	m.log("info", fmt.Sprintf("親機とのペアリング完了: %s", from.Name))
}

func (m *Manager) handleSceneCommand(from PeerRef, msg Message) {
	if msg.ProtocolVersion != ProtocolVersion {
		_ = m.sendSceneAck(from.PeerID, msg.EventID, AckError, "unsupported protocol", 0)
		return
	}

	m.mu.Lock()
	p := m.ensurePeerLocked(from)
	secret := p.Secret
	cfg := m.cfg.Normalize()

	m.gcSeenLocked(time.Now())
	if t, ok := m.seenEvents[msg.EventID]; ok && time.Since(t) < 30*time.Second {
		m.mu.Unlock()
		return
	}
	m.seenEvents[msg.EventID] = time.Now()
	m.trimSeenLocked()
	m.mu.Unlock()

	if strings.TrimSpace(secret) == "" || !verifySceneHMAC(secret, msg) {
		_ = m.sendSceneAck(from.PeerID, msg.EventID, AckError, "invalid_hmac", 0)
		return
	}

	fireAt := time.UnixMilli(msg.FireAtUnixMs)
	if time.Now().After(fireAt.Add(time.Duration(cfg.AcceptLateMs) * time.Millisecond)) {
		_ = m.sendSceneAck(from.PeerID, msg.EventID, AckLateDrop, "late_drop", 0)
		return
	}

	ok, err := m.sceneExists(msg.SceneName)
	if err != nil {
		_ = m.sendSceneAck(from.PeerID, msg.EventID, AckError, err.Error(), 0)
		return
	}
	if !ok {
		_ = m.sendSceneAck(from.PeerID, msg.EventID, AckNotFound, "scene_not_found", 0)
		return
	}

	obsws.WaitUntil(fireAt, 2*time.Millisecond)
	if err := m.applyScene(msg.SceneName, Source(msg.Source)); err != nil {
		_ = m.sendSceneAck(from.PeerID, msg.EventID, AckError, err.Error(), 0)
		return
	}

	latency := time.Since(fireAt).Milliseconds()
	if latency < 0 {
		latency = 0
	}
	_ = m.sendSceneAck(from.PeerID, msg.EventID, AckOK, "", latency)
}

func (m *Manager) handleSceneAck(from PeerRef, msg Message) {
	m.mu.Lock()
	p := m.ensurePeerLocked(from)
	p.LastAckAt = time.Now()
	p.LastAckStatus = msg.Status
	p.LastLatencyMs = msg.LatencyMs
	if pending, ok := m.pendingAcks[msg.EventID]; ok {
		delete(pending, from.PeerID)
		if len(pending) == 0 {
			delete(m.pendingAcks, msg.EventID)
		}
	}
	m.mu.Unlock()

	if msg.Status != string(AckOK) {
		m.log("error", fmt.Sprintf("ACK失敗 [%s] event=%s status=%s err=%s", from.Name, msg.EventID, msg.Status, msg.Error))
		return
	}
	m.log("info", fmt.Sprintf("ACK受信 [%s] event=%s latency=%dms", from.Name, msg.EventID, msg.LatencyMs))
}

func (m *Manager) sendPairAccept(peerID, status, errText, secret string) error {
	msg := Message{
		Type:            MsgPairAccept,
		Status:          status,
		Error:           errText,
		Secret:          secret,
		ProtocolVersion: ProtocolVersion,
		SentAtUnixMs:    time.Now().UnixMilli(),
	}
	payload, err := msg.Marshal()
	if err != nil {
		return err
	}
	return m.transport.Send(peerID, payload)
}

func (m *Manager) sendSceneAck(peerID, eventID string, status AckStatus, errText string, latencyMs int64) error {
	msg := Message{
		Type:            MsgSceneAck,
		ProtocolVersion: ProtocolVersion,
		EventID:         eventID,
		Status:          string(status),
		Error:           errText,
		LatencyMs:       latencyMs,
		SentAtUnixMs:    time.Now().UnixMilli(),
	}
	payload, err := msg.Marshal()
	if err != nil {
		return err
	}
	return m.transport.Send(peerID, payload)
}

func (m *Manager) houseKeepingLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkAckTimeouts()
			m.mu.Lock()
			m.gcSeenLocked(time.Now())
			m.mu.Unlock()
		}
	}
}

func (m *Manager) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.mu.Lock()
			running := m.running
			role := m.role
			peers := m.trustedPeersLocked()
			m.mu.Unlock()

			if !running {
				continue
			}

			hb := Message{Type: MsgHeartbeat, ProtocolVersion: ProtocolVersion, SentAtUnixMs: time.Now().UnixMilli()}
			payload, _ := hb.Marshal()

			if role == RoleParent {
				_ = m.transport.Broadcast(payload)
				continue
			}
			if role == RoleChild {
				for _, p := range peers {
					_ = m.transport.Send(p.PeerID, payload)
				}
			}
		}
	}
}

func (m *Manager) checkAckTimeouts() {
	now := time.Now()
	timedOut := []string{}

	m.mu.Lock()
	for evtID, peers := range m.pendingAcks {
		for peerID, dl := range peers {
			if now.After(dl) {
				timedOut = append(timedOut, fmt.Sprintf("event=%s peer=%s", evtID, peerID))
				delete(peers, peerID)
			}
		}
		if len(peers) == 0 {
			delete(m.pendingAcks, evtID)
		}
	}
	m.mu.Unlock()

	for _, s := range timedOut {
		m.log("error", "ACKタイムアウト: "+s)
	}
}

func (m *Manager) dropPendingAck(eventID, peerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if peers, ok := m.pendingAcks[eventID]; ok {
		delete(peers, peerID)
		if len(peers) == 0 {
			delete(m.pendingAcks, eventID)
		}
	}
}

func (m *Manager) ensurePeerLocked(peer PeerRef) *peerState {
	p, ok := m.peers[peer.PeerID]
	if !ok {
		p = &peerState{TrustedPeer: TrustedPeer{PeerID: peer.PeerID}}
		m.peers[peer.PeerID] = p
	}
	if strings.TrimSpace(peer.Name) != "" {
		p.Name = peer.Name
	}
	if strings.TrimSpace(peer.Platform) != "" {
		p.Platform = peer.Platform
	}
	return p
}

func (m *Manager) trustedPeersLocked() []TrustedPeer {
	out := make([]TrustedPeer, 0, len(m.peers))
	for _, p := range m.peers {
		cp := p.TrustedPeer
		if !p.LastSeenAt.IsZero() {
			cp.LastSeen = p.LastSeenAt.UTC().Format(time.RFC3339)
		}
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name+out[i].PeerID) < strings.ToLower(out[j].Name+out[j].PeerID)
	})
	return out
}

func (m *Manager) gcSeenLocked(now time.Time) {
	for k, t := range m.seenEvents {
		if now.Sub(t) > 30*time.Second {
			delete(m.seenEvents, k)
		}
	}
	m.trimSeenLocked()
}

func (m *Manager) trimSeenLocked() {
	if len(m.seenEvents) <= 256 {
		return
	}
	type kv struct {
		Key string
		At  time.Time
	}
	arr := make([]kv, 0, len(m.seenEvents))
	for k, v := range m.seenEvents {
		arr = append(arr, kv{Key: k, At: v})
	}
	sort.Slice(arr, func(i, j int) bool { return arr[i].At.Before(arr[j].At) })
	for i := 0; i < len(arr)-256; i++ {
		delete(m.seenEvents, arr[i].Key)
	}
}

func (m *Manager) applyScene(scene string, source Source) error {
	if m.opts.ApplyScene == nil {
		return errors.New("ApplyScene callback is not configured")
	}
	return m.opts.ApplyScene(scene, source)
}

func (m *Manager) sceneExists(scene string) (bool, error) {
	if m.opts.SceneExists == nil {
		return true, nil
	}
	return m.opts.SceneExists(scene)
}

func (m *Manager) persistTrustedPeers(peers []TrustedPeer) {
	if m.opts.PersistTrustedPeers == nil {
		return
	}
	if err := m.opts.PersistTrustedPeers(peers); err != nil {
		m.log("error", fmt.Sprintf("TrustedPeers の保存に失敗: %v", err))
	}
}

func (m *Manager) log(level, msg string) {
	if m.opts.Logf != nil {
		m.opts.Logf(level, msg)
	}
}

func randomHex(n int) string {
	if n <= 0 {
		n = 16
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func randomDigits(n int) string {
	if n <= 0 {
		n = 6
	}
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		b := make([]byte, 1)
		if _, err := rand.Read(b); err != nil {
			out[i] = '0'
			continue
		}
		out[i] = '0' + (b[0] % 10)
	}
	return string(out)
}

func subtleEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}
