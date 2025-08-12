package obsws

import "testing"

func TestNormalizeObsAddr(t *testing.T) {
    cases := []struct{
        in  string
        out string
    }{
        {"127.0.0.1:4455", "127.0.0.1:4455"},
        {" ws://127.0.0.1:4455 ", "127.0.0.1:4455"},
        {"wss://example.com:4455", "example.com:4455"},
        {"  example.com:4455  ", "example.com:4455"},
    }
    for _, c := range cases {
        got := NormalizeObsAddr(c.in)
        if got != c.out {
            t.Fatalf("NormalizeObsAddr(%q)=%q; want %q", c.in, got, c.out)
        }
    }
}

