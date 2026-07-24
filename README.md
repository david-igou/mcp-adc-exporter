# mcp-adc-exporter

A Prometheus exporter for Microchip MCP3xxx SPI analog-to-digital
converters. It reads analog channels over Linux `spidev` (Raspberry Pi
and any SBC with an SPI bus) and exposes them as voltage — and
optionally scaled sensor-unit — gauges.

Readings are taken synchronously on each scrape via a custom
`Collector`; there are no background timers. The SPI framing and clock
limits are implemented from the Microchip datasheets and cross-checked
against the Linux kernel `mcp320x` IIO driver — see
[docs/research.md](docs/research.md) for the fully cited research this
implementation is based on.

## Supported chips

| Chip | Resolution | Channels | Default SPI clock | Datasheet max (5V) |
|---|---|---|---|---|
| MCP3004 | 10-bit | 4 | 1.35 MHz | 3.6 MHz |
| MCP3008 | 10-bit | 8 | 1.35 MHz | 3.6 MHz |
| MCP3204 | 12-bit | 4 | 1.0 MHz | 2.0 MHz |
| MCP3208 | 12-bit | 8 | 1.0 MHz | 2.0 MHz |

Defaults are the conservative 2.7V-derived clocks, safe on a 3.3V rail.
The exporter refuses clocks above the 5V datasheet ceiling and warns
below ~10 kHz (sample-cap droop). MCP3002/3202 use a different
command-byte layout and are not yet supported.

## Quick start (Raspberry Pi)

```sh
# enable SPI once: add dtparam=spi=on to /boot/config.txt (or raspi-config)
# wire the chip to SPI0 CE0 -> it appears as /dev/spidev0.0

cp config.example.yaml config.yaml   # edit to match your wiring
./mcp-adc-exporter -config config.yaml
curl localhost:9958/metrics
```

## Configuration

```yaml
listen: ":9958"
devices:
  - name: adc0
    chip: mcp3008          # mcp3004|mcp3008|mcp3204|mcp3208
    spidev: /dev/spidev0.0 # one entry per chip; use spidev0.1 for CE1
    speed_hz: 1350000      # optional; per-chip default if omitted
    vref: 3.3              # REQUIRED: volts at VREF — sets the scale
    channels:
      - index: 0
        name: psu_12v
        mode: single       # single (default) or differential (pair code, Table 5-3)
        scale: 5.7         # value = volts*scale + offset
        offset: 0.0
```

`vref` is mandatory: the chip returns a code relative to VREF, so
absolute volts cannot be computed without it. `scale`/`offset` express
sensor front-ends, e.g. undoing a resistor divider, or an ACS712 ±5A
current sensor as `scale: 5.405` (1/0.185), `offset: -13.51` (−2.5/0.185).

Analog sources should be low impedance (well under 500 Ω); buffer
high-impedance sensors with an op-amp or readings will be low.

## Metrics

| Metric | Description |
|---|---|
| `adc_channel_volts{device,chip,channel,name}` | Voltage at the input pin |
| `adc_channel_raw_code{...}` | Raw conversion code (0..2^N−1) |
| `adc_channel_value{...}` | `volts*scale + offset` (unit is configuration-defined) |
| `adc_read_errors_total{device,channel}` | SPI read failures |
| `adc_exporter_build_info{version,revision}` | Build information |

## Building

```sh
make all         # vet + test + build for the host
make crossbuild  # linux amd64/arm64/armv6 into dist/
```
