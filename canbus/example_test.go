package canbus

import (
    "fmt"
)

func ExampleLoopbackBus() {
    bus := NewLoopbackBus()
    a := bus.Open()
    b := bus.Open()
    defer a.Close()
    defer b.Close()

    go func() { _ = a.Send(MustFrame(0x123, []byte("hi"))) }()
    f, _ := b.Receive()
    fmt.Printf("ID=%03X LEN=%d DATA=%x\n", f.ID, f.Len, f.Data[:f.Len])
    // Output: ID=123 LEN=2 DATA=6869
}

