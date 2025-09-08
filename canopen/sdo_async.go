package canopen

import (
    "encoding/binary"
    "time"

    "github.com/notnil/canbus"
)

// SDOAsyncClient provides non-blocking SDO operations by subscribing to a
// frame multiplexer and matching responses by node/index/subindex.
//
// Use it with canbus.NewMux(bus) to avoid monopolizing bus.Receive.
type SDOAsyncClient struct {
    Bus  canbus.Bus   // for Send
    Mux  *canbus.Mux  // for Receive fan-out
    Node NodeID
}

// DownloadAsync sends an expedited download and returns a channel that will be
// closed with a nil error when the server acknowledges, or with an error if
// the mux/bus closes. It does not block reads from other consumers.
func (c *SDOAsyncClient) DownloadAsync(index uint16, subindex uint8) (<-chan error, error) {
    req, err := SDOExpeditedDownload(c.Node, index, subindex, nil)
    if err != nil {
        return nil, err
    }
    // Subscribe to matching SDO_TX response for this node and index/subindex.
    ch, cancel := c.Mux.Subscribe(func(f canbus.Frame) bool {
        fc, node, err := ParseCOBID(f.ID)
        if err != nil || fc != FC_SDO_TX || node != c.Node || f.Len != 8 {
            return false
        }
        cmd := f.Data[0]
        if (cmd>>5)&0x7 != sdoSCSDownloadInitiate {
            return false
        }
        idx := binary.LittleEndian.Uint16(f.Data[1:3])
        sub := f.Data[3]
        return idx == index && sub == subindex
    }, 1)

    out := make(chan error, 1)
    // Send request before waiting.
    if err := c.Bus.Send(req); err != nil {
        cancel()
        return nil, err
    }
    go func() {
        defer cancel()
        // Wait for first matching frame or closure.
        if _, ok := <-ch; !ok {
            out <- canbus.ErrClosed
            close(out)
            return
        }
        out <- nil
        close(out)
    }()
    return out, nil
}

// UploadAsync sends an expedited upload request and returns a channel that will
// yield the response bytes or an error. The optional timeout cancels waiting.
func (c *SDOAsyncClient) UploadAsync(index uint16, subindex uint8, timeout time.Duration) (<-chan []byte, <-chan error, error) {
    req, err := SDOExpeditedUploadRequest(c.Node, index, subindex)
    if err != nil {
        return nil, nil, err
    }
    ch, cancel := c.Mux.Subscribe(func(f canbus.Frame) bool {
        fc, node, err := ParseCOBID(f.ID)
        if err != nil || fc != FC_SDO_TX || node != c.Node || f.Len != 8 {
            return false
        }
        return true
    }, 2)

    dataCh := make(chan []byte, 1)
    errCh := make(chan error, 1)

    if err := c.Bus.Send(req); err != nil {
        cancel()
        return nil, nil, err
    }

    // Optional timeout
    var timeoutC <-chan time.Time
    if timeout > 0 {
        timer := time.NewTimer(timeout)
        timeoutC = timer.C
        // Ensure timer is stopped when done
        defer func() {
            if !timer.Stop() {
                select { case <-timer.C: default: }
            }
        }()
    }

    go func() {
        defer cancel()
        for {
            select {
            case f, ok := <-ch:
                if !ok {
                    errCh <- canbus.ErrClosed
                    close(errCh)
                    close(dataCh)
                    return
                }
                _, idx, sub, data, perr := parseSDOExpeditedUploadResponse(f)
                if perr != nil || idx != index || sub != subindex {
                    continue
                }
                dataCh <- data
                close(dataCh)
                close(errCh)
                return
            case <-timeoutC:
                errCh <- canbus.ErrClosed
                close(errCh)
                close(dataCh)
                return
            }
        }
    }()
    return dataCh, errCh, nil
}

