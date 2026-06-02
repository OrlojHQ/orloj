package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/store"
)

// scopedResourceKey returns the store-scoped cursor token namespace/name.
func scopedResourceKey(meta resources.ObjectMeta) string {
	return store.ScopedName(resources.NormalizeNamespace(meta.Namespace), meta.Name)
}

// normalizeListCursor accepts bare names (scoped to requestNS) or scoped
// namespace/name tokens for backward-compatible pagination.
func normalizeListCursor(after, requestNS string) string {
	if after == "" {
		return ""
	}
	if store.IsScopedName(after) {
		return after
	}
	return store.ScopedName(resources.NormalizeNamespace(requestNS), after)
}

// boundedSelectorPageLimit applies defaults and maxPaginationLimit for
// label-selector list pages so slice capacity is not user-controlled.
func boundedSelectorPageLimit(limit int) int {
	if limit <= 0 {
		limit = 100
	}
	if limit > maxPaginationLimit {
		return maxPaginationLimit
	}
	return limit
}

// scopedListContinue returns a continue token when more pages may exist.
func scopedListContinue(limit int, lastScannedKey string, hasMore bool) string {
	if limit > 0 && hasMore && lastScannedKey != "" {
		return lastScannedKey
	}
	return ""
}

type listCursorFetcher[T any] func(ctx context.Context, limit int, after, nsFilter string) ([]T, error)

// fetchListPage loads up to limit items, applying label selectors before
// finalizing the continue token so pages are not under-filled or empty with
// a non-empty cursor.
func fetchListPage[T any](
	ctx context.Context,
	r *http.Request,
	fetch listCursorFetcher[T],
	getMeta func(T) resources.ObjectMeta,
) ([]T, string, error) {
	selector, err := labelSelectorFilter(r)
	if err != nil {
		return nil, "", err
	}
	limit, _ := paginationParams(r)
	after := normalizeListCursor(cursorParam(r), requestNamespace(r))
	ns, hasNS := namespaceFilter(r)
	nsFilter := ""
	if hasNS {
		nsFilter = ns
	}
	return fetchListPageWithSelector(ctx, limit, after, nsFilter, selector, fetch, getMeta)
}

func fetchListPageWithSelector[T any](
	ctx context.Context,
	limit int,
	after, nsFilter string,
	selector map[string]string,
	fetch listCursorFetcher[T],
	getMeta func(T) resources.ObjectMeta,
) ([]T, string, error) {
	if len(selector) == 0 {
		items, err := fetch(ctx, limit, after, nsFilter)
		if err != nil {
			return nil, "", err
		}
		cont := ""
		if len(items) > 0 {
			lastKey := scopedResourceKey(getMeta(items[len(items)-1]))
			cont = scopedListContinue(limit, lastKey, len(items) >= limit)
		}
		return items, cont, nil
	}

	pageLimit := boundedSelectorPageLimit(limit)
	result := make([]T, 0, pageLimit)
	cursor := after
	lastScanned := ""
	hasMore := false
	for len(result) < pageLimit {
		batchSize := pageLimit * 2
		if batchSize <= 0 {
			batchSize = 100
		}
		batch, err := fetch(ctx, batchSize, cursor, nsFilter)
		if err != nil {
			return nil, "", err
		}
		if len(batch) == 0 {
			hasMore = false
			break
		}
		for _, item := range batch {
			lastScanned = scopedResourceKey(getMeta(item))
			if !matchMetadataFilters(getMeta(item), "", false, selector) {
				continue
			}
			result = append(result, item)
			if len(result) >= pageLimit {
				hasMore = true
				break
			}
		}
		if len(result) >= pageLimit {
			break
		}
		cursor = scopedResourceKey(getMeta(batch[len(batch)-1]))
		hasMore = len(batch) >= batchSize
		if len(batch) < batchSize {
			hasMore = false
			break
		}
	}
	if len(result) == 0 {
		return result, "", nil
	}
	if !hasMore {
		return result, "", nil
	}
	if lastScanned == "" {
		lastScanned = scopedResourceKey(getMeta(result[len(result)-1]))
	}
	return result, scopedListContinue(pageLimit, lastScanned, true), nil
}

func writeListPageError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if strings.Contains(err.Error(), "label selector") {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return true
	}
	return writeStoreFetchError(w, err)
}
