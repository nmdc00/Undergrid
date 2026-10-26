package network

import (
	"context"

	"github.com/riverqueue/river"
)

// Focused provider/authority fixtures invoke the actual dedicated worker entrypoint.
// Local scans deliberately do not perform provider I/O. Full runtime tests prove
// the durable scheduling seam independently through RunWorker and persisted jobs.
func runSemanticEmbeddingFixture(ctx context.Context, service *Service, id string) error {
	worker := semanticEmbeddingWorker{service: service}
	return worker.Work(ctx, &river.Job[semanticEmbeddingArgs]{Args: semanticEmbeddingArgs{IntentID: id}})
}
func embedThenScanSemanticFixture(ctx context.Context, service *Service, id string) error {
	if err := runSemanticEmbeddingFixture(ctx, service, id); err != nil {
		return err
	}
	return service.scanIntentions(ctx, id)
}
