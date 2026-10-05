package client

import (
	"testing"
	"time"
)

func TestWallClock(t *testing.T) {
	var c Clock = WallClock{}

	before := time.Now()
	if now := c.Now(); now.Before(before) {
		t.Errorf("Now() = %v, before %v", now, before)
	}

	tick, stop := c.Tick(time.Millisecond)
	defer stop()
	select {
	case <-tick:
	case <-time.After(time.Second):
		t.Error("Tick() didn't tick within a second")
	}
}
