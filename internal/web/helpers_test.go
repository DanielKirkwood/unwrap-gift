package web_test

import (
	"bytes"
	"errors"
	"log/slog"
)

// testLogger returns a [slog.Logger] that discards output, shared across
// this package's tests so each test file doesn't redeclare one.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
}

// errAlwaysFail is a fixed sentinel for fakeSessionValidator/
// fakeLoginFlowProvider fixtures that need a non-nil error without caring
// about its message.
var errAlwaysFail = errors.New("fake: always fails")

// testKratosBrowserURL is the fixed Kratos browser-facing URL every test in
// this package uses — not configurable per-test since no test cares about
// its exact value beyond "some URL the login redirect target is built from".
const testKratosBrowserURL = "http://127.0.0.1:4433"

// testPhoneNumber and testFullName are the fixed identity traits every
// wishlist test in this package authenticates as — not configurable
// per-test since no test needs a second, different identity.
const (
	testPhoneNumber = "+447700900000"
	testFullName    = "Dan"
)
