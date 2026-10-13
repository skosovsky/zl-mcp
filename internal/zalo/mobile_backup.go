package zalo

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"strconv"
	"strings"

	"github.com/amrakk/zcago/api"
	"github.com/amrakk/zcago/errs"
	"github.com/amrakk/zcago/listener"
	"github.com/amrakk/zcago/model"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

type mobileBackupRequester interface {
	RequestMobileBackup(context.Context, string) error
}
type mobileBackupReceiver interface {
	SubscribeMobileSync(string, string) (listener.MobileSyncSubscription, func(), error)
}

func (c *Client) ReceiveMobileBackupOffer(
	ctx context.Context,
	progress *domain.MobileBackupObserver,
) (domain.MobileBackupOffer, error) {
	r, ok := c.api.(mobileBackupRequester)
	if !ok {
		return domain.MobileBackupOffer{}, domain.ErrHistoryUnsupported
	}
	l, ok := c.api.Listener().(mobileBackupReceiver)
	if !ok {
		return domain.MobileBackupOffer{}, domain.ErrHistoryUnsupported
	}
	owner := c.AccountID()
	offer, err := receiveMobileBackupOffer(
		ctx,
		owner,
		r,
		l,
		progress,
		func() (*rsa.PrivateKey, error) { return rsa.GenerateKey(rand.Reader, mobileOfferRSAKeyBits) },
	)
	if c.AccountID() != owner {
		return domain.MobileBackupOffer{}, domain.ErrMobileBackupInvalid
	}
	return offer, err
}

func receiveMobileBackupOffer(
	parent context.Context,
	owner string,
	r mobileBackupRequester,
	l mobileBackupReceiver,
	progress *domain.MobileBackupObserver,
	generate func() (*rsa.PrivateKey, error),
) (domain.MobileBackupOffer, error) {
	if parent == nil || owner == "" {
		return domain.MobileBackupOffer{}, domain.ErrMobileBackupInvalid
	}
	ctx, stop := context.WithTimeout(parent, receiveMobileBackupOfferTimeout)
	defer stop()
	if ctx.Err() != nil {
		return domain.MobileBackupOffer{}, ctx.Err()
	}
	mapper, ok := r.(mobileIdentityRequester)
	if !ok {
		return domain.MobileBackupOffer{}, domain.ErrHistoryUnsupported
	}
	key, err := generate()
	if err != nil || key == nil || key.N == nil || key.D == nil || key.N.BitLen() != 2048 || key.E != 65537 {
		return domain.MobileBackupOffer{}, domain.ErrMobileBackupInvalid
	}
	if ctx.Err() != nil {
		return domain.MobileBackupOffer{}, ctx.Err()
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return domain.MobileBackupOffer{}, domain.ErrMobileBackupInvalid
	}
	public := base64.StdEncoding.EncodeToString(der)
	sub, unsubscribe, err := l.SubscribeMobileSync(public, nativeClientWeb)
	if err != nil {
		return domain.MobileBackupOffer{}, domain.ErrMobileBackupRejected
	}
	defer unsubscribe()
	state := func(s string) error {
		if progress != nil && progress.Progress != nil {
			return progress.Progress(s)
		}
		return nil
	}
	if progress != nil && progress.BeforeDispatch != nil {
		if beforeDispatchErr := progress.BeforeDispatch(public); beforeDispatchErr != nil {
			return domain.MobileBackupOffer{}, beforeDispatchErr
		}
	}
	if ctx.Err() != nil {
		return domain.MobileBackupOffer{}, ctx.Err()
	}
	unknown, dispatchErr := dispatchMobileRequest(ctx, r, public, state)
	if dispatchErr != nil {
		return domain.MobileBackupOffer{}, dispatchErr
	}
	session := mobileOfferSession{
		parent: parent,
		ctx:    ctx,
		owner:  owner,
		public: public,
		key:    key,
		mapper: mapper,
		sub:    sub,
		state:  state,
	}
	return session.wait(unknown)
}

