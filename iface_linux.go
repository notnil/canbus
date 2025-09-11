//go:build linux

package canbus

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"
)

// Linux network interface helpers (no external deps).
// These functions toggle the IFF_UP flag via ioctl on a SOCK_DGRAM socket.
//
// Notes:
// - Bringing interfaces up/down requires CAP_NET_ADMIN. When run without
//   sufficient privileges they will return EPERM.
// - See README for guidance on granting capabilities to an unprivileged binary.

const (
	ifNameSize    = 16      // IFNAMSIZ
	siocGIFFlags  = 0x8913  // SIOCGIFFLAGS
	siocSIFFlags  = 0x8914  // SIOCSIFFLAGS
	iffUp         = 0x1     // IFF_UP
)

// ifreqFlags mirrors the layout of struct ifreq for flag operations on Linux.
// sizeof(struct ifreq) = 40 on most 64-bit Linux: 16 (name) + 24 (union).
// For the flags variant, the union begins with a 2-byte short followed by pad.
type ifreqFlags struct {
	Name  [ifNameSize]byte
	Flags uint16
	pad   [22]byte
}

func getInterfaceFlags(name string) (uint16, error) {
	if len(name) == 0 || len(name) >= ifNameSize {
		return 0, fmt.Errorf("canbus: invalid interface name %q", name)
	}
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return 0, err
	}
	defer syscall.Close(fd)
	var ifr ifreqFlags
	copy(ifr.Name[:], name)
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(siocGIFFlags), uintptr(unsafe.Pointer(&ifr)))
	if errno != 0 {
		return 0, errno
	}
	return ifr.Flags, nil
}

func setInterfaceFlags(name string, flags uint16) error {
	if len(name) == 0 || len(name) >= ifNameSize {
		return fmt.Errorf("canbus: invalid interface name %q", name)
	}
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return err
	}
	defer syscall.Close(fd)
	var ifr ifreqFlags
	copy(ifr.Name[:], name)
	ifr.Flags = flags
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(siocSIFFlags), uintptr(unsafe.Pointer(&ifr)))
	if errno != 0 {
		return errno
	}
	return nil
}

// IsInterfaceUp returns true if the Linux network interface has IFF_UP set.
func IsInterfaceUp(name string) (bool, error) {
	flags, err := getInterfaceFlags(name)
	if err != nil {
		return false, err
	}
	return (flags & iffUp) != 0, nil
}

// SetInterfaceUp sets IFF_UP on the given interface. Requires CAP_NET_ADMIN.
func SetInterfaceUp(name string) error {
	flags, err := getInterfaceFlags(name)
	if err != nil {
		return err
	}
	if (flags & iffUp) != 0 {
		return nil
	}
	return setInterfaceFlags(name, flags|iffUp)
}

// SetInterfaceDown clears IFF_UP on the given interface. Requires CAP_NET_ADMIN.
func SetInterfaceDown(name string) error {
	flags, err := getInterfaceFlags(name)
	if err != nil {
		return err
	}
	if (flags & iffUp) == 0 {
		return nil
	}
	return setInterfaceFlags(name, flags &^ iffUp)
}

// RequireRootOrCapNetAdmin can be used to map EPERM to a clearer error message.
// It returns a wrapped error advising to grant CAP_NET_ADMIN to the binary.
func RequireRootOrCapNetAdmin(err error) error {
	if errors.Is(err, syscall.EPERM) {
		return fmt.Errorf("operation requires CAP_NET_ADMIN (or root): %w", err)
	}
	return err
}

// LinuxCANInterfaceOptions controls common CAN interface parameters through the system `ip` tool.
//
// Notes:
// - Changing bitrate/restart-ms typically requires the interface to be DOWN.
//   You can call SetInterfaceDown(name) first and bring it back up after configuring.
// - These operations require CAP_NET_ADMIN.
type LinuxCANInterfaceOptions struct {
	// Bitrate sets the arbitration bit-rate in bits per second (e.g., 125000, 500000, 1000000).
	// If nil, bitrate is left unchanged.
	Bitrate *uint32

	// RestartMs sets automatic bus-off recovery delay in milliseconds.
	// If nil, restart-ms is left unchanged. Set to 0 to disable auto-restart.
	RestartMs *uint32

	// TxQueueLen sets the transmit queue length (number of packets).
	// If nil, txqueuelen is left unchanged.
	TxQueueLen *int
}

// ConfigureLinuxCANInterface applies the provided options to a Linux CAN network interface
// by invoking the system `ip` command (iproute2). Only the non-nil fields are applied.
// Requires CAP_NET_ADMIN (or root). Errors are wrapped with guidance when permissions are insufficient.
func ConfigureLinuxCANInterface(name string, opts LinuxCANInterfaceOptions) error {
	if len(name) == 0 || len(name) >= ifNameSize {
		return fmt.Errorf("canbus: invalid interface name %q", name)
	}

	// 1) Apply txqueuelen if requested (can be changed while interface is up on most drivers)
	if opts.TxQueueLen != nil {
		cmd := exec.Command("ip", "link", "set", "dev", name, "txqueuelen", fmt.Sprintf("%d", *opts.TxQueueLen))
		if out, err := cmd.CombinedOutput(); err != nil {
			return RequireRootOrCapNetAdmin(fmt.Errorf("ip link set txqueuelen failed: %w; output: %s", err, string(out)))
		}
	}

	// 2) Apply CAN-specific settings (bitrate, restart-ms) together if any provided
	if opts.Bitrate != nil || opts.RestartMs != nil {
		args := []string{"link", "set", "dev", name, "type", "can"}
		if opts.Bitrate != nil {
			args = append(args, "bitrate", fmt.Sprintf("%d", *opts.Bitrate))
		}
		if opts.RestartMs != nil {
			args = append(args, "restart-ms", fmt.Sprintf("%d", *opts.RestartMs))
		}
		cmd := exec.Command("ip", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return RequireRootOrCapNetAdmin(fmt.Errorf("ip link set type can failed: %w; output: %s", err, string(out)))
		}
	}
	return nil
}

