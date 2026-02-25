//go:build bluetooth_native

package btsync

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"tinygo.org/x/bluetooth"
)

var (
	syncServiceUUID = mustParseUUID("f2b0d4f6-7b4d-4b7f-a04d-6031deaf0c01")
	syncRXUUID      = mustParseUUID("f2b0d4f6-7b4d-4b7f-a04d-6031deaf0c02")
	syncTXUUID      = mustParseUUID("f2b0d4f6-7b4d-4b7f-a04d-6031deaf0c03")
)

type parentPeerConn struct {
	ref      PeerRef
	device   bluetooth.Device
	rxChar   bluetooth.DeviceCharacteristic
	txChar   bluetooth.DeviceCharacteristic
	rxBuffer []byte
}

type tinyGoTransport struct {
	mu sync.Mutex

	adapter *bluetooth.Adapter

	role        Role
	localPeer   PeerRef
	running     bool
	onMessage   func(from PeerRef, payload []byte)
	onPeerEvent func(PeerEvent)

	peers      map[string]*parentPeerConn
	connecting map[string]struct{}

	childParent      PeerRef
	childConnected   bool
	childInputBuffer []byte

	childRX         bluetooth.Characteristic
	childTX         bluetooth.Characteristic
	childAdvStopper func() error
}

func NewNativeTransport() Transport {
	return &tinyGoTransport{
		adapter:    bluetooth.DefaultAdapter,
		peers:      map[string]*parentPeerConn{},
		connecting: map[string]struct{}{},
	}
}

func (t *tinyGoTransport) SupportsRole(role Role) error {
	switch role {
	case RoleParent:
		return nil
	case RoleChild:
		if runtime.GOOS == "darwin" {
			return errors.New("macOS は子機モードをサポートしていません")
		}
		if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
			return fmt.Errorf("子機モードは %s では未対応です", runtime.GOOS)
		}
		return nil
	case RoleOff:
		return nil
	default:
		return fmt.Errorf("unsupported role: %s", role)
	}
}

func (t *tinyGoTransport) LocalPeer() PeerRef {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.localPeer
}

func (t *tinyGoTransport) Start(ctx Context, role Role, deviceName string, onMessage func(from PeerRef, payload []byte), onPeerEvent func(PeerEvent)) error {
	if err := t.SupportsRole(role); err != nil {
		return err
	}

	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return nil
	}
	t.role = role
	t.onMessage = onMessage
	t.onPeerEvent = onPeerEvent
	t.localPeer = PeerRef{PeerID: randomHex(6), Name: strings.TrimSpace(deviceName), Platform: runtime.GOOS}
	if t.localPeer.Name == "" {
		t.localPeer.Name = defaultDeviceName()
	}
	t.running = true
	t.peers = map[string]*parentPeerConn{}
	t.connecting = map[string]struct{}{}
	t.childParent = PeerRef{}
	t.childConnected = false
	t.childInputBuffer = nil
	t.childAdvStopper = nil
	t.mu.Unlock()

	if err := t.adapter.Enable(); err != nil {
		t.mu.Lock()
		t.running = false
		t.mu.Unlock()
		return err
	}

	t.adapter.SetConnectHandler(func(device bluetooth.Device, connected bool) {
		peerID := normalizePeerID(device.Address.String())
		peer := PeerRef{PeerID: peerID, Name: peerID, Platform: runtime.GOOS}

		t.mu.Lock()
		roleNow := t.role
		if roleNow == RoleParent {
			if !connected {
				if pc, ok := t.peers[peerID]; ok {
					peer = pc.ref
					delete(t.peers, peerID)
				}
				delete(t.connecting, peerID)
			}
		} else if roleNow == RoleChild {
			t.childConnected = connected
			if connected {
				t.childParent = peer
			} else {
				t.childParent = PeerRef{}
			}
		}
		cb := t.onPeerEvent
		t.mu.Unlock()

		if cb != nil {
			cb(PeerEvent{Peer: peer, Connected: connected, At: time.Now()})
		}
	})

	switch role {
	case RoleParent:
		go t.parentScanLoop(ctx)
		return nil
	case RoleChild:
		return t.childStart(ctx, deviceName)
	default:
		return nil
	}
}

func (t *tinyGoTransport) Stop() error {
	t.mu.Lock()
	running := t.running
	role := t.role
	peers := make([]*parentPeerConn, 0, len(t.peers))
	for _, p := range t.peers {
		peers = append(peers, p)
	}
	t.running = false
	t.peers = map[string]*parentPeerConn{}
	t.connecting = map[string]struct{}{}
	t.childConnected = false
	t.childParent = PeerRef{}
	t.childInputBuffer = nil
	t.mu.Unlock()

	if !running {
		return nil
	}

	_ = t.adapter.StopScan()
	for _, p := range peers {
		_ = p.device.Disconnect()
	}
	if role == RoleChild {
		_ = t.childStop()
	}
	return nil
}

func (t *tinyGoTransport) Broadcast(payload []byte) error {
	t.mu.Lock()
	role := t.role
	t.mu.Unlock()

	switch role {
	case RoleParent:
		t.mu.Lock()
		ids := make([]string, 0, len(t.peers))
		for id := range t.peers {
			ids = append(ids, id)
		}
		t.mu.Unlock()
		for _, id := range ids {
			if err := t.Send(id, payload); err != nil {
				return err
			}
		}
		return nil
	case RoleChild:
		return t.childSend(payload)
	default:
		return errors.New("transport is not started")
	}
}

