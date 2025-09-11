package canopen

import (
    "encoding/binary"
    "fmt"
    "time"

    "github.com/notnil/canbus"
)

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
    // expeditedMode selects how the command byte is encoded for expedited
    // downloads.
    expeditedMode ExpeditedMode
}

// ExpeditedMode selects the encoding for expedited SDO download command byte.
type ExpeditedMode int

const (
    // ExpeditedModeSpec encodes the command byte strictly per CiA 301 bitfields
    // (yielding 0x2C/0x2D/0x2E/0x2F for 4/3/2/1 bytes respectively).
    ExpeditedModeSpec ExpeditedMode = iota
    // ExpeditedModeClassic encodes using the widely used legacy values:
    // 0x23/0x27/0x2B/0x2F for 4/3/2/1 bytes respectively.
    ExpeditedModeClassic
)

// NewSDOClient constructs an SDOClient. If mux is non-nil, operations will
// subscribe for responses via mux to avoid blocking other receivers. timeout
// applies to mux-based waits; zero means wait indefinitely.
func NewSDOClient(bus canbus.Bus, node NodeID, mux *canbus.Mux, timeout time.Duration) *SDOClient {
    if mux == nil {
        panic("canopen: SDOClient requires a non-nil Mux")
    }
    return &SDOClient{bus: bus, node: node, mux: mux, timeout: timeout, expeditedMode: ExpeditedModeSpec}
}

// NewSDOClientWithMode constructs an SDOClient with a specific expedited
// encoding mode.
func NewSDOClientWithMode(bus canbus.Bus, node NodeID, mux *canbus.Mux, timeout time.Duration, mode ExpeditedMode) *SDOClient {
    if mux == nil {
        panic("canopen: SDOClient requires a non-nil Mux")
    }
    return &SDOClient{bus: bus, node: node, mux: mux, timeout: timeout, expeditedMode: mode}
}

// SetClassicExpedited enables or disables the classic expedited download
// encoding for the command byte (0x23/0x27/0x2B/0x2F). When disabled, the
// command byte is encoded strictly per CiA 301 bitfields (yielding
// 0x2C/0x2D/0x2E/0x2F for 4/3/2/1 bytes respectively).
// (runtime setter removed; select mode via constructor)

