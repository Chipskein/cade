package sqlitestore

import (
	"context"
	"database/sql"
)

// convertVectorsToInt8 (schema version 11) rewrites chunk_embeddings from
// float32 into int8 scaled per vector (#66): a quarter of the space, and
// 99.4% of the exact top-10 kept on real vectors (#40). It reuses the
// compaction's rewrite, so the new table has no empty positions either.
// Derived data, but rebuilding it takes hours of GPU: the step backs up.
func convertVectorsToInt8(ctx context.Context, tx *sql.Tx) error {
	dimensions, found, err := storedDimensions(ctx, tx)
	if err != nil || !found {
		return err
	}
	return rewriteVectorTable(ctx, tx, dimensions, int8Conversion)
}

// int8Conversion reads the float32 blocks and stages each vector as int8.
var int8Conversion = vectorRewrite{componentBytes: float32Bytes, convert: float32BlobToInt8}

func float32BlobToInt8(stored []byte) ([]byte, error) {
	vector, err := decodeFloat32s(stored)
	if err != nil {
		return nil, err
	}
	return encodeInt8Vector(vector), nil
}
