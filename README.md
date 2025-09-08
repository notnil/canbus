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
  - SDO expedited helpers and a minimal synchronous SDO client
  - Async SDO client with frame multiplexer that doesn't block other reads

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

The `canopen` subpackage provides small, composable helpers for common CANopen tasks: NMT, heartbeat, EMCY, SDO expedited transfers, and a minimal synchronous SDO client that works over any `canbus.Bus` (e.g., `LoopbackBus` or SocketCAN).

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
    client := bus.Open()
    server := bus.Open()
    defer client.Close()
    defer server.Close()

    // Minimal CANopen SDO server: replies to download and upload for a single entry.
    go func() {
        for {
            f, err := server.Receive()
            if err != nil { return }
            fc, node, err := canopen.ParseCOBID(f.ID)
            if err != nil || fc != canopen.FC_SDO_RX || node != 0x22 { continue }
            switch f.Data[0] >> 5 {
            case 1: // initiate download request
                var rsp canbus.Frame
                rsp.ID = canopen.COBID(canopen.FC_SDO_TX, node)
                rsp.Len = 8
                rsp.Data[0] = byte(3 << 5) // download response
                rsp.Data[1] = f.Data[1]
                rsp.Data[2] = f.Data[2]
                rsp.Data[3] = f.Data[3]
                _ = server.Send(rsp)
            case 2: // initiate upload request
                var rsp canbus.Frame
                rsp.ID = canopen.COBID(canopen.FC_SDO_TX, node)
                rsp.Len = 8
                rsp.Data[0] = byte(2<<5) | (1<<3) | (1<<2) | 0x01 // e=1, s=1, n=1 (3 bytes)
                rsp.Data[1] = f.Data[1]
                rsp.Data[2] = f.Data[2]
                rsp.Data[3] = f.Data[3]
                rsp.Data[4] = 0xDE; rsp.Data[5] = 0xAD; rsp.Data[6] = 0xBE
                _ = server.Send(rsp)
            }
        }
    }()

    // Client side: perform expedited SDO download then upload
    c := &canopen.SDOClient{Bus: client, Node: 0x22}
    if err := c.Download(0x2000, 0x01, []byte{0xAA, 0xBB}); err != nil {
        log.Fatal(err)
    }
    data, err := c.Upload(0x2000, 0x01)
    if err != nil { log.Fatal(err) }
    fmt.Printf("SDO read: % X\n", data) // prints: SDO read: DE AD BE
}
```

Async SDO (non-blocking reads)
------------------------------

Use the `canbus.Mux` to fan-out frames to subscribers and the `canopen.SDOAsyncClient` to issue SDO requests without monopolizing `Receive()`.

```go
package main

import (
    "fmt"
    "time"

    "github.com/notnil/canbus"
    "github.com/notnil/canbus/canopen"
)

func main() {
    lb := canbus.NewLoopbackBus()
    // Open two endpoints: one for sending, one for receiving/muxing
    tx := lb.Open()
    rx := lb.Open()
    defer tx.Close()
    defer rx.Close()

    mux := canbus.NewMux(rx)
    defer mux.Close()

    client := canopen.SDOAsyncClient{Bus: tx, Mux: mux, Node: 0x22}

    // Simulated server
    srv := lb.Open()
    defer srv.Close()
    go func() {
        for {
            f, err := srv.Receive(); if err != nil { return }
            fc, node, _ := canopen.ParseCOBID(f.ID)
            if fc != canopen.FC_SDO_RX || node != 0x22 { continue }
            switch f.Data[0] >> 5 {
            case 1: // download
                var rsp canbus.Frame
                rsp.ID = canopen.COBID(canopen.FC_SDO_TX, node)
                rsp.Len = 8
                rsp.Data[0] = byte(3 << 5)
                rsp.Data[1], rsp.Data[2], rsp.Data[3] = f.Data[1], f.Data[2], f.Data[3]
                _ = srv.Send(rsp)
            case 2: // upload
                var rsp canbus.Frame
                rsp.ID = canopen.COBID(canopen.FC_SDO_TX, node)
                rsp.Len = 8
                rsp.Data[0] = byte(2<<5) | (1<<3) | (1<<2) | 0x01 // expedited 3 bytes
                rsp.Data[1], rsp.Data[2], rsp.Data[3] = f.Data[1], f.Data[2], f.Data[3]
                rsp.Data[4], rsp.Data[5], rsp.Data[6] = 0xDE, 0xAD, 0xBE
                _ = srv.Send(rsp)
            }
        }
    }()

    // Download without blocking other receivers
    done, err := client.DownloadAsync(0x2000, 0x01)
    if err != nil { panic(err) }
    if e := <-done; e != nil { panic(e) }

    // Upload with timeout
    dataCh, errCh, err := client.UploadAsync(0x2000, 0x01, 2*time.Second)
    if err != nil { panic(err) }
    select {
    case data := <-dataCh:
        fmt.Printf("SDO read: % X\n", data)
    case e := <-errCh:
        panic(e)
    }
}
```

License
MIT
