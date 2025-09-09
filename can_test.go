package canbus

import (
	"bytes"
	"fmt"
	"testing"
	"time"
)

func TestFrame_Validate_Marshal_Unmarshal_String(t *testing.T) {
	cases := []struct {
		name     string
		frame    Frame
		wantStr  string
		wantErr  error
	}{
		{
			name:    "standard frame with data",
			frame:   MustFrame(0x123, []byte{0xDE, 0xAD}),
			wantStr: "123 [2] DE AD",
			wantErr: nil,
		},
		{
			name:    "extended RTR, zero length",
			frame:   Frame{ID: 0x1ABCDEFF, Extended: true, RTR: true, Len: 0},
			wantStr: "1ABCDEFF [0] RTR",
			wantErr: nil,
		},
	}

	for _, tc := range cases {
		// Validate
		if got := tc.frame.Validate(); got != tc.wantErr {
			t.Fatalf("%s: Validate() error = %v, want %v", tc.name, got, tc.wantErr)
		}
		// Marshal/Unmarshal roundtrip
		b, err := tc.frame.MarshalBinary()
		if err != nil {
			t.Fatalf("%s: MarshalBinary() error = %v", tc.name, err)
		}
		var g Frame
		if err := g.UnmarshalBinary(b); err != nil {
			t.Fatalf("%s: UnmarshalBinary() error = %v", tc.name, err)
		}
		if g != tc.frame {
			t.Fatalf("%s: roundtrip mismatch: got %+v want %+v", tc.name, g, tc.frame)
		}
		// String
		if got := g.String(); got != tc.wantStr {
			t.Fatalf("%s: String() = %q, want %q", tc.name, got, tc.wantStr)
		}
	}

	// Invalid cases
	{
		f := Frame{ID: 0x800, Len: 0} // standard, out of range
		if err := f.Validate(); err == nil {
			t.Fatalf("expected invalid standard ID")
		}
	}
	{
		f := Frame{ID: 0x20000000, Extended: true} // extended, out of range
		if err := f.Validate(); err == nil {
			t.Fatalf("expected invalid extended ID")
		}
	}
	{
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("MustFrame should panic for len>8")
			}
		}()
		_ = MustFrame(0x123, make([]byte, 9))
	}
}

func TestLoopbackBus_SendReceive_MultiEndpoint(t *testing.T) {
	bus := NewLoopbackBus()
	defer bus.Close()

	a := bus.Open()
	b := bus.Open()
	c := bus.Open()
	defer a.Close()
	defer b.Close()
	defer c.Close()

	send := MustFrame(0x321, []byte("hello"))

	done := make(chan error, 1)
	go func() { done <- a.Send(send) }()

	gotB, err := b.Receive()
	if err != nil {
		t.Fatalf("receive b: %v", err)
	}
	gotC, err := c.Receive()
	if err != nil {
		t.Fatalf("receive c: %v", err)
	}
	if gotB.ID != send.ID || gotB.Len != send.Len || !bytes.Equal(gotB.Data[:gotB.Len], send.Data[:send.Len]) {
		t.Fatalf("b mismatch: got %+v want %+v", gotB, send)
	}
	if gotC.ID != send.ID || gotC.Len != send.Len || !bytes.Equal(gotC.Data[:gotC.Len], send.Data[:send.Len]) {
		t.Fatalf("c mismatch: got %+v want %+v", gotC, send)
	}
	if err := <-done; err != nil {
		t.Fatalf("send: %v", err)
	}
	if gotB.String() != "321 [5] 68 65 6C 6C 6F" {
		t.Fatalf("string: got %q", gotB.String())
	}
}

func TestLoopbackBus_CloseBehavior(t *testing.T) {
	bus := NewLoopbackBus()
	a := bus.Open()
	b := bus.Open()

	// Close endpoint and ensure it errors
	_ = a.Close()
	if _, err := a.Receive(); err == nil {
		t.Fatalf("closed endpoint should error on Receive")
	}
	if err := a.Send(MustFrame(0x1, nil)); err == nil {
		t.Fatalf("closed endpoint should error on Send")
	}

	// Close bus and ensure other endpoint errors after close
	_ = bus.Close()
	if _, err := b.Receive(); err == nil {
		t.Fatalf("endpoint should error after bus close")
	}
	if err := b.Send(MustFrame(0x1, nil)); err == nil {
		t.Fatalf("endpoint should error on Send after bus close")
	}
}

