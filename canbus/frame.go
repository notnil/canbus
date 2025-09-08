package canbus

import (
    "encoding/binary"
    "errors"
    "fmt"
)

// Frame represents a classical CAN (2.0A/2.0B) frame.
//
// Supported features:
//   - Standard (11-bit) and Extended (29-bit) identifiers
//   - Data frames and Remote Transmission Request (RTR)
//   - Data length 0-8 bytes (classical CAN)
//
// Not implemented: CAN FD specific fields.
type Frame struct {
    ID       uint32 // 11-bit (std) or 29-bit (ext)
    Extended bool   // true for 29-bit identifier
    RTR      bool   // remote transmission request
    Len      uint8  // 0..8
    Data     [8]byte
}

// Validation limits.
const (
    maxStdID = 0x7FF
    maxExtID = 0x1FFFFFFF
)

var (
    ErrInvalidID  = errors.New("canbus: invalid identifier")
    ErrInvalidLen = errors.New("canbus: invalid data length")
)

// Validate returns an error if the frame is not valid.
func (f Frame) Validate() error {
    if f.Len > 8 {
        return ErrInvalidLen
    }
    if f.Extended {
        if f.ID > maxExtID {
            return ErrInvalidID
        }
    } else {
        if f.ID > maxStdID {
            return ErrInvalidID
        }
    }
    return nil
}

// MustFrame constructs a Frame and panics if invalid. Convenience for examples.
func MustFrame(id uint32, data []byte) Frame {
    var f Frame
    f.ID = id
    if id > maxStdID {
        f.Extended = true
    }
    if len(data) > 8 {
        panic(ErrInvalidLen)
    }
    f.Len = uint8(len(data))
    copy(f.Data[:], data)
    if err := f.Validate(); err != nil {
        panic(err)
    }
    return f
}

// MarshalBinary encodes the frame to the Linux SocketCAN "struct can_frame" layout
// (16 bytes) for classical CAN. This layout is widely used and suitable for
// capture or transport. It intentionally does not include timestamping.
//
// Layout (little-endian):
//   0..3  can_id (with flags: EFF/RTR/ERR)
//   4     can_dlc (data length code)
//   5..7  padding (set to zero)
//   8..15 data bytes
func (f Frame) MarshalBinary() ([]byte, error) {
    if err := f.Validate(); err != nil {
        return nil, err
    }
    var id uint32 = f.ID
    const (
        canEffFlag = 0x80000000
        canRtrFlag = 0x40000000
    )
    if f.Extended {
        id |= canEffFlag
    }
    if f.RTR {
        id |= canRtrFlag
    }
    buf := make([]byte, 16)
    binary.LittleEndian.PutUint32(buf[0:4], id)
    buf[4] = f.Len
    copy(buf[8:16], f.Data[:])
    return buf, nil
}

// UnmarshalBinary decodes a frame from the Linux SocketCAN can_frame layout.
func (f *Frame) UnmarshalBinary(data []byte) error {
    if len(data) < 16 {
        return fmt.Errorf("canbus: need 16 bytes, got %d", len(data))
    }
    id := binary.LittleEndian.Uint32(data[0:4])
    const (
        canEffFlag = 0x80000000
        canRtrFlag = 0x40000000
        canEffMask = 0x1FFFFFFF
        canStdMask = 0x7FF
    )
    f.Extended = id&canEffFlag != 0
    f.RTR = id&canRtrFlag != 0
    if f.Extended {
        f.ID = id & canEffMask
    } else {
        f.ID = id & canStdMask
    }
    f.Len = uint8(data[4])
    copy(f.Data[:], data[8:16])
    return f.Validate()
}

