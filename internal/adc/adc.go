// Package adc implements the MCP3xxx conversion protocol over an SPI
// connection. Framing and limits follow the Microchip datasheets
// (DS20001295E, DS21298E) and the Linux mcp320x IIO driver; see
// docs/research.md for the verified sources.
package adc

import (
	"fmt"
	"sort"
	"sync"
)

// ChipSpec describes one supported MCP3xxx variant.
type ChipSpec struct {
	Name           string
	Bits           int // result width; LSB = VREF / 2^Bits
	Channels       int
	DefaultSpeedHz int // conservative 2.7V-derived default clock
	MaxSpeedHz     int // datasheet ceiling at 5V supply
}

// MinSpeedHz is the practical clock floor for the whole family: below
// ~10 kHz the sample-and-hold capacitor droops during conversion.
const MinSpeedHz = 10_000

// chips holds the parts whose framing was verified in docs/research.md.
// MCP3002/3202 use a different command-byte layout and are not yet
// supported.
var chips = map[string]ChipSpec{
	"mcp3004": {Name: "mcp3004", Bits: 10, Channels: 4, DefaultSpeedHz: 1_350_000, MaxSpeedHz: 3_600_000},
	"mcp3008": {Name: "mcp3008", Bits: 10, Channels: 8, DefaultSpeedHz: 1_350_000, MaxSpeedHz: 3_600_000},
	"mcp3204": {Name: "mcp3204", Bits: 12, Channels: 4, DefaultSpeedHz: 1_000_000, MaxSpeedHz: 2_000_000},
	"mcp3208": {Name: "mcp3208", Bits: 12, Channels: 8, DefaultSpeedHz: 1_000_000, MaxSpeedHz: 2_000_000},
}

// Lookup returns the spec for a chip name, or an error naming the
// supported set.
func Lookup(name string) (ChipSpec, error) {
	if spec, ok := chips[name]; ok {
		return spec, nil
	}
	names := make([]string, 0, len(chips))
	for n := range chips {
		names = append(names, n)
	}
	sort.Strings(names)
	return ChipSpec{}, fmt.Errorf("unsupported chip %q (supported: %v)", name, names)
}

// Conn is the SPI transport the device reads through.
type Conn interface {
	Tx(w, r []byte) error
	Close() error
}

// Device is one ADC chip on an SPI bus.
type Device struct {
	Spec ChipSpec
	VRef float64

	mu   sync.Mutex
	conn Conn
}

// NewDevice wraps an open SPI connection.
func NewDevice(conn Conn, spec ChipSpec, vref float64) *Device {
	return &Device{Spec: spec, VRef: vref, conn: conn}
}

// frame builds the 3-byte transaction for one conversion.
//
// 10-bit (MCP3004/3008): [0x01, (SGL<<3|ch)<<4, 0x00] — start bit is
// the LSB of byte 1; byte 2 carries SGL/DIFF and D2..D0 in its high
// nibble.
//
// 12-bit (MCP3204/3208): [0b00000-start-SGL-D2, D1D0<<6, 0x00] — the
// wider result pushes the command two bits earlier.
func frame(spec ChipSpec, channel int, differential bool) [3]byte {
	sgl := byte(1)
	if differential {
		sgl = 0
	}
	switch spec.Bits {
	case 10:
		return [3]byte{0x01, (sgl<<3 | byte(channel)) << 4, 0x00}
	case 12:
		return [3]byte{0x04 | sgl<<1 | byte(channel>>2), byte(channel&0x03) << 6, 0x00}
	}
	panic(fmt.Sprintf("adc: no framing for %d-bit chip", spec.Bits))
}

// decode extracts the conversion result: the null bit precedes the
// data, leaving the top result bits in the low bits of byte 2 and the
// low 8 bits in byte 3.
func decode(spec ChipSpec, rx [3]byte) int {
	mask := byte(1<<(spec.Bits-8) - 1)
	return int(rx[1]&mask)<<8 | int(rx[2])
}

// ReadRaw performs one conversion and returns the straight-binary code
// (0 .. 2^Bits-1). In differential mode, channel selects the
// pseudo-differential pair configuration (datasheet Table 5-3).
func (d *Device) ReadRaw(channel int, differential bool) (int, error) {
	if channel < 0 || channel >= d.Spec.Channels {
		return 0, fmt.Errorf("%s: channel %d out of range 0..%d", d.Spec.Name, channel, d.Spec.Channels-1)
	}
	tx := frame(d.Spec, channel, differential)
	var rx [3]byte
	d.mu.Lock()
	err := d.conn.Tx(tx[:], rx[:])
	d.mu.Unlock()
	if err != nil {
		return 0, err
	}
	return decode(d.Spec, rx), nil
}

// Volts converts a raw code to the input voltage: raw * VREF / 2^Bits.
func (d *Device) Volts(raw int) float64 {
	return float64(raw) * d.VRef / float64(int(1)<<d.Spec.Bits)
}

// Close releases the SPI connection.
func (d *Device) Close() error {
	return d.conn.Close()
}
