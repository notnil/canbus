package canbus

import (
    "context"
    "errors"
)

// Bus represents a CAN bus connection which can send and receive CAN frames.
// Implementations should be safe for concurrent use by multiple goroutines.
type Bus interface {
    // Send transmits a frame. It may block until the frame is queued or sent.
    // Context cancellation should abort the operation and return the context error.
    Send(ctx context.Context, frame Frame) error

    // Receive retrieves the next available frame. It should block until a frame
    // is available or the context is cancelled.
    Receive(ctx context.Context) (Frame, error)

    // Close releases resources. Further Send/Receive may return an error.
    Close() error
}

// ErrClosed indicates the bus or endpoint has been closed.
var ErrClosed = errors.New("canbus: closed")

