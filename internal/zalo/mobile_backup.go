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
	"time"

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

func (c *Client) ReceiveMobileBackupOffer(ctx context.Context, progress *domain.MobileBackupObserver) (domain.MobileBackupOffer, error) {
	r, ok := c.api.(mobileBackupRequester)
	if !ok {
		return domain.MobileBackupOffer{}, domain.ErrHistoryUnsupported
	}
	l, ok := c.api.Listener().(mobileBackupReceiver)
	if !ok {
		return domain.MobileBackupOffer{}, domain.ErrHistoryUnsupported
	}
	return receiveMobileBackupOffer(ctx, c.AccountID(), r, l, progress, func() (*rsa.PrivateKey, error) { return rsa.GenerateKey(rand.Reader, 2048) })
}

func receiveMobileBackupOffer(parent context.Context, owner string, r mobileBackupRequester, l mobileBackupReceiver, progress *domain.MobileBackupObserver, generate func() (*rsa.PrivateKey, error)) (domain.MobileBackupOffer, error) {
	if parent == nil || owner == "" {
		return domain.MobileBackupOffer{}, domain.ErrMobileBackupInvalid
	}
	ctx, stop := context.WithTimeout(parent, 180*time.Second)
	defer stop()
	if ctx.Err() != nil {
		return domain.MobileBackupOffer{}, ctx.Err()
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
	sub, unsubscribe, err := l.SubscribeMobileSync(public, "Web")
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
		if err := progress.BeforeDispatch(public); err != nil {
			return domain.MobileBackupOffer{}, err
		}
	}
	if ctx.Err() != nil {
		return domain.MobileBackupOffer{}, ctx.Err()
	}
	err = r.RequestMobileBackup(ctx, public)
	unknown := errors.Is(err, api.ErrMobileBackupUnknown)
	if err != nil && !unknown {
		if errors.Is(err, errs.ErrAuthenticationRequired) {
			return domain.MobileBackupOffer{}, domain.ErrAuthenticationRequired
		}
		if errors.Is(err, api.ErrMobileBackupUnavailable) {
			return domain.MobileBackupOffer{}, domain.ErrHistoryUnsupported
		}
		return domain.MobileBackupOffer{}, domain.ErrMobileBackupRejected
	}
	if unknown {
		if err := state("request_result_unknown"); err != nil {
			return domain.MobileBackupOffer{}, err
		}
	} else {
		if err := state("waiting_for_confirmation"); err != nil {
			return domain.MobileBackupOffer{}, err
		}
	}
	for {
		select {
		case <-sub.Errors:
			return domain.MobileBackupOffer{}, domain.ErrMobileBackupInvalid
		default:
		}
		select {
		case <-ctx.Done():
			if unknown {
				return domain.MobileBackupOffer{}, domain.ErrMobileBackupUnknown
			}
			return domain.MobileBackupOffer{}, ctx.Err()
		case <-sub.Errors:
			return domain.MobileBackupOffer{}, domain.ErrMobileBackupInvalid
		case event := <-sub.Events:
			if event.PublicKey != public {
				return domain.MobileBackupOffer{}, domain.ErrMobileBackupInvalid
			}
			switch event.Action {
			case "user_confirm":
				if event.PCName != "Web" || event.UserAction == nil {
					return domain.MobileBackupOffer{}, domain.ErrMobileBackupInvalid
				}
				switch *event.UserAction {
				case 0:
					return domain.MobileBackupOffer{}, domain.ErrMobileBackupRejected
				case 2:
					if err := state("mobile_restoring"); err != nil {
						return domain.MobileBackupOffer{}, err
					}
				case 1, 3:
					if err := state("waiting_for_backup"); err != nil {
						return domain.MobileBackupOffer{}, err
					}
				default:
					return domain.MobileBackupOffer{}, domain.ErrMobileBackupInvalid
				}
			case "transfer_error":
				if event.PCName != "Web" {
					return domain.MobileBackupOffer{}, domain.ErrMobileBackupInvalid
				}
				// The native client checks active/idle status before backup failure.
				// These controls are progress, despite the misleading action name.
				if event.Status != nil && (*event.Status == 1 || *event.Status == 2) {
					kind := "mobile_active"
					if *event.Status == 2 {
						kind = "mobile_idle"
					}
					slog.Info("mobile_backup_phone_status", "status", kind)
					continue
				}
				if event.ErrorCode != nil && *event.ErrorCode != 0 {
					slog.Warn("mobile_backup_transfer_rejected", "upstream_error_code", *event.ErrorCode)
					return domain.MobileBackupOffer{}, domain.ErrMobileBackupRejected
				}
				return domain.MobileBackupOffer{}, domain.ErrMobileBackupInvalid
			case "syncmsg_info":
				offer, err := mobileBackupOffer(event, owner, key)
				if err != nil {
					return domain.MobileBackupOffer{}, err
				}
				select {
				case <-sub.Errors:
					return domain.MobileBackupOffer{}, domain.ErrMobileBackupInvalid
				default:
				}
				if ctx.Err() != nil {
					return domain.MobileBackupOffer{}, ctx.Err()
				}
				if err := state("offer_ready"); err != nil {
					return domain.MobileBackupOffer{}, err
				}
				return offer, nil
			default:
				return domain.MobileBackupOffer{}, domain.ErrMobileBackupInvalid
			}
		}
	}
}

func mobileBackupOffer(e model.MobileSyncEvent, owner string, key *rsa.PrivateKey) (domain.MobileBackupOffer, error) {
	u, err := url.Parse(e.URL)
	if err != nil || len(e.URL) > 4096 || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || len(e.EncryptedKey) > 4096 || !historyNumber(e.FromSequence) || len(e.FromSequence) > 20 || len(e.FromSequence) > 1 && e.FromSequence[0] == '0' {
		return domain.MobileBackupOffer{}, domain.ErrMobileBackupInvalid
	}
	if _, err := strconv.ParseUint(e.FromSequence, 10, 64); err != nil {
		return domain.MobileBackupOffer{}, domain.ErrMobileBackupInvalid
	}
	var info struct {
		Format *int `json:"db_format"`
	}
	if e.UID != owner || e.FileSize == 0 || e.FileSize > 512<<20 || json.Unmarshal([]byte(e.DatabaseInfo), &info) != nil || info.Format == nil {
		return domain.MobileBackupOffer{}, domain.ErrMobileBackupInvalid
	}
	if *info.Format != 1 {
		return domain.MobileBackupOffer{}, domain.ErrHistoryUnsupported
	}
	ciphertext, err := base64.StdEncoding.Strict().DecodeString(e.EncryptedKey)
	if err != nil || len(ciphertext) != key.Size() {
		return domain.MobileBackupOffer{}, domain.ErrMobileBackupInvalid
	}
	plain, err := rsa.DecryptPKCS1v15(rand.Reader, key, ciphertext)
	if err != nil {
		return domain.MobileBackupOffer{}, domain.ErrMobileBackupInvalid
	}
	defer clear(plain)
	if len(plain) < 16 || len(plain) > 128 {
		return domain.MobileBackupOffer{}, domain.ErrMobileBackupInvalid
	}
	return domain.MobileBackupOffer{URL: e.URL, KeyText: strings.ToUpper(hex.EncodeToString(plain)), FileSize: e.FileSize, FromSequence: e.FromSequence}, nil
}
