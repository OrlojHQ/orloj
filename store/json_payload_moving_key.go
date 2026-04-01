package store

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"sync"

	"github.com/OrlojHQ/orloj/resources"
)

// jsonPayloadMovingKey moves a JSON-row store entry from oldStoreKey to key(item) when they differ.
// normalize must validate/coerce item; keyOf returns scopedNameFromMeta; specOf returns the spec for
// change detection; metaOf returns &item.Metadata; upsertSQL writes one row; upsertSameKey is the
// store's ordinary Upsert used when old and new keys match.
func jsonPayloadMovingKey[T any](
	ctx context.Context,
	db *sql.DB,
	items map[string]T,
	memMu sync.Locker,
	oldStoreKey string,
	item T,
	kind string,
	table string,
	normalize func(T) error,
	keyOf func(T) string,
	specOf func(T) any,
	upsertSQL func(ctx context.Context, ex dbExecer, key string, item T) error,
	metaOf func(*T) *resources.ObjectMeta,
	upsertSameKey func(context.Context, T) (T, error),
) (T, error) {
	var zero T
	if err := normalize(item); err != nil {
		return zero, err
	}
	oldKey := normalizeLookupName(oldStoreKey)
	newKey := keyOf(item)
	if oldKey == newKey {
		return upsertSameKey(ctx, item)
	}

	if db != nil {
		tx, err := db.Begin()
		if err != nil {
			return zero, err
		}
		defer tx.Rollback()

		existingAtOld, foundOld, err := getFromTableForUpdate[T](ctx, tx, table, oldKey)
		if err != nil {
			return zero, err
		}
		if !foundOld {
			return zero, fmt.Errorf("%s %q not found", kind, oldStoreKey)
		}

		_, foundNew, err := getFromTableForUpdate[T](ctx, tx, table, newKey)
		if err != nil {
			return zero, err
		}
		if foundNew {
			return zero, fmt.Errorf("cannot rename %s to %q: %w", kind, metaOf(&item).Name, ErrResourceAlreadyExists)
		}

		specChanged := !reflect.DeepEqual(specOf(existingAtOld), specOf(item))
		if err := initializeUpdateMetadata(kind, metaOf(&item), *metaOf(&existingAtOld), specChanged); err != nil {
			return zero, err
		}

		res, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE name = $1`, table), oldKey)
		if err != nil {
			return zero, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return zero, err
		}
		if n == 0 {
			return zero, fmt.Errorf("%s %q not found during rename", kind, oldKey)
		}

		if err := upsertSQL(ctx, tx, newKey, item); err != nil {
			return zero, err
		}
		if err := tx.Commit(); err != nil {
			return zero, err
		}
		return item, nil
	}

	memMu.Lock()
	defer memMu.Unlock()
	existingAtOld, foundOld := items[oldKey]
	if !foundOld {
		return zero, fmt.Errorf("%s %q not found", kind, oldStoreKey)
	}
	if _, taken := items[newKey]; taken {
		return zero, fmt.Errorf("cannot rename %s to %q: %w", kind, metaOf(&item).Name, ErrResourceAlreadyExists)
	}

	specChanged := !reflect.DeepEqual(specOf(existingAtOld), specOf(item))
	if err := initializeUpdateMetadata(kind, metaOf(&item), *metaOf(&existingAtOld), specChanged); err != nil {
		return zero, err
	}
	delete(items, oldKey)
	items[newKey] = item
	return item, nil
}
