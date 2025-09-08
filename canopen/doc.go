// Package canopen provides high-level helpers for building CANopen nodes
// on top of the core canbus primitives.
//
// This package focuses on small, well-factored building blocks that cover
// the most commonly used parts of CANopen:
//   - COB-ID helpers and function code mapping
//   - NMT commands and node state encoding/decoding
//   - Heartbeat (NMT error control) producer/consumer byte
//   - Emergency (EMCY) frame encode/decode
//   - SDO expedited transfers (encode/decode) and a minimal synchronous client
//
// The APIs here do not attempt to implement the full CANopen stack or
// object dictionary. Instead, they provide composable types and helpers that
// are easy to test and integrate into applications.
package canopen

