package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/store"
)

func (s *Server) handleSealedSecrets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit, _ := paginationParams(r)
		after := cursorParam(r)
		ns, hasNS := namespaceFilter(r)
		nsFilter := ""
		if hasNS {
			nsFilter = ns
		}
		items, err := s.stores.SealedSecrets.ListCursor(r.Context(), limit, after, nsFilter)
		if writeStoreFetchError(w, err) {
			return
		}
		cont := listContinue(limit, len(items), "")
		if len(items) > 0 {
			cont = listContinue(limit, len(items), items[len(items)-1].Metadata.Name)
		}
		selector, err := labelSelectorFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(selector) > 0 {
			filtered := make([]resources.SealedSecret, 0, len(items))
			for _, item := range items {
				if !matchMetadataFilters(item.Metadata, "", false, selector) {
					continue
				}
				filtered = append(filtered, item)
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, resources.SealedSecretList{ListMeta: resources.ListMeta{Continue: cont}, Items: items})
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		obj, err := resources.ParseSealedSecretManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := applyRequestNamespace(r, &obj.Metadata); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		existing, ok, err := s.stores.SealedSecrets.Get(r.Context(), store.ScopedName(obj.Metadata.Namespace, obj.Metadata.Name))
		if writeStoreFetchError(w, err) {
			return
		}
		if ok {
			obj.Status = existing.Status
		}
		obj, err = s.stores.SealedSecrets.Upsert(r.Context(), obj)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		s.logApply("SealedSecret", obj.Metadata.Name)
		s.publishResourceEvent("SealedSecret", obj.Metadata.Name, "created", obj)
		writeJSON(w, http.StatusCreated, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSealedSecretByName(w http.ResponseWriter, r *http.Request) {
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/sealed-secrets/"), "/")
	if name == "" {
		http.Error(w, "sealed secret name is required", http.StatusBadRequest)
		return
	}
	key := scopedNameForRequest(r, name)
	switch r.Method {
	case http.MethodGet:
		obj, ok, err := s.stores.SealedSecrets.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("sealedsecret %q not found", name), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		if err := s.stores.SealedSecrets.Delete(r.Context(), key); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		s.publishResourceEvent("SealedSecret", name, "deleted", map[string]any{"metadata": map[string]string{"name": name, "namespace": requestNamespace(r)}})
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		current, ok, err := s.stores.SealedSecrets.Get(r.Context(), key)
		if writeStoreFetchError(w, err) {
			return
		}
		if !ok {
			http.Error(w, fmt.Sprintf("sealedsecret %q not found", name), http.StatusNotFound)
			return
		}
		obj, err := resources.ParseSealedSecretManifest(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
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
		if bodyKey != key {
			obj, err = s.stores.SealedSecrets.UpsertMovingKey(r.Context(), key, obj)
		} else {
			obj, err = s.stores.SealedSecrets.Upsert(r.Context(), obj)
		}
		if err != nil {
			if strings.Contains(err.Error(), store.ErrResourceAlreadyExists.Error()) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			writeStoreError(w, err)
			return
		}
		s.publishResourceEvent("SealedSecret", obj.Metadata.Name, "updated", obj)
		writeJSON(w, http.StatusOK, obj)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSealingKeyPublic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	active, ok, err := s.stores.SealingKeys.GetActive(r.Context())
	if err != nil {
		http.Error(w, "sealing key unavailable: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	if !ok {
		http.Error(w, "sealing key unavailable: no active sealing key is configured", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"keyId":        active.KeyID,
		"algorithm":    resources.SealingAlgorithm,
		"publicKeyPEM": active.PublicKeyPEM,
	})
}
