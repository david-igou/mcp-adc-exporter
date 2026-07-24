package adc

import "testing"

type fakeConn struct {
	lastTx []byte
	rx     [3]byte
}

func (f *fakeConn) Tx(w, r []byte) error {
	f.lastTx = append([]byte(nil), w...)
	copy(r, f.rx[:])
	return nil
}

func (f *fakeConn) Close() error { return nil }

// Vectors from the verified sources in docs/research.md: the canonical
// spidev pattern [1, (8+ch)<<4, 0] for MCP3008 and the classic 12-bit
// framing for MCP3208.
func TestFrame(t *testing.T) {
	tests := []struct {
		chip         string
		channel      int
		differential bool
		want         [3]byte
	}{
		{"mcp3008", 0, false, [3]byte{0x01, 0x80, 0x00}},
		{"mcp3008", 3, false, [3]byte{0x01, 0xB0, 0x00}},
		{"mcp3008", 7, false, [3]byte{0x01, 0xF0, 0x00}},
		{"mcp3008", 0, true, [3]byte{0x01, 0x00, 0x00}},
		{"mcp3004", 2, false, [3]byte{0x01, 0xA0, 0x00}},
		{"mcp3208", 0, false, [3]byte{0x06, 0x00, 0x00}},
		{"mcp3208", 5, false, [3]byte{0x07, 0x40, 0x00}},
		{"mcp3208", 5, true, [3]byte{0x05, 0x40, 0x00}},
		{"mcp3204", 3, false, [3]byte{0x06, 0xC0, 0x00}},
	}
	for _, tt := range tests {
		spec, err := Lookup(tt.chip)
		if err != nil {
			t.Fatal(err)
		}
		if got := frame(spec, tt.channel, tt.differential); got != tt.want {
			t.Errorf("frame(%s, ch%d, diff=%v) = %#v, want %#v", tt.chip, tt.channel, tt.differential, got, tt.want)
		}
	}
}

func TestDecode(t *testing.T) {
	tests := []struct {
		chip string
		rx   [3]byte
		want int
	}{
		{"mcp3008", [3]byte{0xFF, 0xFF, 0xFF}, 1023}, // junk bits above the null bit are masked
		{"mcp3008", [3]byte{0x00, 0x02, 0x8F}, 655},
		{"mcp3008", [3]byte{0x00, 0x00, 0x00}, 0},
		{"mcp3208", [3]byte{0xFF, 0xFF, 0xFF}, 4095},
		{"mcp3208", [3]byte{0x00, 0x08, 0x00}, 2048},
	}
	for _, tt := range tests {
		spec, err := Lookup(tt.chip)
		if err != nil {
			t.Fatal(err)
		}
		if got := decode(spec, tt.rx); got != tt.want {
			t.Errorf("decode(%s, %#v) = %d, want %d", tt.chip, tt.rx, got, tt.want)
		}
	}
}

func TestReadRawAndVolts(t *testing.T) {
	spec, _ := Lookup("mcp3008")
	fake := &fakeConn{rx: [3]byte{0x00, 0x02, 0x00}} // code 512 = half scale
	dev := NewDevice(fake, spec, 3.3)

	raw, err := dev.ReadRaw(3, false)
	if err != nil {
		t.Fatal(err)
	}
	if raw != 512 {
		t.Errorf("raw = %d, want 512", raw)
	}
	if want := []byte{0x01, 0xB0, 0x00}; string(fake.lastTx) != string(want) {
		t.Errorf("tx = %#v, want %#v", fake.lastTx, want)
	}
	if v := dev.Volts(raw); v != 1.65 {
		t.Errorf("volts = %v, want 1.65", v)
	}

	if _, err := dev.ReadRaw(8, false); err == nil {
		t.Error("expected out-of-range error for channel 8")
	}
}

func TestLookupUnknown(t *testing.T) {
	if _, err := Lookup("mcp3202"); err == nil {
		t.Error("expected error for unsupported mcp3202")
	}
}
