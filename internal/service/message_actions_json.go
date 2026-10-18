package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

// Probe arguments include a bounded target array; the older mobile-ledger
// recognizer deliberately accepts objects only and must retain that grammar.
func uniqueActionProbeJSON(body []byte) bool {
	if !utf8.Valid(body) || len(body) > maximumOwnerRequestBytes {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if uniqueActionValue(decoder, 0) != nil {
		return false
	}
	_, err := decoder.Token()
	return errors.Is(err, io.EOF)
}

func uniqueActionValue(decoder *json.Decoder, depth int) error {
	if depth > maximumLedgerJSONDepth {
		return errors.New("action JSON nesting exceeds bounds")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		err = uniqueActionObject(decoder, depth)
	case '[':
		err = uniqueActionArray(decoder, depth)

	default:
		return errors.New("invalid action JSON delimiter")
	}
	if err != nil {
		return err
	}
	end, err := decoder.Token()
	if err != nil {
		return err
	}
	closing := json.Delim('}')
	if delimiter == '[' {
		closing = ']'
	}
	if end != closing {
		return errors.New("invalid action JSON ending")
	}
	return nil
}

func uniqueActionObject(decoder *json.Decoder, depth int) error {
	keys := map[string]bool{}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return err
		}
		name, ok := key.(string)
		if !ok || keys[name] {
			return errors.New("duplicate action JSON key")
		}
		keys[name] = true
		if err = uniqueActionValue(decoder, depth+1); err != nil {
			return err
		}
	}
	return nil
}

const maximumProbeMessageTargets = 50

func uniqueActionArray(decoder *json.Decoder, depth int) error {
	count := 0
	for decoder.More() {
		count++
		if count > maximumProbeMessageTargets {
			return errors.New("action target array exceeds bounds")
		}
		if err := uniqueActionValue(decoder, depth+1); err != nil {
			return err
		}
	}
	return nil
}
