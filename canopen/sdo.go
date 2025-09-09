package canopen

import (
    "encoding/binary"
    "fmt"
    "time"

    "github.com/notnil/canbus"
)

// SDO command specifiers (CCS/SCS) per CiA 301
const (
    // Initiate (expedited or segmented negotiated during initiate)
    sdoCCSDownloadInitiate = 1 // client->server
    sdoCCSUploadInitiate   = 2 // client->server
    sdoSCSDownloadInitiate = 3 // server->client
    sdoSCSUploadInitiate   = 2 // server->client

    // Segmented phase
    sdoCCSDownloadSegment  = 0 // client->server
    sdoSCSDownloadSegment  = 1 // server->client
    sdoCCSUploadSegment    = 3 // client->server
    sdoSCSUploadSegment    = 0 // server->client

    // Abort
    sdoCCSAbort            = 4
    sdoSCSAbort            = 4
)

// (no generic command builder is exposed; helpers below encode per CiA 301)

// sdoExpeditedDownload builds client->server expedited download frame (write).
// It encodes index/subindex and up to 4 data bytes.
func sdoExpeditedDownload(target NodeID, index uint16, subindex uint8, data []byte) (canbus.Frame, error) {
    if err := target.Validate(); err != nil {
        return canbus.Frame{}, err
    }
    if len(data) > 4 {
        return canbus.Frame{}, fmt.Errorf("canopen: expedited download max 4 bytes, got %d", len(data))
    }
    var f canbus.Frame
    f.ID = COBID(FC_SDO_RX, target)
    f.Len = 8
    // Build command byte per spec: CCS=1, expedited=1 if <=4, size indicated=1
    // n = number of unused bytes in bytes 4..7
    n := uint8(4 - len(data))
    // Bits: 7..5 ccs, 4: toggle/rsv=0, 3 e, 2 s, 1..0 n
    cmd := byte(0)
    cmd |= byte(sdoCCSDownloadInitiate) << 5
    cmd |= 1 << 3 // e
    cmd |= 1 << 2 // s
    cmd |= (n & 0x3)
    f.Data[0] = cmd
    binary.LittleEndian.PutUint16(f.Data[1:3], index)
    f.Data[3] = subindex
    // Data fill little-endian into bytes 4..7 for convenience. Application must
    // interpret endian according to object.
    for i := 0; i < len(data); i++ {
        f.Data[4+i] = data[i]
    }
    return f, nil
}

// parseSDOExpeditedDownload decodes an expedited initiate download request.
func parseSDOExpeditedDownload(f canbus.Frame) (NodeID, uint16, uint8, []byte, error) {
    fc, node, err := ParseCOBID(f.ID)
    if err != nil {
        return 0, 0, 0, nil, err
    }
    if fc != FC_SDO_RX {
        return 0, 0, 0, nil, fmt.Errorf("canopen: not SDO rx frame (id=0x%X)", f.ID)
    }
    if f.Len != 8 {
        return 0, 0, 0, nil, fmt.Errorf("canopen: SDO frame len %d, want 8", f.Len)
    }
    cmd := f.Data[0]
    if (cmd>>5)&0x7 != sdoCCSDownloadInitiate {
        return 0, 0, 0, nil, fmt.Errorf("canopen: not initiate download (cmd=0x%02X)", cmd)
    }
    expedited := (cmd & (1 << 3)) != 0
    sizeIndicated := (cmd & (1 << 2)) != 0
    if !expedited || !sizeIndicated {
        return 0, 0, 0, nil, fmt.Errorf("canopen: only expedited+size indicated supported (cmd=0x%02X)", cmd)
    }
    n := int(cmd & 0x3)
    if n < 0 || n > 3 {
        return 0, 0, 0, nil, fmt.Errorf("canopen: invalid n=%d", n)
    }
    size := 4 - n
    idx := binary.LittleEndian.Uint16(f.Data[1:3])
    sub := f.Data[3]
    out := make([]byte, size)
    copy(out, f.Data[4:4+size])
    return node, idx, sub, out, nil
}

// sdoExpeditedUploadRequest builds client->server request to read an object.
func sdoExpeditedUploadRequest(target NodeID, index uint16, subindex uint8) (canbus.Frame, error) {
    if err := target.Validate(); err != nil {
        return canbus.Frame{}, err
    }
    var f canbus.Frame
    f.ID = COBID(FC_SDO_RX, target)
    f.Len = 8
    cmd := byte(0)
    cmd |= byte(sdoCCSUploadInitiate) << 5
    f.Data[0] = cmd
    binary.LittleEndian.PutUint16(f.Data[1:3], index)
    f.Data[3] = subindex
    return f, nil
}

