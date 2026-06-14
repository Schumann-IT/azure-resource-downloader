package handlers

import (
	"context"
	"fmt"
	"sync"

	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/models"
)

// BuildFetchRequests creates fetch requests from command-line arguments.
//
// Selection precedence: explicit resourceIDs, then resourceGroup, then type
// listing. The resourceTypes slice acts as a filter on the registered
// handlers: when empty, all registered types are considered.
//
// The second return value lists resource types that could not be listed at all
// (missing permissions or no subscription) and the third lists types whose
// listing succeeded but returned no resources; callers should surface both in
// the execution summary.
//
// Listing calls are independent per type and network-bound, so they run
// concurrently with a bounded worker pool of listConcurrency goroutines.
func (r *Registry) BuildFetchRequests(ctx context.Context, resourceIDs []string, resourceGroup string, resourceTypes []string, subscriptionID string, listConcurrency int) ([]*models.FetchRequest, []models.SkippedType, []string, error) {
	var requests []*models.FetchRequest
	var skippedTypes []models.SkippedType
	var emptyTypes []string
	log := logger.Default

	// If specific resource IDs are provided, use them
	if len(resourceIDs) > 0 {
		for _, id := range resourceIDs {
			requests = append(requests, &models.FetchRequest{
				ResourceID:   id,
				Subscription: subscriptionID,
			})
		}
		return requests, nil, nil, nil
	}

	// If resource group is specified, build resource ID
	if resourceGroup != "" {
		if subscriptionID == "" {
			log.Warn("Cannot download ARM resources because of missing subscription, skipping resource group",
				"resource_group", resourceGroup)
			return requests, nil, nil, nil
		}
		rgID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", subscriptionID, resourceGroup)
		requests = append(requests, &models.FetchRequest{
			ResourceID:    rgID,
			ResourceType:  "Microsoft.Resources/resourceGroups",
			ResourceGroup: resourceGroup,
			Subscription:  subscriptionID,
		})
		return requests, nil, nil, nil
	}

	// Otherwise, list resources by type. resourceTypes acts as a filter; when no
	// type is given, every registered type is considered. Listing calls are
	// independent per type and network-bound, so they run concurrently with a
	// bounded worker pool. Results are collected into per-type slots so the
	// output order stays deterministic regardless of completion order.
	types := resourceTypes
	if len(types) == 0 {
		types = r.GetAllTypes()
		log.Info("No --type filter given, considering all registered types", "count", len(types))
	}

	if listConcurrency < 1 {
		listConcurrency = 1
	}

	// listOutcome holds the result of listing a single resource type. Exactly
	// one of skipped/empty/requests is meaningful per type.
	type listOutcome struct {
		requests []*models.FetchRequest
		skipped  *models.SkippedType
		empty    bool
	}
	outcomes := make([]listOutcome, len(types))

	var wg sync.WaitGroup
	sem := make(chan struct{}, listConcurrency)

	for i, resourceType := range types {
		handler, err := r.Get(resourceType)
		if err != nil {
			// A missing handler is a configuration error, not a runtime
			// permission issue: abort the whole run before launching workers.
			log.Error("No handler for resource type", "type", resourceType, "error", err)
			return nil, nil, nil, fmt.Errorf("no handler registered for resource type %s: %w", resourceType, err)
		}

		// ARM (subscription-scoped) types cannot be listed without a
		// subscription. Skip them with a clear warning so tenant-level
		// Microsoft Graph types still download. This needs no network call, so
		// it is recorded synchronously.
		if subscriptionID == "" && models.DetectAPIType(resourceType) == models.APIAzureResourceManager {
			log.Warn("Cannot download ARM resources because of missing subscription, skipping type",
				"type", resourceType)
			outcomes[i].skipped = &models.SkippedType{ResourceType: resourceType, Reason: "no subscription available"}
			continue
		}

		wg.Add(1)
		go func(i int, resourceType string, handler models.ResourceHandler) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			log.Info("Listing all resources of type", "type", resourceType)

			resourceList, err := handler.List(ctx)
			if err != nil {
				// Don't abort the whole run because one type fails (e.g. missing
				// permissions for a single Graph collection); log and continue so
				// the remaining types are still downloaded.
				log.Warn("Failed to list resources, skipping type", "type", resourceType, "reason", azure.ErrorSummary(err))
				log.Debug("Listing failed", "type", resourceType, "error", err)
				outcomes[i].skipped = &models.SkippedType{ResourceType: resourceType, Reason: azure.ErrorSummary(err)}
				return
			}

			log.Info("Found resources", "type", resourceType, "count", len(resourceList))

			if len(resourceList) == 0 {
				log.Warn("No resources found",
					"type", resourceType,
					"note", "This could be due to: (1) No resources of this type exist, (2) Insufficient permissions, or (3) Resources exist in a different scope (e.g., tenant vs subscription)")
				outcomes[i].empty = true
				return
			}

			reqs := make([]*models.FetchRequest, 0, len(resourceList))
			for _, resourceID := range resourceList {
				reqs = append(reqs, &models.FetchRequest{
					ResourceID:   resourceID,
					ResourceType: resourceType,
					Subscription: subscriptionID,
				})
			}
			outcomes[i].requests = reqs
		}(i, resourceType, handler)
	}

	wg.Wait()

	// Flatten per-type outcomes in the original type order so the result is
	// deterministic regardless of which listing finished first.
	for i, o := range outcomes {
		switch {
		case o.skipped != nil:
			skippedTypes = append(skippedTypes, *o.skipped)
		case o.empty:
			emptyTypes = append(emptyTypes, types[i])
		default:
			requests = append(requests, o.requests...)
		}
	}

	return requests, skippedTypes, emptyTypes, nil
}
