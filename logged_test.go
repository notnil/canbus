package canbus

import (
    "testing"
)

type kvPair struct{
    key string
    val any
}

type testLogEntry struct{
    level LogLevel
    msg   string
    kvs   []kvPair
}

type testLogger struct{
    entries []testLogEntry
}

func (t *testLogger) Log(level LogLevel, msg string, kv ...any) {
    e := testLogEntry{level: level, msg: msg}
    for i := 0; i+1 < len(kv); i+=2 {
        k, _ := kv[i].(string)
        e.kvs = append(e.kvs, kvPair{key: k, val: kv[i+1]})
    }
    t.entries = append(t.entries, e)
}

func hasMsg(entries []testLogEntry, level LogLevel, msg string) bool {
    for _, e := range entries {
        if e.level == level && e.msg == msg {
            return true
        }
    }
    return false
}

func TestLoggedBus_WriteAndReadLogging(t *testing.T) {
    lb := NewLoopbackBus()
    defer lb.Close()

    logger := &testLogger{}
    // Wrap both endpoints to verify read and write logging independently.
    sender := NewLoggedBus(lb.Open(), logger, LevelInfo, false, true)
    receiver := NewLoggedBus(lb.Open(), logger, LevelInfo, true, false)
    defer sender.Close()
    defer receiver.Close()

    frame := MustFrame(0x123, []byte{1,2,3})
    if err := sender.Send(frame); err != nil {
        t.Fatalf("send: %v", err)
    }
    if _, err := receiver.Receive(); err != nil {
        t.Fatalf("receive: %v", err)
    }

    if !hasMsg(logger.entries, LevelInfo, "canbus send") {
        t.Fatalf("expected write log entry")
    }
    if !hasMsg(logger.entries, LevelInfo, "canbus receive") {
        t.Fatalf("expected read log entry")
    }
}

func TestLoggedBus_ErrorLogging(t *testing.T) {
    lb := NewLoopbackBus()
    // Create and immediately close a receiver to force error on Receive
    rx := lb.Open()
    _ = rx.Close()

    logger := &testLogger{}
    wrapped := NewLoggedBus(rx, logger, LevelInfo, true, false)
    _, _ = wrapped.Receive()

    if !hasMsg(logger.entries, LevelError, "canbus receive error") {
        t.Fatalf("expected receive error log entry")
    }
}

