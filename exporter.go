package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"

	jsoniter "github.com/json-iterator/go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var logLevel = new(slog.LevelVar)
var log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))

// FactorioCollector collects metrics from the Factorio JSON file.
type FactorioCollector struct {
	metricsPath string
	mutex       sync.Mutex
	data        jsoniter.Any
}

// Describe implements the prometheus.Collector interface.
func (c *FactorioCollector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(c, ch)
}

// Collect implements the prometheus.Collector interface.
func (c *FactorioCollector) Collect(ch chan<- prometheus.Metric) {
	log.Debug("Collecting metrics")
	// Lock the mutex to prevent data races.
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Read the metrics data from the JSON file.
	err := c.readMetricsData()
	if err != nil {
		log.Error("Error reading metrics data", "error", err)
		return
	}

	c.collectTimeMetrics(ch)
	c.collectPlayerStateMetrics(ch)
	c.collectForceMetrics(ch)
	c.collectPollutionMetrics(ch)
	c.collectSurfaceMetrics(ch)
	c.collectEntityMetrics(ch)
	c.collectRocketMetrics(ch)

	log.Debug("Collected metrics")
}

func (c *FactorioCollector) collectTimeMetrics(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc("factorio_game_tick", "The current tick of the running Factorio game.", nil, nil),
		prometheus.CounterValue,
		c.data.Get("game", "time", "tick").ToFloat64(),
	)

	pausedInt := 0
	if c.data.Get("game", "time", "paused").ToBool() {
		pausedInt = 1
	}

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc("factorio_game_paused", "The current pause state of the running Factorio game.", nil, nil),
		prometheus.GaugeValue,
		float64(pausedInt),
	)
}

func (c *FactorioCollector) collectPlayerStateMetrics(ch chan<- prometheus.Metric) {
	for _, username := range c.data.Get("players").Keys() {
		connectedValue := 0.0
		if c.data.Get("players", username, "connected").ToBool() {
			connectedValue = 1.0
		}
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc("factorio_player_connected", "The current connection state of the player.", []string{"username"}, nil),
			prometheus.GaugeValue,
			connectedValue,
			username,
		)
	}
}

func (c *FactorioCollector) collectForceMetrics(ch chan<- prometheus.Metric) {
	for _, force_name := range c.data.Get("forces").Keys() {
		force := c.data.Get("forces", force_name)
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc("factorio_force_research_progress", "The current research progress percentage (0-1) for a force.", []string{"force"}, nil),
			prometheus.GaugeValue,
			force.Get("research", "progress").ToFloat64(),
			force_name,
		)

		for _, surface_name := range force.Get("items").Keys() {
			surface := force.Get("items", surface_name)
			for _, item_name := range surface.Keys() {
				item := surface.Get(item_name)
				if production := item.Get("production").ToFloat64(); production > 0 {
					ch <- prometheus.MustNewConstMetric(
						prometheus.NewDesc("factorio_force_prototype_production", "The total production of a given prototype for a force.", []string{"force", "prototype", "surface", "type"}, nil),
						prometheus.CounterValue,
						production,
						force_name,
						item_name,
						surface_name,
						"items",
					)
				}
				if consumption := item.Get("consumption").ToFloat64(); consumption > 0 {
					ch <- prometheus.MustNewConstMetric(
						prometheus.NewDesc("factorio_force_prototype_consumption", "The total consumption of a given prototype for a force.", []string{"force", "prototype", "surface", "type"}, nil),
						prometheus.CounterValue,
						consumption,
						force_name,
						item_name,
						surface_name,
						"items",
					)
				}
			}
		}

		for _, surface_name := range force.Get("fluids").Keys() {
			surface_fluids := force.Get("fluids", surface_name)
			for _, fluid_name := range surface_fluids.Keys() {
				fluid := surface_fluids.Get(fluid_name)
				if production := fluid.Get("production").ToFloat64(); production > 0 {
					ch <- prometheus.MustNewConstMetric(
						prometheus.NewDesc("factorio_force_prototype_production", "The total production of a given prototype for a force.", []string{"force", "prototype", "surface", "type"}, nil),
						prometheus.CounterValue,
						production,
						force_name,
						fluid_name,
						surface_name,
						"fluids",
					)
				}
				if consumption := fluid.Get("consumption").ToFloat64(); consumption > 0 {
					ch <- prometheus.MustNewConstMetric(
						prometheus.NewDesc("factorio_force_prototype_consumption", "The total consumption of a given prototype for a force.", []string{"force", "prototype", "surface", "type"}, nil),
						prometheus.CounterValue,
						consumption,
						force_name,
						fluid_name,
						surface_name,
						"fluids",
					)
				}
			}
		}
	}
}

