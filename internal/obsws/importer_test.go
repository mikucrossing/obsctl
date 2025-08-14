package obsws

import "testing"

func TestIsVideoExt(t *testing.T) {
    ok := []string{".mp4", ".mov", ".mkv", ".webm"}
    ng := []string{".txt", ".gif", ".avi", ""}
    for _, e := range ok {
        if !isVideoExt(e) {
            t.Fatalf("expected true for %q", e)
        }
    }
    for _, e := range ng {
        if isVideoExt(e) {
            t.Fatalf("expected false for %q", e)
        }
    }
}

func TestIsImageExt(t *testing.T) {
    ok := []string{".png", ".jpg", ".jpeg", ".webp", ".bmp", ".gif", ".tiff", ".tif"}
    ng := []string{".txt", ".mp4", ".mov", ""}
    for _, e := range ok {
        if !isImageExt(e) {
            t.Fatalf("expected true for %q", e)
        }
    }
    for _, e := range ng {
        if isImageExt(e) {
            t.Fatalf("expected false for %q", e)
        }
    }
}

func TestSanitizeName(t *testing.T) {
    cases := map[string]string{
        "abc":               "abc",
        " a/b ":            "a_b",
        "a\\b":             "a_b",
        "":                  "untitled",
    }
    for in, want := range cases {
        if got := sanitizeName(in); got != want {
            t.Fatalf("sanitizeName(%q)=%q; want %q", in, got, want)
        }
    }

    // Long string is truncated to 120 runes
    long := make([]rune, 150)
    for i := range long { long[i] = 'x' }
    got := sanitizeName(string(long))
    if len([]rune(got)) != 120 {
        t.Fatalf("sanitizeName should truncate to 120 runes, got %d", len([]rune(got)))
    }
}

func TestNormalizeTransitionName(t *testing.T) {
    cases := map[string]string{
        "":       "Fade",
        "fade":   "Fade",
        "FADE":   "Fade",
        " cut ":  "Cut",
        "CuT":    "Cut",
        "other":  "Fade",
    }
    for in, want := range cases {
        if got := normalizeTransitionName(in); got != want {
            t.Fatalf("normalizeTransitionName(%q)=%q; want %q", in, got, want)
        }
    }
}

func TestNormalizeMonitoringType(t *testing.T) {
    cases := map[string]string{
        "":                         "OBS_MONITORING_TYPE_NONE",
        "off":                      "OBS_MONITORING_TYPE_NONE",
        "none":                     "OBS_MONITORING_TYPE_NONE",
        "monitor-only":             "OBS_MONITORING_TYPE_MONITOR_ONLY",
        "monitor_only":             "OBS_MONITORING_TYPE_MONITOR_ONLY",
        "monitor-and-output":       "OBS_MONITORING_TYPE_MONITOR_AND_OUTPUT",
        "monitor_and_output":       "OBS_MONITORING_TYPE_MONITOR_AND_OUTPUT",
        " MONITOR-AND-OUTPUT ":     "OBS_MONITORING_TYPE_MONITOR_AND_OUTPUT",
        "unknown":                  "OBS_MONITORING_TYPE_NONE",
    }
    for in, want := range cases {
        if got := normalizeMonitoringType(in); got != want {
            t.Fatalf("normalizeMonitoringType(%q)=%q; want %q", in, got, want)
        }
    }
}
