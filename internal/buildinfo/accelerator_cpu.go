//go:build !cuda && !vulkan && !darwin

package buildinfo

const accelerator = "CPU"
