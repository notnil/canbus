package canopen

import (
    "fmt"

    "github.com/notnil/canbus"
)

// Heartbeat represents an NMT error control heartbeat from a node and
// implements CAN frame marshal/unmarshal.
type Heartbeat struct {
    Node  NodeID
    State NMTState
}

// MarshalCANFrame encodes the heartbeat to a CAN frame.
func (h Heartbeat) MarshalCANFrame() (canbus.Frame, error) {
    return buildHeartbeat(h.Node, h.State)
}

// UnmarshalCANFrame decodes the heartbeat from a CAN frame.
func (h *Heartbeat) UnmarshalCANFrame(f canbus.Frame) error {
    node, state, err := parseHeartbeat(f)
    if err != nil {
        return err
    }
    h.Node = node
    h.State = state
    return nil
}

// buildHeartbeat produces an NMT error control heartbeat frame for node/state.
// A heartbeat contains a single byte with the current NMTState.
func buildHeartbeat(node NodeID, state NMTState) (canbus.Frame, error) {
    if err := node.Validate(); err != nil {
        return canbus.Frame{}, err
    }
    var f canbus.Frame
    f.ID = COBID(FC_NMT_ERRCTRL, node)
    f.Len = 1
    f.Data[0] = byte(state)
    return f, nil
}

// parseHeartbeat parses a heartbeat frame and returns node id and state.
func parseHeartbeat(f canbus.Frame) (NodeID, NMTState, error) {
    if f.Len < 1 {
        return 0, 0, fmt.Errorf("canopen: heartbeat too short: %d", f.Len)
    }
    fc, node, err := ParseCOBID(f.ID)
    if err != nil {
        return 0, 0, err
    }
    if fc != FC_NMT_ERRCTRL {
        return 0, 0, fmt.Errorf("canopen: not a heartbeat frame (id=0x%X)", f.ID)
    }
    return node, NMTState(f.Data[0]), nil
}

// SubscribeHeartbeats subscribes to heartbeat (NMT error control) frames via mux
// and delivers parsed events. If nodeFilter is non-nil, only heartbeats from the
// specified node are delivered. The returned cancel must be called when done.
// The channel will be closed on cancel or if the underlying mux is closed.
func SubscribeHeartbeats(mux *canbus.Mux, nodeFilter *NodeID, buffer int) (<-chan Heartbeat, func()) {
    frames, cancel := mux.Subscribe(func(f canbus.Frame) bool {
        fc, node, err := ParseCOBID(f.ID)
        if err != nil || fc != FC_NMT_ERRCTRL || f.Len < 1 {
            return false
        }
        if nodeFilter != nil && node != *nodeFilter {
            return false
        }
        return true
    }, buffer)

    out := make(chan Heartbeat, buffer)
    go func() {
        defer close(out)
        for f := range frames {
            node, state, err := parseHeartbeat(f)
            if err != nil {
                continue
            }
            out <- Heartbeat{Node: node, State: state}
        }
    }()
    return out, cancel
}

