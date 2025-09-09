package canopen

import (
    "encoding/binary"
    "time"
	"fmt"

    "github.com/notnil/canbus"
)

// Helper: extract SDO command specifier (bits 7..5)
func sdoCmd(f canbus.Frame) byte { return (f.Data[0] >> 5) & 0x7 }

// Helper: server->client filter for a specific node, then delegate to match.
func sdoServerFilterForNode(node NodeID, match func(canbus.Frame) bool) canbus.FrameFilter {
    return func(f canbus.Frame) bool {
        fc, n, err := ParseCOBID(f.ID)
        if err != nil || fc != FC_SDO_TX || n != node || f.Len != 8 {
            return false
        }
        return match(f)
    }
}

// Build initiate segmented download frame (size indicated, e=0, s=1).
func buildSDODownloadInitiateSegmented(node NodeID, index uint16, subindex uint8, total uint32) canbus.Frame {
    var f canbus.Frame
    f.ID = COBID(FC_SDO_RX, node)
    f.Len = 8
    cmd := byte(sdoCCSDownloadInitiate << 5) | (1 << 2) // s=1, e=0
    f.Data[0] = cmd
    binary.LittleEndian.PutUint16(f.Data[1:3], index)
    f.Data[3] = subindex
    binary.LittleEndian.PutUint32(f.Data[4:8], total)
    return f
}

// Build a download segment frame with payload up to 7 bytes.
func buildSDODownloadSegment(node NodeID, payload []byte, toggle byte, last bool) canbus.Frame {
    var f canbus.Frame
    f.ID = COBID(FC_SDO_RX, node)
    f.Len = 8
    cmd := byte(sdoCCSDownloadSegment << 5)
    if toggle&1 == 1 { cmd |= 1 << 4 }
    if last {
        n := byte(7 - len(payload))
        cmd |= 1 // c=1 last
        cmd |= (n & 0x7) << 1
    }
    f.Data[0] = cmd
    copy(f.Data[1:1+len(payload)], payload)
    return f
}

// Match helpers for filters
func sdoMatchAbortFor(index uint16, subindex uint8) func(canbus.Frame) bool {
    return func(f canbus.Frame) bool {
        if sdoCmd(f) != sdoSCSAbort { return false }
        idx := binary.LittleEndian.Uint16(f.Data[1:3])
        sub := f.Data[3]
        return idx == index && sub == subindex
    }
}

func sdoMatchDownloadInitiateOK(index uint16, subindex uint8) func(canbus.Frame) bool {
    return func(f canbus.Frame) bool {
        if sdoCmd(f) != sdoSCSDownloadInitiate { return false }
        idx := binary.LittleEndian.Uint16(f.Data[1:3])
        sub := f.Data[3]
        return idx == index && sub == subindex
    }
}

func sdoMatchDownloadSegAck(toggle byte) func(canbus.Frame) bool {
    return func(f canbus.Frame) bool {
        if sdoCmd(f) != sdoSCSDownloadSegment { return false }
        t := (f.Data[0] >> 4) & 0x1
        return t == (toggle & 0x1)
    }
}

func sdoMatchUploadInitiate() func(canbus.Frame) bool {
    return func(f canbus.Frame) bool {
        return sdoCmd(f) == sdoSCSUploadInitiate
    }
}

func sdoMatchUploadSeg(toggle byte) func(canbus.Frame) bool {
    return func(f canbus.Frame) bool {
        if sdoCmd(f) != sdoSCSUploadSegment { return false }
        t := (f.Data[0] >> 4) & 0x1
        return t == (toggle & 0x1)
    }
}

// Wait helper with timeout semantics used by SDOClient (timeout==0 => wait forever).
// Returns canbus.ErrClosed on timeout or closed channel to match existing behavior.
func waitWithTimeout(ch <-chan canbus.Frame, timeout time.Duration) (canbus.Frame, error) {
    if timeout > 0 {
        select {
        case f, ok := <-ch:
            if !ok { return canbus.Frame{}, canbus.ErrClosed }
            return f, nil
        case <-time.After(timeout):
            return canbus.Frame{}, canbus.ErrClosed
        }
    }
    f, ok := <-ch
    if !ok { return canbus.Frame{}, canbus.ErrClosed }
    return f, nil
}

// Parse upload segment response into data bytes and last flag.
func parseSDOUploadSegmentData(f canbus.Frame) (data []byte, last bool, err error) {
    // Extract data and flags per CiA 301
    cFlag := (f.Data[0] & 0x1) != 0
    n := int((f.Data[0] >> 1) & 0x7)
    end := 8
    if cFlag { end = 8 - n }
    if end < 1 || end > 8 {
        return nil, false, fmt.Errorf("canopen: invalid segment length")
    }
    return f.Data[1:end], cFlag, nil
}


