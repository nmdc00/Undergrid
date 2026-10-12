package network

import (
	"context"
	"embed"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Migrate is called by a separately authorized migration command, never API startup.
func Migrate(ctx context.Context, conn *pgx.Conn) error {
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock(74932518)"); err != nil {
		return err
	}
	defer conn.Exec(context.Background(), "SELECT pg_advisory_unlock(74932518)")
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "CREATE TABLE IF NOT EXISTS public.undergrid_migrations(name text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())"); err != nil {
		return err
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		var exists bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM public.undergrid_migrations WHERE name=$1)", entry.Name()).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		sql, err := migrations.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("migration %s: %w", entry.Name(), err)
		}
		if _, err = tx.Exec(ctx, "INSERT INTO public.undergrid_migrations(name) VALUES($1)", entry.Name()); err != nil {
			return err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, conn.Config().ConnString())
	if err != nil {
		return err
	}
	defer pool.Close()
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), &rivermigrate.Config{Schema: "undergrid_queue"})
	if err != nil {
		return err
	}
	if _, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return err
	}
	if _, err = conn.Exec(ctx, `GRANT ALL ON ALL TABLES IN SCHEMA undergrid_queue TO undergrid_worker;
GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA undergrid_queue TO undergrid_worker,undergrid_app;
GRANT SELECT,INSERT,UPDATE ON undergrid_queue.river_job TO undergrid_app;
GRANT SELECT ON public.undergrid_migrations TO undergrid_app,undergrid_worker;
INSERT INTO public.undergrid_migrations(name) VALUES('river-0.49.0-ready') ON CONFLICT DO NOTHING`); err != nil {
		return err
	}
	return nil
}
