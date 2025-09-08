//go:build linux

package canbus

import (
    "context"
    "errors"
    "net"
    "os"
    "syscall"
    "time"
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
func (s *socketCAN) Send(ctx context.Context, frame Frame) error {
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
            // Wait for fd to be writable or ctx canceled
            if err := s.waitWritable(ctx); err != nil {
                return err
            }
            continue
        }
        return werr
    }
}

// Receive reads one frame (blocking respecting context).
func (s *socketCAN) Receive(ctx context.Context) (Frame, error) {
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
            if err := s.waitReadable(ctx); err != nil {
                return Frame{}, err
            }
            continue
        }
        return Frame{}, rerr
    }
}

func (s *socketCAN) waitReadable(ctx context.Context) error {
    return s.wait(ctx, true, false)
}

func (s *socketCAN) waitWritable(ctx context.Context) error {
    return s.wait(ctx, false, true)
}

func (s *socketCAN) wait(ctx context.Context, r, w bool) error {
    // Use ppoll via Select with timeout from context. Fallback to small sleeps for simplicity.
    for {
        // Determine timeout from context
        var timeout *syscall.Timeval
        if deadline, ok := ctx.Deadline(); ok {
            d := time.Until(deadline)
            if d <= 0 {
                return ctx.Err()
            }
            timeout = &syscall.Timeval{Sec: int64(d / time.Second), Usec: int64((d % time.Second) / time.Microsecond)}
        } else {
            // Small backoff to avoid busy loop if no deadline
            timeout = &syscall.Timeval{Sec: 0, Usec: 50_000}
        }

        var readfds, writefds syscall.FdSet
        if r {
            fdSetAdd(&readfds, s.fd)
        }
        if w {
            fdSetAdd(&writefds, s.fd)
        }
        // nfds is highest fd + 1
        nfds := s.fd + 1
        _, err := syscall.Select(nfds, &readfds, &writefds, nil, timeout)
        if err == nil {
            // Ready (or timeout without error). If timeout and not ready, check context.
            select {
            case <-ctx.Done():
                return ctx.Err()
            default:
                return nil
            }
        }
        if err == syscall.EINTR {
            // retry
            continue
        }
        return err
    }
}

// Helpers for FD sets since x/sys is not allowed.
func fdSetAdd(set *syscall.FdSet, fd int) {
    set.Bits[fd/64] |= int64(1) << (uint(fd) % 64)
}