func mobileBackupOffer(e model.MobileSyncEvent, owner string, key *rsa.PrivateKey) (domain.MobileBackupOffer, error) {
	u, err := url.Parse(e.URL)
	if err != nil || len(e.URL) > 4096 || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		u.Fragment != "" ||
		len(e.EncryptedKey) > 4096 ||
		!historyNumber(e.FromSequence) ||
		len(e.FromSequence) > 20 ||
		len(e.FromSequence) > 1 && e.FromSequence[0] == '0' {
		return invalidMobileOffer("OFFER_ENVELOPE")
	}
	if _, parseUintErr := strconv.ParseUint(e.FromSequence, 10, 64); parseUintErr != nil {
		return invalidMobileOffer("OFFER_SEQUENCE")
	}
	var info struct {
		Format *int `json:"db_format"`
	}
	uid, uidErr := strconv.ParseUint(e.UID, 10, 64)
	if uidErr != nil || uid == 0 || strconv.FormatUint(uid, 10) != e.UID || e.FileSize == 0 || e.FileSize > 512<<20 ||
		json.Unmarshal([]byte(e.DatabaseInfo), &info) != nil ||
		info.Format == nil {
		return invalidMobileOffer("OFFER_ACCOUNT_OR_DATABASE")
	}
	if *info.Format != 1 {
		return domain.MobileBackupOffer{}, domain.ErrHistoryUnsupported
	}
	ciphertext, err := base64.StdEncoding.Strict().DecodeString(e.EncryptedKey)
	if err != nil || len(ciphertext) != key.Size() {
		return invalidMobileOffer("OFFER_CIPHER_ENCODING")
	}
	//nolint:staticcheck // SA1019: the upstream offer protocol requires PKCS#1 v1.5; OAEP would reject compatible offers.
	plain, err := rsa.DecryptPKCS1v15(rand.Reader, key, ciphertext)
	if err != nil {
		return invalidMobileOffer("OFFER_RSA_DECRYPTION")
	}
	defer clear(plain)
	if len(plain) < 16 || len(plain) > 128 {
		return invalidMobileOffer("OFFER_KEY_LENGTH")
	}
	return domain.MobileBackupOffer{
		PlainAccountID:   e.UID,
		SessionAccountID: owner,
		URL:              e.URL,
		KeyText:          strings.ToUpper(hex.EncodeToString(plain)),
		FileSize:         e.FileSize,
		FromSequence:     e.FromSequence,
	}, nil
}

func invalidMobileOffer(reason string) (domain.MobileBackupOffer, error) {
	//nolint:sloglint // Use the service-configured redacting logger; fields remain a bounded allowlist without raw errors or native identities.
	slog.Warn("mobile_backup_offer_invalid", "reason", reason)
	return domain.MobileBackupOffer{}, domain.ErrMobileBackupInvalid
}

// Only explicitly selected scalar diagnostics; never format a control event.
func logMobileControl(event model.MobileSyncEvent, public string) {
	action := "unknown"
	switch event.Action {
	case "user_confirm", "transfer_error", "syncmsg_info":
		action = event.Action
	}
	args := []any{
		"action",
		action,
		"key_match",
		event.PublicKey == public,
		"pc_match",
		event.PCName == nativeClientWeb,
		"has_url",
		event.URL != "",
		"has_encrypted_key",
		event.EncryptedKey != "",
		"archive_bytes",
		event.FileSize,
		"has_db_info",
		event.DatabaseInfo != "",
	}
	if event.Status != nil {
		args = append(args, "status", *event.Status)
	}
	if event.ErrorCode != nil {
		args = append(args, "upstream_error_code", *event.ErrorCode)
	}
	if event.UserAction != nil {
		args = append(args, "user_action", *event.UserAction)
	}
	//nolint:sloglint // Use the service-configured redacting logger; fields remain a bounded allowlist without raw errors or native identities.
	slog.Info("mobile_backup_control", args...)
}

