package zalo

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/amrakk/zcago/model"
	"github.com/skosovsky/zl-mcp/docs/contracts"
)

func TestMobileProducerCountDiagnosticsAreBoundedAndUnverified(t *testing.T) {
	// Arrange: optional untrusted metadata, including exact integers above 2^53.
	schema, err := contracts.Compile("mobile_backup_producer_counts", "output")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ info, state, total string }{
		{`{"backup_db":{"msg_total":9007199254740993,"msg_thread":18446744073709551615},"secret":"private-canary"}`, "available", "9007199254740993"},
		{`{"backup_db":{"msg_total":0,"msg_thread":0}}`, "available", "0"},
		{`{"db_format":1}`, "absent", ""},
		{`{"backup_db":null}`, "absent", ""},
		{`{"backup_db":{"msg_total":1}}`, "invalid", ""},
		{`{"backup_db":{"msg_total":"private-canary","msg_thread":1}}`, "invalid", ""},
		{`{"backup_db":{"msg_total":-1,"msg_thread":1}}`, "invalid", ""},
		{`{"backup_db":{"msg_total":1.5,"msg_thread":1}}`, "invalid", ""},
		{`{"backup_db":{"msg_total":1e3,"msg_thread":1}}`, "invalid", ""},
		{`{"backup_db":{"msg_total":18446744073709551616,"msg_thread":1}}`, "invalid", ""},
		{`{"backup_db":[]}`, "invalid", ""},
		{`null`, "invalid", ""},
		{`{`, "invalid", ""},
		{strings.Repeat(" ", 16<<10) + `{}`, "invalid", ""},
	}
	for _, c := range cases {
		// Act: inspect and serialize the same typed object used by the JSON log.
		got := inspectMobileProducerCounts(c.info)
		encoded, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		decoder := json.NewDecoder(bytes.NewReader(encoded))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			t.Fatal(err)
		}
		// Assert: schema, exact values, no partial claims or raw metadata.
		if got.State != c.state || got.Verification != "unverified" || schema.Validate(value) != nil || bytes.Contains(encoded, []byte("private-canary")) {
			t.Fatal("invalid or sensitive diagnostic output")
		}
		if c.total != "" && !bytes.Contains(encoded, []byte(`"msg_total":`+c.total)) {
			t.Fatal("producer count precision lost")
		}
	}
}

func TestVerifiedMobileOfferLogsOnlyProducerCounts(t *testing.T) {
	// Arrange: the production receiver, synthetic encryption and private metadata.
	key := mobileOfferKey(t)
	f := newMobileOfferFake()
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	defer slog.SetDefault(previous)
	f.dispatch = func(public string) {
		confirm := 1
		f.events <- model.MobileSyncEvent{Action: "user_confirm", PublicKey: public, PCName: "Web", UserAction: &confirm}
		e := syntheticOffer(t, public)
		e.DatabaseInfo = `{"db_format":1,"backup_db":{"msg_total":129,"msg_thread":3},"secret":"private-canary"}`
		f.events <- e
	}
	// Act: validate the envelope, decrypt and map the owner through the real path.
	offer, err := receiveMobileBackupOffer(context.Background(), "9007199254740993", f, f, nil, func() (*rsa.PrivateKey, error) { return key, nil })
	// Assert: one observation only, never credentials or admission proof.
	logs := output.String()
	if err != nil || offer.FileSize != 16 || strings.Count(logs, `"msg":"mobile_backup_producer_counts"`) != 1 || !strings.Contains(logs, `"msg_total":129`) || !strings.Contains(logs, `"verification":"unverified"`) || strings.Contains(logs, "private-canary") || strings.Contains(logs, offer.KeyText) || strings.Contains(logs, offer.URL) {
		t.Fatal("producer count observation missing or private data logged")
	}
}

func TestInvalidOptionalProducerCountsDoNotRejectOffer(t *testing.T) {
	// Arrange: the required envelope is valid; an optional count has a wrong type.
	key := mobileOfferKey(t)
	f := newMobileOfferFake()
	f.dispatch = func(public string) {
		confirm := 1
		f.events <- model.MobileSyncEvent{Action: "user_confirm", PublicKey: public, PCName: "Web", UserAction: &confirm}
		e := syntheticOffer(t, public)
		e.DatabaseInfo = `{"db_format":1,"backup_db":{"msg_total":"invalid","msg_thread":3}}`
		f.events <- e
	}
	// Act: optional diagnostics must not alter envelope acceptance or dispatch.
	offer, err := receiveMobileBackupOffer(context.Background(), "9007199254740993", f, f, nil, func() (*rsa.PrivateKey, error) { return key, nil })
	// Assert: same valid source receipt, one dispatch and released subscription.
	if err != nil || offer.FileSize != 16 || f.requests != 1 || f.releases != 1 {
		t.Fatal("optional count changed offer acceptance")
	}
}
