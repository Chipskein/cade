package config

import "fmt"

// BackgroundConfig is how `ingest start` and `ingest --gentle` share
// the machine, so a long ingestion can run for hours without keeping it at
// its limit (issue #41). Those runs also get the lowest CPU and I/O
// priority.
type BackgroundConfig struct {
	// Threads replaces embedding.threads and generation.threads; 0 means
	// one per physical core, as there.
	Threads int `json:"threads"`
	// GPULayers replaces embedding.gpu_layers and generation.gpu_layers:
	// 0 keeps the GPU free, -1 offloads every layer.
	GPULayers int `json:"gpu_layers"`
	// BusyPercent is the share of the wall clock the models may work:
	// after each call they rest, so 50 halves the average CPU or GPU load.
	// A GPU cannot be niced, so this is what limits it.
	BusyPercent int `json:"busy_percent"`
	// MaxImagesPerRun replaces ingest.max_images_per_run: time matters
	// less when nobody is waiting.
	MaxImagesPerRun int `json:"max_images_per_run"`
}

// Bounds of background.busy_percent: 100 is no rest at all.
const (
	minBusyPercent = 1
	maxBusyPercent = 100
)

// Two threads leave most of a 6-core CPU free; 50% busy halves the load;
// 500 images take ~30 min at 50% on an RTX 3060 and ~6 h on a 6-core CPU.
func defaultBackground() BackgroundConfig {
	return BackgroundConfig{Threads: 2, GPULayers: allGPULayers, BusyPercent: 50, MaxImagesPerRun: 500}
}

// WithBackgroundLimits is c as a background or gentle ingestion runs it.
//
//	cfg = cfg.WithBackgroundLimits()
func (c Config) WithBackgroundLimits() Config {
	limits := c.Ingest.Background
	c.Embedding.Threads, c.Generation.Threads = limits.Threads, limits.Threads
	c.Embedding.GPULayers, c.Generation.GPULayers = limits.GPULayers, limits.GPULayers
	c.Ingest.MaxImagesPerRun = limits.MaxImagesPerRun
	return c
}

func (background BackgroundConfig) validate(imagesOn bool) error {
	if background.BusyPercent < minBusyPercent || background.BusyPercent > maxBusyPercent {
		return fmt.Errorf("ingest.background.busy_percent is %d, expected %d to %d", background.BusyPercent, minBusyPercent, maxBusyPercent)
	}
	if background.Threads < 0 {
		return fmt.Errorf("ingest.background.threads is %d, expected 0 (one per core) or more", background.Threads)
	}
	if imagesOn && background.MaxImagesPerRun <= 0 {
		return fmt.Errorf("ingest.background.max_images_per_run is %d, expected a positive count with sources.images on", background.MaxImagesPerRun)
	}
	return nil
}
