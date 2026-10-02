package collector

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type Upstream interface {
	AccountID() string
	Groups(context.Context) ([]domain.Group, error)
	Group(context.Context, string) (domain.Group, *string, error)
	Inspect(context.Context, string) (domain.Invite, error)
	Join(context.Context, string) error
}
type JoinManager struct {
	Store     *storage.Store
	API       Upstream
	AllowJoin bool
	ctx       context.Context
	gate      chan struct{}
	wg        sync.WaitGroup
}
type preview struct {
	ID       string    `json:"id"`
	URL      string    `json:"url"`
	GroupID  string    `json:"group_id"`
	Name     string    `json:"name"`
	Approval bool      `json:"approval"`
	Expires  time.Time `json:"expires"`
}

func NewJoin(ctx context.Context, s *storage.Store, api Upstream, allow bool) *JoinManager {
	return &JoinManager{Store: s, API: api, AllowJoin: allow, ctx: ctx, gate: make(chan struct{}, 1)}
}
func (j *JoinManager) Wait() { j.wg.Wait() }
func safeError(code, message, instruction string) error {
	return &domain.Error{Code: code, Message: message, NextAction: domain.NextAction{Instruction: instruction}, Details: map[string]any{}}
}
func ValidateInvite(raw string) (string, error) {
	u, e := url.Parse(raw)
	if e != nil || len(raw) > 2048 || u.Scheme != "https" || u.Host != "zalo.me" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
		return "", domain.Invalid("Use an HTTPS invitation such as https://zalo.me/g/example without query or fragment.")
	}
	path := strings.TrimPrefix(u.Path, "/g/")
	if !strings.HasPrefix(u.Path, "/g/") || path == "" {
		return "", domain.Invalid("Invitation must have /g/<code> path.")
	}
	for _, r := range path {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return "", domain.Invalid("Invalid invitation code.")
		}
	}
	return u.String(), nil
}
func (j *JoinManager) Inspect(ctx context.Context, raw string) (map[string]any, error) {
	link, e := ValidateInvite(raw)
	if e != nil {
		return nil, e
	}
	v, e := j.API.Inspect(ctx, link)
	if e != nil {
		if errors.Is(e, domain.ErrAuthenticationRequired) {
			return nil, safeError("NOT_AUTHENTICATED", "Zalo authentication is required.", "Stop service and run local login.")
		}
		return nil, safeError("UPSTREAM_UNAVAILABLE", "Could not inspect this invitation.", "Check the invitation or retry later.")
	}
	var id, expires *string
	if v.Group.ID != "" {
		p := preview{ID: uuid.NewString(), URL: link, GroupID: v.Group.ID, Name: v.Group.Name, Approval: v.ApprovalRequired, Expires: time.Now().UTC().Add(10 * time.Minute)}
		b, e := json.Marshal(p)
		if e != nil {
			return nil, e
		}
		if _, e = j.Store.DB.ExecContext(ctx, "INSERT INTO join_previews VALUES(?,?,?)", p.ID, string(b), p.Expires.Format(time.RFC3339Nano)); e != nil {
			return nil, e
		}
		id = &p.ID
		t := p.Expires.Format(time.RFC3339Nano)
		expires = &t
	}
	member := "unknown"
	groups, e := j.API.Groups(ctx)
	if e == nil {
		member = "not_member"
		for _, g := range groups {
			if g.ID == v.Group.ID {
				member = "member"
			}
		}
	}
	return map[string]any{"group_id": v.Group.ID, "name": v.Group.Name, "description": v.Description, "member_count": v.Group.MemberCount, "approval_required": v.ApprovalRequired, "membership": member, "preview_id": id, "preview_expires_at": expires}, nil
}
func (j *JoinManager) Preview(ctx context.Context, id string) (preview, error) {
	var b string
	p := preview{}
	e := j.Store.DB.QueryRowContext(ctx, "SELECT payload FROM join_previews WHERE id=?", id).Scan(&b)
	if e != nil {
		return p, safeError("NOT_FOUND", "Preview not found.", "Inspect the invitation again.")
	}
	if e = json.Unmarshal([]byte(b), &p); e != nil {
		return p, e
	}
	if !time.Now().Before(p.Expires) {
		return p, safeError("APPROVAL_EXPIRED", "Preview expired.", "Inspect the invitation again.")
	}
	return p, nil
}
func tokenHash(t string) string { h := sha256.Sum256([]byte(t)); return hex.EncodeToString(h[:]) }
func (j *JoinManager) Approve(ctx context.Context, id string) (string, error) {
	if !j.AllowJoin {
		return "", safeError("PERMISSION_DENIED", "Joining is disabled in local configuration.", "Set permissions.allow_join=true if joining is intended.")
	}
	p, e := j.Preview(ctx, id)
	if e != nil {
		return "", e
	}
	b := make([]byte, 32)
	if _, e = rand.Read(b); e != nil {
		return "", e
	}
	token := hex.EncodeToString(b)
	// Approval cannot outlive its preview, so the reviewed conditions never become stale silently.
	_, e = j.Store.DB.ExecContext(ctx, "INSERT INTO join_approvals VALUES(?,?,?,NULL)", tokenHash(token), id, p.Expires.Format(time.RFC3339Nano))
	return token, e
}
func (j *JoinManager) Operation(ctx context.Context, id string) (map[string]any, error) {
	var b string
	e := j.Store.DB.QueryRowContext(ctx, "SELECT payload FROM join_requests WHERE operation_id=?", id).Scan(&b)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, safeError("NOT_FOUND", "Operation not found.", "Use the operation_id returned by zalo_join_group.")
	}
	if e != nil {
		return nil, e
	}
	v := map[string]any{}
	e = json.Unmarshal([]byte(b), &v)
	return v, e
}
func (j *JoinManager) Start(ctx context.Context, token, requestID string) (map[string]any, error) {
	if _, e := uuid.Parse(requestID); e != nil {
		return nil, domain.Invalid("request_id must be a UUID.")
	}
	if e := j.enter(ctx); e != nil {
		return nil, e
	}
	defer j.leave()
	hash := tokenHash(token)
	var existingHash, payload string
	e := j.Store.DB.QueryRowContext(ctx, "SELECT token_hash,payload FROM join_requests WHERE request_id=?", requestID).Scan(&existingHash, &payload)
	if e == nil {
		if existingHash != hash {
			return nil, safeError("REQUEST_ID_CONFLICT", "request_id belongs to another approval.", "Retry only with the original request_id and plan_token.")
		}
		v := map[string]any{}
		e = json.Unmarshal([]byte(payload), &v)
		return v, e
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return nil, e
	}
	if !j.AllowJoin {
		return nil, safeError("PERMISSION_DENIED", "Joining is disabled.", "Enable joining in local configuration if intended.")
	}
	var previewID, expiry string
	var consumed sql.NullString
	e = j.Store.DB.QueryRowContext(ctx, "SELECT preview_id,expires_at,request_id FROM join_approvals WHERE token_hash=?", hash).Scan(&previewID, &expiry, &consumed)
	if errors.Is(e, sql.ErrNoRows) || consumed.Valid {
		return nil, safeError("APPROVAL_REQUIRED", "No unused trusted approval was found.", "Review the preview with local zl-mcp approve-join.")
	}
	if e != nil {
		return nil, e
	}
	at, e := time.Parse(time.RFC3339Nano, expiry)
	if e != nil {
		return nil, e
	}
	if !time.Now().Before(at) {
		return nil, safeError("APPROVAL_EXPIRED", "Approval expired.", "Inspect and approve the invitation again.")
	}
	p, e := j.Preview(ctx, previewID)
	if e != nil {
		return nil, e
	}
	// Do not issue a membership mutation until its target and conditions have been revalidated.
	checked, e := j.API.Inspect(ctx, p.URL)
	if e != nil {
		return nil, safeError("UPSTREAM_UNAVAILABLE", "Cannot revalidate the invitation.", "Retry with the same request_id and plan_token later.")
	}
	if checked.Group.ID != p.GroupID || checked.Group.Name != p.Name || checked.ApprovalRequired != p.Approval {
		return nil, safeError("PREVIEW_CHANGED", "Invitation conditions changed.", "Inspect and approve the invitation again.")
	}
	// The persisted ledger survives collector restarts; a process-local timestamp
	// would permit a new membership attempt immediately after every restart.
	var lastStarted string
	if err := j.Store.DB.QueryRowContext(ctx, "SELECT created_at FROM join_requests ORDER BY rowid DESC LIMIT 1").Scan(&lastStarted); err == nil {
		last, parseErr := time.Parse(time.RFC3339Nano, lastStarted)
		if parseErr != nil {
			return nil, parseErr
		}
		if time.Since(last) < time.Minute {
			return nil, &domain.Error{Code: "RATE_LIMITED", Message: "Wait before starting another join.", Retryable: true, NextAction: domain.NextAction{Instruction: "Retry the same request after one minute."}, Details: map[string]any{"retry_after_ms": 60000}}
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	operationID := uuid.NewString()
	result := map[string]any{"operation_id": operationID, "request_id": requestID, "status": "running", "group_id": p.GroupID, "collection_enabled": j.Store.Allowed(p.GroupID), "checked_at": time.Now().UTC().Format(time.RFC3339Nano), "retry_safe": false, "poll_after_ms": 1000, "error": nil}
	b, e := json.Marshal(result)
	if e != nil {
		return nil, e
	}
	tx, e := j.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	consumedResult, e := tx.ExecContext(ctx, "UPDATE join_approvals SET request_id=? WHERE token_hash=? AND request_id IS NULL", requestID, hash)
	if e != nil {
		return nil, e
	}
	n, e := consumedResult.RowsAffected()
	if e != nil {
		return nil, e
	}
	if n != 1 {
		return nil, safeError("APPROVAL_REQUIRED", "Approval was already consumed.", "Inspect and approve again.")
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO join_requests VALUES(?,?,?,?,?)", requestID, operationID, hash, string(b), time.Now().UTC().Format(time.RFC3339Nano)); e != nil {
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	j.wg.Add(1)
	go func() {
		defer j.wg.Done()
		copyResult := map[string]any{}
		for key, value := range result {
			copyResult[key] = value
		}
		j.execute(p, copyResult)
	}()
	return result, nil
}
func (j *JoinManager) execute(p preview, result map[string]any) {
	ctx, cancel := context.WithTimeout(j.ctx, 45*time.Second)
	defer cancel()
	status := "unknown"
	defer func() {
		result["status"] = status
		result["checked_at"] = time.Now().UTC().Format(time.RFC3339Nano)
		result["poll_after_ms"] = nil
		result["retry_safe"] = false
		b, err := json.Marshal(result)
		if err != nil {
			slog.Error("join outcome encoding failed", "operation_id", result["operation_id"])
			return
		}
		// Persist even when cancellation interrupted waiting for the API gate.
		// A storage failure leaves the original running ledger for Recover on restart.
		saveCtx, cancelSave := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelSave()
		if _, err = j.Store.DB.ExecContext(saveCtx, "UPDATE join_requests SET payload=? WHERE operation_id=?", string(b), result["operation_id"]); err != nil {
			slog.Error("join outcome persistence failed", "operation_id", result["operation_id"])
		}
	}()
	if e := j.enter(ctx); e != nil {
		result["error"] = map[string]any{"code": "OPERATION_INTERRUPTED", "message": "Join operation was interrupted.", "next_action": domain.NextAction{Instruction: "Check collector status and membership; do not send another join automatically."}}
		return
	}
	defer j.leave()
	groups, e := j.API.Groups(ctx)
	member := false
	if e == nil {
		for _, g := range groups {
			if g.ID == p.GroupID {
				member = true
				break
			}
		}
	}
	if member {
		status = "already_member"
	} else if e == nil {
		err := j.API.Join(ctx, p.URL)
		checkCtx, stop := context.WithTimeout(j.ctx, 15*time.Second)
		after, checkErr := j.API.Groups(checkCtx)
		stop()
		if checkErr == nil {
			for _, g := range after {
				if g.ID == p.GroupID {
					status = "joined"
					break
				}
			}
		}
		// Confirmed membership takes precedence over the join response, since an
		// administrator may approve immediately. Only an explicit upstream signal
		// establishes pending; an invite flag or opaque success string does not.
		if status == "unknown" && errors.Is(err, domain.ErrJoinPendingApproval) {
			status = "pending_approval"
		}
		if err != nil && status == "unknown" {
			result["error"] = map[string]any{"code": "UPSTREAM_UNAVAILABLE", "message": "Join outcome could not be confirmed.", "next_action": domain.NextAction{Instruction: "Check membership in Zalo; do not send another join automatically."}}
		}
	}
}
func (j *JoinManager) Recover(ctx context.Context) error {
	rows, e := j.Store.DB.QueryContext(ctx, "SELECT operation_id,payload FROM join_requests")
	if e != nil {
		return e
	}
	results := []map[string]any{}
	for rows.Next() {
		var id, b string
		if e = rows.Scan(&id, &b); e != nil {
			rows.Close()
			return e
		}
		v := map[string]any{}
		if e = json.Unmarshal([]byte(b), &v); e != nil {
			rows.Close()
			return e
		}
		if v["status"] == "running" {
			v["status"] = "unknown"
			v["poll_after_ms"] = nil
			v["retry_safe"] = false
			results = append(results, v)
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, v := range results {
		b, e := json.Marshal(v)
		if e != nil {
			return e
		}
		if _, e = j.Store.DB.ExecContext(ctx, "UPDATE join_requests SET payload=? WHERE operation_id=?", string(b), v["operation_id"]); e != nil {
			return e
		}
	}
	return nil
}

func (j *JoinManager) enter(ctx context.Context) error {
	select {
	case j.gate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (j *JoinManager) leave() { <-j.gate }

// Reconcile confirms existing operations from a successfully fetched membership
// catalog. Absence from that catalog never proves rejection and never triggers Join.
func (j *JoinManager) Reconcile(ctx context.Context, groups []domain.Group) error {
	members := make(map[string]bool, len(groups))
	for _, g := range groups {
		members[g.ID] = true
	}
	rows, err := j.Store.DB.QueryContext(ctx, "SELECT operation_id,payload FROM join_requests WHERE json_extract(payload,'$.status') IN ('unknown','pending_approval')")
	if err != nil {
		return err
	}
	type change struct{ id, old, payload string }
	changes := []change{}
	for rows.Next() {
		var id, raw string
		if err = rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return err
		}
		value := map[string]any{}
		if err = json.Unmarshal([]byte(raw), &value); err != nil {
			rows.Close()
			return err
		}
		groupID, ok := value["group_id"].(string)
		if !ok || !members[groupID] {
			continue
		}
		value["status"] = "joined"
		value["checked_at"] = time.Now().UTC().Format(time.RFC3339Nano)
		value["poll_after_ms"] = nil
		value["error"] = nil
		value["retry_safe"] = false
		value["collection_enabled"] = j.Store.Allowed(groupID)
		encoded, e := json.Marshal(value)
		if e != nil {
			rows.Close()
			return e
		}
		changes = append(changes, change{id, raw, string(encoded)})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	tx, err := j.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, c := range changes {
		if _, err = tx.ExecContext(ctx, "UPDATE join_requests SET payload=? WHERE operation_id=? AND payload=?", c.payload, c.id, c.old); err != nil {
			return err
		}
	}
	return tx.Commit()
}
