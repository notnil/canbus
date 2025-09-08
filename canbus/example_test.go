package canbus

import (
    "context"
    "fmt"
    "time"
)

func ExampleLoopbackBus() {
    bus := NewLoopbackBus()
    a := bus.Open()
    b := bus.Open()
    defer a.Close()
    defer b.Close()

    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()

    go func() { _ = a.Send(ctx, MustFrame(0x123, []byte("hi"))) }()
    f, _ := b.Receive(ctx)
    fmt.Printf("ID=%03X LEN=%d DATA=%x\n", f.ID, f.Len, f.Data[:f.Len])
    // Output: ID=123 LEN=2 DATA=6869
}

