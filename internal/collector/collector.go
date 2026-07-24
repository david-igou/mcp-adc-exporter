// Package collector exposes ADC readings as Prometheus metrics.
// Readings are taken synchronously on scrape (no background timers),
// per the Prometheus exporter-writing guidance.
package collector

import (
	"fmt"
	"log"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/david-igou/mcp-adc-exporter/internal/adc"
	"github.com/david-igou/mcp-adc-exporter/internal/config"
	"github.com/david-igou/mcp-adc-exporter/internal/spi"
)

var (
	channelLabels = []string{"device", "chip", "channel", "name"}

	descVolts = prometheus.NewDesc("adc_channel_volts",
		"Voltage at an MCP3xxx ADC input channel.", channelLabels, nil)
	descRaw = prometheus.NewDesc("adc_channel_raw_code",
		"Raw straight-binary conversion code (0 .. 2^resolution-1).", channelLabels, nil)
	descValue = prometheus.NewDesc("adc_channel_value",
		"Channel voltage with the configured scale/offset applied; unit depends on configuration.", channelLabels, nil)
)

type channel struct {
	cfg    config.Channel
	labels []string
}

type device struct {
	dev      *adc.Device
	name     string
	channels []channel
}

// Collector reads every configured channel on each scrape.
type Collector struct {
	devices    []device
	readErrors *prometheus.CounterVec
}

// New opens every configured chip and validates channels against its
// spec. openConn is swappable for tests; pass nil for the real spidev.
func New(cfg *config.Config, openConn func(path string, speedHz uint32) (adc.Conn, error)) (*Collector, error) {
	if openConn == nil {
		openConn = func(path string, speedHz uint32) (adc.Conn, error) { return spi.Open(path, speedHz) }
	}
	c := &Collector{
		readErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "adc_read_errors_total",
			Help: "SPI read failures, by device and channel.",
		}, []string{"device", "channel"}),
	}
	for _, dc := range cfg.Devices {
		spec, err := adc.Lookup(dc.Chip)
		if err != nil {
			return nil, fmt.Errorf("device %s: %w", dc.Name, err)
		}
		speed := dc.SpeedHz
		if speed == 0 {
			speed = spec.DefaultSpeedHz
		}
		if speed > spec.MaxSpeedHz {
			return nil, fmt.Errorf("device %s: speed_hz %d exceeds %s datasheet maximum %d (5V supply; lower supplies allow less)",
				dc.Name, speed, spec.Name, spec.MaxSpeedHz)
		}
		if speed < adc.MinSpeedHz {
			log.Printf("warning: device %s: speed_hz %d is below ~%d Hz; sample-cap droop degrades linearity", dc.Name, speed, adc.MinSpeedHz)
		}
		d := device{name: dc.Name}
		for _, cc := range dc.Channels {
			if cc.Index < 0 || cc.Index >= spec.Channels {
				return nil, fmt.Errorf("device %s channel %s: index %d out of range 0..%d for %s",
					dc.Name, cc.Name, cc.Index, spec.Channels-1, spec.Name)
			}
			d.channels = append(d.channels, channel{
				cfg:    cc,
				labels: []string{dc.Name, spec.Name, fmt.Sprint(cc.Index), cc.Name},
			})
			// Pre-seed the error series at 0 so rate()/increase() can
			// see the first failure.
			c.readErrors.WithLabelValues(dc.Name, fmt.Sprint(cc.Index))
		}
		conn, err := openConn(dc.SPIDev, uint32(speed))
		if err != nil {
			return nil, fmt.Errorf("device %s: open %s: %w", dc.Name, dc.SPIDev, err)
		}
		d.dev = adc.NewDevice(conn, spec, dc.VRef)
		c.devices = append(c.devices, d)
	}
	return c, nil
}

// Describe implements prometheus.Collector.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- descVolts
	ch <- descRaw
	ch <- descValue
	c.readErrors.Describe(ch)
}

// Collect implements prometheus.Collector.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	for _, d := range c.devices {
		for _, cc := range d.channels {
			raw, err := d.dev.ReadRaw(cc.cfg.Index, cc.cfg.Mode == "differential")
			if err != nil {
				log.Printf("read %s channel %d: %v", d.name, cc.cfg.Index, err)
				c.readErrors.WithLabelValues(d.name, fmt.Sprint(cc.cfg.Index)).Inc()
				continue
			}
			volts := d.dev.Volts(raw)
			ch <- prometheus.MustNewConstMetric(descVolts, prometheus.GaugeValue, volts, cc.labels...)
			ch <- prometheus.MustNewConstMetric(descRaw, prometheus.GaugeValue, float64(raw), cc.labels...)
			ch <- prometheus.MustNewConstMetric(descValue, prometheus.GaugeValue, volts*cc.cfg.Scale+cc.cfg.Offset, cc.labels...)
		}
	}
	c.readErrors.Collect(ch)
}

// Close releases all SPI connections.
func (c *Collector) Close() {
	for _, d := range c.devices {
		_ = d.dev.Close()
	}
}
