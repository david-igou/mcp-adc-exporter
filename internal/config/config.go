// Package config loads and validates the exporter's YAML configuration.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// Channel maps one analog input to a named metric series.
type Channel struct {
	Index  int     `yaml:"index"`
	Name   string  `yaml:"name"`
	Mode   string  `yaml:"mode"`   // "single" (default) or "differential"
	Scale  float64 `yaml:"scale"`  // physical units per volt (default 1.0)
	Offset float64 `yaml:"offset"` // added after scaling
}

// Device is one ADC chip on a spidev bus.
type Device struct {
	Name     string    `yaml:"name"`
	Chip     string    `yaml:"chip"`
	SPIDev   string    `yaml:"spidev"`
	SpeedHz  int       `yaml:"speed_hz"`
	VRef     float64   `yaml:"vref"`
	Channels []Channel `yaml:"channels"`
}

// Config is the top-level exporter configuration.
type Config struct {
	Listen  string   `yaml:"listen"`
	Devices []Device `yaml:"devices"`
}

// Load reads, parses, and validates a config file, applying defaults.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	// Reject unknown keys: a typo like "scail:" must fail loudly instead
	// of silently shipping unscaled readings.
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.Listen == "" {
		cfg.Listen = ":9958"
	}
	if len(cfg.Devices) == 0 {
		return nil, fmt.Errorf("%s: no devices configured", path)
	}
	seen := map[string]bool{}
	seenSPIDev := map[string]string{}
	for i := range cfg.Devices {
		d := &cfg.Devices[i]
		if d.Name == "" {
			return nil, fmt.Errorf("device %d: name is required", i)
		}
		if seen[d.Name] {
			return nil, fmt.Errorf("duplicate device name %q", d.Name)
		}
		seen[d.Name] = true
		if d.SPIDev == "" {
			return nil, fmt.Errorf("device %s: spidev is required", d.Name)
		}
		if other, ok := seenSPIDev[d.SPIDev]; ok {
			return nil, fmt.Errorf("devices %s and %s share spidev %s", other, d.Name, d.SPIDev)
		}
		seenSPIDev[d.SPIDev] = d.Name
		if d.VRef <= 0 {
			return nil, fmt.Errorf("device %s: vref must be > 0", d.Name)
		}
		if len(d.Channels) == 0 {
			return nil, fmt.Errorf("device %s: no channels configured", d.Name)
		}
		chNames := map[string]bool{}
		for j := range d.Channels {
			c := &d.Channels[j]
			if c.Name == "" {
				return nil, fmt.Errorf("device %s channel %d: name is required", d.Name, c.Index)
			}
			if chNames[c.Name] {
				return nil, fmt.Errorf("device %s: duplicate channel name %q", d.Name, c.Name)
			}
			chNames[c.Name] = true
			switch c.Mode {
			case "":
				c.Mode = "single"
			case "single", "differential":
			default:
				return nil, fmt.Errorf("device %s channel %s: mode must be single or differential", d.Name, c.Name)
			}
			if c.Scale == 0 {
				c.Scale = 1.0
			}
		}
	}
	return &cfg, nil
}