// parseSDOExpeditedUploadResponse parses server->client expedited upload response.
func parseSDOExpeditedUploadResponse(f canbus.Frame) (NodeID, uint16, uint8, []byte, error) {
    fc, node, err := ParseCOBID(f.ID)
    if err != nil {
        return 0, 0, 0, nil, err
    }
    if fc != FC_SDO_TX {
        return 0, 0, 0, nil, fmt.Errorf("canopen: not SDO tx frame (id=0x%X)", f.ID)
    }
    if f.Len != 8 {
        return 0, 0, 0, nil, fmt.Errorf("canopen: SDO frame len %d, want 8", f.Len)
    }
    cmd := f.Data[0]
    // For upload response, SCS=2 in bits 7..5, e and s set for expedited with size indicated
    if (cmd>>5)&0x7 != sdoSCSUploadInitiate {
        return 0, 0, 0, nil, fmt.Errorf("canopen: not upload response (cmd=0x%02X)", cmd)
    }
    expedited := (cmd & (1 << 3)) != 0
    sizeIndicated := (cmd & (1 << 2)) != 0
    if !expedited || !sizeIndicated {
        return 0, 0, 0, nil, fmt.Errorf("canopen: only expedited+size indicated supported (cmd=0x%02X)", cmd)
    }
    n := int(cmd & 0x3)
    size := 4 - n
    idx := binary.LittleEndian.Uint16(f.Data[1:3])
    sub := f.Data[3]
    out := make([]byte, size)
    copy(out, f.Data[4:4+size])
    return node, idx, sub, out, nil
}

// SDOClient provides a synchronous-looking SDO interface.
//
// SDOClient requires a Mux and always waits for responses via the multiplexer
// so other consumers of Receive are not blocked.
//
// Timeout is optional and applies to mux-based waits; zero means wait indefinitely.
type SDOClient struct {
    bus     canbus.Bus
    mux     *canbus.Mux
    node    NodeID
    timeout time.Duration
}

// NewSDOClient constructs an SDOClient. If mux is non-nil, operations will
// subscribe for responses via mux to avoid blocking other receivers. timeout
// applies to mux-based waits; zero means wait indefinitely.
func NewSDOClient(bus canbus.Bus, node NodeID, mux *canbus.Mux, timeout time.Duration) *SDOClient {
    if mux == nil {
        panic("canopen: SDOClient requires a non-nil Mux")
    }
    return &SDOClient{bus: bus, node: node, mux: mux, timeout: timeout}
}

