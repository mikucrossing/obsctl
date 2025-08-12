package obsws

import (
    "runtime"
    "time"
)

// WaitUntil は指定時刻まで待機する。大部分は Sleep し、最後のわずかな時間は
// Gosched を挟みつつスピンして精度を上げる。
func WaitUntil(t time.Time, spinWin time.Duration) {
    d := time.Until(t)
    if d <= 0 {
        return
    }
    if spinWin < 0 {
        spinWin = 0
    }
    if d > spinWin {
        time.Sleep(d - spinWin)
    }
    for time.Until(t) > 0 {
        runtime.Gosched()
    }
}

