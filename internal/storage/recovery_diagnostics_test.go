package storage

import (
	"context"
	"strings"
	"testing"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestRecoveryDeliveryReferenceIsStableAndDomainSeparated(t *testing.T) {
	// Arrange: state-backed key and row IDs shared by two independent tables.
	s, path, _, _ := journalFixture(t)
	reference := s.RecoveryDeliveryReference(1)
	// Act: close/reopen the same state, as after a service restart.
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenWithPolicy(context.Background(), path, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	// Assert: retry/restart stable, table-separated, state-keyed and opaque.
	if !strings.HasPrefix(reference, "rdlv_") || len(reference) != 48 ||
		reopened.RecoveryDeliveryReference(1) != reference ||
		reopened.RecoveryDeliveryReference(2) == reference ||
		reopened.DeliveryReference(1) == reference {
		t.Fatal("invalid recovery delivery reference")
	}
	other, _, _, _ := journalFixture(t)
	defer other.Close()
	if other.RecoveryDeliveryReference(1) == reference {
		t.Fatal("reference not state-keyed")
	}
}
