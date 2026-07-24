// mcp-adc-exporter exposes analog readings from Microchip MCP3xxx SPI
// ADCs as Prometheus metrics.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/david-igou/mcp-adc-exporter/internal/collector"
	"github.com/david-igou/mcp-adc-exporter/internal/config"
)

var (
	version  = "dev"
	revision = "unknown"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to the YAML configuration file")
	listen := flag.String("listen", "", "listen address (overrides the config file)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("mcp-adc-exporter %s (%s)\n", version, revision)
		return
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if *listen != "" {
		cfg.Listen = *listen
	}

	c, err := collector.New(cfg, nil)
	if err != nil {
		log.Fatalf("setup: %v", err)
	}
	defer c.Close()

	reg := prometheus.NewRegistry()
	reg.MustRegister(c)
	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "adc_exporter_build_info",
		Help: "Build information for mcp-adc-exporter.",
	}, []string{"version", "revision"})
	buildInfo.WithLabelValues(version, revision).Set(1)
	reg.MustRegister(buildInfo)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `<html><body><h1>mcp-adc-exporter</h1><p><a href="/metrics">Metrics</a></p></body></html>`)
	})
	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("mcp-adc-exporter %s listening on %s (%d device(s))", version, cfg.Listen, len(cfg.Devices))
	if err := srv.ListenAndServe(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
