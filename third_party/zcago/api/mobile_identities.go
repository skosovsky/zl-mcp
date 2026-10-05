package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/amrakk/zcago/errs"
	"github.com/amrakk/zcago/internal/cryptox"
	"github.com/amrakk/zcago/internal/httpx"
	"github.com/amrakk/zcago/session"
)

var ErrMobileIdentities = errors.New("mobile identity mapping failed")

type mobileIdentitiesFn func(context.Context, []string, []string) ([]byte, error)

// GetMobileIdentityMapping returns bounded unwrapped mapping data. Callers must
// validate typed positional identities before using it; no fallback is provided.
func (a *api) GetMobileIdentityMapping(ctx context.Context, direct, groups []string) ([]byte, error) {
	fn, err := mobileIdentitiesFactory("https://zwid.api.zalo.me/api/znoise")(a.sc, a)
	if err != nil {
		return nil, ErrMobileIdentities
	}
	return fn(ctx, direct, groups)
}

func mobileIdentitiesPayload(direct, groups []string) ([]byte, error) {
	if len(direct)+len(groups) == 0 || len(direct)+len(groups) > 1000 {
		return nil, ErrMobileIdentities
	}
	payload := make(map[string][]json.Number)
	for category, ids := range [][]string{direct, groups} {
		if len(ids) == 0 {
			continue
		}
		values := make([]json.Number, len(ids))
		seen := make(map[string]bool, len(ids))
		for i, id := range ids {
			if len(id) == 0 || len(id) > 20 || id[0] == '0' || seen[id] {
				return nil, ErrMobileIdentities
			}
			for _, c := range id {
				if c < '0' || c > '9' {
					return nil, ErrMobileIdentities
				}
			}
			if _, err := strconv.ParseUint(id, 10, 64); err != nil {
				return nil, ErrMobileIdentities
			}
			seen[id] = true
			values[i] = json.Number(id)
		}
		name := "fids"
		if category == 1 {
			name = "gids"
		}
		payload[name] = values
	}
	return json.Marshal(payload)
}

func mobileIdentitiesFactory(base string) endpointFactory[json.RawMessage, mobileIdentitiesFn] {
	return apiFactory[json.RawMessage, mobileIdentitiesFn]()(func(a *api, sc session.Context, u factoryUtils[json.RawMessage]) (mobileIdentitiesFn, error) {
		return func(ctx context.Context, direct, groups []string) ([]byte, error) {
			if ctx == nil || ctx.Err() != nil || sc.UID() == "" || sc.IMEI() == "" {
				return nil, ErrMobileIdentities
			}
			payload, err := mobileIdentitiesPayload(direct, groups)
			if err != nil {
				return nil, err
			}
			encrypted, err := u.EncodeAES(string(payload))
			clear(payload)
			if err != nil {
				return nil, ErrMobileIdentities
			}
			ctx, stop := context.WithTimeout(ctx, 10*time.Second)
			defer stop()
			response, err := httpx.RequestOnce(ctx, a.sc, u.MakeURL(base, map[string]any{"nretry": 0}, true), &httpx.RequestOptions{Method: http.MethodPost, Body: httpx.BuildFormBody(map[string]string{"params": encrypted})})
			if err != nil {
				return nil, ErrMobileIdentities
			}
			defer response.Body.Close()
			if response.StatusCode == http.StatusUnauthorized {
				return nil, errs.ErrAuthenticationRequired
			}
			if response.StatusCode != http.StatusOK {
				return nil, ErrMobileIdentities
			}
			const wireLimit = 512 << 10
			wire, err := io.ReadAll(io.LimitReader(response.Body, wireLimit+1))
			defer clear(wire)
			if err != nil || len(wire) > wireLimit {
				return nil, ErrMobileIdentities
			}
			response.Body = io.NopCloser(bytes.NewReader(wire))
			decoded, err := httpx.DecodeResponse(response)
			if err != nil {
				return nil, ErrMobileIdentities
			}
			defer decoded.Close()
			body, err := io.ReadAll(io.LimitReader(decoded, wireLimit+1))
			defer clear(body)
			if err != nil || len(body) > wireLimit {
				return nil, ErrMobileIdentities
			}
			code, encryptedData, err := mobileIdentityEnvelope(body)
			if err != nil || code != 0 {
				return nil, ErrMobileIdentities
			}
			var cipher string
			if json.Unmarshal(encryptedData, &cipher) != nil || cipher == "" {
				return nil, ErrMobileIdentities
			}

			plain, err := cryptox.DecodeAESCBC(sc.SecretKey().Bytes(), cipher)
			defer clear(plain)
			if err != nil || len(plain) > 256<<10 {
				return nil, ErrMobileIdentities
			}
			code, mapping, err := mobileIdentityEnvelope(plain)
			if err != nil || code != 0 || len(mapping) == 0 || bytes.Equal(mapping, []byte("null")) || ctx.Err() != nil {
				return nil, ErrMobileIdentities
			}
			return append([]byte(nil), mapping...), nil

		}, nil
	})
}

// Reject duplicate envelope fields instead of silently accepting the last code/data.
func mobileIdentityEnvelope(data []byte) (int, json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return 0, nil, ErrMobileIdentities
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err = decoder.Token()
		name, ok := token.(string)
		if err != nil || !ok || len(fields) >= 16 {
			return 0, nil, ErrMobileIdentities
		}
		if _, exists := fields[name]; exists {
			return 0, nil, ErrMobileIdentities
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return 0, nil, ErrMobileIdentities
		}
		fields[name] = value
	}
	token, err = decoder.Token()
	if err != nil || token != json.Delim('}') {
		return 0, nil, ErrMobileIdentities
	}
	if _, err = decoder.Token(); err != io.EOF {
		return 0, nil, ErrMobileIdentities
	}
	var code *int
	if json.Unmarshal(fields["error_code"], &code) != nil || code == nil {
		return 0, nil, ErrMobileIdentities
	}
	return *code, fields["data"], nil
}
