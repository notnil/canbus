package canopen

import (
    "github.com/notnil/canbus"
)

// FrameMarshaler encodes a typed CANopen entity into a CAN frame.
type FrameMarshaler interface {
    MarshalCANFrame() (canbus.Frame, error)
}

// FrameUnmarshaler decodes a typed CANopen entity from a CAN frame.
type FrameUnmarshaler interface {
    UnmarshalCANFrame(canbus.Frame) error
}

// FrameCodec combines marshaling and unmarshaling of CAN frames.
type FrameCodec interface {
    FrameMarshaler
    FrameUnmarshaler
}