func (c *FactorioCollector) collectPollutionMetrics(ch chan<- prometheus.Metric) {
	for _, surface_name := range c.data.Get("pollution").Keys() {
		surface_pollution := c.data.Get("pollution", surface_name)
		for _, entity_name := range surface_pollution.Keys() {
			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc("factorio_surface_pollution_production", "The pollution produced or consumed from various sources.", []string{"source", "surface"}, nil),
				prometheus.GaugeValue,
				surface_pollution.Get(entity_name).ToFloat64(),
				entity_name,
				surface_name,
			)
		}
	}
}

func (c *FactorioCollector) collectSurfaceMetrics(ch chan<- prometheus.Metric) {
	for _, surface_name := range c.data.Get("surfaces").Keys() {
		surface := c.data.Get("surfaces", surface_name)
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc("factorio_surface_pollution_total", "The total pollution on a given surface.", []string{"surface"}, nil),
			prometheus.GaugeValue,
			surface.Get("pollution").ToFloat64(),
			surface_name,
		)
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc("factorio_surface_ticks_per_day", "The number of ticks per day on a given surface.", []string{"surface"}, nil),
			prometheus.GaugeValue,
			surface.Get("ticks_per_day").ToFloat64(),
			surface_name,
		)
	}
}

func (c *FactorioCollector) collectEntityMetrics(ch chan<- prometheus.Metric) {
	for _, surface_name := range c.data.Get("surfaces").Keys() {
		surface := c.data.Get("surfaces", surface_name)
		for _, entity_name := range surface.Get("entities").Keys() {
			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc("factorio_entity_count", "The total number of entities.", []string{"force", "name", "surface"}, nil),
				prometheus.GaugeValue,
				surface.Get("entities", entity_name).ToFloat64(),
				"player",
				entity_name,
				surface_name,
			)
		}
	}
}

func (c *FactorioCollector) collectRocketMetrics(ch chan<- prometheus.Metric) {
	for _, force_name := range c.data.Get("forces").Keys() {
		force_data := c.data.Get("forces", force_name)
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc("factorio_rockets_launched", "The total number of rockets launched.", []string{"force"}, nil),
			prometheus.CounterValue,
			float64(force_data.Get("rockets", "launches").ToInt()),
			force_name,
		)
		for _, item_name := range force_data.Get("rockets", "items").Keys() {
			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc("factorio_items_launched", "The total number of items launched in rockets.", []string{"force", "name"}, nil),
				prometheus.CounterValue,
				float64(force_data.Get("rockets", "items", item_name).ToInt()),
				force_name,
				item_name,
			)
		}
	}
}

// readMetricsData reads the metrics data from the JSON file.
func (c *FactorioCollector) readMetricsData() error {
	data, err := os.ReadFile(c.metricsPath)
	if err != nil {
		return fmt.Errorf("failed to read metrics file: %w", err)
	}

	c.data = jsoniter.Get(data)

	return nil
}

var metricsPath = flag.String("path", "/factorio/script-output/metrics.json", "The path to the script-output/metrics.json file")
var metricsBind = flag.String("bind", "127.0.0.1:9102", "The hostname and port to listen on")
var verbose = flag.Bool("verbose", false, "Enable verbose logging")

func main() {
	// Get the metrics path and port from the command line.
	flag.Parse()

	if *verbose {
		logLevel.Set(slog.LevelDebug)
	}

	// Create a new FactorioCollector.
	collector := &FactorioCollector{
		metricsPath: *metricsPath,
	}

	// Register the collector with Prometheus.
	prometheus.MustRegister(collector)

	// Start the HTTP server.
	log.Info("Starting Prometheus exporter", "interface", *metricsBind)
	err := http.ListenAndServe(*metricsBind, promhttp.Handler())
	if err != nil {
		log.Error("Failed to serve", "error", err)
	}
}