func dispatchMobileRequest(
	ctx context.Context,
	r mobileBackupRequester,
	public string,
	state func(string) error,
) (bool, error) {
	err := r.RequestMobileBackup(ctx, public)
	unknown := errors.Is(err, api.ErrMobileBackupUnknown)
	if err != nil && !unknown {
		if errors.Is(err, errs.ErrAuthenticationRequired) {
			return false, domain.ErrAuthenticationRequired
		}
		if errors.Is(err, api.ErrMobileBackupUnavailable) {
			return false, domain.ErrHistoryUnsupported
		}
		return false, domain.ErrMobileBackupRejected
	}
	if unknown {
		if err := state("request_result_unknown"); err != nil {
			return false, err
		}
	} else {
		if err := state("waiting_for_confirmation"); err != nil {
			return false, err
		}
	}
	return unknown, nil
}

type mobileOfferSession struct {
	parent, ctx   context.Context
	owner, public string
	key           *rsa.PrivateKey
	mapper        mobileIdentityRequester
	sub           listener.MobileSyncSubscription
	state         func(string) error
}

func (session *mobileOfferSession) wait(unknown bool) (domain.MobileBackupOffer, error) {
	for {
		select {
		case <-session.sub.Errors:
			//nolint:sloglint // Use the service-configured redacting logger; fields remain a bounded allowlist without raw errors or native identities.
			slog.WarnContext(session.parent, "mobile_backup_control_invalid", "stage", "receiver")
			return domain.MobileBackupOffer{}, domain.ErrMobileBackupInvalid
		default:
		}
		select {
		case <-session.ctx.Done():
			if unknown {
				return domain.MobileBackupOffer{}, domain.ErrMobileBackupUnknown
			}
			return domain.MobileBackupOffer{}, session.ctx.Err()
		case <-session.sub.Errors:
			//nolint:sloglint // Use the service-configured redacting logger; fields remain a bounded allowlist without raw errors or native identities.
			slog.WarnContext(session.parent, "mobile_backup_control_invalid", "stage", "receiver")
			return domain.MobileBackupOffer{}, domain.ErrMobileBackupInvalid
		case event := <-session.sub.Events:
			logMobileControl(event, session.public)
			if event.PublicKey != session.public {
				return domain.MobileBackupOffer{}, domain.ErrMobileBackupInvalid
			}
			offer, ready, controlErr := session.control(event)
			if controlErr != nil || ready {
				return offer, controlErr
			}
		}
	}
}

func (session *mobileOfferSession) control(event model.MobileSyncEvent) (domain.MobileBackupOffer, bool, error) {
	switch event.Action {
	case "user_confirm":
		return domain.MobileBackupOffer{}, false, session.confirm(event)
	case "transfer_error":
		return domain.MobileBackupOffer{}, false, session.transfer(event)
	case "syncmsg_info":
		offer, err := session.acceptOffer(event)
		return offer, err == nil, err
	default:
		return domain.MobileBackupOffer{}, false, domain.ErrMobileBackupInvalid
	}
}

func (session *mobileOfferSession) confirm(event model.MobileSyncEvent) error {
	if event.PCName != nativeClientWeb || event.UserAction == nil {
		return domain.ErrMobileBackupInvalid
	}
	switch *event.UserAction {
	case 0:
		return domain.ErrMobileBackupRejected
	case 2:
		if err := session.state("mobile_restoring"); err != nil {
			return err
		}
	//nolint:mnd // Native control IDs 1/3 map to waiting and 2 to restoring; retain the exact codec table.
	case 1, 3:
		if err := session.state("waiting_for_backup"); err != nil {
			return err
		}
	default:
		return domain.ErrMobileBackupInvalid
	}
	return nil
}

