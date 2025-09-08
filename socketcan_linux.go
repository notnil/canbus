//go:build linux

package canbus

import (
	"errors"
	"net"
	"os"
	"syscall"
	"unsafe"
)

// socketCAN implements Bus over Linux SocketCAN using raw syscalls only.
type socketCAN struct {
	fd     int
	file   *os.File
	closed chan struct{}
}

// DialSocketCAN opens a raw CAN socket bound to the given interface name (e.g., "can0").
func DialSocketCAN(iface string) (Bus, error) {
	// Create socket: AF_CAN, SOCK_RAW, CAN_RAW (protocol 1)
	const AF_CAN = 29
	const CAN_RAW = 1
	fd, err := syscall.Socket(AF_CAN, syscall.SOCK_RAW, CAN_RAW)
	if err != nil {
		return nil, err
	}

	// Query interface index via net.InterfaceByName
	netIf, err := net.InterfaceByName(iface)
	if err != nil {
		syscall.Close(fd)
		return nil, err
	}

	// Bind to interface
	// struct sockaddr_can { sa_family_t can_family; int can_ifindex; union { ... } addr; };
	// We provide a compatible memory layout via unsafe and call bind(2) directly.
	type sockaddrCAN struct {
		Family  uint16
		_pad    uint16
		Ifindex int32
		Addr    [8]byte
	}
	sa := sockaddrCAN{Family: AF_CAN, Ifindex: int32(netIf.Index)}
	_, _, e := syscall.Syscall(syscall.SYS_BIND, uintptr(fd), uintptr(unsafe.Pointer(&sa)), unsafe.Sizeof(sa))
	if e != 0 {
		syscall.Close(fd)
		return nil, e
	}

	// Set non-blocking mode for context-aware operations
	if err := syscall.SetNonblock(fd, true); err != nil {
		syscall.Close(fd)
		return nil, err
	}

	f := os.NewFile(uintptr(fd), "socketcan")
	return &socketCAN{fd: fd, file: f, closed: make(chan struct{})}, nil
}

func (s *socketCAN) Close() error {
	select {
	case <-s.closed:
		return nil
	default:
	}
	close(s.closed)
	// Closing file also closes the fd
	return s.file.Close()
}

// Send writes one frame using the Linux can_frame binary layout.
func (s *socketCAN) Send(frame Frame) error {
	if err := frame.Validate(); err != nil {
		return err
	}
	buf, err := frame.MarshalBinary()
	if err != nil {
		return err
	}
	for {
		// Try write
		n, werr := syscall.Write(s.fd, buf)
		if werr == nil {
			if n != len(buf) {
				return errors.New("canbus: short write")
			}
			return nil
		}
		if werr == syscall.EAGAIN || werr == syscall.EWOULDBLOCK {
			// Busy-wait with small yield
			syscall.Select(0, nil, nil, nil, &syscall.Timeval{Usec: 1000})
			continue
		}
		return werr
	}
}

// Receive reads one frame (blocking respecting context).
func (s *socketCAN) Receive() (Frame, error) {
	var f Frame
	buf := make([]byte, 16)
	for {
		n, rerr := syscall.Read(s.fd, buf)
		if rerr == nil {
			if n != len(buf) {
				return Frame{}, errors.New("canbus: short read")
			}
			if err := f.UnmarshalBinary(buf); err != nil {
				return Frame{}, err
			}
			return f, nil
		}
		if rerr == syscall.EAGAIN || rerr == syscall.EWOULDBLOCK {
			syscall.Select(0, nil, nil, nil, &syscall.Timeval{Usec: 1000})
			continue
		}
		return Frame{}, rerr
	}
}

// Helpers for FD sets since x/sys is not allowed.
func fdSetAdd(set *syscall.FdSet, fd int) {
	set.Bits[fd/64] |= int64(1) << (uint(fd) % 64)
}

