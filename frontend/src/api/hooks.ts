import { useEffect, useMemo } from "react";
import {
  useQuery,
  useMutation,
  useQueryClient,
  useInfiniteQuery,
  type UseQueryResult,
} from "@tanstack/react-query";
import * as client from "./client";
import type {
  Agent,
  AgentSystem,
  ModelEndpoint,
  Tool,
  Secret,
  Memory,
  MemoryEntriesResponse,
  AgentPolicy,
  AgentRole,
  ToolPermission,
  ToolApproval,
  Task,
  TaskSchedule,
  TaskWebhook,
  Worker,
  McpServer,
  TaskMetrics,
  TaskMessage,
  CapabilitySnapshot,
} from "./types";
import { RESOURCE_ENDPOINTS } from "./types";
import { useAppStore } from "../store";

const REFETCH_INTERVAL = 8000;
/** Page size for resource lists that auto-fetch all pages via cursor. */
const RESOURCE_LIST_PAGE_LIMIT = 200;
/** Page size for the Tasks index (explicit “Load more”). */
const TASK_LIST_PAGE_LIMIT = 50;

function useNamespace() {
  return useAppStore((s) => s.namespace);
}

function resourceKey(kind: string, ns: string, name?: string) {
  return name ? [kind, ns, name] : [kind, ns];
}

export type ResourceListOptions = {
  allNamespaces?: boolean;
  labelSelector?: string;
};

function useResourceList<T>(kind: string, path: string, options?: ResourceListOptions) {
  const ns = useNamespace();
  const labelKey = options?.labelSelector?.trim() ?? "";
  const queryKey = options?.allNamespaces
    ? [kind, "list", labelKey]
    : [...resourceKey(kind, ns), labelKey];

  const infinite = useInfiniteQuery({
    queryKey,
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) =>
      client.list<T>(path, {
        limit: RESOURCE_LIST_PAGE_LIMIT,
        after: pageParam,
        allNamespaces: options?.allNamespaces,
        labelSelector: options?.labelSelector?.trim() || undefined,
      }),
    getNextPageParam: (lastPage) => {
      const c = lastPage.continue?.trim();
      return c || undefined;
    },
    refetchInterval: REFETCH_INTERVAL,
  });

  useEffect(() => {
    if (!infinite.hasNextPage || infinite.isFetchingNextPage) return;
    void infinite.fetchNextPage();
  }, [infinite.hasNextPage, infinite.isFetchingNextPage, infinite.fetchNextPage]);

  const flatData = useMemo(
    () => infinite.data?.pages.flatMap((p) => p.items ?? []),
    [infinite.data],
  );

  return {
    ...infinite,
    data: flatData,
  } as unknown as UseQueryResult<T[], Error>;
}

function useResourceGet<T>(kind: string, path: string, name: string) {
  const ns = useNamespace();
  return useQuery<T>({
    queryKey: resourceKey(kind, ns, name),
    queryFn: () => client.get<T>(path, name),
    enabled: !!name,
    refetchInterval: REFETCH_INTERVAL,
  });
}

export function useAgentSystems() {
  return useResourceList<AgentSystem>("AgentSystem", RESOURCE_ENDPOINTS.AgentSystem);
}
export function useAgentSystem(name: string) {
  return useResourceGet<AgentSystem>("AgentSystem", RESOURCE_ENDPOINTS.AgentSystem, name);
}

export function useAgents() {
  return useResourceList<Agent>("Agent", RESOURCE_ENDPOINTS.Agent);
}
export function useAgent(name: string) {
  return useResourceGet<Agent>("Agent", RESOURCE_ENDPOINTS.Agent, name);
}

export function useModelEndpoints() {
  return useResourceList<ModelEndpoint>("ModelEndpoint", RESOURCE_ENDPOINTS.ModelEndpoint);
}
export function useModelEndpoint(name: string) {
  return useResourceGet<ModelEndpoint>("ModelEndpoint", RESOURCE_ENDPOINTS.ModelEndpoint, name);
}

export function useTools(listOpts?: Pick<ResourceListOptions, "labelSelector">) {
  return useResourceList<Tool>("Tool", RESOURCE_ENDPOINTS.Tool, {
    labelSelector: listOpts?.labelSelector,
  });
}
export function useTool(name: string) {
  return useResourceGet<Tool>("Tool", RESOURCE_ENDPOINTS.Tool, name);
}

export function useSecrets() {
  return useResourceList<Secret>("Secret", RESOURCE_ENDPOINTS.Secret);
}
export function useSecret(name: string) {
  return useResourceGet<Secret>("Secret", RESOURCE_ENDPOINTS.Secret, name);
}

