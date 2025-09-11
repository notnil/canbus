package canbus

import (
    "context"
    "log/slog"
)

// LoggedBus is a Bus decorator that logs Send/Receive operations using a
// slog.Logger.

// NewLoggedBus wraps the given Bus and logs reads, writes, or both at the given
// level. When logReads/logWrites are false, the corresponding operation is not
// logged.
func NewLoggedBus(inner Bus, logger *slog.Logger, level slog.Level, logReads, logWrites bool) Bus {
    return &loggedBus{
        inner:     inner,
        logger:    logger,
        level:     level,
        logReads:  logReads,
        logWrites: logWrites,
    }
}

type loggedBus struct {
    inner     Bus
    logger    *slog.Logger
    level     slog.Level
    logReads  bool
    logWrites bool
}

// Send logs the frame and the result when write logging is enabled.
func (l *loggedBus) Send(frame Frame) error {
    if l.logWrites {
        l.logger.Log(context.Background(), l.level, "canbus send",
            "id", frame.ID,
            "extended", frame.Extended,
            "rtr", frame.RTR,
            "len", int(frame.Len),
            "data", frame.Data[:frame.Len],
            "string", frame.String(),
        )
    }
    err := l.inner.Send(frame)
    if l.logWrites && err != nil {
        l.logger.Log(context.Background(), slog.LevelError, "canbus send error",
            "id", frame.ID,
            "error", err,
        )
    }
    return err
}

// Receive logs the received frame or error when read logging is enabled.
func (l *loggedBus) Receive() (Frame, error) {
    f, err := l.inner.Receive()
    if l.logReads {
        if err != nil {
            l.logger.Log(context.Background(), slog.LevelError, "canbus receive error",
                "error", err,
            )
        } else {
            l.logger.Log(context.Background(), l.level, "canbus receive",
                "id", f.ID,
                "extended", f.Extended,
                "rtr", f.RTR,
                "len", int(f.Len),
                "data", f.Data[:f.Len],
                "string", f.String(),
            )
        }
    }
    return f, err
}

// Close forwards to the inner Bus without logging.
func (l *loggedBus) Close() error {
    return l.inner.Close()
}

