package service

import (
	"reflect"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skosovsky/zl-mcp/internal/buildinfo"
)

func TestBridgeIdentityPreservesServingBackendAndUnrelatedMetadata(t *testing.T) {
	// Arrange: backend may have a different revision from the executing bridge.
	backend := map[string]any{"version": "v1", "revision": "backend-revision", "dirty": false}
	result := &mcp.CallToolResult{
		Meta: mcp.Meta{buildinfo.MetadataKey: map[string]any{"backend": backend}, "other": "preserved"},
	}
	// Act
	got := withBridgeIdentity(result)
	// Assert
	identity, ok := got.Meta[buildinfo.MetadataKey].(map[string]any)
	if !ok || !reflect.DeepEqual(identity["backend"], backend) ||
		!reflect.DeepEqual(identity["bridge"], buildinfo.Current()) ||
		got.Meta["other"] != "preserved" {
		t.Fatalf("identity not preserved: %+v", got.Meta)
	}
}
func TestBridgeIdentityDoesNotPretendLegacyBackendMatchesBridge(t *testing.T) {
	// Arrange
	result := &mcp.CallToolResult{}
	// Act
	got := withBridgeIdentity(result)
	// Assert
	identity, ok := got.Meta[buildinfo.MetadataKey].(map[string]any)
	if !ok {
		t.Fatal("missing build metadata")
	}
	backend, ok := identity["backend"].(buildinfo.Identity)
	if !ok || backend.Version != "unknown" || backend.Revision != "unknown" || backend.Dirty != nil {
		t.Fatal("legacy backend identity guessed")
	}
	if withBridgeIdentity(nil) != nil {
		t.Fatal("nil failed result changed")
	}
}
