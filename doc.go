// Package canbus provides core types and utilities for working with
// Controller Area Network (CAN) in Go without external dependencies.
//
// It includes:
//   - A core Frame type with validation and binary marshaling helpers
//   - An in-memory loopback bus for tests and simulations
//   - A Linux SocketCAN driver (linux-only) via raw syscalls
package canbus

