package canopen

import (
    "bytes"
    "encoding/binary"
    "fmt"
    "testing"
    "time"

    "github.com/notnil/canbus"
)

func TestCOBIDHelpers(t *testing.T) {
    if id := COBID(FC_TPDO1, 1); id != 0x181 {
        t.Fatalf("tpdo1 id: 0x%X", id)
    }
    if fc, node, err := ParseCOBID(0x5FF); err != nil || fc != FC_SDO_TX || node != 0x7F {
        t.Fatalf("parse sdo tx: fc=%v node=%v err=%v", fc, node, err)
    }
}

func TestNMTBuildParse(t *testing.T) {
    f := BuildNMT(NMTStart, 0)
    if cmd, node, err := ParseNMT(f); err != nil || cmd != NMTStart || node != 0 {
        t.Fatalf("nmt parse mismatch: cmd=%v node=%d err=%v", cmd, node, err)
    }
}

func TestHeartbeat(t *testing.T) {
    f, err := BuildHeartbeat(10, StateOperational)
    if err != nil { t.Fatal(err) }
    node, st, err := ParseHeartbeat(f)
    if err != nil { t.Fatal(err) }
    if node != 10 || st != StateOperational {
        t.Fatalf("heartbeat mismatch node=%d st=%v", node, st)
    }
}

func TestEMCY(t *testing.T) {
    e := Emergency{ErrorCode: 0x1234, ErrorRegister: 0x05}
    f, err := BuildEMCY(5, e)
    if err != nil { t.Fatal(err) }
    node, g, err := ParseEMCY(f)
    if err != nil { t.Fatal(err) }
    if node != 5 || g.ErrorCode != 0x1234 || g.ErrorRegister != 0x05 {
        t.Fatalf("emcy mismatch: node=%d g=%+v", node, g)
    }
}

func TestSDOExpeditedHelpers(t *testing.T) {
    data := []byte{0xDE, 0xAD, 0xBE, 0xEF}
    f, err := sdoExpeditedDownload(0x23, 0x2000, 0x01, data)
    if err != nil { t.Fatal(err) }
    node, idx, sub, got, err := parseSDOExpeditedDownload(f)
    if err != nil { t.Fatal(err) }
    if node != 0x23 || idx != 0x2000 || sub != 0x01 || !bytes.Equal(got, data) {
        t.Fatalf("sdo parse mismatch: node=%d idx=0x%X sub=%d data=%x", node, idx, sub, got)
    }

    req, err := sdoExpeditedUploadRequest(0x23, 0x1018, 0x00)
    if err != nil { t.Fatal(err) }
    if fc, node, err := ParseCOBID(req.ID); err != nil || fc != FC_SDO_RX || node != 0x23 {
        t.Fatalf("upload req cobid: fc=%v node=%d err=%v", fc, node, err)
    }
}

func TestSDOClientDownloadUpload(t *testing.T) {
    bus := canbus.NewLoopbackBus()
    clientEp := bus.Open()
    serverEp := bus.Open()
    defer clientEp.Close()
    defer serverEp.Close()

    // Minimal server: respond to download and upload requests for a single entry.
    stored := []byte{0x01, 0x02, 0x03}
    go func() {
        for {
            f, err := serverEp.Receive()
            if err != nil { return }
            fc, node, err := ParseCOBID(f.ID)
            if err != nil { continue }
            if fc != FC_SDO_RX || node != 0x22 {
                continue
            }
            cmd := f.Data[0] >> 5
            switch cmd {
            case sdoCCSDownloadInitiate:
                // respond with download response
                var rsp canbus.Frame
                rsp.ID = COBID(FC_SDO_TX, node)
                rsp.Len = 8
                rsp.Data[0] = byte(sdoSCSDownloadInitiate << 5)
                rsp.Data[1] = f.Data[1]
                rsp.Data[2] = f.Data[2]
                rsp.Data[3] = f.Data[3]
                _ = serverEp.Send(rsp)
            case sdoCCSUploadInitiate:
                var rsp canbus.Frame
                rsp.ID = COBID(FC_SDO_TX, node)
                rsp.Len = 8
                // SCS=2, e=1, s=1, n=1 (3 bytes data)
                rsp.Data[0] = byte(sdoSCSUploadInitiate<<5) | (1<<3) | (1<<2) | 0x01
                binary.LittleEndian.PutUint16(rsp.Data[1:3], 0x2000)
                rsp.Data[3] = 0x01
                copy(rsp.Data[4:], stored)
                _ = serverEp.Send(rsp)
            }
        }
    }()

    mux := canbus.NewMux(clientEp)
    defer mux.Close()
    c := NewSDOClient(clientEp, 0x22, mux, 0)
    if err := c.Download(0x2000, 0x01, []byte{0xAA, 0xBB}); err != nil {
        t.Fatalf("download: %v", err)
    }
    // We ignore what server stores; upload returns fixed stored bytes
    data, err := c.Upload(0x2000, 0x01)
    if err != nil { t.Fatalf("upload: %v", err) }
    if !bytes.Equal(data, stored) {
        t.Fatalf("upload mismatch: %x", data)
    }
}