func (session *mobileOfferSession) transfer(event model.MobileSyncEvent) error {
	if event.PCName != nativeClientWeb {
		return domain.ErrMobileBackupInvalid
	}
	// The native client checks active/idle status before backup failure.
	// These controls are progress, despite the misleading action name.
	if event.Status != nil && (*event.Status == 1 || *event.Status == 2) {
		kind := "mobile_active"
		if *event.Status == 2 {
			kind = "mobile_idle"
		}
		//nolint:sloglint // Use the service-configured redacting logger; fields remain a bounded allowlist without raw errors or native identities.
		slog.InfoContext(session.parent, "mobile_backup_phone_status", "status", kind)
		return nil
	}
	if event.ErrorCode != nil && *event.ErrorCode != 0 {
		//nolint:sloglint // Use the service-configured redacting logger; fields remain a bounded allowlist without raw errors or native identities.
		slog.WarnContext(session.parent, "mobile_backup_transfer_rejected", "upstream_error_code", *event.ErrorCode)
		return domain.ErrMobileBackupRejected
	}
	if event.ErrorCode != nil && *event.ErrorCode == 0 {
		// Native handling does not classify a zero-code control as failure.
		// Retain the original deadline and wait for an explicit offer.
		//nolint:sloglint // Use the service-configured redacting logger; fields remain a bounded allowlist without raw errors or native identities.
		slog.InfoContext(session.parent, "mobile_backup_phone_status", "status", "transfer_no_error")
		return nil
	}
	return domain.ErrMobileBackupInvalid
}

func (session *mobileOfferSession) acceptOffer(event model.MobileSyncEvent) (domain.MobileBackupOffer, error) {
	offer, err := mobileBackupOffer(event, session.owner, session.key)
	if err != nil {
		return domain.MobileBackupOffer{}, err
	}
	pairs, err := mapMobileIdentities(
		session.ctx,
		//nolint:exhaustruct_v5 // MobileIdentityRequest preserves omitted optional DTO fields and zero counters in the existing wire/persistence projection.
		domain.MobileIdentityRequest{Direct: []string{event.UID}},
		session.mapper,
	)
	if err != nil {
		//nolint:sloglint // Use the service-configured redacting logger; fields remain a bounded allowlist without raw errors or native identities.
		slog.WarnContext(session.parent, "mobile_backup_account_invalid", "reason", "MAPPING_FAILED")
		return domain.MobileBackupOffer{}, err
	}
	if len(pairs) != 1 || pairs[0].Group || pairs[0].Plain != event.UID || pairs[0].Session != session.owner {
		//nolint:sloglint // Use the service-configured redacting logger; fields remain a bounded allowlist without raw errors or native identities.
		slog.WarnContext(session.parent, "mobile_backup_account_invalid", "reason", "MAPPED_OWNER_MISMATCH")
		return domain.MobileBackupOffer{}, domain.ErrMobileBackupInvalid
	}
	//nolint:sloglint // Use the service-configured redacting logger; fields remain a bounded allowlist without raw errors or native identities.
	slog.InfoContext(session.parent, "mobile_backup_account_verified")
	select {
	case <-session.sub.Errors:
		//nolint:sloglint // Use the service-configured redacting logger; fields remain a bounded allowlist without raw errors or native identities.
		slog.WarnContext(session.parent, "mobile_backup_control_invalid", "stage", "receiver")
		return domain.MobileBackupOffer{}, domain.ErrMobileBackupInvalid
	default:
	}
	if session.ctx.Err() != nil {
		return domain.MobileBackupOffer{}, session.ctx.Err()
	}
	if err := session.state("offer_ready"); err != nil {
		return domain.MobileBackupOffer{}, err
	}
	//nolint:sloglint // Use the service-configured redacting logger; fields remain a bounded allowlist without raw errors or native identities.
	slog.InfoContext(
		session.parent,
		"mobile_backup_producer_counts",
		"claims",
		inspectMobileProducerCounts(event.DatabaseInfo),
	)
	return offer, nil
}
