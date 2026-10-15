package network

import (
	"context"
	"github.com/jackc/pgx/v5"
	"net/url"
	"strconv"
	"testing"
)

func assertNetworkFixtureLogin(t *testing.T, dsn, expected string) {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	var actual string
	var elevated bool
	err = conn.QueryRow(context.Background(), `SELECT session_user,rolsuper OR rolbypassrls OR EXISTS(SELECT FROM pg_tables WHERE schemaname='undergrid' AND tableowner=session_user) FROM pg_roles WHERE rolname=session_user`).Scan(&actual, &elevated)
	if err != nil || elevated || actual != expected {
		t.Fatalf("unsafe network fixture login: got=%s expected=%s elevated=%v err=%v", actual, expected, elevated, err)
	}
}

// ConnConfig.ConnString returns its original input, not a serialization of edits.
// Construct the intended login explicitly and inspect that actual connection.
func networkFixtureRoleDSN(t *testing.T, db *pgx.Conn, role string) string {
	t.Helper()
	cfg, err := pgx.ParseConfig(db.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	u := url.URL{Scheme: "postgres", User: url.User(role), Path: "/" + cfg.Database, RawQuery: url.Values{"host": {cfg.Host}, "port": {strconv.Itoa(int(cfg.Port))}, "sslmode": {"disable"}}.Encode()}
	dsn := u.String()
	assertNetworkFixtureLogin(t, dsn, role)
	return dsn
}
