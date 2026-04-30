package store

import (
	"context"
	"fmt"
	"reflect"

	"github.com/OrlojHQ/orloj/resources"
)

func (s *AgentSystemStore) UpsertMovingKey(ctx context.Context, oldStoreKey string, item resources.AgentSystem) (resources.AgentSystem, error) {
	return jsonPayloadMovingKey(ctx, s.db, s.items, &s.mu, oldStoreKey, item,
		"AgentSystem", tableAgentSystems,
		func(it resources.AgentSystem) error { return it.Normalize() },
		func(it resources.AgentSystem) string { return scopedNameFromMeta(it.Metadata) },
		func(it resources.AgentSystem) any { return it.Spec },
		upsertAgentSystemSQL,
		func(it *resources.AgentSystem) *resources.ObjectMeta { return &it.Metadata },
		func(ctx context.Context, it resources.AgentSystem) (resources.AgentSystem, error) {
			return s.Upsert(ctx, it)
		},
	)
}

func (s *ToolStore) UpsertMovingKey(ctx context.Context, oldStoreKey string, item resources.Tool) (resources.Tool, error) {
	return jsonPayloadMovingKey(ctx, s.db, s.items, &s.mu, oldStoreKey, item,
		"Tool", tableTools,
		func(it resources.Tool) error { return it.Normalize() },
		func(it resources.Tool) string { return scopedNameFromMeta(it.Metadata) },
		func(it resources.Tool) any { return it.Spec },
		upsertToolSQL,
		func(it *resources.Tool) *resources.ObjectMeta { return &it.Metadata },
		func(ctx context.Context, it resources.Tool) (resources.Tool, error) { return s.Upsert(ctx, it) },
	)
}

func (s *MemoryStore) UpsertMovingKey(ctx context.Context, oldStoreKey string, item resources.Memory) (resources.Memory, error) {
	return jsonPayloadMovingKey(ctx, s.db, s.items, &s.mu, oldStoreKey, item,
		"Memory", tableMemories,
		func(it resources.Memory) error { return it.Normalize() },
		func(it resources.Memory) string { return scopedNameFromMeta(it.Metadata) },
		func(it resources.Memory) any { return it.Spec },
		upsertMemorySQL,
		func(it *resources.Memory) *resources.ObjectMeta { return &it.Metadata },
		func(ctx context.Context, it resources.Memory) (resources.Memory, error) { return s.Upsert(ctx, it) },
	)
}

func (s *ContextAdapterStore) UpsertMovingKey(ctx context.Context, oldStoreKey string, item resources.ContextAdapter) (resources.ContextAdapter, error) {
	return jsonPayloadMovingKey(ctx, s.db, s.items, &s.mu, oldStoreKey, item,
		"ContextAdapter", tableContextAdapters,
		func(it resources.ContextAdapter) error { return it.Normalize() },
		func(it resources.ContextAdapter) string { return scopedNameFromMeta(it.Metadata) },
		func(it resources.ContextAdapter) any { return it.Spec },
		upsertContextAdapterSQL,
		func(it *resources.ContextAdapter) *resources.ObjectMeta { return &it.Metadata },
		func(ctx context.Context, it resources.ContextAdapter) (resources.ContextAdapter, error) {
			return s.Upsert(ctx, it)
		},
	)
}

func (s *AgentPolicyStore) UpsertMovingKey(ctx context.Context, oldStoreKey string, item resources.AgentPolicy) (resources.AgentPolicy, error) {
	return jsonPayloadMovingKey(ctx, s.db, s.items, &s.mu, oldStoreKey, item,
		"AgentPolicy", tableAgentPolicies,
		func(it resources.AgentPolicy) error { return it.Normalize() },
		func(it resources.AgentPolicy) string { return scopedNameFromMeta(it.Metadata) },
		func(it resources.AgentPolicy) any { return it.Spec },
		upsertAgentPolicySQL,
		func(it *resources.AgentPolicy) *resources.ObjectMeta { return &it.Metadata },
		func(ctx context.Context, it resources.AgentPolicy) (resources.AgentPolicy, error) {
			return s.Upsert(ctx, it)
		},
	)
}

func (s *AgentRoleStore) UpsertMovingKey(ctx context.Context, oldStoreKey string, item resources.AgentRole) (resources.AgentRole, error) {
	return jsonPayloadMovingKey(ctx, s.db, s.items, &s.mu, oldStoreKey, item,
		"AgentRole", tableAgentRoles,
		func(it resources.AgentRole) error { return it.Normalize() },
		func(it resources.AgentRole) string { return scopedNameFromMeta(it.Metadata) },
		func(it resources.AgentRole) any { return it.Spec },
		upsertAgentRoleSQL,
		func(it *resources.AgentRole) *resources.ObjectMeta { return &it.Metadata },
		func(ctx context.Context, it resources.AgentRole) (resources.AgentRole, error) {
			return s.Upsert(ctx, it)
		},
	)
}

