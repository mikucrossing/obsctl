//go:build midi_native

package midi

import (
    "fmt"
    "strings"
    "sync"
    "time"

    "gitlab.com/gomidi/midi"
    "gitlab.com/gomidi/rtmididrv"
)

// inputWrap は rtmididrv のポート/ドライバをまとめて Close する薄いラッパです。
type inputWrap struct{
    drv *rtmididrv.Driver
    in  midi.In
    once sync.Once
}

// OpenInput は指定名の入力ポートを開き、イベントをチャネルで返す。
// デバイス名は完全一致。合致しない場合はエラー。
func OpenInput(deviceName string) (Input, <-chan Event, error) {
    drv, err := rtmididrv.New()
    if err != nil {
        return nil, nil, fmt.Errorf("rtmididrv.New: %w", err)
    }
    ins, err := drv.Ins()
    if err != nil {
        _ = drv.Close()
        return nil, nil, fmt.Errorf("MIDI入力列挙に失敗: %w", err)
    }
    var in midi.In
    // 優先: 完全一致 → 部分一致
    for _, p := range ins {
        if p.String() == deviceName {
            in = p
            break
        }
    }
    if in == nil {
        for _, p := range ins {
            if strings.Contains(p.String(), deviceName) {
                in = p
                break
            }
        }
    }
    if in == nil {
        _ = drv.Close()
        return nil, nil, fmt.Errorf("MIDI入力デバイスが見つかりません: %s", deviceName)
    }
    if err := in.Open(); err != nil {
        _ = drv.Close()
        return nil, nil, fmt.Errorf("入力オープン失敗: %w", err)
    }

    evCh := make(chan Event, 128)

    // 生バイトを受け取り、代表的なチャンネルメッセージのみ正規化して流す
    if err := in.SetListener(func(bt []byte, _ int64) {
        if len(bt) == 0 {
            return
        }
        status := bt[0]
        // Realtime/System Common は無視
        if status >= 0xF0 {
            return
        }
        typ := status >> 4
        ch := (status & 0x0F) + 1 // 1-16
        now := time.Now()

        switch typ {
        case 0x08: // NoteOff
            if len(bt) >= 3 {
                e := Event{Type: NoteOff, Channel: ch, Data1: bt[1] & 0x7F, Data2: bt[2] & 0x7F, Time: now}
                select { case evCh <- e: default: }
            }
        case 0x09: // NoteOn（Vel==0 は NoteOff）
            if len(bt) >= 3 {
                vel := bt[2] & 0x7F
                t := NoteOn
                if vel == 0 {
                    t = NoteOff
                }
                e := Event{Type: t, Channel: ch, Data1: bt[1] & 0x7F, Data2: vel, Time: now}
                select { case evCh <- e: default: }
            }
        case 0x0B: // ControlChange
            if len(bt) >= 3 {
                e := Event{Type: ControlChange, Channel: ch, Data1: bt[1] & 0x7F, Data2: bt[2] & 0x7F, Time: now}
                select { case evCh <- e: default: }
            }
        case 0x0C: // ProgramChange
            if len(bt) >= 2 {
                e := Event{Type: ProgramChange, Channel: ch, Data1: bt[1] & 0x7F, Time: now}
                select { case evCh <- e: default: }
            }
        default:
            // ignore other channel messages
        }
    }); err != nil {
        _ = in.Close()
        _ = drv.Close()
        return nil, nil, fmt.Errorf("リスナ設定失敗: %w", err)
    }

    // ラッパを返す
    return &inputWrap{drv: drv, in: in}, evCh, nil
}

func (w *inputWrap) Close() error {
    var err error
    w.once.Do(func(){
        // best-effort 停止
        _ = w.in.Close()
        err = w.drv.Close()
    })
    return err
}

// ListInputs は利用可能な入力デバイスの名称一覧を返す。
func ListInputs() ([]string, error) {
    drv, err := rtmididrv.New()
    if err != nil {
        return nil, err
    }
    defer drv.Close()
    ins, err := drv.Ins()
    if err != nil {
        return nil, err
    }
    names := make([]string, 0, len(ins))
    for _, i := range ins {
        names = append(names, i.String())
    }
    return names, nil
}
