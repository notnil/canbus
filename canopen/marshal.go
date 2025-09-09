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

// EmergencyFrame represents a full EMCY message including its source node id.
type EmergencyFrame struct {
    Node    NodeID
    Payload Emergency
}

func (e EmergencyFrame) MarshalCANFrame() (canbus.Frame, error) {
    return buildEMCY(e.Node, e.Payload)
}

func (e *EmergencyFrame) UnmarshalCANFrame(f canbus.Frame) error {
    node, payload, err := parseEMCY(f)
    if err != nil {
        return err
    }
    e.Node = node
    e.Payload = payload
    return nil
}

// HeartbeatFrame represents an NMT error control heartbeat from a node.
type HeartbeatFrame struct {
    Node  NodeID
    State NMTState
}

func (h HeartbeatFrame) MarshalCANFrame() (canbus.Frame, error) {
    return buildHeartbeat(h.Node, h.State)
}

func (h *HeartbeatFrame) UnmarshalCANFrame(f canbus.Frame) error {
    node, state, err := parseHeartbeat(f)
    if err != nil {
        return err
    }
    h.Node = node
    h.State = state
    return nil
}

// NMTFrame represents an NMT command (broadcast or targeted to a node).
// A Node value of 0 encodes broadcast per CiA 301.
type NMTFrame struct {
    Command NMTCommand
    Node    uint8
}

func (n NMTFrame) MarshalCANFrame() (canbus.Frame, error) {
    // BuildNMT never returns error; keep signature uniform by returning nil error
    f := buildNMT(n.Command, n.Node)
    return f, nil
}

func (n *NMTFrame) UnmarshalCANFrame(f canbus.Frame) error {
    cmd, node, err := parseNMT(f)
    if err != nil {
        return err
    }
    n.Command = cmd
    n.Node = node
    return nil
}

