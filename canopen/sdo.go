package canopen

import (
    "encoding/binary"
    "fmt"
    "time"

    "github.com/notnil/canbus"
)

// SDO command specifiers for initiate download/upload expedited
const (
    sdoCCSDownloadInitiate = 1 // client->server
    sdoCCSUploadInitiate   = 2 // client->server
    sdoSCSDownloadInitiate = 3 // server->client
    sdoSCSUploadInitiate   = 2 // server->client
)

// (no generic command builder is exposed; helpers below encode per CiA 301)

// SDOExpeditedDownload builds client->server expedited download frame (write).
// It encodes index/subindex and up to 4 data bytes.
func SDOExpeditedDownload(target NodeID, index uint16, subindex uint8, data []byte) (canbus.Frame, error) {
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

// SDOExpeditedUploadRequest builds client->server request to read an object.
func SDOExpeditedUploadRequest(target NodeID, index uint16, subindex uint8) (canbus.Frame, error) {
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
// If Mux is set, it waits for responses via the multiplexer so other consumers
// of Receive are not blocked. If Mux is nil, it falls back to directly reading
// from Bus.Receive (legacy behavior).
//
// Timeout is optional and only applies when using Mux. A zero timeout waits
// indefinitely for the matching response.
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
    return &SDOClient{bus: bus, node: node, mux: mux, timeout: timeout}
}

// Download writes up to 4 bytes to index/subindex using expedited transfer.
func (c *SDOClient) Download(index uint16, subindex uint8, data []byte) error {
    req, err := SDOExpeditedDownload(c.node, index, subindex, data)
    if err != nil {
        return err
    }

    // Fast path using mux if available.
    if c.mux != nil {
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

    // Legacy fallback: block on Bus.Receive.
    if err := c.bus.Send(req); err != nil {
        return err
    }
    for {
        f, err := c.bus.Receive()
        if err != nil {
            return err
        }
        fc, node, perr := ParseCOBID(f.ID)
        if perr != nil {
            continue
        }
        if fc != FC_SDO_TX || node != c.node || f.Len != 8 {
            continue
        }
        cmd := f.Data[0]
        if (cmd>>5)&0x7 != sdoSCSDownloadInitiate {
            continue
        }
        idx := binary.LittleEndian.Uint16(f.Data[1:3])
        sub := f.Data[3]
        if idx == index && sub == subindex {
            return nil
        }
    }
}

// Upload reads up to 4 bytes via expedited transfer.
func (c *SDOClient) Upload(index uint16, subindex uint8) ([]byte, error) {
    req, err := SDOExpeditedUploadRequest(c.node, index, subindex)
    if err != nil {
        return nil, err
    }

    if c.mux != nil {
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

        if c.timeout > 0 {
            timeout := time.After(c.timeout)
            for {
                select {
                case f, ok := <-ch:
                    if !ok { return nil, canbus.ErrClosed }
                    _, idx, sub, data, perr := parseSDOExpeditedUploadResponse(f)
                    if perr != nil || idx != index || sub != subindex { continue }
                    return data, nil
                case <-timeout:
                    return nil, canbus.ErrClosed
                }
            }
        }
        for {
            f, ok := <-ch
            if !ok { return nil, canbus.ErrClosed }
            _, idx, sub, data, perr := parseSDOExpeditedUploadResponse(f)
            if perr != nil || idx != index || sub != subindex { continue }
            return data, nil
        }
    }

    // Legacy fallback: block on Bus.Receive.
    if err := c.bus.Send(req); err != nil {
        return nil, err
    }
    for {
        f, err := c.bus.Receive()
        if err != nil {
            return nil, err
        }
        fc, node, perr := ParseCOBID(f.ID)
        if perr != nil {
            continue
        }
        if fc != FC_SDO_TX || node != c.node || f.Len != 8 {
            continue
        }
        _, idx, sub, data, perr := parseSDOExpeditedUploadResponse(f)
        if perr != nil {
            continue
        }
        if idx == index && sub == subindex {
            return data, nil
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