func TestSDOSegmentedDownloadUpload(t *testing.T) {
    bus := canbus.NewLoopbackBus()
    clientEp := bus.Open()
    serverEp := bus.Open()
    defer clientEp.Close()
    defer serverEp.Close()

    // Test data > 4 bytes to force segmented
    writeData := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
    readData := []byte{0xCA, 0xFE, 0xBA, 0xBE, 0x01, 0x02, 0x03, 0x04, 0xAA, 0xBB, 0xCC, 0xDD}

    // Minimal segmented server
    go func() {
        var stored []byte
        for {
            f, err := serverEp.Receive()
            if err != nil { return }
            fc, node, err := ParseCOBID(f.ID)
            if err != nil || fc != FC_SDO_RX || node != 0x33 { continue }

            switch f.Data[0] >> 5 {
            case sdoCCSDownloadInitiate:
                // Respond to initiate (segmented)
                var rsp canbus.Frame
                rsp.ID = COBID(FC_SDO_TX, node)
                rsp.Len = 8
                rsp.Data[0] = byte(sdoSCSDownloadInitiate << 5)
                rsp.Data[1], rsp.Data[2], rsp.Data[3] = f.Data[1], f.Data[2], f.Data[3]
                _ = serverEp.Send(rsp)
                // Then handle segments until c=1
                toggle := byte(0)
                for {
                    seg, err := serverEp.Receive()
                    if err != nil { return }
                    if (seg.Data[0]>>5)&0x7 != sdoCCSDownloadSegment { continue }
                    t := (seg.Data[0] >> 4) & 0x1
                    if t != (toggle & 0x1) {
                        // Ignore unexpected toggles in test
                    }
                    cFlag := (seg.Data[0] & 0x1) != 0
                    n := int((seg.Data[0] >> 1) & 0x7)
                    end := 8
                    if cFlag { end = 8 - n }
                    stored = append(stored, seg.Data[1:end]...)
                    // Ack
                    var ack canbus.Frame
                    ack.ID = COBID(FC_SDO_TX, node)
                    ack.Len = 8
                    ack.Data[0] = byte(sdoSCSDownloadSegment<<5)
                    if t == 1 { ack.Data[0] |= 1 << 4 }
                    _ = serverEp.Send(ack)
                    if cFlag { break }
                    toggle ^= 1
                }
            case sdoCCSUploadInitiate:
                // Reply with segmented initiate, size indicated
                var rsp canbus.Frame
                rsp.ID = COBID(FC_SDO_TX, node)
                rsp.Len = 8
                rsp.Data[0] = byte(sdoSCSUploadInitiate << 5) | (1 << 2)
                binary.LittleEndian.PutUint16(rsp.Data[1:3], 0x3000)
                rsp.Data[3] = 0x02
                binary.LittleEndian.PutUint32(rsp.Data[4:8], uint32(len(readData)))
                _ = serverEp.Send(rsp)
                // Serve segments upon request
                sent := 0
                toggle := byte(0)
                for sent < len(readData) {
                    req, err := serverEp.Receive()
                    if err != nil { return }
                    if (req.Data[0]>>5)&0x7 != sdoCCSUploadSegment { continue }
                    t := (req.Data[0] >> 4) & 0x1
                    if t != (toggle & 0x1) {
                        // ignore
                    }
                    remain := len(readData) - sent
                    segLen := 7
                    if remain < segLen { segLen = remain }
                    last := segLen == remain
                    var seg canbus.Frame
                    seg.ID = COBID(FC_SDO_TX, node)
                    seg.Len = 8
                    seg.Data[0] = byte(sdoSCSUploadSegment << 5)
                    if t == 1 { seg.Data[0] |= 1 << 4 }
                    if last {
                        n := byte(7 - segLen)
                        seg.Data[0] |= 1 // c=1 last
                        seg.Data[0] |= (n & 0x7) << 1
                    }
                    copy(seg.Data[1:1+segLen], readData[sent:sent+segLen])
                    _ = serverEp.Send(seg)
                    sent += segLen
                    toggle ^= 1
                }
            }
        }
    }()

    mux := canbus.NewMux(clientEp)
    defer mux.Close()
    c := NewSDOClient(clientEp, 0x33, mux, time.Second)

    if err := c.Download(0x3000, 0x02, writeData); err != nil {
        t.Fatalf("segmented download: %v", err)
    }

    data, err := c.Upload(0x3000, 0x02)
    if err != nil { t.Fatalf("segmented upload: %v", err) }
    if !bytes.Equal(data, readData) {
        t.Fatalf("segmented upload mismatch: got % X want % X", data, readData)
    }
}