export function useMemories() {
  return useResourceList<Memory>("Memory", RESOURCE_ENDPOINTS.Memory);
}
export function useMemory(name: string) {
  return useResourceGet<Memory>("Memory", RESOURCE_ENDPOINTS.Memory, name);
}
export function useMemoryEntries(
  name: string,
  params?: { prefix?: string; q?: string; limit?: number },
) {
  const ns = useNamespace();
  return useQuery<MemoryEntriesResponse>({
    queryKey: ["MemoryEntries", ns, name, params],
    queryFn: () => client.listMemoryEntries(name, params),
    enabled: !!name,
    refetchInterval: REFETCH_INTERVAL,
  });
}

export function useAgentPolicies() {
  return useResourceList<AgentPolicy>("AgentPolicy", RESOURCE_ENDPOINTS.AgentPolicy);
}
export function useAgentPolicy(name: string) {
  return useResourceGet<AgentPolicy>("AgentPolicy", RESOURCE_ENDPOINTS.AgentPolicy, name);
}

export function useAgentRoles() {
  return useResourceList<AgentRole>("AgentRole", RESOURCE_ENDPOINTS.AgentRole);
}
export function useAgentRole(name: string) {
  return useResourceGet<AgentRole>("AgentRole", RESOURCE_ENDPOINTS.AgentRole, name);
}

export function useToolPermissions() {
  return useResourceList<ToolPermission>("ToolPermission", RESOURCE_ENDPOINTS.ToolPermission);
}
export function useToolPermission(name: string) {
  return useResourceGet<ToolPermission>("ToolPermission", RESOURCE_ENDPOINTS.ToolPermission, name);
}

export function useToolApprovals() {
  return useResourceList<ToolApproval>("ToolApproval", RESOURCE_ENDPOINTS.ToolApproval);
}
export function useToolApproval(name: string) {
  return useResourceGet<ToolApproval>("ToolApproval", RESOURCE_ENDPOINTS.ToolApproval, name);
}

export function useApproveToolApproval() {
  const qc = useQueryClient();
  const ns = useNamespace();
  return useMutation({
    mutationFn: (name: string) => client.postAction("tool-approvals", name, "approve"),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["ToolApproval", ns] }),
  });
}

export function useDenyToolApproval() {
  const qc = useQueryClient();
  const ns = useNamespace();
  return useMutation({
    mutationFn: (name: string) => client.postAction("tool-approvals", name, "deny"),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["ToolApproval", ns] }),
  });
}

/** Full task list for dashboard, search, and dropdowns (auto-paginates all pages). */
export function useTasks() {
  return useResourceList<Task>("Task", RESOURCE_ENDPOINTS.Task);
}

/** Paged task list for the Tasks index (`limit` / cursor, explicit “Load more”). */
export function useTaskList(listOpts?: Pick<ResourceListOptions, "labelSelector">) {
  const ns = useNamespace();
  const labelKey = listOpts?.labelSelector?.trim() ?? "";
  const infinite = useInfiniteQuery({
    queryKey: [...resourceKey("Task", ns), "paged", labelKey],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) =>
      client.list<Task>(RESOURCE_ENDPOINTS.Task, {
        limit: TASK_LIST_PAGE_LIMIT,
        after: pageParam,
        labelSelector: listOpts?.labelSelector?.trim() || undefined,
      }),
    getNextPageParam: (lastPage) => lastPage.continue?.trim() || undefined,
    refetchInterval: REFETCH_INTERVAL,
  });

  const data = useMemo(
    () => infinite.data?.pages.flatMap((p) => p.items ?? []),
    [infinite.data],
  );

  return {
    data,
    isLoading: infinite.isPending && infinite.isFetching,
    isError: infinite.isError,
    error: infinite.error,
    refetch: infinite.refetch,
    hasNextPage: infinite.hasNextPage,
    fetchNextPage: infinite.fetchNextPage,
    isFetchingNextPage: infinite.isFetchingNextPage,
    isFetching: infinite.isFetching,
  };
}

export function useTask(name: string) {
  return useResourceGet<Task>("Task", RESOURCE_ENDPOINTS.Task, name);
}

export function useTaskSchedules() {
  return useResourceList<TaskSchedule>("TaskSchedule", RESOURCE_ENDPOINTS.TaskSchedule);
}
export function useTaskSchedule(name: string) {
  return useResourceGet<TaskSchedule>("TaskSchedule", RESOURCE_ENDPOINTS.TaskSchedule, name);
}

export function useTaskWebhooks() {
  return useResourceList<TaskWebhook>("TaskWebhook", RESOURCE_ENDPOINTS.TaskWebhook);
}
export function useTaskWebhook(name: string) {
  return useResourceGet<TaskWebhook>("TaskWebhook", RESOURCE_ENDPOINTS.TaskWebhook, name);
}

export function useTaskMessages(name: string, filters?: Record<string, string>) {
  const ns = useNamespace();
  return useQuery<TaskMessage[]>({
    queryKey: ["TaskMessages", ns, name, filters],
    queryFn: () => client.getMessages<TaskMessage>(name, filters),
    enabled: !!name,
    refetchInterval: REFETCH_INTERVAL,
  });
}

