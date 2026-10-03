// Package zalo is the only package exposing zcago types to the upstream protocol.
package zalo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/amrakk/zcago/errs"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/amrakk/zcago"
	"github.com/amrakk/zcago/model"
	"github.com/amrakk/zcago/session/auth"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/local"
)

type Client struct {
	api zcago.API
	jar *persistentJar
}

type savedSession struct {
	Version     int               `json:"version"`
	Credentials zcago.Credentials `json:"credentials"`
	Cookies     []savedCookie     `json:"cookies"`
}

func engine(jar *persistentJar) zcago.Zalo {
	return zcago.NewZalo(zcago.WithLogging(false), zcago.WithCheckUpdate(false), zcago.WithSelfListen(true), zcago.WithHTTPClient(&http.Client{Timeout: 30 * time.Second, Jar: jar}))
}
func Login(ctx context.Context, dir string, out io.Writer) (*Client, error) {
	qr := filepath.Join(dir, "qr.png")
	defer os.Remove(qr)
	var callbackErr error
	jar := newPersistentJar()
	a, err := engine(jar).LoginQR(ctx, &zcago.LoginQROption{}, func(e auth.LoginQREvent) {
		switch v := e.(type) {
		case auth.EventQRCodeGenerated:
			if callbackErr = v.Actions.SaveToFile(ctx, qr); callbackErr != nil {
				_ = v.Actions.Abort(ctx)
				return
			}
			fmt.Fprintf(out, "Scan the QR in the Zalo mobile app: %s\n", qr)
		case auth.EventQRCodeExpired:
			fmt.Fprintln(out, "QR expired. Restart login to create another QR.")
			_ = v.Actions.Abort(ctx)
		case auth.EventQRCodeScanned:
			fmt.Fprintln(out, "QR scanned; confirm login in the mobile app.")
		}
	})
	if callbackErr != nil {
		return nil, fmt.Errorf("could not save private QR image")
	}
	if err != nil {
		return nil, fmt.Errorf("QR login failed; check connectivity and mobile confirmation")
	}
	c := &Client{api: a, jar: jar}
	return c, nil
}
func Restore(ctx context.Context, dir string) (*Client, error) {
	return restoreWithLogin(ctx, dir, func(ctx context.Context, jar *persistentJar, credentials zcago.Credentials) (zcago.API, error) {
		return engine(jar).Login(ctx, credentials)
	})
}

func restoreWithLogin(ctx context.Context, dir string, login func(context.Context, *persistentJar, zcago.Credentials) (zcago.API, error)) (*Client, error) {
	b, e := local.ReadPrivate(filepath.Join(dir, "session.json"))
	if e != nil {
		if errors.Is(e, os.ErrNotExist) {
			return nil, domain.ErrAuthenticationRequired
		}
		return nil, &domain.Error{Code: "STORAGE_ERROR", Message: "Cannot read private saved session.", NextAction: domain.NextAction{Instruction: "Check state directory ownership and private file permissions."}, Details: map[string]any{}}
	}
	var saved savedSession
	if json.Unmarshal(b, &saved) != nil || saved.Version != 2 || len(saved.Cookies) == 0 || !saved.Credentials.IsValid() {
		return nil, domain.ErrAuthenticationRequired
	}
	jar := newPersistentJar()
	for _, entry := range saved.Cookies {
		u, err := url.Parse(entry.Origin)
		if err != nil || u.Scheme != "https" || (u.Hostname() != "zalo.me" && !strings.HasSuffix(u.Hostname(), ".zalo.me")) {
			return nil, domain.ErrAuthenticationRequired
		}
		jar.SetCookies(u, []*http.Cookie{&entry.Cookie})
	}
	saved.Credentials.Cookie = nil
	a, e := login(ctx, jar, saved.Credentials)
	if e != nil {
		logStructuralFailure("session restore upstream failure", e)
		if restoreAuthenticationFailure(e) {
			return nil, domain.ErrAuthenticationRequired
		}
		return nil, &domain.Error{Code: "UPSTREAM_UNAVAILABLE", Message: "Saved session could not be verified.", Retryable: true, NextAction: domain.NextAction{Instruction: "Check connectivity and retry collection; use local login only if authentication is rejected."}, Details: map[string]any{}}
	}
	return &Client{api: a, jar: jar}, nil
}

