package network

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestDatabaseIsolationAndAuditImmutability(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("UNDERGRID_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("real PostgreSQL required")
	}
	db, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(ctx)
	if err = Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SET LOCAL ROLE undergrid_app; SET LOCAL search_path=undergrid,pg_catalog"); err != nil {
		t.Fatal(err)
	}
	var super, bypass bool
	if err = tx.QueryRow(ctx, "SELECT rolsuper,rolbypassrls FROM pg_roles WHERE rolname=current_user").Scan(&super, &bypass); err != nil || super || bypass {
		t.Fatalf("unsafe role: super=%v bypass=%v %v", super, bypass, err)
	}
	for _, table := range []string{"principals", "agents", "credentials", "intents", "intent_revisions", "events", "inbox", "idempotency"} {
		var n int
		if err = tx.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("unbound role sees %s: %d %v", table, n, err)
		}
	}
	if _, err = tx.Exec(ctx, "UPDATE events SET type='tampered'"); err == nil {
		t.Fatal("application can rewrite audit evidence")
	}
}

func TestIntentValidationRejectsUnsupportedFinancialAuthority(t *testing.T) {
	input := IntentInput{Objective: "dataset", Output: Output{"json_dataset", "source_catalog_v1"}, Visibility: "private", Watch: Watch{true, []string{"inbox"}}}
	if input.validate() == nil {
		t.Fatal("missing expiration accepted")
	}
	// A budget is a typed statement, not an arbitrary financial command.
	valid := []byte(`{"amount_minor":100,"currency":"EUR","kind":"stated_budget"}`)
	invalid := []byte(`{"amount_minor":100,"currency":"EUR","kind":"authorized_live_commitment"}`)
	if err := ValidateBudget(valid); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBudget(invalid); err == nil {
		t.Fatal("network accepted financial authority")
	}
}
