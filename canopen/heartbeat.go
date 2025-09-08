package canopen

import (
    "fmt"

    "github.com/notnil/canbus"
)

// BuildHeartbeat produces an NMT error control heartbeat frame for node/state.
// A heartbeat contains a single byte with the current NMTState.
func BuildHeartbeat(node NodeID, state NMTState) (canbus.Frame, error) {
    if err := node.Validate(); err != nil {
        return canbus.Frame{}, err
    }
    var f canbus.Frame
    f.ID = COBID(FC_NMT_ERRCTRL, node)
    f.Len = 1
    f.Data[0] = byte(state)
    return f, nil
}

// ParseHeartbeat parses a heartbeat frame and returns node id and state.
func ParseHeartbeat(f canbus.Frame) (NodeID, NMTState, error) {
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

