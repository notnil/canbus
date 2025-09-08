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
    f, err := SDOExpeditedDownload(0x23, 0x2000, 0x01, data)
    if err != nil { t.Fatal(err) }
    node, idx, sub, got, err := parseSDOExpeditedDownload(f)
    if err != nil { t.Fatal(err) }
    if node != 0x23 || idx != 0x2000 || sub != 0x01 || !bytes.Equal(got, data) {
        t.Fatalf("sdo parse mismatch: node=%d idx=0x%X sub=%d data=%x", node, idx, sub, got)
    }

    req, err := SDOExpeditedUploadRequest(0x23, 0x1018, 0x00)
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

    c := NewSDOClient(clientEp, 0x22, nil, 0)
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

