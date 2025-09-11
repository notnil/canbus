package canbus

import (
    "context"
    "log/slog"
)

// LoggedBus is a Bus decorator that logs Send/Receive operations using a
// slog.Logger.

// LogOption is a bitmask for selecting which operations to log.
type LogOption uint8

const (
    LogNone  LogOption = 0
    LogRead  LogOption = 1 << iota
    LogWrite
    LogAll = LogRead | LogWrite
)

// NewLoggedBus wraps the given Bus and logs selected operations at the given
// level.
func NewLoggedBus(inner Bus, logger *slog.Logger, level slog.Level, opts LogOption) Bus {
    return &loggedBus{
        inner:     inner,
        logger:    logger,
        level:     level,
        opts:      opts,
    }
}

// NewLoggedBusWithFilter wraps the given Bus and logs selected operations but
// only for frames that satisfy the provided filter. If filter is nil, all
// frames are considered for logging (same as NewLoggedBus behavior).
func NewLoggedBusWithFilter(inner Bus, logger *slog.Logger, level slog.Level, opts LogOption, filter FrameFilter) Bus {
    return &loggedBus{
        inner:     inner,
        logger:    logger,
        level:     level,
        opts:      opts,
        filter:    filter,
    }
}

type loggedBus struct {
    inner     Bus
    logger    *slog.Logger
    level     slog.Level
    opts      LogOption
    filter    FrameFilter
}

// Send logs the frame and the result when write logging is enabled.
func (l *loggedBus) Send(frame Frame) error {
    if l.opts&LogWrite != 0 && (l.filter == nil || l.filter(frame)) {
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
    if l.opts&LogWrite != 0 && err != nil {
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
    if l.opts&LogRead != 0 {
        if err != nil {
            l.logger.Log(context.Background(), slog.LevelError, "canbus receive error",
                "error", err,
            )
        } else {
            if l.filter == nil || l.filter(f) {
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
    }
    return f, err
}

// Close forwards to the inner Bus without logging.
func (l *loggedBus) Close() error {
    return l.inner.Close()
}

