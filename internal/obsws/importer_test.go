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