// Download writes data to index/subindex. It uses expedited transfer for sizes
// up to 4 bytes and segmented transfer for larger payloads.
func (c *SDOClient) Download(index uint16, subindex uint8, data []byte) error {
    if len(data) <= 4 {
        req, err := sdoExpeditedDownload(c.node, index, subindex, data)
        if err != nil {
            return err
        }

        ch, cancel := c.mux.Subscribe(func(f canbus.Frame) bool {
            fc, node, err := ParseCOBID(f.ID)
            if err != nil || fc != FC_SDO_TX || node != c.node || f.Len != 8 {
                return false
            }
            if (f.Data[0]>>5)&0x7 != sdoSCSDownloadInitiate {
                return false
            }
            idx := binary.LittleEndian.Uint16(f.Data[1:3])
            sub := f.Data[3]
            return idx == index && sub == subindex
        }, 1)
        defer cancel()

        if err := c.bus.Send(req); err != nil {
            return err
        }

        if c.timeout > 0 {
            select {
            case _, ok := <-ch:
                if !ok { return canbus.ErrClosed }
                return nil
            case <-time.After(c.timeout):
                return canbus.ErrClosed
            }
        }
        if _, ok := <-ch; !ok {
            return canbus.ErrClosed
        }
        return nil
    }

    // Segmented download
    // Initiate segmented download with size indicated
    var init canbus.Frame
    init.ID = COBID(FC_SDO_RX, c.node)
    init.Len = 8
    cmd := byte(sdoCCSDownloadInitiate << 5) | (1 << 2) // s=1, e=0
    init.Data[0] = cmd
    binary.LittleEndian.PutUint16(init.Data[1:3], index)
    init.Data[3] = subindex
    total := uint32(len(data))
    binary.LittleEndian.PutUint32(init.Data[4:8], total)

    // Wait for initiate response
    chInit, cancelInit := c.mux.Subscribe(func(f canbus.Frame) bool {
        fc, node, err := ParseCOBID(f.ID)
        if err != nil || fc != FC_SDO_TX || node != c.node || f.Len != 8 {
            return false
        }
        if (f.Data[0]>>5)&0x7 != sdoSCSDownloadInitiate { return false }
        idx := binary.LittleEndian.Uint16(f.Data[1:3])
        sub := f.Data[3]
        return idx == index && sub == subindex
    }, 1)
    defer cancelInit()
    if err := c.bus.Send(init); err != nil { return err }
    if c.timeout > 0 {
        select {
        case _, ok := <-chInit:
            if !ok { return canbus.ErrClosed }
        case <-time.After(c.timeout):
            return canbus.ErrClosed
        }
    } else {
        if _, ok := <-chInit; !ok { return canbus.ErrClosed }
    }

    // Send segments with toggle bit alternated, wait for ack after each
    toggle := byte(0)
    sent := 0
    for sent < len(data) {
        remain := len(data) - sent
        segLen := 7
        if remain < segLen { segLen = remain }
        last := sent+segLen == len(data)
        var seg canbus.Frame
        seg.ID = COBID(FC_SDO_RX, c.node)
        seg.Len = 8
        segCmd := byte(sdoCCSDownloadSegment << 5)
        if toggle&1 == 1 { segCmd |= 1 << 4 }
        if last {
            n := byte(7 - segLen) // number of unused bytes in last segment
            segCmd |= 1 // c=1 last
            segCmd |= (n & 0x7) << 1
        }
        seg.Data[0] = segCmd
        copy(seg.Data[1:1+segLen], data[sent:sent+segLen])

        // Prepare waiter for ack
        chSeg, cancelSeg := c.mux.Subscribe(func(f canbus.Frame) bool {
            fc, node, err := ParseCOBID(f.ID)
            if err != nil || fc != FC_SDO_TX || node != c.node || f.Len != 8 {
                return false
            }
            if (f.Data[0]>>5)&0x7 != sdoSCSDownloadSegment { return false }
            // Toggle bit must match
            t := (f.Data[0] >> 4) & 0x1
            return t == (toggle & 0x1)
        }, 1)

        // Send and wait
        if err := c.bus.Send(seg); err != nil { cancelSeg(); return err }
        if c.timeout > 0 {
            select {
            case _, ok := <-chSeg:
                cancelSeg(); if !ok { return canbus.ErrClosed }
            case <-time.After(c.timeout):
                cancelSeg(); return canbus.ErrClosed
            }
        } else {
            if _, ok := <-chSeg; !ok { cancelSeg(); return canbus.ErrClosed }
            cancelSeg()
        }

        sent += segLen
        toggle ^= 1
    }
    return nil
}

