package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

type stdioByteObservation struct {
	queueObservation
	bytes      []int64
	tools      []string
	transports []domain.Transport
}

func (o *stdioByteObservation) ResponseBytes(_ context.Context, tool string, transport domain.Transport, size int64) {
	o.bytes = append(o.bytes, size)
	o.tools = append(o.tools, tool)
	o.transports = append(o.transports, transport)
}
func TestBorrowedSTDIOObservationCountsSDKFrameIncludingNewline(t *testing.T) {
	// Arrange: real SDK IO framing with opaque string ID, Unicode, escaping and no second provider.
	observer := &stdioByteObservation{queueObservation: queueObservation{recording: true}}
	var output bytes.Buffer
	transport := &observedStdioTransport{reader: io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":"9007199254740993","method":"tools/call","params":{"name":"zalo_get_message_context","arguments":{"text":"PRIVATE_SENTINEL_INPUT"}}}` + "\n")), writer: stdioOutput{&output}, observer: observer}
	connection, err := transport.Connect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	// Act: one actual SDK response is written; telemetry neither serializes nor rewrites it.
	message, err := connection.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	request := message.(*jsonrpc.Request)
	reply := &jsonrpc.Response{ID: request.ID, Result: json.RawMessage(`{"text":"界🍀 <>&\nquote\""}`)}
	if err := connection.Write(context.Background(), reply); err != nil {
		t.Fatal(err)
	}
	// Assert: full accepted wire bytes match exactly and ID/content are unchanged.
	var decoded struct {
		ID     string            `json:"id"`
		Result map[string]string `json:"result"`
	}
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ID != "9007199254740993" || decoded.Result["text"] != "界🍀 <>&\nquote\"" || !strings.HasSuffix(output.String(), "\n") {
		t.Fatal("observer changed SDK frame")
	}
	if len(observer.bytes) != 1 || observer.bytes[0] != int64(output.Len()) || observer.tools[0] != "zalo_get_message_context" || observer.transports[0] != domain.TransportSTDIO {
		t.Fatal("STDIO actual wire measurement missing")
	}
	if strings.Contains(strings.Join(observer.tools, ""), "PRIVATE_SENTINEL") {
		t.Fatal("STDIO input content reached dimensions")
	}
	if len(connection.(*observedStdioConnection).requests) != 0 {
		t.Fatal("completed response identity retained")
	}
}

type failedStdioOutput struct {
	accepted int
	err      error
}

func (w failedStdioOutput) Write(data []byte) (int, error) { return min(len(data), w.accepted), w.err }
func (failedStdioOutput) Close() error                     { return nil }
func TestSTDIOWriterFailureIsNotRetriedOrReplaced(t *testing.T) {
	// Arrange: the real SDK sees an original partial writer error.
	original := errors.New("PRIVATE_SENTINEL_WRITER_FAILURE")
	observer := &stdioByteObservation{queueObservation: queueObservation{recording: true}}
	transport := &observedStdioTransport{reader: io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"zalo_get_status"}}` + "\n")), writer: failedStdioOutput{accepted: 7, err: original}, observer: observer}
	connection, err := transport.Connect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	requestMessage, err := connection.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Act.
	err = connection.Write(context.Background(), &jsonrpc.Response{ID: requestMessage.(*jsonrpc.Request).ID, Result: json.RawMessage(`{}`)})
	// Assert: partial accepted bytes only, same failure identity and no replay.
	if err != original || len(observer.bytes) != 1 || observer.bytes[0] != 7 {
		t.Fatal("STDIO measurement changed failed write or counted unaccepted bytes")
	}
}
