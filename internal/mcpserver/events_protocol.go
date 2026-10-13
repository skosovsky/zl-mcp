package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
)

// EventsProtocolPort preserves the native extension framing. It is separate
// from application tools and supplied by the service composition root.
type EventsProtocolPort interface {
	Call(context.Context, string, string, json.RawMessage) (any, error)
}

type protocolError struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

func (e *protocolError) Error() string { return e.Message }
func (e *protocolError) RPCDetails() (int, string, map[string]any) {
	return e.Code, e.Message, e.Data
}

func projectProtocolError(err error) *protocolError {
	var details interface {
		RPCDetails() (int, string, map[string]any)
	}
	if errors.As(err, &details) {
		code, message, data := details.RPCDetails()
		return &protocolError{Code: code, Message: message, Data: data}
	}
	//nolint:exhaustruct_v5 // Protocol error details remain absent for this error code; preserve the existing JSON-RPC response.
	return &protocolError{Code: -32603, Message: "Internal server error."}
}
