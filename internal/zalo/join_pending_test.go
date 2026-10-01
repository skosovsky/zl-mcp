package zalo

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/amrakk/zcago"
	"github.com/amrakk/zcago/api"
	"github.com/amrakk/zcago/errs"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

type joinResponseAPI struct {
	zcago.API
	responseErr error
	calls       int
}

func (a *joinResponseAPI) JoinGroupLink(context.Context, string) (api.JoinGroupLinkResponse, error) {
	a.calls++
	return "", a.responseErr
}

func TestClientJoinRecognizesOnlyExplicitApprovalCode(t *testing.T) {
	for _, code := range []int{0, 178, 239, 240, 241, 401} {
		for _, pointer := range []bool{false, true} {
			t.Run(fmt.Sprintf("code_%d_pointer_%v", code, pointer), func(t *testing.T) {
				// Arrange: the exact typed boundary seen in the live request, with private text.
				upstreamCode := errs.ZaloErrorCode(code)
				value := errs.ZaloAPIError{Code: &upstreamCode, Message: "private response text"}
				var upstreamErr error = value
				if pointer {
					upstreamErr = &value
				}
				upstreamErr = fmt.Errorf("wrapped: %w", upstreamErr)
				upstream := &joinResponseAPI{responseErr: upstreamErr}
				client := &Client{api: upstream}
				// Act
				err := client.Join(context.Background(), "https://zalo.me/g/synthetic")
				// Assert: exactly one request; only 240 becomes the safe pending sentinel.
				if errors.Is(err, domain.ErrJoinPendingApproval) != (code == 240) || upstream.calls != 1 {
					t.Fatalf("wrong classification: code=%d pending=%v calls=%d", code, errors.Is(err, domain.ErrJoinPendingApproval), upstream.calls)
				}
				if code == 240 && err.Error() != "join pending administrator approval" {
					t.Fatal("unsafe pending error")
				}
				if code != 240 && err != upstreamErr {
					t.Fatal("other error was reclassified")
				}
			})
		}
	}
	// Arrange
	upstream := &joinResponseAPI{}
	client := &Client{api: upstream}
	// Act/Assert: opaque success remains success, not pending evidence.
	if err := client.Join(context.Background(), "https://zalo.me/g/synthetic"); err != nil || upstream.calls != 1 {
		t.Fatal("success changed")
	}
	if err := classifyJoinFailure(errs.ZaloAPIError{Message: "240 approval required"}); errors.Is(err, domain.ErrJoinPendingApproval) {
		t.Fatal("message text guessed as a code")
	}
}