func (s *ToolPermissionStore) UpsertMovingKey(ctx context.Context, oldStoreKey string, item resources.ToolPermission) (resources.ToolPermission, error) {
	return jsonPayloadMovingKey(ctx, s.db, s.items, &s.mu, oldStoreKey, item,
		"ToolPermission", tableToolPermissions,
		func(it resources.ToolPermission) error { return it.Normalize() },
		func(it resources.ToolPermission) string { return scopedNameFromMeta(it.Metadata) },
		func(it resources.ToolPermission) any { return it.Spec },
		upsertToolPermissionSQL,
		func(it *resources.ToolPermission) *resources.ObjectMeta { return &it.Metadata },
		func(ctx context.Context, it resources.ToolPermission) (resources.ToolPermission, error) {
			return s.Upsert(ctx, it)
		},
	)
}

func (s *TaskScheduleStore) UpsertMovingKey(ctx context.Context, oldStoreKey string, item resources.TaskSchedule) (resources.TaskSchedule, error) {
	return jsonPayloadMovingKey(ctx, s.db, s.items, &s.mu, oldStoreKey, item,
		"TaskSchedule", tableTaskSchedules,
		func(it resources.TaskSchedule) error { return it.Normalize() },
		func(it resources.TaskSchedule) string { return scopedNameFromMeta(it.Metadata) },
		func(it resources.TaskSchedule) any { return it.Spec },
		upsertTaskScheduleSQL,
		func(it *resources.TaskSchedule) *resources.ObjectMeta { return &it.Metadata },
		func(ctx context.Context, it resources.TaskSchedule) (resources.TaskSchedule, error) {
			return s.Upsert(ctx, it)
		},
	)
}

func (s *TaskWebhookStore) UpsertMovingKey(ctx context.Context, oldStoreKey string, item resources.TaskWebhook) (resources.TaskWebhook, error) {
	return jsonPayloadMovingKey(ctx, s.db, s.items, &s.mu, oldStoreKey, item,
		"TaskWebhook", tableTaskWebhooks,
		func(it resources.TaskWebhook) error { return it.Normalize() },
		func(it resources.TaskWebhook) string { return scopedNameFromMeta(it.Metadata) },
		func(it resources.TaskWebhook) any { return it.Spec },
		upsertTaskWebhookSQL,
		func(it *resources.TaskWebhook) *resources.ObjectMeta { return &it.Metadata },
		func(ctx context.Context, it resources.TaskWebhook) (resources.TaskWebhook, error) {
			return s.Upsert(ctx, it)
		},
	)
}

func (s *WorkerStore) UpsertMovingKey(ctx context.Context, oldStoreKey string, item resources.Worker) (resources.Worker, error) {
	return jsonPayloadMovingKey(ctx, s.db, s.items, &s.mu, oldStoreKey, item,
		"Worker", tableWorkers,
		func(it resources.Worker) error { return it.Normalize() },
		func(it resources.Worker) string { return scopedNameFromMeta(it.Metadata) },
		func(it resources.Worker) any { return it.Spec },
		upsertWorkerSQL,
		func(it *resources.Worker) *resources.ObjectMeta { return &it.Metadata },
		func(ctx context.Context, it resources.Worker) (resources.Worker, error) { return s.Upsert(ctx, it) },
	)
}

func (s *McpServerStore) UpsertMovingKey(ctx context.Context, oldStoreKey string, item resources.McpServer) (resources.McpServer, error) {
	return jsonPayloadMovingKey(ctx, s.db, s.items, &s.mu, oldStoreKey, item,
		"McpServer", tableMcpServers,
		func(it resources.McpServer) error { return it.Normalize() },
		func(it resources.McpServer) string { return scopedNameFromMeta(it.Metadata) },
		func(it resources.McpServer) any { return it.Spec },
		upsertMcpServerSQL,
		func(it *resources.McpServer) *resources.ObjectMeta { return &it.Metadata },
		func(ctx context.Context, it resources.McpServer) (resources.McpServer, error) {
			return s.Upsert(ctx, it)
		},
	)
}

