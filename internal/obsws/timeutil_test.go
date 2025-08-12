package obsws

import (
    "testing"
    "time"
)

func TestWaitUntilFuture(t *testing.T) {
    start := time.Now()
    target := start.Add(25 * time.Millisecond)
    WaitUntil(target, 2*time.Millisecond)
    elapsed := time.Since(start)
    if elapsed < 25*time.Millisecond {
        t.Fatalf("WaitUntil returned too early: elapsed=%v", elapsed)
    }
    // Allow generous upper bound to avoid flakiness on CI
    if elapsed > 250*time.Millisecond {
        t.Fatalf("WaitUntil took too long: elapsed=%v", elapsed)
    }
}

func TestWaitUntilPast(t *testing.T) {
    start := time.Now()
    target := start.Add(-10 * time.Millisecond)
    WaitUntil(target, 2*time.Millisecond)
    if time.Since(start) > 10*time.Millisecond {
        t.Fatalf("WaitUntil should return immediately for past time")
    }
}

