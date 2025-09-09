canbus
=====

An idiomatic, dependency-free Go library for working with Controller Area Network (CAN) buses. The top-level module provides core CAN types and I/O, and the `canopen` subpackage offers small, composable helpers for common CANopen tasks.

- **Module import**: `github.com/notnil/canbus`
- **CANopen helpers**: `github.com/notnil/canbus/canopen`

What is CAN?
- **CAN (Controller Area Network)** is a robust, real-time field bus used in automotive, robotics, and industrial control.
- **Frames** carry up to 8 data bytes (classical CAN) with 11-bit or 29-bit identifiers.
- **Broadcast medium**: every node can see all frames; filtering happens at the node.

What is CANopen?
- **CANopen** is a higher-level protocol stack (CiA 301) standardized on top of CAN.
- It defines services such as **NMT** (network management), **Heartbeat** (node status), **EMCY** (emergency), and **SDO/PDO** for configuration and process data.
- This library implements a practical subset focused on building blocks you can integrate into your own nodes; it is not a full CANopen stack or object dictionary implementation.

Features
- Core CAN `Frame` type with validation and binary marshaling helpers
- In-memory loopback bus for testing and simulation
- Optional Linux SocketCAN driver (linux-only) implemented via raw syscalls
- Zero external dependencies beyond the Go standard library
- CANopen helpers:
  - COB-ID helpers and function code mapping
  - NMT build/parse utilities
  - Heartbeat (NMT error control) build/parse
  - EMCY encode/decode
  - SDO client supporting expedited (≤4 bytes) and segmented transfers
  

Install
```bash
go get github.com/notnil/canbus
```

Quick start
```go
package main

import (
    "fmt"

    "github.com/notnil/canbus"
)

func main() {
    bus := canbus.NewLoopbackBus()
    a := bus.Open()
    b := bus.Open()
    defer a.Close()
    defer b.Close()

    go func() { _ = a.Send(canbus.MustFrame(0x123, []byte("hi"))) }()

    f, err := b.Receive()
    if err != nil { panic(err) }
    fmt.Printf("ID=%03X LEN=%d DATA=%x\n", f.ID, f.Len, f.Data[:f.Len])
}
```

Linux SocketCAN
- Build tag: enabled automatically on linux. The `socketcan_linux.go` driver uses only `syscall` and raw syscalls.
- Open a socket with an interface name (e.g., `can0`) using `canbus.DialSocketCAN("can0")`.

Example (Linux SocketCAN)
```go
package main

import (
    "fmt"
    "log"
    "time"

    "github.com/notnil/canbus"
)

func main() {
    bus, err := canbus.DialSocketCAN("can0")
    if err != nil { log.Fatal(err) }
    defer bus.Close()

    // Send a frame
    if err := bus.Send(canbus.MustFrame(0x123, []byte{0xDE, 0xAD, 0xBE, 0xEF})); err != nil {
        log.Fatal(err)
    }

    // Receive a frame (blocks)
    go func() {
        for {
            f, err := bus.Receive()
            if err != nil { log.Fatal(err) }
            fmt.Printf("< %03X [%d] % x\n", f.ID, f.Len, f.Data[:f.Len])
        }
    }()

    time.Sleep(2 * time.Second)
}
```

CANopen
-------

The `canopen` subpackage provides small, composable helpers for common CANopen tasks: NMT, heartbeat, EMCY, and SDO (expedited and segmented) with a synchronous SDO client that works over any `canbus.Bus` (e.g., `LoopbackBus` or SocketCAN).

Example (CANopen SDO over loopback)
```go
package main

import (
    "fmt"
    "log"

    "github.com/notnil/canbus"
    "github.com/notnil/canbus/canopen"
)

func main() {
    bus := canbus.NewLoopbackBus()
    // Client uses separate endpoints for send and receive via mux
    clientTx := bus.Open()
    clientRx := bus.Open()
    defer clientTx.Close()
    defer clientRx.Close()

    // Client side: create a mux-backed SDO client, then perform download/upload
    mux := canbus.NewMux(clientRx)
    defer mux.Close()
    c := canopen.NewSDOClient(clientTx, 0x22, mux, 0)
    // Download auto-selects expedited (≤4 bytes) or segmented (>4 bytes)
    if err := c.Download(0x2000, 0x01, []byte{0xAA, 0xBB}); err != nil {
        log.Fatal(err)
    }
    data, err := c.Upload(0x2000, 0x01)
    if err != nil { log.Fatal(err) }
    fmt.Printf("SDO read: % X\n", data) // prints: SDO read: DE AD BE
}
```

License
MIT