func (t *tinyGoTransport) Send(peerID string, payload []byte) error {
	peerID = normalizePeerID(peerID)

	t.mu.Lock()
	role := t.role
	t.mu.Unlock()

	if role == RoleChild {
		return t.childSend(payload)
	}

	t.mu.Lock()
	pc, ok := t.peers[peerID]
	t.mu.Unlock()
	if !ok {
		return fmt.Errorf("peer not connected: %s", peerID)
	}
	return writeFramedWithMTU(payload, func(part []byte) (int, error) {
		return pc.rxChar.WriteWithoutResponse(part)
	}, mtuOf(pc.rxChar))
}

func (t *tinyGoTransport) parentScanLoop(ctx Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := t.adapter.Scan(func(ad *bluetooth.Adapter, result bluetooth.ScanResult) {
			if !result.AdvertisementPayload.HasServiceUUID(syncServiceUUID) {
				return
			}
			peerID := normalizePeerID(result.Address.String())

			t.mu.Lock()
			_, connected := t.peers[peerID]
			_, connecting := t.connecting[peerID]
			if connected || connecting {
				t.mu.Unlock()
				return
			}
			t.connecting[peerID] = struct{}{}
			t.mu.Unlock()

			_ = ad.StopScan()
			go t.connectParentPeer(result)
		})

		select {
		case <-ctx.Done():
			return
		default:
		}

		if err != nil {
			time.Sleep(600 * time.Millisecond)
			continue
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func (t *tinyGoTransport) connectParentPeer(result bluetooth.ScanResult) {
	peerID := normalizePeerID(result.Address.String())

	defer func() {
		t.mu.Lock()
		delete(t.connecting, peerID)
		t.mu.Unlock()
	}()

	device, err := t.adapter.Connect(result.Address, bluetooth.ConnectionParams{})
	if err != nil {
		return
	}

	services, err := device.DiscoverServices([]bluetooth.UUID{syncServiceUUID})
	if err != nil || len(services) == 0 {
		_ = device.Disconnect()
		return
	}

	chars, err := services[0].DiscoverCharacteristics([]bluetooth.UUID{syncRXUUID, syncTXUUID})
	if err != nil || len(chars) == 0 {
		_ = device.Disconnect()
		return
	}

	var rx, tx bluetooth.DeviceCharacteristic
	foundRx := false
	foundTx := false
	for _, c := range chars {
		if c.UUID() == syncRXUUID {
			rx = c
			foundRx = true
		}
		if c.UUID() == syncTXUUID {
			tx = c
			foundTx = true
		}
	}
	if !foundRx || !foundTx {
		_ = device.Disconnect()
		return
	}

	txcopy := tx
	err = txcopy.EnableNotifications(func(value []byte) {
		t.onParentNotification(peerID, value)
	})
	if err != nil {
		_ = device.Disconnect()
		return
	}

	peerName := strings.TrimSpace(result.LocalName())
	if peerName == "" {
		peerName = peerID
	}

	t.mu.Lock()
	t.peers[peerID] = &parentPeerConn{
		ref:    PeerRef{PeerID: peerID, Name: peerName, Platform: "unknown"},
		device: device,
		rxChar: rx,
		txChar: tx,
	}
	cb := t.onPeerEvent
	t.mu.Unlock()

	if cb != nil {
		cb(PeerEvent{Peer: PeerRef{PeerID: peerID, Name: peerName, Platform: "unknown"}, Connected: true, At: time.Now()})
	}
}

func (t *tinyGoTransport) onParentNotification(peerID string, chunk []byte) {
	t.mu.Lock()
	pc, ok := t.peers[peerID]
	if !ok {
		t.mu.Unlock()
		return
	}

	frames, remain := splitFrames(pc.rxBuffer, chunk)
	pc.rxBuffer = remain
	from := pc.ref
	cb := t.onMessage
	t.mu.Unlock()

	if cb == nil {
		return
	}
	for _, frame := range frames {
		cb(from, frame)
	}
}

func splitFrames(prev []byte, chunk []byte) (frames [][]byte, remain []byte) {
	buf := append(append([]byte{}, prev...), chunk...)
	start := 0
	for i := 0; i < len(buf); i++ {
		if buf[i] != '\n' {
			continue
		}
		if i > start {
			line := make([]byte, i-start)
			copy(line, buf[start:i])
			frames = append(frames, line)
		}
		start = i + 1
	}
	if start < len(buf) {
		remain = make([]byte, len(buf)-start)
		copy(remain, buf[start:])
	}
	return frames, remain
}

func writeFramedWithMTU(payload []byte, writer func([]byte) (int, error), mtu int) error {
	framed := make([]byte, 0, len(payload)+1)
	framed = append(framed, payload...)
	framed = append(framed, '\n')

	if mtu <= 0 {
		mtu = 20
	}
	if mtu > 180 {
		mtu = 180
	}

	for len(framed) > 0 {
		n := mtu
		if len(framed) < n {
			n = len(framed)
		}
		if _, err := writer(framed[:n]); err != nil {
			return err
		}
		framed = framed[n:]
		time.Sleep(2 * time.Millisecond)
	}
	return nil
}

func normalizePeerID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}

func mtuOf(ch bluetooth.DeviceCharacteristic) int {
	mtu, err := ch.GetMTU()
	if err != nil || mtu < 23 {
		return 20
	}
	v := int(mtu) - 3
	if v < 20 {
		return 20
	}
	return v
}

func mustParseUUID(s string) bluetooth.UUID {
	u, err := bluetooth.ParseUUID(s)
	if err != nil {
		panic(err)
	}
	return u
}
