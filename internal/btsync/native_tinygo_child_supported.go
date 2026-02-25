//go:build bluetooth_native && (windows || linux)

package btsync

import (
	"errors"
	"strings"

	"tinygo.org/x/bluetooth"
)

func (t *tinyGoTransport) childStart(ctx Context, deviceName string) error {
	var rx bluetooth.Characteristic
	var tx bluetooth.Characteristic

	if err := t.adapter.AddService(&bluetooth.Service{
		UUID: syncServiceUUID,
		Characteristics: []bluetooth.CharacteristicConfig{
			{
				Handle: &rx,
				UUID:   syncRXUUID,
				Flags:  bluetooth.CharacteristicWritePermission | bluetooth.CharacteristicWriteWithoutResponsePermission,
				WriteEvent: func(_ bluetooth.Connection, _ int, value []byte) {
					t.onChildWrite(value)
				},
			},
			{
				Handle: &tx,
				UUID:   syncTXUUID,
				Flags:  bluetooth.CharacteristicNotifyPermission | bluetooth.CharacteristicReadPermission,
			},
		},
	}); err != nil {
		return err
	}

	adv := t.adapter.DefaultAdvertisement()
	name := strings.TrimSpace(deviceName)
	if name == "" {
		name = defaultDeviceName()
	}
	if len(name) > 24 {
		name = name[:24]
	}
	if err := adv.Configure(bluetooth.AdvertisementOptions{
		LocalName:    name,
		ServiceUUIDs: []bluetooth.UUID{syncServiceUUID},
	}); err != nil {
		return err
	}
	if err := adv.Start(); err != nil {
		return err
	}

	t.mu.Lock()
	t.childRX = rx
	t.childTX = tx
	t.childAdvStopper = adv.Stop
	t.mu.Unlock()

	go func() {
		<-ctx.Done()
		_ = adv.Stop()
	}()

	return nil
}

func (t *tinyGoTransport) childStop() error {
	t.mu.Lock()
	stopper := t.childAdvStopper
	t.childAdvStopper = nil
	t.childRX = bluetooth.Characteristic{}
	t.childTX = bluetooth.Characteristic{}
	t.mu.Unlock()

	if stopper != nil {
		return stopper()
	}
	return nil
}

func (t *tinyGoTransport) childSend(payload []byte) error {
	t.mu.Lock()
	connected := t.childConnected
	tx := t.childTX
	t.mu.Unlock()

	if !connected {
		return errors.New("親機に未接続です")
	}
	return writeFramedWithMTU(payload, func(part []byte) (int, error) {
		return tx.Write(part)
	}, 20)
}

func (t *tinyGoTransport) onChildWrite(chunk []byte) {
	t.mu.Lock()
	frames, remain := splitFrames(t.childInputBuffer, chunk)
	t.childInputBuffer = remain
	from := t.childParent
	if strings.TrimSpace(from.PeerID) == "" {
		from = PeerRef{PeerID: "parent", Name: "parent", Platform: "unknown"}
	}
	cb := t.onMessage
	t.mu.Unlock()

	if cb == nil {
		return
	}
	for _, frame := range frames {
		cb(from, frame)
	}
}
