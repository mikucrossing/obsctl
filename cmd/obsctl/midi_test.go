package main

import (
    "os"
    "path/filepath"
    "testing"
    "time"
)

func TestParseNoteMaps_Basics(t *testing.T) {
    in := []string{
        "1:36=SceneA",
        " 2:127=SceneB ",
        "1:128=Bad",    // invalid note
        "0:36=Bad",     // invalid channel
        "x:y=Bad",      // invalid format
        "1:36=",        // empty scene
    }
    got := parseNoteMaps(in)
    if len(got) != 2 {
        t.Fatalf("expected 2 entries, got %d: %#v", len(got), got)
    }
    if got["1:36"] != "SceneA" {
        t.Fatalf("1:36 => %q", got["1:36"])
    }
    if got["2:127"] != "SceneB" {
        t.Fatalf("2:127 => %q", got["2:127"])
    }
}

func TestLoadJSONConfig_Basics(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "midi.json")
    data := []byte(`{
        "device": "DevA",
        "channel": 1,
        "debounce": "120ms",
        "rate_limit": "80ms",
        "mappings": [
          {"type":"note_on","channel":1,"note":36,"scene":"SceneA"},
          {"type":"note_on","channel":2,"note":40,"scene":"SceneB"},
          {"type":"control_change","channel":1,"note":64,"scene":"Ignore"}
        ]
    }`)
    if err := os.WriteFile(path, data, 0o644); err != nil {
        t.Fatal(err)
    }

    device := ""
    channel := ""
    // ゼロ値を渡すと JSON の値が適用される
    var debounce time.Duration
    var ratelimit time.Duration
    noteMap := map[string]string{}

    if err := loadJSONConfig(path, &device, &channel, &debounce, &ratelimit, noteMap); err != nil {
        t.Fatalf("loadJSONConfig error: %v", err)
    }

    if device != "DevA" { t.Fatalf("device=%q", device) }
    if channel != "1" { t.Fatalf("channel=%q", channel) }
    if debounce != 120*time.Millisecond { t.Fatalf("debounce=%v", debounce) }
    if ratelimit != 80*time.Millisecond { t.Fatalf("ratelimit=%v", ratelimit) }
    if len(noteMap) != 2 { t.Fatalf("noteMap size=%d %#v", len(noteMap), noteMap) }
    if noteMap["1:36"] != "SceneA" { t.Fatalf("1:36 => %q", noteMap["1:36"]) }
    if noteMap["2:40"] != "SceneB" { t.Fatalf("2:40 => %q", noteMap["2:40"]) }
}

func TestLoadJSONConfig_CLIOverrides(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "midi.json")
    data := []byte(`{
        "device": "DevA",
        "channel": 1,
        "debounce": "120ms",
        "rate_limit": "80ms",
        "mappings": []
    }`)
    if err := os.WriteFile(path, data, 0o644); err != nil {
        t.Fatal(err)
    }
    device := "CLI_Device"
    channel := "3"
    debounce := 10 * time.Millisecond
    ratelimit := 15 * time.Millisecond
    noteMap := map[string]string{}

    if err := loadJSONConfig(path, &device, &channel, &debounce, &ratelimit, noteMap); err != nil {
        t.Fatalf("loadJSONConfig error: %v", err)
    }
    if device != "CLI_Device" { t.Fatalf("device override failed: %q", device) }
    if channel != "3" { t.Fatalf("channel override failed: %q", channel) }
    if debounce != 10*time.Millisecond { t.Fatalf("debounce override failed: %v", debounce) }
    if ratelimit != 15*time.Millisecond { t.Fatalf("ratelimit override failed: %v", ratelimit) }
}
