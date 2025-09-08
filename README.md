canbus
=====

An idiomatic, dependency-free (standard library only) Go package for working with Controller Area Network (CAN) buses.

Features
- Core CAN frame type with validation and binary marshaling helpers
- In-memory loopback bus for testing and simulation
- Optional Linux SocketCAN driver (linux-only) implemented via raw syscalls
- Zero external dependencies beyond the Go standard library

Quick start
```go
package main

import (
    "context"
    "fmt"
    "time"

    "canbus"
)

func main() {
    bus := canbus.NewLoopbackBus()
    a := bus.Open()
    b := bus.Open()
    defer a.Close()
    defer b.Close()

    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()

    go func() { _ = a.Send(ctx, canbus.MustFrame(0x123, []byte("hi"))) }()

    f, err := b.Receive(ctx)
    if err != nil { panic(err) }
    fmt.Printf("ID=%03X LEN=%d DATA=%x\n", f.ID, f.Len, f.Data[:f.Len])
}
```

Linux SocketCAN
- Build tag: enabled automatically on linux. The `socketcan_linux.go` driver uses only `syscall` and raw syscalls.
- Open a socket with an interface name (e.g., `can0`) using `canbus.DialSocketCAN("can0")`.

License
MIT

# canbus
A lightweight Golang library for working with CAN bus — send, receive, and parse CAN frames with ease.
