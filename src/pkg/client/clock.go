package client

import "time"

// Clock is where the client reads time from: which day's events to load,
// which event is happening now, and the refresh interval. Tests pass their
// own to control all three.
type Clock interface {
	// Now returns the current time.
	Now() time.Time
	// Tick returns a channel that receives every interval, and a function
	// that stops it.
	Tick(interval time.Duration) (<-chan time.Time, func())
}

// WallClock is the real clock, the client's default and the one place it
// reads real time.
type WallClock struct{}

// Now returns time.Now.
func (WallClock) Now() time.Time {
	return time.Now() //nolint:clockinterface // this is the Clock implementation the rule asks for
}

// Tick returns a time.Ticker's channel and its Stop.
func (WallClock) Tick(interval time.Duration) (<-chan time.Time, func()) {
	t := time.NewTicker(interval) //nolint:clockinterface // this is the Clock implementation the rule asks for
	return t.C, t.Stop
}
