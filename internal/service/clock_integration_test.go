//go:build integration

package service_test

import "time"

// fixedClock is a service.Clock that always returns the same instant, so
// tests can assert on event timestamps deterministically.
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// testClock is a fixed instant used across the service integration tests.
var testClock = fixedClock{t: time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)}
