package sqlitestore

import (
	"encoding/binary"
	"fmt"
	"math"
)

// decodeFloat32s reads sqlite-vec's float32 blob: little-endian IEEE 754.
func decodeFloat32s(blob []byte) ([]float32, error) {
	if len(blob)%4 != 0 {
		return nil, fmt.Errorf("embedding blob of %d bytes, expected a multiple of 4", len(blob))
	}
	vector := make([]float32, len(blob)/4)
	for i := range vector {
		vector[i] = math.Float32frombits(binary.LittleEndian.Uint32(blob[4*i:]))
	}
	return vector, nil
}
