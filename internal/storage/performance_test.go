package storage

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"
)

func TestSearchPerformance100k(t *testing.T) {
	if os.Getenv("ZL_MCP_PERF") != "1" {
		t.Skip("run with ZL_MCP_PERF=1 to measure the 100k-message acceptance gate")
	}
	// Arrange: synthetic corpus, single transaction to avoid measuring fixture writes.
	s := openTest(t)
	ctx := context.Background()
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	stmt, e := tx.PrepareContext(
		ctx,
		`INSERT INTO messages(group_id,message_id,sender_id,sent_at,received_at,text,attachments,source) VALUES('g1',?,'sender',?,? ,?,'[]','live')`,
	)
	if e != nil {
		t.Fatal(e)
	}
	at := time.Now().UTC()
	for i := range 100000 {
		ts := at.Add(time.Duration(i) * time.Second).Format("2006-01-02T15:04:05.000000000Z")
		text := fmt.Sprintf("Обсуждение %d: ремонт кондиционера, cà phê и бытовые услуги", i)
		if _, e = stmt.ExecContext(ctx, strconv.Itoa(i), ts, ts, text); e != nil {
			t.Fatal(e)
		}
	}
	//nolint:sqlclosecheck // Close this query before reusing the connection; all return paths close it.
	if e = stmt.Close(); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	for range 5 {
		if _, _, _, e = s.Search(ctx, Search{Query: "ремонт кондиционера", Limit: 20}); e != nil {
			t.Fatal(e)
		}
	}
	// Act: warm-cache first-page latency over 100 independent queries.
	timings := []time.Duration{}
	for range 100 {
		start := time.Now()
		hits, _, _, err := s.Search(ctx, Search{Query: "ремонт кондиционера", Limit: 20})
		timings = append(timings, time.Since(start))
		if err != nil || len(hits) != 20 {
			t.Fatalf("bad result count: %d %v", len(hits), err)
		}
	}
	slices.Sort(timings)
	p95 := timings[94]
	// Assert
	t.Logf("100000 messages, warm first page, 100 samples: p95=%s, median=%s", p95, timings[49])
	if p95 > 500*time.Millisecond {
		t.Fatalf("p95 exceeds spec: %s", p95)
	}
}