func TestSDOAsyncOverLoopback(t *testing.T) {
    lb := canbus.NewLoopbackBus()
    tx := lb.Open()
    rx := lb.Open()
    defer tx.Close()
    defer rx.Close()

    mux := canbus.NewMux(rx)
    defer mux.Close()

    // Server
    srv := lb.Open()
    defer srv.Close()
    go func() {
        for {
            f, err := srv.Receive()
            if err != nil { return }
            fc, node, err := ParseCOBID(f.ID)
            if err != nil || fc != FC_SDO_RX || node != 0x11 { continue }
            switch f.Data[0] >> 5 {
            case 1: // download
                var rsp canbus.Frame
                rsp.ID = COBID(FC_SDO_TX, node)
                rsp.Len = 8
                rsp.Data[0] = byte(3 << 5)
                rsp.Data[1], rsp.Data[2], rsp.Data[3] = f.Data[1], f.Data[2], f.Data[3]
                _ = srv.Send(rsp)
            case 2: // upload
                var rsp canbus.Frame
                rsp.ID = COBID(FC_SDO_TX, node)
                rsp.Len = 8
                rsp.Data[0] = byte(2<<5) | (1<<3) | (1<<2) | 0x01
                rsp.Data[1], rsp.Data[2], rsp.Data[3] = f.Data[1], f.Data[2], f.Data[3]
                rsp.Data[4], rsp.Data[5], rsp.Data[6] = 0xDE, 0xAD, 0xBE
                _ = srv.Send(rsp)
            }
        }
    }()

    client := NewSDOClient(tx, 0x11, mux, time.Second)

    // Issue download and ensure it completes
    if err := client.Download(0x2000, 0x01, []byte{0x01}); err != nil { t.Fatal(err) }

    // Concurrently subscribe to all frames to ensure we still receive others
    all, cancelAll := mux.Subscribe(nil, 8)
    defer cancelAll()

    // Issue upload and ensure data is received and not blocked
    data, err := client.Upload(0x2000, 0x01)
    if err != nil { t.Fatal(err) }
    if got := fmt.Sprintf("% X", data); got != "DE AD BE" { t.Fatalf("unexpected data: %s", got) }

    // Ensure that the general subscriber saw at least one frame, demonstrating fan-out
    select {
    case <-all:
    case <-time.After(500 * time.Millisecond):
        t.Fatal("mux did not fan out frames to general subscriber")
    }
}

