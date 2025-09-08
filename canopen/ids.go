package canopen

import "fmt"

// NodeID represents a CANopen node identifier (1..127).
// Value 0 is used for broadcast in some services (e.g., NMT) and is allowed
// where explicitly documented.
type NodeID uint8

// Validate checks that the node identifier is in the range 1..127.
func (n NodeID) Validate() error {
    if n < 1 || n > 127 {
        return fmt.Errorf("canopen: invalid node id %d (valid 1..127)", n)
    }
    return nil
}

// FunctionCode enumerates CANopen function code bases.
// See CiA 301 table for COB-IDs.
type FunctionCode uint16

const (
    // Fixed COB-IDs (no node id addition)
    FC_SYNC        FunctionCode = 0x080
    FC_TIME        FunctionCode = 0x100

    // PDOs
    FC_TPDO1       FunctionCode = 0x180
    FC_RPDO1       FunctionCode = 0x200
    FC_TPDO2       FunctionCode = 0x280
    FC_RPDO2       FunctionCode = 0x300
    FC_TPDO3       FunctionCode = 0x380
    FC_RPDO3       FunctionCode = 0x400
    FC_TPDO4       FunctionCode = 0x480
    FC_RPDO4       FunctionCode = 0x500

    // SDO
    FC_SDO_TX      FunctionCode = 0x580 // server->client
    FC_SDO_RX      FunctionCode = 0x600 // client->server

    // NMT and error control
    FC_NMT         FunctionCode = 0x000 // broadcast id for NMT
    FC_NMT_ERRCTRL FunctionCode = 0x700 // heartbeat / node guarding

    // Emergency
    FC_EMCY        FunctionCode = 0x080 // + node id
)

// COBID composes the 11-bit CAN identifier for a function code and node id.
// For function codes that are fixed (e.g., SYNC, TIME, NMT), the node id is
// ignored.
func COBID(fc FunctionCode, node NodeID) uint32 {
    base := uint16(fc)
    // Only NMT and TIME use fixed COB-IDs here. SYNC shares the 0x080 base
    // with EMCY but EMCY requires adding node id, so SYNC is not treated as
    // fixed in this helper to avoid ambiguity.
    if fc == FC_NMT || fc == FC_TIME {
        return uint32(base)
    }
    return uint32(base + uint16(node))
}

// ParseCOBID attempts to infer the function code and node id from the 11-bit id.
// Note: For overlapping ranges (e.g. SYNC vs EMCY for node 0), or when multiple
// function codes share bases, the mapping may not be unique. This helper returns
// the most common mapping rules as used in practice.
func ParseCOBID(id uint32) (FunctionCode, NodeID, error) {
    if id > 0x7FF {
        return 0, 0, fmt.Errorf("canopen: invalid 11-bit id 0x%X", id)
    }
    u := uint16(id)
    switch {
    case u == uint16(FC_NMT):
        return FC_NMT, 0, nil
    case u == uint16(FC_SYNC):
        return FC_SYNC, 0, nil
    case u == uint16(FC_TIME):
        return FC_TIME, 0, nil
    }
    // Ranges with node id suffix
    switch {
    case u >= 0x080 && u <= 0x0FF:
        return FC_EMCY, NodeID(u - 0x080), nil
    case u >= 0x180 && u <= 0x1FF:
        return FC_TPDO1, NodeID(u - 0x180), nil
    case u >= 0x200 && u <= 0x27F:
        return FC_RPDO1, NodeID(u - 0x200), nil
    case u >= 0x280 && u <= 0x2FF:
        return FC_TPDO2, NodeID(u - 0x280), nil
    case u >= 0x300 && u <= 0x37F:
        return FC_RPDO2, NodeID(u - 0x300), nil
    case u >= 0x380 && u <= 0x3FF:
        return FC_TPDO3, NodeID(u - 0x380), nil
    case u >= 0x400 && u <= 0x47F:
        return FC_RPDO3, NodeID(u - 0x400), nil
    case u >= 0x480 && u <= 0x4FF:
        return FC_TPDO4, NodeID(u - 0x480), nil
    case u >= 0x500 && u <= 0x57F:
        return FC_RPDO4, NodeID(u - 0x500), nil
    case u >= 0x580 && u <= 0x5FF:
        return FC_SDO_TX, NodeID(u - 0x580), nil
    case u >= 0x600 && u <= 0x67F:
        return FC_SDO_RX, NodeID(u - 0x600), nil
    case u >= 0x700 && u <= 0x77F:
        return FC_NMT_ERRCTRL, NodeID(u - 0x700), nil
    default:
        return 0, 0, fmt.Errorf("canopen: id 0x%X not in CANopen base ranges", id)
    }
}

