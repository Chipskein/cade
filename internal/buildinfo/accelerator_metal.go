//go:build darwin && !cuda && !vulkan

package buildinfo

// On Apple, llama.cpp builds its Metal backend by default, so the plain
// build is the Metal one (#53).
const accelerator = "Metal"
