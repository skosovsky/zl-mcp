package mobilebackup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestIdentityMappingExactTypedPositionalNumbers(t *testing.T) {
	// Arrange: native JS numeric precision is deliberately exceeded.
	request := IdentityRequest{Direct: []string{"9007199254740993", "18446744073709551615"}, Groups: []string{"9007199254740993"}}
	// Act.
	payload, err := IdentityPayload(request)
	if err != nil {
		t.Fatal(err)
	}
	pairs, err := DecodeIdentities(request, []byte(`{"fids":[9007199254740995,"18446744073709551614"],"gids":["g9007199254740997"]}`))
	// Assert: no rounding, category conflation, source-ID fallback or public serialization.
	if err != nil || len(pairs) != 3 || pairs[0].Session != "9007199254740995" || pairs[1].Plain != "18446744073709551615" || !pairs[2].Group || pairs[2].Session != "9007199254740997" {
		t.Fatal("mapping lost identity", err)
	}
	if !bytes.Contains(payload, []byte(`9007199254740993`)) || bytes.Contains(payload, []byte(`"9007199254740993"`)) {
		t.Fatal("payload integer encoding")
	}
	encoded, _ := json.Marshal(pairs[0])
	if string(encoded) != "{}" || strings.Contains(fmt.Sprintf("%#v", pairs[0]), "900719") {
		t.Fatal("mapping exposed")
	}
}

func TestIdentityMappingRejectsAmbiguousReplies(t *testing.T) {
	request := IdentityRequest{Direct: []string{"1", "2"}, Groups: []string{"3"}}
	for _, data := range []string{
		`{"fids":["4","4"],"gids":["5"]}`,
		`{"fids":["4"],"gids":["5"]}`,
		`{"fids":["4","6"],"gids":[]}`,
		`{"fids":["4","6"],"gids":["5"],"fids":["4","6"]}`,
		`{"fids":["4","6"],"gids":["5"],"unknown":0}`,
		`{"fids":["g4","6"],"gids":["5"]}`,
		`{"fids":[4.0,"6"],"gids":["5"]}`,
		`{"fids":[4e0,"6"],"gids":["5"]}`,
		`{"fids":["04","6"],"gids":["5"]}`,
		`{"fids":["18446744073709551616","6"],"gids":["5"]}`,
		`{"fids":[null,"6"],"gids":["5"]}`,
		`{"fids":["0","6"],"gids":["5"]}`,
		`{"fids":["4","6"],"gids":["5"]} {}`,
		strings.Repeat(" ", 256<<10) + `{}`,
	} {
		// Act / Assert: all-or-nothing; fixed error contains no reply data.
		pairs, err := DecodeIdentities(request, []byte(data))
		if !errors.Is(err, ErrIdentities) || pairs != nil {
			t.Fatal("ambiguous mapping accepted")
		}
	}
}

func TestIdentityRequestValidationAndEmptyCategories(t *testing.T) {
	for _, r := range []IdentityRequest{{}, {Direct: []string{"01"}}, {Direct: []string{"1", "1"}}, {Groups: []string{"-1"}}, {Direct: make([]string, 1001)}} {
		// Act / Assert.
		if data, err := IdentityPayload(r); !errors.Is(err, ErrIdentities) || data != nil {
			t.Fatal("invalid identity request")
		}
	}
	request := IdentityRequest{Direct: []string{"1"}}
	payload, err := IdentityPayload(request)
	if err != nil || string(payload) != `{"fids":[1]}` {
		t.Fatal("empty category not omitted")
	}
	if pairs, err := DecodeIdentities(request, []byte(`{"fids":["2"],"gids":[]}`)); err != nil || len(pairs) != 1 {
		t.Fatal("empty returned category rejected")
	}
	if _, err := DecodeIdentities(request, []byte(`{"fids":["2"]}`)); err != nil {
		t.Fatal("absent unrequested category rejected")
	}
	if _, err := DecodeIdentities(request, []byte(`{"gids":[]}`)); !errors.Is(err, ErrIdentities) {
		t.Fatal("missing requested category accepted")
	}
}
