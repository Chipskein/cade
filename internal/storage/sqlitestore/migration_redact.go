package sqlitestore

import (
	"context"
	"database/sql"
	"encoding/json"
	"path"
	"strings"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/privacy"
)

type storedPrivacyEvent struct {
	id                        int64
	source, content, metadata string
}

func redactStoredEvents(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT id, source, content, metadata FROM events`)
	if err != nil {
		return err
	}
	var events []storedPrivacyEvent
	for rows.Next() {
		var ev storedPrivacyEvent
		if err := rows.Scan(&ev.id, &ev.source, &ev.content, &ev.metadata); err != nil {
			rows.Close()
			return err
		}
		events = append(events, ev)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, ev := range events {
		if err := rewritePrivacyEvent(ctx, tx, ev); err != nil {
			return err
		}
	}
	return nil
}

func rewritePrivacyEvent(ctx context.Context, tx *sql.Tx, ev storedPrivacyEvent) error {
	var metadata map[string]string
	if err := json.Unmarshal([]byte(ev.metadata), &metadata); err != nil {
		return err
	}
	if ev.source == "file" && ignoredCredentialPath(metadata["path"]) {
		if _, err := tx.ExecContext(ctx, `DELETE FROM file_modifications WHERE path = ?`, metadata["path"]); err != nil {
			return err
		}
		return deletePrivacyEvent(ctx, tx, ev.id)
	}
	content := privacy.Text(ev.content)
	changed := content != ev.content
	for key, value := range metadata {
		clean := privacy.Text(privacy.URL(value))
		if ev.source == "browser" && key == "url" && clean != value {
			content = strings.ReplaceAll(content, value, clean)
		}
		if clean != value {
			changed = true
		}
		metadata[key] = clean
	}
	if !changed {
		return nil
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE events SET content = ?, metadata = ? WHERE id = ?`, content, string(encoded), ev.id); err != nil {
		return err
	}
	return removePrivacyVectors(ctx, tx, ev.id, content)
}

func ignoredCredentialPath(name string) bool {
	for _, pattern := range config.Defaults().Sources.IgnoredFileGlobs {
		if matched, _ := path.Match(pattern, path.Base(name)); matched {
			return true
		}
	}
	return false
}

func deletePrivacyEvent(ctx context.Context, tx *sql.Tx, id int64) error {
	if err := removePrivacyVectors(ctx, tx, id, ""); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM events WHERE id = ?`, id)
	return err
}

func removePrivacyVectors(ctx context.Context, tx *sql.Tx, id int64, content string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM chunks_fts WHERE rowid IN (SELECT id FROM chunks WHERE event_id = ?)`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM chunk_embeddings WHERE chunk_id IN (SELECT id FROM chunks WHERE event_id = ?)`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM chunks WHERE event_id = ?`, id); err != nil {
		return err
	}
	if content == "" {
		return nil
	}
	return putSetting(ctx, tx, reindexPendingSettingKey, "1")
}
