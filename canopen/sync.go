package canopen

import (
    "fmt"
    "time"

    "github.com/notnil/canbus"
)

// SYNC represents a CANopen SYNC message. Counter is optional (nil => length 0).
type SYNC struct {
    Counter *uint8
}

// MarshalCANFrame encodes the SYNC to a CAN frame.
func (s SYNC) MarshalCANFrame() (canbus.Frame, error) {
    var f canbus.Frame
    f.ID = COBID(FC_SYNC, 0)
    if s.Counter != nil {
        f.Len = 1
        f.Data[0] = *s.Counter
    } else {
        f.Len = 0
    }
    return f, nil
}

// UnmarshalCANFrame decodes the SYNC from a CAN frame.
func (s *SYNC) UnmarshalCANFrame(f canbus.Frame) error {
    fc, _, err := ParseCOBID(f.ID)
    if err != nil {
        return err
    }
    if fc != FC_SYNC {
        return fmt.Errorf("canopen: not a SYNC frame (id=0x%X)", f.ID)
    }
    switch f.Len {
    case 0:
        s.Counter = nil
    case 1:
        v := f.Data[0]
        s.Counter = &v
    default:
        return fmt.Errorf("canopen: SYNC length %d invalid", f.Len)
    }
    return nil
}

// SYNCWriter periodically transmits SYNC frames on the provided bus.
// If WithCounter is true, a counter byte (0..127 then wrap) is included.
type SYNCWriter struct {
    bus        canbus.Bus
    interval   time.Duration
    withCounter bool

    stop chan struct{}
}

// NewSYNCWriter creates a SYNC writer that sends at the given interval.
// If withCounter is true, a modulo-128 counter byte is added per CiA 301.
func NewSYNCWriter(bus canbus.Bus, interval time.Duration, withCounter bool) *SYNCWriter {
    return &SYNCWriter{bus: bus, interval: interval, withCounter: withCounter, stop: make(chan struct{})}
}

// Start launches the background goroutine. Calling Start multiple times has no additional effect.
func (w *SYNCWriter) Start() {
    if w.stop == nil {
        w.stop = make(chan struct{})
    }
    go w.run()
}

// Stop signals the writer to stop and waits for termination.
func (w *SYNCWriter) Stop() {
    if w.stop == nil {
        return
    }
    select {
    case <-w.stop:
        return
    default:
    }
    close(w.stop)
}

func (w *SYNCWriter) run() {
    ticker := time.NewTicker(w.interval)
    defer ticker.Stop()
    var counter uint8 = 0
    for {
        select {
        case <-w.stop:
            return
        case <-ticker.C:
            var frame canbus.Frame
            frame.ID = COBID(FC_SYNC, 0)
            if w.withCounter {
                frame.Len = 1
                frame.Data[0] = counter & 0x7F
                counter = (counter + 1) & 0x7F
            } else {
                frame.Len = 0
            }
            _ = w.bus.Send(frame)
        }
    }
}


