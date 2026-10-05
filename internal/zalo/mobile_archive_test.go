package zalo

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/amrakk/zcago"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

type archiveSDKFake struct {
	zcago.API
	owner  string
	change bool
	data   []byte
	err    error
}

func (f *archiveSDKFake) GetOwnID() string { return f.owner }
func (f *archiveSDKFake) ConsumeMobileArchive(context.Context, *http.Request, *http.Client, func(context.Context, *http.Response) ([]byte, error)) ([]byte, error) {
	if f.change {
		f.owner = "other"
	}
	return f.data, f.err
}
func TestMobileArchiveAdapterRejectsChangedAccountAndPrivateFailure(t *testing.T) {
	for _, change := range []bool{false, true} {
		// Arrange.
		source := &archiveSDKFake{owner: "synthetic", change: change, data: []byte("synthetic ciphertext")}
		if !change {
			source.err = errors.New("private marker")
		}
		client := &Client{api: source}
		// Act.
		data, e := client.ConsumeMobileArchive(context.Background(), nil, nil, nil)
		// Assert.
		if !errors.Is(e, domain.ErrMobileBackupInvalid) || data != nil || !bytes.Equal(source.data, make([]byte, len(source.data))) {
			t.Fatal("account mismatch or private result escaped")
		}
	}
}
