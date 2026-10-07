package mobilebackup

import (
	"bytes"
	"context"
	"database/sql"
)

// snapshotInformation classifies only the pinned native informational action.
// Its body and action parameters are never rendered or executed. Other type-20
// records remain unclassified; desktop MSG_UNDO integers are not backup types.
func snapshotInformation(ctx context.Context, data []byte) bool {
	metadata, err := ParseBinNet(ctx, data)
	defer metadata.Clear()
	return err == nil && ctx.Err() == nil && len(metadata.Attachments) == 1 &&
		metadata.Attachments[0].Action.Present && bytes.Equal(metadata.Attachments[0].Action.Bytes, []byte("msginfo.actionlist"))
}

// classifySnapshotInformation scans the entire selected image, outside date and
// paging filters, using metadata only. It returns no prefix on unknown shapes,
// budget exhaustion or cancellation, and loads neither identities nor text.
func classifySnapshotInformation(ctx context.Context, conn *sql.Conn, expected int) error {
	if expected < 1 || expected > 5000 {
		return ErrSnapshotControls
	}
	rows, err := conn.QueryContext(ctx, "SELECT CASE WHEN typeof(BinNet)='blob' AND length(BinNet)<=262144 THEN BinNet ELSE NULL END FROM ChatContent WHERE MsgType=20 LIMIT 5001")
	if err != nil {
		return ErrSnapshotControls
	}
	defer rows.Close()
	count, decodedBytes := 0, 0
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return ErrSnapshotControls
		}
		count++
		decodedBytes += len(data)
		valid := count <= expected && decodedBytes <= 8<<20 && snapshotInformation(ctx, data)
		clear(data)
		if !valid {
			return ErrSnapshotControls
		}
	}
	if rows.Err() != nil || ctx.Err() != nil || count != expected {
		return ErrSnapshotControls
	}
	return nil
}