// Download writes data to index/subindex. It uses expedited transfer for sizes
// up to 4 bytes and segmented transfer for larger payloads.
func (c *SDOClient) Download(index uint16, subindex uint8, data []byte) error {
    if len(data) <= 4 {
        var req canbus.Frame
        var err error
        switch c.expeditedMode {
        case ExpeditedModeClassic:
            req, err = sdoExpeditedDownloadClassic(c.node, index, subindex, data)
        default:
            req, err = sdoExpeditedDownload(c.node, index, subindex, data)
        }
        if err != nil {
            return err
        }

        ch, cancel := c.mux.Subscribe(func(f canbus.Frame) bool {
            fc, node, err := ParseCOBID(f.ID)
            if err != nil || fc != FC_SDO_TX || node != c.node || f.Len != 8 {
                return false
            }
            cmd := (f.Data[0] >> 5) & 0x7
            if cmd == sdoSCSAbort {
                // Only deliver aborts for our index/subindex
                idx := binary.LittleEndian.Uint16(f.Data[1:3])
                sub := f.Data[3]
                return idx == index && sub == subindex
            }
            if cmd != sdoSCSDownloadInitiate { return false }
            idx := binary.LittleEndian.Uint16(f.Data[1:3])
            sub := f.Data[3]
            return idx == index && sub == subindex
        }, 1)
        defer cancel()

        if err := c.bus.Send(req); err != nil {
            return err
        }

        var rsp canbus.Frame
        if c.timeout > 0 {
            select {
            case f, ok := <-ch:
                if !ok { return canbus.ErrClosed }
                rsp = f
            case <-time.After(c.timeout):
                return canbus.ErrClosed
            }
        } else {
            f, ok := <-ch
            if !ok { return canbus.ErrClosed }
            rsp = f
        }
        if _, ab, ok := parseSDOAbort(rsp); ok {
            return *ab
        }
        return nil
    }

    // Segmented download
    // Initiate segmented download with size indicated
    total := uint32(len(data))
    init := buildSDODownloadInitiateSegmented(c.node, index, subindex, total)

    // Wait for initiate response
    chInit, cancelInit := c.mux.Subscribe(sdoServerFilterForNode(c.node, func(f canbus.Frame) bool {
        if sdoCmd(f) == sdoSCSAbort { return sdoMatchAbortFor(index, subindex)(f) }
        return sdoMatchDownloadInitiateOK(index, subindex)(f)
    }), 1)
    defer cancelInit()
    if err := c.bus.Send(init); err != nil { return err }
    rspInit, err := waitWithTimeout(chInit, c.timeout)
    if err != nil { return err }
    if _, ab, ok := parseSDOAbort(rspInit); ok { return *ab }

    // Send segments with toggle bit alternated, wait for ack after each
    toggle := byte(0)
    sent := 0
    for sent < len(data) {
        remain := len(data) - sent
        segLen := 7
        if remain < segLen { segLen = remain }
        last := sent+segLen == len(data)
        seg := buildSDODownloadSegment(c.node, data[sent:sent+segLen], toggle, last)

        // Prepare waiter for ack
        chSeg, cancelSeg := c.mux.Subscribe(sdoServerFilterForNode(c.node, func(f canbus.Frame) bool {
            if sdoCmd(f) == sdoSCSAbort { return true }
            return sdoMatchDownloadSegAck(toggle)(f)
        }), 1)

        // Send and wait
        if err := c.bus.Send(seg); err != nil { cancelSeg(); return err }
        rspSeg, err := waitWithTimeout(chSeg, c.timeout)
        cancelSeg()
        if err != nil { return err }
        if _, ab, ok := parseSDOAbort(rspSeg); ok { return *ab }

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

    ch, cancel := c.mux.Subscribe(sdoServerFilterForNode(c.node, func(f canbus.Frame) bool {
        if sdoCmd(f) == sdoSCSAbort { return sdoMatchAbortFor(index, subindex)(f) }
        return sdoMatchUploadInitiate()(f)
    }), 2)
    defer cancel()

    if err := c.bus.Send(req); err != nil {
        return nil, err
    }

    // First response decides expedited vs segmented
    first, err := waitWithTimeout(ch, c.timeout)
    if err != nil { return nil, err }

    if _, ab, ok := parseSDOAbort(first); ok {
        if ab.Index == index && ab.Subindex == subindex { return nil, *ab }
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
        chSeg, cancelSeg := c.mux.Subscribe(sdoServerFilterForNode(c.node, func(f canbus.Frame) bool {
            if sdoCmd(f) == sdoSCSAbort { return true }
            return sdoMatchUploadSeg(toggle)(f)
        }), 1)

        if err := c.bus.Send(reqSeg); err != nil { cancelSeg(); return nil, err }
        var rsp canbus.Frame
        rsp, err := waitWithTimeout(chSeg, c.timeout)
        cancelSeg()
        if err != nil { return nil, err }
        if _, ab, ok := parseSDOAbort(rsp); ok { return nil, *ab }

        // Extract data and flags
        segData, last, err := parseSDOUploadSegmentData(rsp)
        if err != nil { return nil, err }
        out = append(out, segData...)

        toggle ^= 1
        if last {
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

// sdoExpeditedDownloadClassic builds expedited download using the widely used
// legacy command byte constants: for size n in bytes the command is
// 0x23 + ((4-n) << 2). This yields: n=4 -> 0x23, n=3 -> 0x27, n=2 -> 0x2B, n=1 -> 0x2F.
// Reads are unaffected; this only changes the request command byte encoding.
func sdoExpeditedDownloadClassic(target NodeID, index uint16, subindex uint8, data []byte) (canbus.Frame, error) {
    if err := target.Validate(); err != nil {
        return canbus.Frame{}, err
    }
    if len(data) == 0 || len(data) > 4 {
        return canbus.Frame{}, fmt.Errorf("canopen: classic expedited requires 1..4 bytes, got %d", len(data))
    }
    var f canbus.Frame
    f.ID = COBID(FC_SDO_RX, target)
    f.Len = 8
    // Calculate classic command byte
    // 0x23 + ((4-n)<<2)
    n := byte(len(data))
    cmd := byte(0x23 + ((4-int(n)) << 2))
    f.Data[0] = cmd
    binary.LittleEndian.PutUint16(f.Data[1:3], index)
    f.Data[3] = subindex
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


