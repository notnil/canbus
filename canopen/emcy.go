package canopen

import (
    "encoding/binary"
    "fmt"

    "github.com/notnil/canbus"
)

// Emergency represents an EMCY message payload.
//
// Layout (8 bytes total):
//   0..1: Error code (little-endian)
//   2:    Error register
//   3..7: Manufacturer specific data
type Emergency struct {
    ErrorCode      uint16
    ErrorRegister  uint8
    Manufacturer   [5]byte
}

// BuildEMCY builds an EMCY frame for the given node.
func BuildEMCY(node NodeID, e Emergency) (canbus.Frame, error) {
    if err := node.Validate(); err != nil {
        return canbus.Frame{}, err
    }
    var f canbus.Frame
    f.ID = COBID(FC_EMCY, node)
    f.Len = 8
    binary.LittleEndian.PutUint16(f.Data[0:2], e.ErrorCode)
    f.Data[2] = e.ErrorRegister
    copy(f.Data[3:8], e.Manufacturer[:])
    return f, nil
}

// ParseEMCY decodes an EMCY payload from a CAN frame.
func ParseEMCY(f canbus.Frame) (NodeID, Emergency, error) {
    if f.Len < 8 {
        return 0, Emergency{}, fmt.Errorf("canopen: emcy too short: %d", f.Len)
    }
    fc, node, err := ParseCOBID(f.ID)
    if err != nil {
        return 0, Emergency{}, err
    }
    if fc != FC_EMCY {
        return 0, Emergency{}, fmt.Errorf("canopen: not an emcy frame (id=0x%X)", f.ID)
    }
    var e Emergency
    e.ErrorCode = binary.LittleEndian.Uint16(f.Data[0:2])
    e.ErrorRegister = f.Data[2]
    copy(e.Manufacturer[:], f.Data[3:8])
    return node, e, nil
}

