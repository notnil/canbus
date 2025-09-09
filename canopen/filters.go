package canopen

import "github.com/notnil/canbus"

// CANopen-typed filters for common services and ranges.

// CANopenNMT matches NMT command frames (COB-ID 0x000, standard IDs).
func CANopenNMT() canbus.FrameFilter {
    return canbus.And(canbus.StandardOnly(), canbus.ByID(uint32(FC_NMT)))
}

// CANopenSYNC matches SYNC frames (COB-ID 0x080, standard IDs).
func CANopenSYNC() canbus.FrameFilter {
    return canbus.And(canbus.StandardOnly(), canbus.ByID(uint32(FC_SYNC)))
}

// CANopenTime matches TIME frames (COB-ID 0x100, standard IDs).
func CANopenTime() canbus.FrameFilter {
    return canbus.And(canbus.StandardOnly(), canbus.ByID(uint32(FC_TIME)))
}

// CANopenHeartbeatAny matches all heartbeat frames (0x700–0x77F).
func CANopenHeartbeatAny() canbus.FrameFilter {
    return canbus.And(canbus.StandardOnly(), canbus.ByMask(uint32(FC_NMT_ERRCTRL), 0x780))
}

// CANopenHeartbeat matches heartbeat from a specific node id.
func CANopenHeartbeat(node NodeID) canbus.FrameFilter {
    return canbus.And(canbus.StandardOnly(), canbus.ByID(COBID(FC_NMT_ERRCTRL, node)))
}

// CANopenEMCYAny matches all emergency messages (0x080–0x0FF).
func CANopenEMCYAny() canbus.FrameFilter {
    return canbus.And(canbus.StandardOnly(), canbus.ByMask(uint32(FC_EMCY), 0x780))
}

// CANopenEMCY matches emergency messages from a specific node id.
func CANopenEMCY(node NodeID) canbus.FrameFilter {
    return canbus.And(canbus.StandardOnly(), canbus.ByID(COBID(FC_EMCY, node)))
}

// SDO default COB-IDs

func CANopenSDORequestAny() canbus.FrameFilter { // client->server (rx at node)
    return canbus.And(canbus.StandardOnly(), canbus.ByMask(uint32(FC_SDO_RX), 0x780))
}

func CANopenSDOResponseAny() canbus.FrameFilter { // server->client (tx from node)
    return canbus.And(canbus.StandardOnly(), canbus.ByMask(uint32(FC_SDO_TX), 0x780))
}

func CANopenSDORequest(node NodeID) canbus.FrameFilter {
    return canbus.And(canbus.StandardOnly(), canbus.ByID(COBID(FC_SDO_RX, node)))
}

func CANopenSDOResponse(node NodeID) canbus.FrameFilter {
    return canbus.And(canbus.StandardOnly(), canbus.ByID(COBID(FC_SDO_TX, node)))
}

// PDO ranges and per-node helpers

func CANopenTPDO1Any() canbus.FrameFilter { return canbus.And(canbus.StandardOnly(), canbus.ByMask(uint32(FC_TPDO1), 0x780)) }
func CANopenTPDO2Any() canbus.FrameFilter { return canbus.And(canbus.StandardOnly(), canbus.ByMask(uint32(FC_TPDO2), 0x780)) }
func CANopenTPDO3Any() canbus.FrameFilter { return canbus.And(canbus.StandardOnly(), canbus.ByMask(uint32(FC_TPDO3), 0x780)) }
func CANopenTPDO4Any() canbus.FrameFilter { return canbus.And(canbus.StandardOnly(), canbus.ByMask(uint32(FC_TPDO4), 0x780)) }

func CANopenRPDO1Any() canbus.FrameFilter { return canbus.And(canbus.StandardOnly(), canbus.ByMask(uint32(FC_RPDO1), 0x780)) }
func CANopenRPDO2Any() canbus.FrameFilter { return canbus.And(canbus.StandardOnly(), canbus.ByMask(uint32(FC_RPDO2), 0x780)) }
func CANopenRPDO3Any() canbus.FrameFilter { return canbus.And(canbus.StandardOnly(), canbus.ByMask(uint32(FC_RPDO3), 0x780)) }
func CANopenRPDO4Any() canbus.FrameFilter { return canbus.And(canbus.StandardOnly(), canbus.ByMask(uint32(FC_RPDO4), 0x780)) }

func CANopenTPDO1(node NodeID) canbus.FrameFilter { return canbus.And(canbus.StandardOnly(), canbus.ByID(COBID(FC_TPDO1, node))) }
func CANopenTPDO2(node NodeID) canbus.FrameFilter { return canbus.And(canbus.StandardOnly(), canbus.ByID(COBID(FC_TPDO2, node))) }
func CANopenTPDO3(node NodeID) canbus.FrameFilter { return canbus.And(canbus.StandardOnly(), canbus.ByID(COBID(FC_TPDO3, node))) }
func CANopenTPDO4(node NodeID) canbus.FrameFilter { return canbus.And(canbus.StandardOnly(), canbus.ByID(COBID(FC_TPDO4, node))) }

func CANopenRPDO1(node NodeID) canbus.FrameFilter { return canbus.And(canbus.StandardOnly(), canbus.ByID(COBID(FC_RPDO1, node))) }
func CANopenRPDO2(node NodeID) canbus.FrameFilter { return canbus.And(canbus.StandardOnly(), canbus.ByID(COBID(FC_RPDO2, node))) }
func CANopenRPDO3(node NodeID) canbus.FrameFilter { return canbus.And(canbus.StandardOnly(), canbus.ByID(COBID(FC_RPDO3, node))) }
func CANopenRPDO4(node NodeID) canbus.FrameFilter { return canbus.And(canbus.StandardOnly(), canbus.ByID(COBID(FC_RPDO4, node))) }