func restoreAuthenticationFailure(err error) bool {
	if errors.Is(err, errs.ErrAuthenticationRequired) {
		return true
	}
	var value errs.ZaloAPIError
	var pointer *errs.ZaloAPIError
	var code *errs.ZaloErrorCode
	if errors.As(err, &value) {
		code = value.Code
	} else if errors.As(err, &pointer) && pointer != nil {
		code = pointer.Code
	}
	// Scoped to saved-session login: code 102 was reproduced after live logout.
	// Do not apply this interpretation to unrelated Zalo endpoints or raw text.
	return code != nil && int(*code) == 102
}
func (c *Client) Save(dir string) error {
	sc, e := c.api.GetContext()
	if e != nil {
		return fmt.Errorf("cannot read authenticated session")
	}
	if c.jar == nil {
		return fmt.Errorf("session cookie scope unavailable")
	}
	language := sc.Language()
	cred := zcago.Credentials{IMEI: sc.IMEI(), UserAgent: sc.UserAgent(), Language: &language}
	b, e := json.Marshal(savedSession{Version: 2, Credentials: cred, Cookies: c.jar.snapshot()})
	if e != nil {
		return fmt.Errorf("cannot serialize session")
	}
	return local.WritePrivate(filepath.Join(dir, "session.json"), b)
}
func (c *Client) AccountID() string { return c.api.GetOwnID() }
func (c *Client) Groups(ctx context.Context) ([]domain.Group, error) {
	all, e := c.api.GetAllGroups(ctx)
	if e != nil {
		return nil, e
	}
	ids := []string{}
	for id := range all.GridVerMap {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	groups := []domain.Group{}
	for start := 0; start < len(ids); start += 50 {
		end := min(start+50, len(ids))
		resp, e := c.api.GetGroupInfo(ctx, ids[start:end]...)
		if e != nil {
			return nil, e
		}
		for _, id := range ids[start:end] {
			if g, ok := resp.GridInfoMap[id]; ok {
				count := g.TotalMember
				groups = append(groups, domain.Group{ID: id, Name: g.Name, MemberCount: &count})
			}
		}
	}
	return groups, nil
}
func (c *Client) Group(ctx context.Context, id string) (domain.Group, *string, error) {
	r, e := c.api.GetGroupInfo(ctx, id)
	if e != nil {
		return domain.Group{}, nil, e
	}
	g, ok := r.GridInfoMap[id]
	if !ok {
		return domain.Group{}, nil, fmt.Errorf("group unavailable")
	}
	n := g.TotalMember
	d := g.Description
	return domain.Group{ID: id, Name: g.Name, MemberCount: &n}, &d, nil
}
func (c *Client) Inspect(ctx context.Context, link string) (domain.Invite, error) {
	r, e := c.api.GetGroupLinkInfo(ctx, link, 1)
	if e != nil {
		return domain.Invite{}, e
	}
	if r == nil || r.GroupID == "" {
		return domain.Invite{}, fmt.Errorf("invitation response has no group ID")
	}
	n := r.TotalMember
	d := r.Desc
	return domain.Invite{Group: domain.Group{ID: r.GroupID, Name: r.Name, MemberCount: &n}, Description: &d, ApprovalRequired: r.Setting.JoinAppr == 1}, nil
}
func (c *Client) Join(ctx context.Context, link string) error {
	_, e := c.api.JoinGroupLink(ctx, link)
	logJoinFailure(e)
	return classifyJoinFailure(e)
}

func classifyJoinFailure(err error) error {
	var value errs.ZaloAPIError
	var pointer *errs.ZaloAPIError
	var code *errs.ZaloErrorCode
	if errors.As(err, &value) {
		code = value.Code
	} else if errors.As(err, &pointer) && pointer != nil {
		code = pointer.Code
	}
	// zcago documents 240 for moderated joins; the live Test request reproduced
	// this exact typed code on 2026-10-01. Do not interpret text or invite flags.
	if code != nil && int(*code) == 240 {
		return domain.ErrJoinPendingApproval
	}
	return err
}

// logJoinFailure records only protocol structure. Error strings, JSON values,
// URLs and response bodies can contain credentials or account data.
func logJoinFailure(err error) {
	logStructuralFailure("join upstream failure", err)
}

func logStructuralFailure(message string, err error) {
	if err == nil {
		return
	}
	kind := "unclassified"
	var code *errs.ZaloErrorCode
	var apiErr errs.ZaloAPIError
	var apiErrPointer *errs.ZaloAPIError
	var decodeErr *json.UnmarshalTypeError
	var syntaxErr *json.SyntaxError
	var transportErr *url.Error
	switch {
	case errors.As(err, &apiErr):
		kind, code = "api", apiErr.Code
	case errors.As(err, &apiErrPointer) && apiErrPointer != nil:
		kind, code = "api", apiErrPointer.Code
	case errors.As(err, &decodeErr), errors.As(err, &syntaxErr):
		kind = "decode"
	case errors.As(err, &transportErr):
		kind = "transport"
	case errors.Is(err, context.Canceled):
		kind = "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		kind = "deadline"
	}
	attrs := []any{"failure_kind", kind}
	if code != nil {
		attrs = append(attrs, "upstream_code", int(*code))
	}
	slog.Warn(message, attrs...)
}
func Convert(m model.GroupMessage, source string) (domain.Message, error) {
	return convertMessage(m.Data.TMessage, domain.ConversationRef{Type: domain.ConversationGroup, ID: m.ThreadID()}, source)
}

func ConvertDirect(m model.UserMessage, source string) (domain.Message, error) {
	return convertMessage(m.Data, domain.ConversationRef{Type: domain.ConversationDirect, ID: m.ThreadID()}, source)
}

func convertMessage(d model.TMessage, ref domain.ConversationRef, source string) (domain.Message, error) {
	ms, e := strconv.ParseInt(d.TS, 10, 64)
	if e != nil || ms <= 0 {
		return domain.Message{}, fmt.Errorf("unsupported Zalo message timestamp")
	}
	msg := domain.Message{Conversation: ref, ID: d.MsgID, SenderID: d.UIDFrom, SentAt: time.UnixMilli(ms).UTC(), AttachmentTypes: []string{}, Source: source}
	if ref.Type == domain.ConversationGroup {
		msg.GroupID = ref.ID
	}
	if d.DName != "" {
		name := d.DName
		msg.SenderName = &name
	}
	if d.Content.String != nil {
		msg.Text = *d.Content.String
	} else if d.Content.Attachment != nil {
		msg.Text = d.Content.Attachment.Title
		msg.AttachmentTypes = append(msg.AttachmentTypes, d.MsgType)
	}
	if d.Quote != nil && d.Quote.GlobalMsgID != 0 {
		reply := strconv.FormatInt(d.Quote.GlobalMsgID, 10)
		msg.ReplyTo = &reply
	}
	if msg.ID == "" || !msg.Ref().Valid() || msg.SenderID == "" {
		return domain.Message{}, fmt.Errorf("unsupported Zalo message identifiers")
	}
	return msg, nil
}
