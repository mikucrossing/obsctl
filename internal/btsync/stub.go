//go:build !bluetooth_native

package btsync

import (
	"errors"
	"fmt"
	"runtime"
)

type unsupportedTransport struct {
	local PeerRef
}

func NewNativeTransport() Transport {
	return &unsupportedTransport{local: PeerRef{PeerID: fmt.Sprintf("local-%s", runtime.GOOS), Name: defaultDeviceName(), Platform: runtime.GOOS}}
}

func (t *unsupportedTransport) Start(_ Context, role Role, _ string, _ func(from PeerRef, payload []byte), _ func(PeerEvent)) error {
	return t.SupportsRole(role)
}

func (t *unsupportedTransport) Stop() error { return nil }

func (t *unsupportedTransport) Send(_ string, _ []byte) error {
	return errors.New("bluetooth_native ビルドタグが必要です")
}

func (t *unsupportedTransport) Broadcast(_ []byte) error {
	return errors.New("bluetooth_native ビルドタグが必要です")
}

func (t *unsupportedTransport) SupportsRole(_ Role) error {
	return errors.New("このビルドにはBluetooth同期機能が含まれていません（-tags bluetooth_native でビルドしてください）")
}

func (t *unsupportedTransport) LocalPeer() PeerRef { return t.local }
