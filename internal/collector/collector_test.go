package collector

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/david-igou/mcp-adc-exporter/internal/adc"
	"github.com/david-igou/mcp-adc-exporter/internal/config"
)

type fakeConn struct{ rx [3]byte }

func (f *fakeConn) Tx(w, r []byte) error {
	copy(r, f.rx[:])
	return nil
}

func (f *fakeConn) Close() error { return nil }

func testConfig() *config.Config {
	return &config.Config{
		Devices: []config.Device{{
			Name:   "adc0",
			Chip:   "mcp3008",
			SPIDev: "/dev/spidev0.0",
			VRef:   3.3,
			Channels: []config.Channel{
				{Index: 3, Name: "battery", Mode: "single", Scale: 2.0, Offset: 0.5},
			},
		}},
	}
}

func TestCollect(t *testing.T) {
	// code 512 = half scale = 1.65 V at vref 3.3
	open := func(path string, speedHz uint32) (adc.Conn, error) {
		if speedHz != 1_350_000 {
			t.Errorf("default speed = %d, want 1350000", speedHz)
		}
		return &fakeConn{rx: [3]byte{0x00, 0x02, 0x00}}, nil
	}
	c, err := New(testConfig(), open)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	want := `
# HELP adc_channel_raw_code Raw straight-binary conversion code (0 .. 2^resolution-1).
# TYPE adc_channel_raw_code gauge
adc_channel_raw_code{channel="3",chip="mcp3008",device="adc0",name="battery"} 512
# HELP adc_channel_value Channel voltage with the configured scale/offset applied; unit depends on configuration.
# TYPE adc_channel_value gauge
adc_channel_value{channel="3",chip="mcp3008",device="adc0",name="battery"} 3.8
# HELP adc_channel_volts Voltage at an MCP3xxx ADC input channel.
# TYPE adc_channel_volts gauge
adc_channel_volts{channel="3",chip="mcp3008",device="adc0",name="battery"} 1.65
`
	if err := testutil.CollectAndCompare(c, strings.NewReader(want),
		"adc_channel_volts", "adc_channel_raw_code", "adc_channel_value"); err != nil {
		t.Error(err)
	}
}

func TestNewRejectsBadConfig(t *testing.T) {
	open := func(path string, speedHz uint32) (adc.Conn, error) { return &fakeConn{}, nil }

	cfg := testConfig()
	cfg.Devices[0].Chip = "mcp9999"
	if _, err := New(cfg, open); err == nil {
		t.Error("expected error for unknown chip")
	}

	cfg = testConfig()
	cfg.Devices[0].Channels[0].Index = 9
	if _, err := New(cfg, open); err == nil {
		t.Error("expected error for out-of-range channel")
	}

	cfg = testConfig()
	cfg.Devices[0].SpeedHz = 5_000_000
	if _, err := New(cfg, open); err == nil {
		t.Error("expected error for over-spec SPI clock")
	}
}
