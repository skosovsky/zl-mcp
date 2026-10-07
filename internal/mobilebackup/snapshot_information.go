package mobilebackup

import (
	"bytes"
	"context"
	"database/sql"
)

// snapshotInformation classifies only the pinned native informational action.
// This is a diagnostic subset, not the type-20 exclusion rule. Neither its body
// nor action parameters are rendered; desktop MSG_UNDO is not a backup type.
func snapshotInformation(ctx context.Context, data []byte) bool {
	metadata, err := ParseBinNet(ctx, data)
	defer metadata.Clear()
	return err == nil && ctx.Err() == nil && len(metadata.Attachments) == 1 &&
		metadata.Attachments[0].Action.Present && bytes.Equal(metadata.Attachments[0].Action.Bytes, []byte("msginfo.actionlist"))
}

// classifySnapshotInformation scans the entire selected image, outside date and
// paging filters, using metadata only. Unknown metadata cannot make a native-excluded row visible.
// It returns no prefix on query errors,
// budget exhaustion or cancellation, and loads neither identities nor text.
func classifySnapshotInformation(ctx context.Context, conn *sql.Conn, expected int) (int, error) {
	if expected < 1 || expected > 5000 {
		return 0, ErrSnapshotControls
	}
	rows, err := conn.QueryContext(ctx, "SELECT CASE WHEN typeof(BinNet)='blob' AND length(BinNet)<=262144 THEN BinNet ELSE NULL END FROM ChatContent WHERE MsgType=20 LIMIT 5001")
	if err != nil {
		return 0, ErrSnapshotControls
	}
	defer rows.Close()
	count, decodedBytes, information := 0, 0, 0
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return 0, ErrSnapshotControls
		}
		count++
		decodedBytes += len(data)
		if count > expected || decodedBytes > 8<<20 || ctx.Err() != nil {
			clear(data)
			return 0, ErrSnapshotControls
		}
		if snapshotInformation(ctx, data) {
			information++
		}
		clear(data)
	}
	if rows.Err() != nil || ctx.Err() != nil || count != expected {
		return 0, ErrSnapshotControls
	}
	return information, nil
}
