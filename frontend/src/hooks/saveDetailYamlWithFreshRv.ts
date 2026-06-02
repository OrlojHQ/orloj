import type { QueryClient } from "@tanstack/react-query";
import { get as apiGet, listAll } from "../api/client";
import { resourceKey } from "../api/hooks";
import { RESOURCE_ENDPOINTS, type ResourceKind, type Worker } from "../api/types";
import { ensureRequestNamespace } from "../utils/requestNamespace";

export type MutateUpdateArgs = {
  name: string;
  body: unknown;
  rv?: string;
  namespace?: string;
};

type HasMeta = { metadata: { name: string; resourceVersion?: string } };

/**
 * Re-fetch the resource, merge a fresh resourceVersion into the body, PUT, then update the React Query detail cache.
 * Avoids 404/409 from stale route names or resourceVersion when the list/detail cache races with invalidation.
 */
export async function saveDetailYamlWithFreshRv<T extends HasMeta>(opts: {
  queryClient: QueryClient;
  detailQueryKey: readonly unknown[];
  fetchFresh: () => Promise<T>;
  routeName: string;
  body: unknown;
  mutate: (args: MutateUpdateArgs) => Promise<T>;
  mutateExtras?: { namespace?: string };
  requestNamespace?: string;
}): Promise<T> {
  const { queryClient, detailQueryKey, fetchFresh, routeName, body, mutate, mutateExtras, requestNamespace } = opts;
  const qk = [...detailQueryKey];
  await queryClient.cancelQueries({ queryKey: qk });
  const fresh = await queryClient.fetchQuery({
    queryKey: qk,
    queryFn: fetchFresh,
    staleTime: 0,
  });
  const doc = body as T;
  const liveRv = fresh.metadata.resourceVersion ?? doc.metadata?.resourceVersion ?? "";
  const merged = { ...doc, metadata: { ...doc.metadata, resourceVersion: liveRv } } as T;
  const payload = requestNamespace ? ensureRequestNamespace(merged, requestNamespace) : merged;
  const updated = await mutate({
    name: routeName,
    body: payload,
    rv: liveRv,
    ...mutateExtras,
  });
  const newKey = [...detailQueryKey.slice(0, -1), updated.metadata.name];
  queryClient.setQueryData(newKey, updated);
  return updated;
}

/** Namespace-scoped resources using `resourceKey(kind, ns, name)` for GET/PUT and React Query cache. */
export async function saveNamespacedResourceYaml<T extends HasMeta>(
  queryClient: QueryClient,
  kind: ResourceKind,
  namespace: string,
  routeName: string,
  body: unknown,
  mutate: (args: MutateUpdateArgs) => Promise<T>,
  mutateExtras?: { namespace?: string },
): Promise<T> {
  const path = RESOURCE_ENDPOINTS[kind];
  const qk = resourceKey(kind, namespace, routeName);
  return saveDetailYamlWithFreshRv({
    queryClient,
    detailQueryKey: qk,
    fetchFresh: () =>
      apiGet<T>(path, routeName, mutateExtras?.namespace ? { namespace: mutateExtras.namespace } : undefined),
    routeName,
    body,
    mutate,
    mutateExtras: { ...mutateExtras, namespace: mutateExtras?.namespace ?? namespace },
    requestNamespace: namespace,
  });
}

/** Workers use a cluster-wide list for GET and `["Worker","detail",name]` for the detail cache. */
export async function saveWorkerYaml(
  queryClient: QueryClient,
  routeName: string,
  body: unknown,
  mutate: (args: MutateUpdateArgs) => Promise<Worker>,
): Promise<Worker> {
  const qk = ["Worker", "detail", routeName];
  await queryClient.cancelQueries({ queryKey: qk });
  const fresh = await queryClient.fetchQuery({
    queryKey: qk,
    queryFn: async () => {
      const items = await listAll<Worker>(RESOURCE_ENDPOINTS.Worker, { allNamespaces: true });
      const hit = items.find((w) => w.metadata.name === routeName);
      if (!hit) throw new Error(`Worker "${routeName}" not found`);
      return hit;
    },
    staleTime: 0,
  });
  const doc = body as Worker;
  const liveRv = fresh.metadata.resourceVersion ?? doc.metadata?.resourceVersion ?? "";
  const merged = { ...doc, metadata: { ...doc.metadata, resourceVersion: liveRv } };
  const ns = fresh.metadata.namespace?.trim() || "default";
  const payload = ensureRequestNamespace(merged, ns);
  const updated = await mutate({
    name: routeName,
    body: payload,
    rv: liveRv,
    namespace: ns,
  });
  queryClient.setQueryData(["Worker", "detail", updated.metadata.name], updated);
  return updated;
}
