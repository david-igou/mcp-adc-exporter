//go:build linux

// Package spi is a minimal Linux spidev client: open a /dev/spidevX.Y
// device, set mode/speed, and run full-duplex transfers via
// SPI_IOC_MESSAGE. It avoids a hardware-abstraction dependency because
// the exporter only ever needs Tx().
package spi

import (
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

// spidev ioctl request numbers from <linux/spi/spidev.h>.
const (
	iocWrMode        = 0x40016b01 // SPI_IOC_WR_MODE
	iocWrBitsPerWord = 0x40016b03 // SPI_IOC_WR_BITS_PER_WORD
	iocWrMaxSpeedHz  = 0x40046b04 // SPI_IOC_WR_MAX_SPEED_HZ
	iocMessage1      = 0x40206b00 // SPI_IOC_MESSAGE(1)
)

// iocTransfer mirrors struct spi_ioc_transfer (32 bytes).
type iocTransfer struct {
	txBuf       uint64
	rxBuf       uint64
	length      uint32
	speedHz     uint32
	delayUsecs  uint16
	bitsPerWord uint8
	csChange    uint8
	txNbits     uint8
	rxNbits     uint8
	wordDelay   uint8
	pad         uint8
}

// Conn is an open spidev device configured for one chip.
type Conn struct {
	f       *os.File
	speedHz uint32
}

// Open opens path (e.g. /dev/spidev0.0) and configures SPI mode 0,
// 8 bits per word, and the given clock speed.
func Open(path string, speedHz uint32) (*Conn, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	c := &Conn{f: f, speedHz: speedHz}
	mode := uint8(0) // MCP3xxx support modes 0,0 and 1,1; use 0,0
	bits := uint8(8)
	if err := c.ioctl(iocWrMode, unsafe.Pointer(&mode)); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("%s: set mode: %w", path, err)
	}
	if err := c.ioctl(iocWrBitsPerWord, unsafe.Pointer(&bits)); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("%s: set bits per word: %w", path, err)
	}
	if err := c.ioctl(iocWrMaxSpeedHz, unsafe.Pointer(&speedHz)); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("%s: set speed: %w", path, err)
	}
	return c, nil
}

// Tx performs one full-duplex transfer: len(w) bytes are clocked out
// while len(r) bytes are clocked in, with CS asserted for the whole
// transaction.
func (c *Conn) Tx(w, r []byte) error {
	if len(w) != len(r) || len(w) == 0 {
		return fmt.Errorf("spi: tx/rx buffers must be equal and non-empty (%d/%d)", len(w), len(r))
	}
	tr := iocTransfer{
		txBuf:       uint64(uintptr(unsafe.Pointer(&w[0]))),
		rxBuf:       uint64(uintptr(unsafe.Pointer(&r[0]))),
		length:      uint32(len(w)),
		speedHz:     c.speedHz,
		bitsPerWord: 8,
	}
	err := c.ioctl(iocMessage1, unsafe.Pointer(&tr))
	runtime.KeepAlive(w)
	runtime.KeepAlive(r)
	return err
}

// Close closes the underlying device.
func (c *Conn) Close() error {
	return c.f.Close()
}

func (c *Conn) ioctl(req uintptr, arg unsafe.Pointer) error {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, c.f.Fd(), req, uintptr(arg))
	if errno != 0 {
		return errno
	}
	return nil
}
