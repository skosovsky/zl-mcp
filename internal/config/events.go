package config

import (
	"errors"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/netpolicy"
)

func (c Config) validateEvents() error {
	schema, err := contracts.Compile("service_events", "input")
	if err != nil {
		return err
	}
	urls := make([]any, 0, len(c.Events.LocalCallbackURLs))
	for _, raw := range c.Events.LocalCallbackURLs {
		if _, err = netpolicy.LocalCallbackURL(raw); err != nil {
			return errors.New("invalid events.local_callback_urls")
		}
		urls = append(urls, raw)
	}
	if err = schema.Validate(map[string]any{"local_callback_urls": urls}); err != nil {
		return errors.New("invalid events.local_callback_urls")
	}
	return nil
}
