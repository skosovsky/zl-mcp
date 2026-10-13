package storage

import (
	"context"
	"encoding/json"
)

func (s *Store) SetPreloadStatus(ctx context.Context, status map[string]any) error {
	body, err := json.Marshal(status)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(
		ctx,
		`INSERT INTO metadata(key,value) VALUES('preload_status',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		string(body),
	)
	return err
}

func (s *Store) PreloadStatus(ctx context.Context) (map[string]any, error) {
	var body string
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE((SELECT value FROM metadata WHERE key='preload_status'),'{}')`).
		Scan(&body); err != nil {
		return nil, err
	}
	status := map[string]any{}
	if err := json.Unmarshal([]byte(body), &status); err != nil {
		return nil, err
	}
	if len(status) == 0 {
		status = map[string]any{
			fieldStatus:          stateNotAttemptedText,
			fieldLastAttemptAt:   nil,
			fieldLastSuccessAt:   nil,
			"observed_count":     0,
			"permitted_count":    0,
			fieldCatalogComplete: false,
			"messages_imported":  false,
			fieldStopReason:      nil,
		}
	}
	return status, nil
}
