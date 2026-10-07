package mobilebackup

import (
	"context"
	"database/sql"
)

// Targets belong to one immutable SQLite image. They are never corpus tombstones.
type snapshotRecallTargets struct {
	global map[string]struct{}
	client map[[2]string]struct{}
}

func (snapshotRecallTargets) String() string   { return "archive recall targets [redacted]" }
func (snapshotRecallTargets) GoString() string { return "archive recall targets [redacted]" }
func (t *snapshotRecallTargets) clear() {
	if t != nil {
		clear(t.global)
		clear(t.client)
	}
}
func (t *snapshotRecallTargets) suppresses(row SQLiteRow) bool {
	if t == nil {
		return false
	}
	if row.MessageID != "" {
		_, ok := t.global[row.MessageID]
		return ok
	}
	_, ok := t.client[[2]string{row.SenderID, row.ClientID}]
	return ok
}

// Read only scalar target identity and state, never recalled content or metadata.
func classifySnapshotRecalls(ctx context.Context, conn *sql.Conn, expected int) (targets *snapshotRecallTargets, err error) {
	if expected < 1 || expected > 5000 {
		return nil, ErrSnapshotControls
	}
	targets = &snapshotRecallTargets{global: map[string]struct{}{}, client: map[[2]string]struct{}{}}
	defer func() {
		if err != nil {
			targets.clear()
			targets = nil
		}
	}()
	rows, e := conn.QueryContext(ctx, "SELECT SenderId,GlbMsgId,CliMsgId,'',TimeStamp,TTL,MsgType,MsgStatus,NULL FROM ChatContent WHERE MsgType=36 LIMIT 5001")
	if e != nil {
		return targets, ErrSnapshotControls
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var values [9]any
		if rows.Scan(&values[0], &values[1], &values[2], &values[3], &values[4], &values[5], &values[6], &values[7], &values[8]) != nil {
			return targets, ErrSnapshotControls
		}
		row, reason := sqliteRowMode(values, 0, 253402300800000, false)
		clear(values[:])
		count++
		key := [2]string{row.SenderID, row.ClientID}
		_, globalDuplicate := targets.global[row.MessageID]
		_, clientDuplicate := targets.client[key]
		if reason != "" || row.Type != 36 || row.Status != 3 || globalDuplicate || clientDuplicate || count > expected || ctx.Err() != nil {
			return targets, ErrSnapshotControls
		}
		targets.global[row.MessageID] = struct{}{}
		targets.client[key] = struct{}{}
	}
	if rows.Err() != nil || ctx.Err() != nil || count != expected {
		return targets, ErrSnapshotControls
	}
	return targets, nil
}
