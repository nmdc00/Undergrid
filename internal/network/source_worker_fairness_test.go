package network

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

func TestSourceWorkerSlowSourceDoesNotBlockHealthySource(t *testing.T) {
	_, db := fixture(t)
	role := "source_fair_" + strings.ReplaceAll(NewID(), "-", "")
	identifier := pgx.Identifier{role}.Sanitize()
	if _, err := db.Exec(context.Background(), "CREATE ROLE "+identifier+" LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS; GRANT undergrid_worker TO "+identifier); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(context.Background(), "DROP ROLE "+identifier)
	runtimeDSN := networkFixtureRoleDSN(t, db, role)
	blocked := make(chan struct{}, 1)
	publisher := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/slow" {
			select {
			case blocked <- struct{}{}:
			default:
			}
			<-req.Context().Done()
			return
		}
		io.WriteString(w, `{"items":[{"id":"healthy","version":"1","url":"https://publisher.example/item","title":"Healthy source","status":"active"}],"next_page":0}`)
	}))
	defer publisher.Close()
	configs := []SourceConfig{}
	for _, path := range []string{"/slow", "/healthy"} {
		configs = append(configs, SourceConfig{ID: NewID(), Kind: "capability_manifest", Endpoint: "https://publisher.example" + path, Publisher: "fixture", AccessBasis: "fixture access", RepublicationBasis: "fixture metadata", TombstoneBasis: "fixture provenance", Fields: []string{"title"}})
	}
	refresher, err := NewSourceRefresher(runtimeDSN, configs)
	if err != nil {
		t.Fatal(err)
	}
	transport := publisher.Client().Transport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, strings.TrimPrefix(publisher.URL, "https://"))
	}
	transport.TLSClientConfig.InsecureSkipVerify = true
	refresher.client.Transport = transport
	poolConfig, err := pgxpool.ParseConfig(runtimeDSN)
	if err != nil {
		t.Fatal(err)
	}
	poolConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET ROLE undergrid_worker")
		return err
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	workers := river.NewWorkers()
	river.AddWorker(workers, &sourceRefreshWorker{refresher: refresher})
	queue, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Schema: "undergrid_queue", Workers: workers, Queues: map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 2}, sourceRefreshQueue: {MaxWorkers: 2, FetchCooldown: 10 * time.Millisecond, FetchPollInterval: 10 * time.Millisecond}}, SoftStopTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err = queue.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		stop, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if err := queue.StopAndCancel(stop); err != nil {
			t.Error(err)
		}
	}()
	if _, err = queue.Insert(ctx, sourceRefreshArgs{}, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-blocked:
	case <-time.After(5 * time.Second):
		t.Fatal("slow source did not start")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var count int
		if err = db.QueryRow(ctx, `SELECT count(*) FROM undergrid.eligible_external_observations WHERE source_id=$1`, configs[1].ID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count == 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("healthy source starved behind unrelated blocked source")
}

func TestSourceWorkerCompletedJobsDoNotSuppressRefresh(t *testing.T) {
	_, db := fixture(t)
	config := SourceConfig{ID: NewID(), Kind: "capability_manifest", Endpoint: "https://publisher.example/feed", Publisher: "fixture", AccessBasis: "fixture access", RepublicationBasis: "fixture metadata", TombstoneBasis: "fixture provenance", Fields: []string{"title"}}
	refresher, err := NewSourceRefresher(db.Config().ConnString(), []SourceConfig{config})
	if err != nil {
		t.Fatal(err)
	}
	worker := sourceRefreshWorker{refresher: refresher}
	ctx := context.Background()
	if err = worker.Work(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `UPDATE undergrid_queue.river_job SET state='completed',finalized_at=now() WHERE kind='permissioned_source_refresh_v1' AND args->>'source_id'=$1`, config.ID); err != nil {
		t.Fatal(err)
	}
	if err = worker.Work(ctx, nil); err != nil {
		t.Fatal(err)
	}
	var queued int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM undergrid_queue.river_job WHERE kind='permissioned_source_refresh_v1' AND args->>'source_id'=$1 AND state='available'`, config.ID).Scan(&queued); err != nil || queued != 1 {
		t.Fatalf("completed job suppressed next source refresh: queued=%d err=%v", queued, err)
	}
	// Repeated coordinators coalesce the pending wakeup, not completed history.
	if err = worker.Work(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM undergrid_queue.river_job WHERE kind='permissioned_source_refresh_v1' AND args->>'source_id'=$1 AND state='available'`, config.ID).Scan(&queued); err != nil || queued != 1 {
		t.Fatalf("duplicate queued refresh: %d %v", queued, err)
	}
	if _, err = db.Exec(ctx, `UPDATE undergrid_queue.river_job SET state='retryable',scheduled_at=now()+interval '1 hour' WHERE kind='permissioned_source_refresh_v1' AND args->>'source_id'=$1 AND state='available'`, config.ID); err != nil {
		t.Fatal(err)
	}
	if err = worker.Work(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM undergrid_queue.river_job WHERE kind='permissioned_source_refresh_v1' AND args->>'source_id'=$1 AND state='available'`, config.ID).Scan(&queued); err != nil || queued != 1 {
		t.Fatalf("delayed retry swallowed immediate source wakeup: %d %v", queued, err)
	}

}
