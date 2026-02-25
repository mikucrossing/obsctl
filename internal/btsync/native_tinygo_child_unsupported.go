//go:build bluetooth_native && !(windows || linux)

package btsync

import "errors"

func (t *tinyGoTransport) childStart(_ Context, _ string) error {
	return errors.New("このOSでは子機モードをサポートしていません")
}

func (t *tinyGoTransport) childStop() error { return nil }

func (t *tinyGoTransport) childSend(_ []byte) error {
	return errors.New("このOSでは子機モードをサポートしていません")
}
