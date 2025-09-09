package canopen

import (
    "encoding/binary"
    "fmt"

    "github.com/notnil/canbus"
)

// SDOAbort represents an SDO abort response.
type SDOAbort struct {
    Index    uint16
    Subindex uint8
    Code     uint32
}

func (e SDOAbort) Error() string {
    if msg, ok := sdoAbortText[e.Code]; ok {
        return fmt.Sprintf("canopen: sdo abort 0x%08X @ %04X:%02X: %s", e.Code, e.Index, e.Subindex, msg)
    }
    return fmt.Sprintf("canopen: sdo abort 0x%08X @ %04X:%02X", e.Code, e.Index, e.Subindex)
}

// parseSDOAbort returns node id, abort error (if this frame is an abort), and ok flag.
func parseSDOAbort(f canbus.Frame) (NodeID, *SDOAbort, bool) {
    fc, node, err := ParseCOBID(f.ID)
    if err != nil || fc != FC_SDO_TX || f.Len != 8 {
        return 0, nil, false
    }
    // Abort server->client has SCS=4 (bits 7..5)
    if ((f.Data[0] >> 5) & 0x7) != sdoSCSAbort {
        return 0, nil, false
    }
    ab := &SDOAbort{
        Index:    binary.LittleEndian.Uint16(f.Data[1:3]),
        Subindex: f.Data[3],
        Code:     binary.LittleEndian.Uint32(f.Data[4:8]),
    }
    return node, ab, true
}

// Common SDO abort codes (subset of CiA 301)
var sdoAbortText = map[uint32]string{
    0x05030000: "toggle bit not alternated",
    0x05040000: "SDO protocol timeout",
    0x05040001: "command specifier invalid or unknown",
    0x06010000: "unsupported access to object",
    0x06010001: "attempt to read a write-only object",
    0x06010002: "attempt to write a read-only object",
    0x06020000: "object does not exist",
    0x06040041: "object cannot be mapped to PDO",
    0x06040042: "PDO length exceeded",
    0x06040043: "general parameter incompatibility",
    0x06040047: "internal incompatibility in device",
    0x06060000: "hardware error",
    0x06070010: "data type does not match (length)",
    0x06070012: "data type does not match (length too high)",
    0x06070013: "data type does not match (length too low)",
    0x06090011: "sub-index does not exist",
    0x06090030: "value range exceeded (min)",
    0x06090031: "value range exceeded (max)",
    0x06090036: "invalid value for parameter",
    0x08000000: "general error",
    0x08000020: "data cannot be transferred/stored",
    0x08000021: "local control",
    0x08000022: "device state",
    0x08000023: "OD dynamic generation fails",
}