func (s *SecretStore) UpsertMovingKey(ctx context.Context, oldStoreKey string, item resources.Secret) (resources.Secret, error) {
	if err := item.Normalize(); err != nil {
		return resources.Secret{}, err
	}
	if s.requireEncryption && len(s.encryptionKey) == 0 && len(item.Spec.Data) > 0 {
		return resources.Secret{}, fmt.Errorf("secret encryption is required but no encryption key is configured; set ORLOJ_SECRET_ENCRYPTION_KEY")
	}
	oldKey := normalizeLookupName(oldStoreKey)
	newKey := scopedNameFromMeta(item.Metadata)
	if oldKey == newKey {
		return s.Upsert(ctx, item)
	}

	if s.db != nil {
		tx, err := s.db.Begin()
		if err != nil {
			return resources.Secret{}, err
		}
		defer tx.Rollback()

		existingAtOld, foundOld, err := s.getDecryptedFrom(ctx, tx, oldKey)
		if err != nil {
			return resources.Secret{}, err
		}
		if !foundOld {
			return resources.Secret{}, fmt.Errorf("secret %q not found", oldStoreKey)
		}

		_, foundNew, err := getFromTableForUpdate[resources.Secret](ctx, tx, tableSecrets, newKey)
		if err != nil {
			return resources.Secret{}, err
		}
		if foundNew {
			return resources.Secret{}, fmt.Errorf("cannot rename secret to %q: %w", item.Metadata.Name, ErrResourceAlreadyExists)
		}

		specChanged := !reflect.DeepEqual(existingAtOld.Spec, item.Spec)
		if err := initializeUpdateMetadata("Secret", &item.Metadata, existingAtOld.Metadata, specChanged); err != nil {
			return resources.Secret{}, err
		}

		res, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE name = $1`, tableSecrets), oldKey)
		if err != nil {
			return resources.Secret{}, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return resources.Secret{}, err
		}
		if n == 0 {
			return resources.Secret{}, fmt.Errorf("secret %q not found during rename", oldKey)
		}

		toStore := item
		if len(s.encryptionKey) > 0 && len(toStore.Spec.Data) > 0 {
			enc, err := encryptSecretData(s.encryptionKey, toStore.Spec.Data)
			if err != nil {
				return resources.Secret{}, err
			}
			toStore.Spec.Data = enc
		}
		if err := upsertSecretSQL(ctx, tx, newKey, toStore); err != nil {
			return resources.Secret{}, err
		}
		if err := tx.Commit(); err != nil {
			return resources.Secret{}, err
		}
		return item, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	existingAtOld, foundOld := s.items[oldKey]
	if !foundOld {
		return resources.Secret{}, fmt.Errorf("secret %q not found", oldStoreKey)
	}
	if _, taken := s.items[newKey]; taken {
		return resources.Secret{}, fmt.Errorf("cannot rename secret to %q: %w", item.Metadata.Name, ErrResourceAlreadyExists)
	}

	specChanged := !reflect.DeepEqual(existingAtOld.Spec, item.Spec)
	if err := initializeUpdateMetadata("Secret", &item.Metadata, existingAtOld.Metadata, specChanged); err != nil {
		return resources.Secret{}, err
	}
	delete(s.items, oldKey)
	s.items[newKey] = item
	return item, nil
}

func (s *TaskStore) UpsertMovingKey(ctx context.Context, oldStoreKey string, item resources.Task) (resources.Task, error) {
	if err := item.Normalize(); err != nil {
		return resources.Task{}, err
	}
	oldKey := normalizeLookupName(oldStoreKey)
	newKey := scopedNameFromMeta(item.Metadata)
	if oldKey == newKey {
		return s.Upsert(ctx, item)
	}

	if s.db != nil {
		tx, err := s.db.Begin()
		if err != nil {
			return resources.Task{}, err
		}
		defer tx.Rollback()

		metaOld, foundOld, err := getUpsertMetaForUpdate(ctx, tx, tableTasks, oldKey)
		if err != nil {
			return resources.Task{}, err
		}
		if !foundOld {
			return resources.Task{}, fmt.Errorf("task %q not found", oldStoreKey)
		}

		_, foundNew, err := getUpsertMetaForUpdate(ctx, tx, tableTasks, newKey)
		if err != nil {
			return resources.Task{}, err
		}
		if foundNew {
			return resources.Task{}, fmt.Errorf("cannot rename task to %q: %w", item.Metadata.Name, ErrResourceAlreadyExists)
		}

		newHash := specHash(item.Spec)
		specChanged := metaOld.SpecHash == "" || metaOld.SpecHash != newHash
		existing := resources.ObjectMeta{
			Generation:      metaOld.Generation,
			ResourceVersion: metaOld.ResourceVersion,
			CreatedAt:       metaOld.CreatedAt,
		}
		if err := initializeUpdateMetadata("Task", &item.Metadata, existing, specChanged); err != nil {
			return resources.Task{}, err
		}

		res, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE name = $1`, tableTasks), oldKey)
		if err != nil {
			return resources.Task{}, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return resources.Task{}, err
		}
		if n == 0 {
			return resources.Task{}, fmt.Errorf("task %q not found during rename", oldKey)
		}

		if err := renameTaskLogsSQL(ctx, tx, oldKey, newKey); err != nil {
			return resources.Task{}, err
		}
		if err := upsertTaskSQL(ctx, tx, newKey, item); err != nil {
			return resources.Task{}, err
		}
		if err := tx.Commit(); err != nil {
			return resources.Task{}, err
		}
		return item, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	existingAtOld, foundOld := s.items[oldKey]
	if !foundOld {
		return resources.Task{}, fmt.Errorf("task %q not found", oldStoreKey)
	}
	if _, taken := s.items[newKey]; taken {
		return resources.Task{}, fmt.Errorf("cannot rename task to %q: %w", item.Metadata.Name, ErrResourceAlreadyExists)
	}

	specChanged := !reflect.DeepEqual(existingAtOld.Spec, item.Spec)
	if err := initializeUpdateMetadata("Task", &item.Metadata, existingAtOld.Metadata, specChanged); err != nil {
		return resources.Task{}, err
	}
	delete(s.items, oldKey)
	s.items[newKey] = item
	if logs, ok := s.logs[oldKey]; ok {
		s.logs[newKey] = logs
		delete(s.logs, oldKey)
	}
	return item, nil
}
