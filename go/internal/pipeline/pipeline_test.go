package pipeline

import (
	"context"
	"testing"

	"azure-resource-downloader/internal/handlers"
	"azure-resource-downloader/internal/models"
)

// cancelledContext returns a context that is already cancelled, so every
// pipeline worker observes ctx.Err() != nil on its first iteration and takes
// the drain-on-cancel path.
func cancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestMarkCompleteness(t *testing.T) {
	tests := []struct {
		name       string
		summary    ExecutionSummary
		wantOK     bool
		wantReason string
	}{
		{
			name: "complete",
			summary: ExecutionSummary{
				TotalResources: 2,
				Results:        make([]*models.WriteResult, 2),
			},
			wantOK: true,
		},
		{
			name: "missing results",
			summary: ExecutionSummary{
				TotalResources: 3,
				Results:        make([]*models.WriteResult, 2),
			},
			wantOK:     false,
			wantReason: "only 2 of 3 requests produced a result",
		},
		{
			name: "cancelled",
			summary: ExecutionSummary{
				TotalResources:     2,
				Results:            make([]*models.WriteResult, 2),
				CancelledResources: 1,
			},
			wantOK:     false,
			wantReason: "1 requests were cancelled",
		},
		{
			name: "skipped types",
			summary: ExecutionSummary{
				TotalResources: 1,
				Results:        make([]*models.WriteResult, 1),
				SkippedTypes:   []models.SkippedType{{ResourceType: "Microsoft.Graph/groups", Reason: "forbidden"}},
			},
			wantOK:     false,
			wantReason: "1 resource types could not be listed",
		},
		{
			name: "multiple reasons joined",
			summary: ExecutionSummary{
				TotalResources:     3,
				Results:            make([]*models.WriteResult, 2),
				CancelledResources: 1,
				SkippedTypes:       []models.SkippedType{{ResourceType: "Microsoft.Graph/groups"}},
			},
			wantOK:     false,
			wantReason: "only 2 of 3 requests produced a result; 1 requests were cancelled; 1 resource types could not be listed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.summary
			s.MarkCompleteness()
			if s.Complete != tc.wantOK {
				t.Fatalf("Complete = %v, want %v (reason %q)", s.Complete, tc.wantOK, s.IncompleteReason)
			}
			if s.IncompleteReason != tc.wantReason {
				t.Fatalf("IncompleteReason = %q, want %q", s.IncompleteReason, tc.wantReason)
			}
		})
	}
}

func TestFetcherDrainsOnCancel(t *testing.T) {
	reg := handlers.NewEmptyRegistry()
	// azureClient is nil deliberately: on a cancelled context the worker emits a
	// cancelled result before ever reaching the fetch path that would use it.
	f := NewFetcher(nil, reg, 4, 0)

	requests := make([]*models.FetchRequest, 0, 10)
	for i := 0; i < 10; i++ {
		requests = append(requests, &models.FetchRequest{
			ResourceID:   "id",
			ResourceType: "Microsoft.Graph/groups",
		})
	}

	var got []*models.FetchResult
	for r := range f.Fetch(cancelledContext(), requests) {
		got = append(got, r)
	}

	if len(got) != len(requests) {
		t.Fatalf("got %d results for %d requests; every request must produce exactly one", len(got), len(requests))
	}
	for i, r := range got {
		if !r.Cancelled {
			t.Errorf("result %d: Cancelled = false, want true", i)
		}
	}
}

func TestTransformerDrainsOnCancel(t *testing.T) {
	reg := handlers.NewEmptyRegistry()
	tr := NewTransformer(reg, 4, nil, nil)

	in := make(chan *models.FetchResult, 10)
	for i := 0; i < 10; i++ {
		in <- &models.FetchResult{ResourceID: "id", ResourceType: "Microsoft.Graph/groups"}
	}
	close(in)

	var got []*models.TransformResult
	for r := range tr.Transform(cancelledContext(), in) {
		got = append(got, r)
	}

	if len(got) != 10 {
		t.Fatalf("got %d results for 10 inputs; every input must produce exactly one", len(got))
	}
	for i, r := range got {
		if !r.Cancelled {
			t.Errorf("result %d: Cancelled = false, want true", i)
		}
	}
}

