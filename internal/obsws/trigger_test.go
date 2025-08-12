package obsws

import (
    "errors"
    "testing"
    "time"
)

func TestToMediaActionConst(t *testing.T) {
    cases := map[string]string{
        "play":   "OBS_WEBSOCKET_MEDIA_INPUT_ACTION_PLAY",
        "pause":  "OBS_WEBSOCKET_MEDIA_INPUT_ACTION_PAUSE",
        "stop":   "OBS_WEBSOCKET_MEDIA_INPUT_ACTION_STOP",
        "restart": "OBS_WEBSOCKET_MEDIA_INPUT_ACTION_RESTART",
        "resume": "OBS_WEBSOCKET_MEDIA_INPUT_ACTION_RESUME",
        "none":   "",
    }
    for in, want := range cases {
        got, ok := toMediaActionConst(in)
        if !ok {
            t.Fatalf("expected ok for %q", in)
        }
        if got != want {
            t.Fatalf("toMediaActionConst(%q)=%q; want %q", in, got, want)
        }
    }
    if _, ok := toMediaActionConst("unknown"); ok {
        t.Fatalf("expected !ok for unknown action")
    }
}

func TestWithTimeout(t *testing.T) {
    // Should succeed before timeout
    err := withTimeout(func() error { return nil }, 10*time.Millisecond)
    if err != nil { t.Fatalf("unexpected error: %v", err) }

    // Should time out
    start := time.Now()
    err = withTimeout(func() error {
        time.Sleep(50 * time.Millisecond)
        return nil
    }, 10*time.Millisecond)
    if err == nil { t.Fatalf("expected timeout error") }
    if time.Since(start) > 200*time.Millisecond {
        t.Fatalf("withTimeout took too long")
    }

    // Propagate error
    want := errors.New("boom")
    if got := withTimeout(func() error { return want }, 10*time.Millisecond); got == nil {
        t.Fatalf("expected error to propagate")
    }
}

