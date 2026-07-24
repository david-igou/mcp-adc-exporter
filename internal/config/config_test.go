package config

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad(t *testing.T) {
	cfg, err := Load(write(t, `
devices:
  - name: adc0
    chip: mcp3008
    spidev: /dev/spidev0.0
    vref: 3.3
    channels:
      - index: 0
        name: battery
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != ":9958" {
		t.Errorf("default listen = %q, want :9958", cfg.Listen)
	}
	ch := cfg.Devices[0].Channels[0]
	if ch.Mode != "single" || ch.Scale != 1.0 {
		t.Errorf("channel defaults = mode %q scale %v, want single 1.0", ch.Mode, ch.Scale)
	}
}

func TestLoadErrors(t *testing.T) {
	cases := map[string]string{
		"no devices":     `listen: ":9958"`,
		"missing vref":   "devices:\n  - name: a\n    spidev: /dev/spidev0.0\n    channels:\n      - {index: 0, name: x}",
		"missing spidev": "devices:\n  - name: a\n    vref: 3.3\n    channels:\n      - {index: 0, name: x}",
		"bad mode":       "devices:\n  - name: a\n    spidev: /dev/spidev0.0\n    vref: 3.3\n    channels:\n      - {index: 0, name: x, mode: bogus}",
		"dup channel":    "devices:\n  - name: a\n    spidev: /dev/spidev0.0\n    vref: 3.3\n    channels:\n      - {index: 0, name: x}\n      - {index: 1, name: x}",
	}
	for name, content := range cases {
		if _, err := Load(write(t, content)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
