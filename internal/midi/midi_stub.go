//go:build !midi_native

package midi

import "errors"

// OpenInput は指定デバイスを開き、イベントチャネルを返す。
// デフォルトビルド（midi_nativeタグなし）では未対応。
func OpenInput(deviceName string) (Input, <-chan Event, error) {
    return nil, nil, errors.New("native MIDI driver is not included in this build (build with -tags midi_native)")
}

// ListInputs は利用可能なMIDI入力デバイス名を返す。
// デフォルトビルド（midi_nativeタグなし）では未対応。
func ListInputs() ([]string, error) {
    return nil, errors.New("native MIDI driver is not included in this build (build with -tags midi_native)")
}
