package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// This subprocess exercises the deployed mode parser and actual isolated worker
// login without providing any approved RPC/facilitator configuration.
func TestUSDCWorkerModeSubprocess(t *testing.T) {
	if os.Getenv("UNDERGRID_USDC_MODE_FIXTURE_HELPER") == "1" {
		flag.CommandLine = flag.NewFlagSet("sandbox", flag.ContinueOnError)
		os.Args = []string{"undergrid-sandbox", "-mode", "usdc-worker"}
		fmt.Print(run())
		return
	}
	admin := os.Getenv("UNDERGRID_SANDBOX_TEST_DATABASE_URL")
	if admin == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("CI requires separate financial fixture")
		}
		t.Skip("requires financial PostgreSQL fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(context.Background())
	role := "usdc_mode_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	identifier := pgx.Identifier{role}.Sanitize()
	if _, err = db.Exec(ctx, "CREATE ROLE "+identifier+" LOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOREPLICATION; GRANT undergrid_sandbox_worker TO "+identifier); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(context.Background(), "DROP ROLE "+identifier)
	config := db.Config()
	workerURL := url.URL{Scheme: "postgres", User: url.User(role), Host: "localhost:" + strconv.Itoa(int(config.Port)), Path: "/" + config.Database, RawQuery: url.Values{"host": {config.Host}, "sslmode": {"disable"}}.Encode()}
	actual, err := pgx.Connect(ctx, workerURL.String())
	if err != nil {
		t.Fatal(err)
	}
	var user string
	var super, bypass, app bool
	err = actual.QueryRow(ctx, `SELECT current_user,rolsuper,rolbypassrls,pg_has_role(current_user,'undergrid_sandbox_app','MEMBER') FROM pg_roles WHERE rolname=current_user`).Scan(&user, &super, &bypass, &app)
	actual.Close(ctx)
	if err != nil || user != role || super || bypass || app {
		t.Fatalf("invalid financial worker login user=%s super=%v bypass=%v app=%v err=%v", user, super, bypass, app, err)
	}
	t.Setenv("UNDERGRID_SANDBOX_DATABASE_URL", workerURL.String())
	t.Setenv("UNDERGRID_USDC_MODE_FIXTURE_HELPER", "1")
	for _, key := range []string{"UNDERGRID_SANDBOX_USDC_RPC_URL", "UNDERGRID_SANDBOX_USDC_FACILITATOR_URL", "UNDERGRID_SANDBOX_USDC_PROVIDER_APPROVAL"} {
		t.Setenv(key, "")
	}
	child, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	command := exec.CommandContext(child, os.Args[0], "-test.run=^TestUSDCWorkerModeSubprocess$")
	// The worker subprocess receives its own financial login only: no parent
	// migration/network/provider credentials from the test orchestrator.
	command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "UNDERGRID_SANDBOX_DATABASE_URL=" + workerURL.String(), "UNDERGRID_USDC_MODE_FIXTURE_HELPER=1"}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("USDC mode subprocess: %v %s", err, output)
	}
	if got := strings.TrimSpace(string(output)); got != "invalid isolated USDC TEST worker configurationPASS" {
		t.Fatalf("missing fail-closed USDC worker runtime: %q", got)
	}
}
