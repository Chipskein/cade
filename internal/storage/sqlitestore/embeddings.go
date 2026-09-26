package sqlitestore

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
)

// EmbeddingsFor looks each event up by its vec0 primary key. Point lookups
// avoid joining events with the virtual table, which vec0 would plan as a
// full scan of every vector.
func (s *Store) EmbeddingsFor(ctx context.Context, uids []string) (map[string][]float32, error) {
	embeddings := make(map[string][]float32, len(uids))
	ready, err := s.vectorTableExists(ctx)
	if err != nil || !ready {
		return embeddings, err
	}
	for _, uid := range uids {
		vector, found, err := s.embeddingFor(ctx, uid)
		if err != nil {
			return nil, err
		}
		if found {
			embeddings[uid] = vector
		}
	}
	return embeddings, nil
}

func (s *Store) embeddingFor(ctx context.Context, uid string) ([]float32, bool, error) {
	var blob []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT embedding FROM event_embeddings WHERE event_id = (SELECT id FROM events WHERE uid = ?)`, uid).Scan(&blob)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read embedding of event %q: %w", uid, err)
	}
	vector, err := decodeFloat32s(blob)
	return vector, err == nil, err
}

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
