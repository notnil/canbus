package canbus

import (
    "context"
    "log/slog"
    "testing"
)

type recordSink struct{
    records []slog.Record
}

func (s *recordSink) Enabled(context.Context, slog.Level) bool { return true }
func (s *recordSink) Handle(_ context.Context, r slog.Record) error {
    // Make a deep copy of attributes because slog reuses the record during processing
    attrs := make([]slog.Attr, 0, r.NumAttrs())
    r.Attrs(func(a slog.Attr) bool { attrs = append(attrs, a); return true })
    nr := slog.Record{Time: r.Time, Level: r.Level, PC: r.PC, Message: r.Message}
    for _, a := range attrs { nr.AddAttrs(a) }
    s.records = append(s.records, nr)
    return nil
}
func (s *recordSink) WithAttrs(attrs []slog.Attr) slog.Handler { return s }
func (s *recordSink) WithGroup(name string) slog.Handler { return s }

func hasSlogMsg(records []slog.Record, level slog.Level, msg string) bool {
    for _, r := range records {
        if r.Level == level && r.Message == msg {
            return true
        }
    }
    return false
}

func TestLoggedBus_WriteAndReadLogging(t *testing.T) {
    lb := NewLoopbackBus()
    defer lb.Close()

    sink := &recordSink{}
    logger := slog.New(sink)

    // Wrap both endpoints to verify read and write logging independently.
    sender := NewLoggedBus(lb.Open(), logger, slog.LevelInfo, LogWrite)
    receiver := NewLoggedBus(lb.Open(), logger, slog.LevelInfo, LogRead)
    defer sender.Close()
    defer receiver.Close()

    frame := MustFrame(0x123, []byte{1,2,3})
    if err := sender.Send(frame); err != nil {
        t.Fatalf("send: %v", err)
    }
    if _, err := receiver.Receive(); err != nil {
        t.Fatalf("receive: %v", err)
    }

    if !hasSlogMsg(sink.records, slog.LevelInfo, "canbus send") {
        t.Fatalf("expected write log entry")
    }
    if !hasSlogMsg(sink.records, slog.LevelInfo, "canbus receive") {
        t.Fatalf("expected read log entry")
    }
}

func TestLoggedBus_ErrorLogging(t *testing.T) {
    lb := NewLoopbackBus()
    // Create and immediately close a receiver to force error on Receive
    rx := lb.Open()
    _ = rx.Close()

    sink := &recordSink{}
    logger := slog.New(sink)
    wrapped := NewLoggedBus(rx, logger, slog.LevelInfo, LogRead)
    _, _ = wrapped.Receive()

    if !hasSlogMsg(sink.records, slog.LevelError, "canbus receive error") {
        t.Fatalf("expected receive error log entry")
    }
}

func TestLoggedBus_FilterSkipsSYNCAndHeartbeat(t *testing.T) {
    lb := NewLoopbackBus()
    defer lb.Close()

    sink := &recordSink{}
    logger := slog.New(sink)

    // Filter that excludes SYNC (0x080) and any heartbeat frames (0x700-0x77F)
    exclude := Not(Or(
        ByID(0x080),              // SYNC
        ByMask(0x700, 0x780),     // Heartbeats
    ))

    sender := NewLoggedBusWithFilter(lb.Open(), logger, slog.LevelInfo, LogWrite, exclude)
    receiver := NewLoggedBusWithFilter(lb.Open(), logger, slog.LevelInfo, LogRead, exclude)
    defer sender.Close()
    defer receiver.Close()

    // Build one SYNC, one heartbeat (node 1), and one arbitrary data frame
    syncFrame := MustFrame(0x080, nil)
    hbFrame := MustFrame(0x700+0x01, []byte{0x05})
    dataFrame := MustFrame(0x123, []byte{0xDE, 0xAD})

    if err := sender.Send(syncFrame); err != nil { t.Fatalf("send sync: %v", err) }
    if err := sender.Send(hbFrame); err != nil { t.Fatalf("send hb: %v", err) }
    if err := sender.Send(dataFrame); err != nil { t.Fatalf("send data: %v", err) }

    // Drain on receiver to trigger read logs
    for i := 0; i < 3; i++ {
        if _, err := receiver.Receive(); err != nil { t.Fatalf("receive: %v", err) }
    }

    // Expect only one send and one receive log total at info level
    var sendCount, recvCount int
    for _, r := range sink.records {
        if r.Level == slog.LevelInfo && r.Message == "canbus send" { sendCount++ }
        if r.Level == slog.LevelInfo && r.Message == "canbus receive" { recvCount++ }
    }
    if sendCount != 1 { t.Fatalf("expected 1 send log, got %d", sendCount) }
    if recvCount != 1 { t.Fatalf("expected 1 receive log, got %d", recvCount) }
}

