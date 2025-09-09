package canopen

import (
    "fmt"

    "github.com/notnil/canbus"
)

// NMTCommand is the command specifier for NMT service.
type NMTCommand uint8

const (
    NMTStart           NMTCommand = 0x01
    NMTStop            NMTCommand = 0x02
    NMTEnterPreOperational NMTCommand = 0x80
    NMTResetNode       NMTCommand = 0x81
    NMTResetCommunication NMTCommand = 0x82
)

// NMTState encodes the node state as used in heartbeat.
type NMTState uint8

const (
    StateBootup   NMTState = 0x00
    StateStopped  NMTState = 0x04
    StateOperational NMTState = 0x05
    StatePreOperational NMTState = 0x7F
)

// buildNMT builds an NMT command frame. node 0 means broadcast.
func buildNMT(cmd NMTCommand, node uint8) canbus.Frame {
    var f canbus.Frame
    f.ID = COBID(FC_NMT, 0)
    f.Len = 2
    f.Data[0] = byte(cmd)
    f.Data[1] = byte(node)
    return f
}

// parseNMT decodes an NMT frame payload returning command and target node.
func parseNMT(f canbus.Frame) (NMTCommand, uint8, error) {
    if f.ID != COBID(FC_NMT, 0) {
        return 0, 0, fmt.Errorf("canopen: not an NMT frame (id=0x%X)", f.ID)
    }
    if f.Len < 2 {
        return 0, 0, fmt.Errorf("canopen: NMT frame too short: %d", f.Len)
    }
    return NMTCommand(f.Data[0]), f.Data[1], nil
}

// NMT represents an NMT command (broadcast or targeted to a node) and
// implements CAN frame marshal/unmarshal.
// A Node value of 0 encodes broadcast per CiA 301.
type NMT struct {
    Command NMTCommand
    Node    uint8
}

// MarshalCANFrame encodes the NMT command to a CAN frame.
func (n NMT) MarshalCANFrame() (canbus.Frame, error) {
    f := buildNMT(n.Command, n.Node)
    return f, nil
}

// UnmarshalCANFrame decodes the NMT command from a CAN frame.
func (n *NMT) UnmarshalCANFrame(f canbus.Frame) error {
    cmd, node, err := parseNMT(f)
    if err != nil {
        return err
    }
    n.Command = cmd
    n.Node = node
    return nil
}

