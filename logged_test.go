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

