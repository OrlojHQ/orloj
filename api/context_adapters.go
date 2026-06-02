package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/store"
)

func (s *Server) handleContextAdapters(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, cont, err := fetchListPage(r.Context(), r, s.stores.ContextAdapters.ListCursor, func(item resources.ContextAdapter) resources.ObjectMeta { return item.Metadata })
		if writeListPageError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, resources.ContextAdapterList{ListMeta: resources.ListMeta{Continue: cont}, Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseContextAdapterManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		existing, ok, err := s.stores.ContextAdapters.Get(r.Context(), store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) {
			return
		}
		if ok {
			obj.Status = existing.Status
		}
		obj, err = s.stores.ContextAdapters.Upsert(r.Context(), obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.logApply("ContextAdapter", obj.Metadata.Name)
		s.publishResourceEvent("ContextAdapter", obj.Metadata.Name, "created", obj)
		writeJSON(w, http.StatusCreated, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleContextAdapterByName(w http.ResponseWriter, r *http.Request) {
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/context-adapters/"), "/")
	if name == "" {
		http.Error(w, "context adapter name is required", http.StatusBadRequest)
		return
	}
	key := scopedNameForRequest(r, name)
	switch r.Method {
	case http.MethodGet:
		obj, ok, err := s.stores.ContextAdapters.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("context adapter %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		if err := s.stores.ContextAdapters.Delete(r.Context(), key); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.publishResourceEvent("ContextAdapter", name, "deleted", map[string]any{"metadata": map[string]string{"name": name, "namespace": requestNamespace(r)}})
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseContextAdapterManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, ok, err := s.stores.ContextAdapters.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("context adapter %q not found", name), http.StatusNotFound)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := requireUpdatePrecondition(r.Header.Get("If-Match"), &obj.Metadata, current.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		obj.Status = current.Status
		bodyKey := store.ScopedName(requestNamespace(r), strings.TrimSpace(obj.Metadata.Name))
		var upsertErr error
		if bodyKey != key {
			obj, upsertErr = s.stores.ContextAdapters.UpsertMovingKey(r.Context(), key, obj)
		} else {
			obj, upsertErr = s.stores.ContextAdapters.Upsert(r.Context(), obj)
		}
		if upsertErr != nil {
			if errors.Is(upsertErr, store.ErrResourceAlreadyExists) {
				http.Error(w, upsertErr.Error(), http.StatusConflict)
				return
			}
			writeStoreError(w, upsertErr)
			return
		}
		s.publishResourceEvent("ContextAdapter", obj.Metadata.Name, "updated", obj)
		writeJSON(w, http.StatusOK, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