export function useTaskMetrics(name: string) {
  const ns = useNamespace();
  return useQuery<TaskMetrics>({
    queryKey: ["TaskMetrics", ns, name],
    queryFn: () => client.getMetrics<TaskMetrics>(name),
    enabled: !!name,
    refetchInterval: REFETCH_INTERVAL,
  });
}

export function useTaskLogs(name: string) {
  const ns = useNamespace();
  return useQuery<string>({
    queryKey: ["TaskLogs", ns, name],
    queryFn: () => client.getLogs("tasks", name),
    enabled: !!name,
    refetchInterval: REFETCH_INTERVAL,
  });
}

export function useAgentLogs(name: string) {
  const ns = useNamespace();
  return useQuery<string>({
    queryKey: ["AgentLogs", ns, name],
    queryFn: () => client.getLogs("agents", name),
    enabled: !!name,
    refetchInterval: REFETCH_INTERVAL,
  });
}

export function useWorkers() {
  return useResourceList<Worker>("Worker", RESOURCE_ENDPOINTS.Worker, { allNamespaces: true });
}

export function useMcpServers() {
  return useResourceList<McpServer>("McpServer", RESOURCE_ENDPOINTS.McpServer);
}
export function useMcpServer(name: string) {
  return useResourceGet<McpServer>("McpServer", RESOURCE_ENDPOINTS.McpServer, name);
}

/**
 * Resolves a worker by short name across namespaces (list is cluster-wide; GET is namespace-scoped).
 * Uses cursor-paginated `listAll` so large worker sets stay bounded per request.
 */
export function useWorker(name: string) {
  return useQuery<Worker>({
    queryKey: ["Worker", "detail", name],
    queryFn: async () => {
      const items = await client.listAll<Worker>(RESOURCE_ENDPOINTS.Worker, { allNamespaces: true });
      const hit = items.find((w) => w.metadata.name === name);
      if (!hit) {
        throw new Error(`Worker "${name}" not found`);
      }
      return hit;
    },
    enabled: !!name,
    refetchInterval: REFETCH_INTERVAL,
  });
}

export function useNamespaces() {
  return useQuery<string[]>({
    queryKey: ["namespaces"],
    queryFn: client.listNamespaces,
    refetchInterval: REFETCH_INTERVAL,
  });
}

export function useHealthCheck() {
  return useQuery<boolean>({
    queryKey: ["healthCheck"],
    queryFn: client.healthCheck,
    refetchInterval: 10000,
  });
}

export function useCapabilities() {
  return useQuery<CapabilitySnapshot>({
    queryKey: ["capabilities"],
    queryFn: () => client.getCapabilities<CapabilitySnapshot>(),
    staleTime: 60_000,
  });
}

export function useCreateResource(kind: string) {
  const qc = useQueryClient();
  const ns = useNamespace();
  const path = RESOURCE_ENDPOINTS[kind as keyof typeof RESOURCE_ENDPOINTS];
  return useMutation({
    mutationFn: (body: unknown) => client.create(path, body),
    onSuccess: () => {
      if (kind === "Worker") {
        qc.invalidateQueries({ queryKey: ["Worker"] });
      } else {
        qc.invalidateQueries({ queryKey: [kind, ns] });
      }
    },
  });
}

export function useUpdateResource(kind: string) {
  const qc = useQueryClient();
  const ns = useNamespace();
  const path = RESOURCE_ENDPOINTS[kind as keyof typeof RESOURCE_ENDPOINTS];
  return useMutation({
    mutationFn: ({
      name,
      body,
      rv,
      namespace: resourceNs,
    }: {
      name: string;
      body: unknown;
      rv?: string;
      namespace?: string;
    }) => client.update(path, name, body, rv, resourceNs ? { namespace: resourceNs } : undefined),
    onSuccess: () => {
      if (kind === "Worker") {
        qc.invalidateQueries({ queryKey: ["Worker"] });
      } else {
        qc.invalidateQueries({ queryKey: [kind, ns] });
      }
    },
  });
}

export function useDeleteResource(kind: string) {
  const qc = useQueryClient();
  const ns = useNamespace();
  const path = RESOURCE_ENDPOINTS[kind as keyof typeof RESOURCE_ENDPOINTS];
  return useMutation({
    mutationFn: (target: string | { name: string; namespace?: string }) => {
      if (typeof target === "string") {
        return client.del(path, target);
      }
      return client.del(path, target.name, target.namespace ? { namespace: target.namespace } : undefined);
    },
    onSuccess: () => {
      if (kind === "Worker") {
        qc.invalidateQueries({ queryKey: ["Worker"] });
      } else {
        qc.invalidateQueries({ queryKey: [kind, ns] });
      }
    },
  });
}
