package toolruntime

import (
	"testing"

	"github.com/skosovsky/zl-mcp/internal/app/membership"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestTypedOperationOutcomes(t *testing.T) {
	// Arrange: states are native typed DTO fields, including a nil-Go-error business failure.
	tests := []struct {
		name   string
		result *Result
		want   domain.Outcome
	}{
		{"sent", &Result{SendStatus: &domain.SendOperation{Status: "sent"}}, domain.OutcomeSuccess},
		{"ambiguous send", &Result{SendStatus: &domain.SendOperation{Status: "unknown"}}, domain.OutcomeUnknown},
		{"rejected send", &Result{SendStatus: &domain.SendOperation{Status: "failed"}}, domain.OutcomeError},
		{"pending send", &Result{SendStatus: &domain.SendOperation{Status: "pending"}}, domain.OutcomeUnknown},
		{"history partial", &Result{ImportStatus: &domain.HistoryImportStatus{State: "partial"}}, domain.OutcomePartial},
		{"history paused", &Result{ImportStatus: &domain.HistoryImportStatus{State: "paused"}}, domain.OutcomePartial},
		{"history cancelled", &Result{ImportStatus: &domain.HistoryImportStatus{State: "cancelled"}}, domain.OutcomeCancelled},
		{"history unsupported", &Result{ImportStatus: &domain.HistoryImportStatus{State: "unsupported"}}, domain.OutcomeError},
		{"joined", &Result{JoinStatus: &membership.JoinStatus{Status: "joined"}}, domain.OutcomeSuccess},
		{"pending approval", &Result{JoinStatus: &membership.JoinStatus{Status: "pending_approval"}}, domain.OutcomeUnknown},
		{"join embedded business error", &Result{JoinStatus: &membership.JoinStatus{Status: "unknown", Error: &membership.JoinFailure{Code: "UPSTREAM_UNAVAILABLE", Message: "PRIVATE_SENTINEL"}}}, domain.OutcomeError},
		{"unknown arbitrary state", &Result{SendStatus: &domain.SendOperation{Status: "PRIVATE_SENTINEL"}}, domain.OutcomeUnknown},
		{"missing result", nil, domain.OutcomeUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act.
			got := tt.result.Outcome()
			// Assert.
			if got != tt.want {
				t.Fatalf("got %s want %s", got.String(), tt.want.String())
			}
		})
	}
}