// Upload reads an object. It supports both expedited and segmented transfers.
func (c *SDOClient) Upload(index uint16, subindex uint8) ([]byte, error) {
    req, err := sdoExpeditedUploadRequest(c.node, index, subindex)
    if err != nil {
        return nil, err
    }

    ch, cancel := c.mux.Subscribe(func(f canbus.Frame) bool {
        fc, node, err := ParseCOBID(f.ID)
        if err != nil || fc != FC_SDO_TX || node != c.node || f.Len != 8 {
            return false
        }
        // Further filtering happens after parse to check index/subindex
        return true
    }, 2)
    defer cancel()

    if err := c.bus.Send(req); err != nil {
        return nil, err
    }

    // First response decides expedited vs segmented
    var first canbus.Frame
    if c.timeout > 0 {
        select {
        case f, ok := <-ch:
            if !ok { return nil, canbus.ErrClosed }
            first = f
        case <-time.After(c.timeout):
            return nil, canbus.ErrClosed
        }
    } else {
        f, ok := <-ch
        if !ok { return nil, canbus.ErrClosed }
        first = f
    }

    // Try expedited parse
    if _, idx, sub, data, perr := parseSDOExpeditedUploadResponse(first); perr == nil && idx == index && sub == subindex {
        return data, nil
    }

    // Segmented upload initiate response expected
    if (first.Data[0]>>5)&0x7 != sdoSCSUploadInitiate {
        return nil, fmt.Errorf("canopen: unexpected SDO response 0x%02X", first.Data[0])
    }
    // e=0 for segmented
    if (first.Data[0]&(1<<3)) != 0 {
        return nil, fmt.Errorf("canopen: unexpected expedited flag in segmented upload response")
    }
    // size indicated?
    var total int = -1
    if (first.Data[0]&(1<<2)) != 0 {
        total = int(binary.LittleEndian.Uint32(first.Data[4:8]))
    }
    // Index/subindex must match
    if binary.LittleEndian.Uint16(first.Data[1:3]) != index || first.Data[3] != subindex {
        return nil, fmt.Errorf("canopen: upload initiate index mismatch")
    }

    // Now perform segmented upload loop
    out := make([]byte, 0, 256)
    toggle := byte(0)
    for {
        // Send upload segment request
        var reqSeg canbus.Frame
        reqSeg.ID = COBID(FC_SDO_RX, c.node)
        reqSeg.Len = 8
        cmd := byte(sdoCCSUploadSegment << 5)
        if toggle&1 == 1 { cmd |= 1 << 4 }
        reqSeg.Data[0] = cmd
        // rest bytes zero

        // Subscribe for matching segment response with toggle
        chSeg, cancelSeg := c.mux.Subscribe(func(f canbus.Frame) bool {
            fc, node, err := ParseCOBID(f.ID)
            if err != nil || fc != FC_SDO_TX || node != c.node || f.Len != 8 { return false }
            if (f.Data[0]>>5)&0x7 != sdoSCSUploadSegment { return false }
            t := (f.Data[0] >> 4) & 0x1
            return t == (toggle & 0x1)
        }, 1)

        if err := c.bus.Send(reqSeg); err != nil { cancelSeg(); return nil, err }
        var rsp canbus.Frame
        if c.timeout > 0 {
            select {
            case f, ok := <-chSeg:
                cancelSeg(); if !ok { return nil, canbus.ErrClosed }
                rsp = f
            case <-time.After(c.timeout):
                cancelSeg(); return nil, canbus.ErrClosed
            }
        } else {
            f, ok := <-chSeg
            cancelSeg()
            if !ok { return nil, canbus.ErrClosed }
            rsp = f
        }

        // Extract data and flags
        cFlag := (rsp.Data[0] & 0x1) != 0
        n := int((rsp.Data[0] >> 1) & 0x7) // number of unused bytes in this segment if last
        segDataEnd := 8
        if cFlag {
            segDataEnd = 8 - n
        }
        if segDataEnd < 1 || segDataEnd > 8 { return nil, fmt.Errorf("canopen: invalid segment length") }
        out = append(out, rsp.Data[1:segDataEnd]...)

        toggle ^= 1
        if cFlag {
            if total >= 0 && len(out) != total {
                // Some devices may not set size; tolerate mismatch only if size unknown
                if total >= 0 { return nil, fmt.Errorf("canopen: segmented upload size mismatch: got %d want %d", len(out), total) }
            }
            return out, nil
        }
    }
}

// Typed marshal/unmarshal helpers for common expedited cases (<=4 bytes)

func (c *SDOClient) WriteU8(index uint16, subindex uint8, value uint8) error {
    return c.Download(index, subindex, []byte{value})
}

func (c *SDOClient) WriteU16(index uint16, subindex uint8, value uint16) error {
    var b [2]byte
    binary.LittleEndian.PutUint16(b[:], value)
    return c.Download(index, subindex, b[:])
}

func (c *SDOClient) WriteU32(index uint16, subindex uint8, value uint32) error {
    var b [4]byte
    binary.LittleEndian.PutUint32(b[:], value)
    return c.Download(index, subindex, b[:])
}

func (c *SDOClient) ReadU8(index uint16, subindex uint8) (uint8, error) {
    b, err := c.Upload(index, subindex)
    if err != nil { return 0, err }
    if len(b) < 1 { return 0, fmt.Errorf("canopen: sdo read u8: empty") }
    return b[0], nil
}

func (c *SDOClient) ReadU16(index uint16, subindex uint8) (uint16, error) {
    b, err := c.Upload(index, subindex)
    if err != nil { return 0, err }
    if len(b) != 2 { return 0, fmt.Errorf("canopen: sdo read u16: got %d bytes", len(b)) }
    return binary.LittleEndian.Uint16(b), nil
}

func (c *SDOClient) ReadU32(index uint16, subindex uint8) (uint32, error) {
    b, err := c.Upload(index, subindex)
    if err != nil { return 0, err }
    if len(b) != 4 { return 0, fmt.Errorf("canopen: sdo read u32: got %d bytes", len(b)) }
    return binary.LittleEndian.Uint32(b), nil
}