func TestTransformerPropagatesUpstreamStatuses(t *testing.T) {
	reg := handlers.NewEmptyRegistry()
	tr := NewTransformer(reg, 2, nil, nil)

	in := make(chan *models.FetchResult, 3)
	in <- &models.FetchResult{ResourceID: "c", ResourceType: "t", Cancelled: true}
	in <- &models.FetchResult{ResourceID: "s", ResourceType: "t", Skipped: true, SkipReason: "forbidden"}
	in <- &models.FetchResult{ResourceID: "e", ResourceType: "t", Error: context.DeadlineExceeded}
	close(in)

	byID := map[string]*models.TransformResult{}
	for r := range tr.Transform(context.Background(), in) {
		byID[r.ResourceID] = r
	}

	if len(byID) != 3 {
		t.Fatalf("got %d results, want 3", len(byID))
	}
	if !byID["c"].Cancelled {
		t.Error("cancelled fetch result must propagate as cancelled")
	}
	if !byID["s"].Skipped || byID["s"].SkipReason != "forbidden" {
		t.Error("skipped fetch result must propagate as skipped with its reason")
	}
	if byID["e"].Error == nil {
		t.Error("errored fetch result must propagate its error")
	}
}

func TestWriterDrainsOnCancel(t *testing.T) {
	w := NewWriter(t.TempDir(), 4, true /* dryRun */, false)

	in := make(chan *models.TransformResult, 10)
	for i := 0; i < 10; i++ {
		in <- &models.TransformResult{ResourceID: "id", ResourceType: "Microsoft.Graph/groups"}
	}
	close(in)

	var got []*models.WriteResult
	for r := range w.Write(cancelledContext(), in) {
		got = append(got, r)
	}

	if len(got) != 10 {
		t.Fatalf("got %d results for 10 inputs; every input must produce exactly one", len(got))
	}
	for i, r := range got {
		if !r.Cancelled {
			t.Errorf("result %d: Cancelled = false, want true", i)
		}
	}
}

func TestWriterPropagatesUpstreamStatuses(t *testing.T) {
	w := NewWriter(t.TempDir(), 2, true /* dryRun */, false)

	in := make(chan *models.TransformResult, 4)
	in <- &models.TransformResult{ResourceID: "c", ResourceType: "t", Cancelled: true}
	in <- &models.TransformResult{ResourceID: "s", ResourceType: "t", Skipped: true, SkipReason: "forbidden"}
	in <- &models.TransformResult{ResourceID: "f", ResourceType: "t", Filtered: true}
	in <- &models.TransformResult{ResourceID: "e", ResourceType: "t", Error: context.DeadlineExceeded}
	close(in)

	byID := map[string]*models.WriteResult{}
	for r := range w.Write(context.Background(), in) {
		byID[r.ResourceID] = r
	}

	if len(byID) != 4 {
		t.Fatalf("got %d results, want 4", len(byID))
	}
	if !byID["c"].Cancelled {
		t.Error("cancelled transform result must propagate as cancelled")
	}
	if !byID["s"].Skipped {
		t.Error("skipped transform result must propagate as skipped")
	}
	if !byID["f"].Filtered {
		t.Error("filtered transform result must propagate as filtered")
	}
	if byID["e"].Error == nil {
		t.Error("errored transform result must propagate its error")
	}
}

// TestPipelineStagesAccountForEveryRequestOnCancel wires the three stages
// together exactly as Pipeline.Execute does and asserts that, under
// cancellation, every request still yields exactly one write result. This is
// the guarantee the len(Results) == TotalResources invariant in Execute rests
// on: a timeout must never silently drop a request (which would later look like
// a deletion and could be pruned from disk).
func TestPipelineStagesAccountForEveryRequestOnCancel(t *testing.T) {
	reg := handlers.NewEmptyRegistry()
	f := NewFetcher(nil, reg, 4, 0)
	tr := NewTransformer(reg, 4, nil, nil)
	w := NewWriter(t.TempDir(), 4, true /* dryRun */, false)

	const n = 25
	requests := make([]*models.FetchRequest, 0, n)
	for i := 0; i < n; i++ {
		requests = append(requests, &models.FetchRequest{ResourceID: "id", ResourceType: "Microsoft.Graph/groups"})
	}

	ctx := cancelledContext()
	fetchResults := f.Fetch(ctx, requests)
	transformResults := tr.Transform(ctx, fetchResults)
	writeResults := w.Write(ctx, transformResults)

	var got []*models.WriteResult
	for r := range writeResults {
		got = append(got, r)
	}

	if len(got) != n {
		t.Fatalf("got %d results for %d requests; the completeness invariant requires exactly one per request", len(got), n)
	}
	for i, r := range got {
		if !r.Cancelled {
			t.Errorf("result %d: Cancelled = false, want true", i)
		}
	}
}