func TestFilters_Basics(t *testing.T) {
	f1 := MustFrame(0x100, []byte{1})
	f2 := MustFrame(0x101, []byte{2})
	f3 := Frame{ID: 0x1ABCDEFF, Extended: true, Len: 0}

	if !ByID(0x100)(f1) || ByID(0x100)(f2) {
		t.Fatalf("ByID failure")
	}
	if !(ByIDs(0x100, 0x102)(f1)) || ByIDs(0x100, 0x102)(f2) {
		t.Fatalf("ByIDs failure")
	}
	if !ByRange(0x100, 0x1FF)(f2) || ByRange(0x200, 0x2FF)(f2) {
		t.Fatalf("ByRange failure")
	}
	// Use a mask that distinguishes 0x100 from 0x101 (all 11 std bits)
	if !ByMask(0x100, 0x7FF)(f1) || ByMask(0x100, 0x7FF)(f2) {
		t.Fatalf("ByMask failure")
	}
	if !StandardOnly()(f1) || StandardOnly()(f3) {
		t.Fatalf("StandardOnly failure")
	}
	if !ExtendedOnly()(f3) || ExtendedOnly()(f1) {
		t.Fatalf("ExtendedOnly failure")
	}
	data := f1
	data.RTR = false
	if !DataOnly()(data) {
		t.Fatalf("DataOnly failure")
	}
	rtr := f1
	rtr.RTR = true
	if !RTROnly()(rtr) {
		t.Fatalf("RTROnly failure")
	}
	if !And(ByID(0x100), DataOnly())(data) || And(ByID(0x100), DataOnly())(rtr) {
		t.Fatalf("And failure")
	}
	if !Or(ByID(0x100), ByID(0x999))(f1) || Or(ByID(0x999), ByID(0x998))(f1) {
		t.Fatalf("Or failure")
	}
	if Not(ByID(0x100))(f1) || !Not(ByID(0x999))(f1) {
		t.Fatalf("Not failure")
	}
}

func TestMux_Subscribe_Filtering_And_Close(t *testing.T) {
	bus := NewLoopbackBus()
	defer bus.Close()
	m := NewMux(bus.Open())
	defer m.Close()

	// Two subscribers with different filters
	chA, cancelA := m.Subscribe(ByID(0x100), 1)
	chB, cancelB := m.Subscribe(ByRange(0x200, 0x2FF), 2)
	defer cancelB()

	producer := bus.Open()
	defer producer.Close()

	send := func(id uint32) { _ = producer.Send(MustFrame(id, []byte{1, 2, 3})) }

	send(0x100) // should go to A
	send(0x210) // should go to B
	send(0x105) // should go to no one

	// Receive with small timeouts to avoid flakiness
	select {
	case f := <-chA:
		if f.ID != 0x100 { t.Fatalf("A got %03X", f.ID) }
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timeout waiting for A")
	}
	select {
	case f := <-chB:
		if f.ID != 0x210 { t.Fatalf("B got %03X", f.ID) }
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timeout waiting for B")
	}
	select {
	case f := <-chA:
		t.Fatalf("A should be empty, got %03X", f.ID)
	case <-time.After(100 * time.Millisecond):
	}
	select {
	case f := <-chB:
		t.Fatalf("B should be empty, got %03X", f.ID)
	case <-time.After(100 * time.Millisecond):
	}

	// Cancel subscriber A; verify channel is closed (non-blocking read yields ok=false)
	cancelA()
	select {
	case _, ok := <-chA:
		if ok { t.Fatalf("A should be closed") }
	default:
		// If not yet observed closed, that's fine; it must not receive further frames
	}

	// Further frames for A's filter should not be delivered; channel remains closed
	send(0x100)
	select {
	case _, ok := <-chA:
		if ok { t.Fatalf("A should remain closed") }
	case <-time.After(100 * time.Millisecond):
	}

	// Close mux and ensure channels close
	_ = m.Close()
	_, okB := <-chB
	if okB {
		t.Fatalf("B should be closed after mux close")
	}
}

func ExampleLoopbackBus() {
	bus := NewLoopbackBus()
	a := bus.Open()
	b := bus.Open()
	defer a.Close()
	defer b.Close()

	go func() { _ = a.Send(MustFrame(0x123, []byte("hi"))) }()
	f, _ := b.Receive()
	fmt.Printf("ID=%03X LEN=%d DATA=%x\n", f.ID, f.Len, f.Data[:f.Len])
	// Output: ID=123 LEN=2 DATA=6869
}
