package canbus

import (
    "bytes"
    "context"
    "testing"
    "time"
)

func TestFrameValidateAndBinary(t *testing.T) {
    f := MustFrame(0x123, []byte{1, 2, 3, 4})
    if err := f.Validate(); err != nil {
        t.Fatalf("validate: %v", err)
    }
    b, err := f.MarshalBinary()
    if err != nil {
        t.Fatalf("marshal: %v", err)
    }
    var g Frame
    if err := g.UnmarshalBinary(b); err != nil {
        t.Fatalf("unmarshal: %v", err)
    }
    if g != f {
        t.Fatalf("roundtrip mismatch: got %+v want %+v", g, f)
    }
}

func TestExtendedAndRTR(t *testing.T) {
    var f Frame
    f.ID = 0x1ABCDEFF
    f.Extended = true
    f.RTR = true
    f.Len = 0
    if err := f.Validate(); err != nil {
        t.Fatalf("validate: %v", err)
    }
    b, err := f.MarshalBinary()
    if err != nil {
        t.Fatalf("marshal: %v", err)
    }
    var g Frame
    if err := g.UnmarshalBinary(b); err != nil {
        t.Fatalf("unmarshal: %v", err)
    }
    if g.ID != f.ID || !g.Extended || !g.RTR {
        t.Fatalf("flags lost: got %+v", g)
    }
}

func TestLoopbackBus(t *testing.T) {
    bus := NewLoopbackBus()
    a := bus.Open()
    b := bus.Open()
    defer a.Close()
    defer b.Close()

    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()

    send := MustFrame(0x321, []byte("hello"))

    done := make(chan error, 1)
    go func() { done <- a.Send(ctx, send) }()

    got, err := b.Receive(ctx)
    if err != nil {
        t.Fatalf("receive: %v", err)
    }
    if got.ID != send.ID || got.Len != send.Len || !bytes.Equal(got.Data[:got.Len], send.Data[:send.Len]) {
        t.Fatalf("mismatch: got %+v want %+v", got, send)
    }
    if err := <-done; err != nil {
        t.Fatalf("send: %v", err)
    }
}

