package api

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/amrakk/zcago/errs"
	"github.com/amrakk/zcago/internal/cryptox"
	"github.com/amrakk/zcago/internal/httpx"
	"github.com/amrakk/zcago/internal/jsonx"
	"github.com/amrakk/zcago/session"
)

var ErrMobileBackupUnavailable = errors.New("mobile backup service unavailable")
var ErrMobileBackupInput = errors.New("invalid mobile backup request")
var ErrMobileBackupUnknown = errors.New("mobile backup request result unknown")
var ErrMobileBackupRejected = errors.New("mobile backup request rejected")

type MobileBackupRequestFn func(context.Context, string) error

// RequestMobileBackup requests only an initial transfer, never automatic retry.
func (a *api) RequestMobileBackup(ctx context.Context, publicKey string) error {
	fn, err := mobileBackupFactory(false)(a.sc, a)
	if err != nil {
		return err
	}
	return fn(ctx, publicKey)
}
func (a *api) CancelMobileBackup(ctx context.Context, publicKey string) error {
	fn, err := mobileBackupFactory(true)(a.sc, a)
	if err != nil {
		return err
	}
	return fn(ctx, publicKey)
}

func mobileBackupFactory(cancel bool) endpointFactory[json.RawMessage, MobileBackupRequestFn] {
	return apiFactory[json.RawMessage, MobileBackupRequestFn]()(func(a *api, sc session.Context, u factoryUtils[json.RawMessage]) (MobileBackupRequestFn, error) {
		base := jsonx.FirstOr(sc.GetZpwService("file"), "")
		if base == "" {
			return nil, ErrMobileBackupUnavailable
		}
		path := "/api/message/pull_mobile_msg"
		if cancel {
			path = "/api/message/cancel_pull_mobile_msg"
		}
		serviceURL := u.MakeURL(strings.TrimSuffix(base, "/")+path, nil, true)
		return func(ctx context.Context, publicKey string) error {
			if ctx == nil || !validMobileBackupKey(publicKey) || sc.IMEI() == "" || sc.UID() == "" {
				return ErrMobileBackupInput
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			ctx, stop := context.WithTimeout(ctx, 10*time.Second)
			defer stop()
			params := map[string]any{"pc_name": "Web", "public_key": publicKey, "imei": sc.IMEI()}
			if !cancel {
				params["from_seq_id"] = 0
				params["is_retry"] = 0
				params["min_seq_id"] = 0
				params["temp_key"] = ""
			}
			encrypted, err := u.EncodeAES(jsonx.Stringify(params))
			if err != nil {
				return ErrMobileBackupInput
			}
			response, err := httpx.RequestOnce(ctx, a.sc, u.MakeURL(serviceURL, map[string]any{"params": encrypted, "nretry": 0}, true), &httpx.RequestOptions{Method: http.MethodGet})
			if err != nil {
				return ErrMobileBackupUnknown
			}
			defer response.Body.Close()
			if response.StatusCode == http.StatusUnauthorized {
				return errs.ErrAuthenticationRequired
			}
			if response.StatusCode >= 400 && response.StatusCode < 500 {
				return ErrMobileBackupRejected
			}
			if response.StatusCode < 200 || response.StatusCode >= 300 {
				return ErrMobileBackupUnknown
			}
			const limit = 64 << 10
			wire, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
			if err != nil || len(wire) > limit {
				return ErrMobileBackupUnknown
			}
			response.Body = io.NopCloser(bytes.NewReader(wire))
			decoded, err := httpx.DecodeResponse(response)
			if err != nil {
				return ErrMobileBackupUnknown
			}
			defer decoded.Close()
			body, err := io.ReadAll(io.LimitReader(decoded, limit+1))
			if err != nil || len(body) > limit {
				return ErrMobileBackupUnknown
			}
			return mobileBackupAcknowledgement(body, sc.SecretKey().Bytes())
		}, nil
	})
}

func mobileBackupAcknowledgement(body, sessionKey []byte) error {
	var envelope struct {
		Code *int    `json:"error_code"`
		Data *string `json:"data"`
	}
	if json.Unmarshal(body, &envelope) != nil || envelope.Code == nil {
		return ErrMobileBackupUnknown
	}
	if *envelope.Code != 0 {
		code := errs.ZaloErrorCode(*envelope.Code)
		return errs.NewZaloAPIError("mobile backup request rejected", &code)
	}
	if envelope.Data == nil || *envelope.Data == "" {
		return nil
	}
	plain, err := cryptox.DecodeAESCBC(sessionKey, *envelope.Data)
	if err != nil {
		return ErrMobileBackupUnknown
	}
	defer clear(plain)
	var inner struct {
		Code *int `json:"error_code"`
	}
	if json.Unmarshal(plain, &inner) != nil || inner.Code == nil {
		return ErrMobileBackupUnknown
	}
	if *inner.Code != 0 {
		code := errs.ZaloErrorCode(*inner.Code)
		return errs.NewZaloAPIError("mobile backup request rejected", &code)
	}
	return nil
}

func validMobileBackupKey(text string) bool {
	if text == "" || len(text) > 1024 {
		return false
	}
	der, err := base64.StdEncoding.Strict().DecodeString(text)
	if err != nil || base64.StdEncoding.EncodeToString(der) != text {
		return false
	}
	key, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return false
	}
	pub, ok := key.(*rsa.PublicKey)
	return ok && pub.N.BitLen() == 2048 && pub.E == 65537
}
