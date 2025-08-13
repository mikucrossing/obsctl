package midi

import "time"

// Type は MIDI イベント種別。
type Type string

const (
    NoteOn        Type = "note_on"
    NoteOff       Type = "note_off"
    ControlChange Type = "control_change"
    ProgramChange Type = "program_change"
)

// Event は正規化されたMIDIイベント。
type Event struct {
    Type    Type
    Channel uint8 // 1-16
    Data1   uint8 // Note番号 / CC番号 / Program番号
    Data2   uint8 // Velocity / CC値（ProgramChangeでは未使用）
    Time    time.Time
}

// Input はオープン済みのMIDI入力デバイスを表す。
type Input interface {
    Close() error
}

