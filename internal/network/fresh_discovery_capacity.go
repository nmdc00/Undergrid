package network

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

const freshDiscoveryQueue = "intention_fresh_discovery"
const freshWatchFanoutQueue = "intention_fresh_watch_fanout"
const freshWatchRefreshQueue = "intention_fresh_watch_refresh"

// Age changes scheduling ownership, never job identity or domain authority.
// These three command/pair kinds deliberately have no River unique key: every
// committed change has its own wakeup, even when an older one is retained.
// Future unique jobs are excluded because queue may participate in their key.
func retainAgedDiscovery(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	tag, err := pool.Exec(ctx, `WITH aged AS (
 SELECT id FROM undergrid_queue.river_job
 WHERE state IN ('available','retryable','scheduled') AND unique_key IS NULL
 AND created_at<clock_timestamp()-interval '30 seconds'
 AND ((queue=$1 AND kind='intention_scan_v1') OR
      (queue IN($2,$3) AND kind='intention_watch_refresh_v1'))
 ORDER BY id LIMIT 256 FOR UPDATE SKIP LOCKED
 ) UPDATE undergrid_queue.river_job j SET queue=CASE j.queue
 WHEN $1 THEN $4 WHEN $2 THEN $5 WHEN $3 THEN $6 END
 FROM aged WHERE j.id=aged.id`, freshDiscoveryQueue, freshWatchFanoutQueue, freshWatchRefreshQueue, discoveryQueue, watchFanoutQueue, watchRefreshQueue)
	return tag.RowsAffected(), err
}

// Each statement moves at most 256 rows. Startup reconciles accepted backlog
// before consumers fetch; context cancellation bounds recovery and shutdown.
func retainDiscoveryStartup(ctx context.Context, pool *pgxpool.Pool) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		count, err := retainAgedDiscovery(ctx, pool)
		if err != nil {
			return err
		}
		if count == 0 {
			return nil
		}
	}
}
