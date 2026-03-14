import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import * as client from "./client";
import type {
  Agent,
  AgentSystem,
  ModelEndpoint,
  Tool,
  Secret,
  Memory,
  AgentPolicy,
  AgentRole,
  ToolPermission,
  Task,
  TaskSchedule,
  TaskWebhook,
  Worker,
  TaskMetrics,
  TaskMessage,
} from "./types";
import { RESOURCE_ENDPOINTS } from "./types";
import { useAppStore } from "../store";

const REFETCH_INTERVAL = 8000;

function useNamespace() {
  return useAppStore((s) => s.namespace);
}

function resourceKey(kind: string, ns: string, name?: string) {
  return name ? [kind, ns, name] : [kind, ns];
}

function useResourceList<T>(kind: string, path: string) {
  const ns = useNamespace();
  return useQuery<T[]>({
    queryKey: resourceKey(kind, ns),
    queryFn: () => client.list<T>(path),
    refetchInterval: REFETCH_INTERVAL,
  });
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

export function useTools() {
  return useResourceList<Tool>("Tool", RESOURCE_ENDPOINTS.Tool);
}
export function useTool(name: string) {
  return useResourceGet<Tool>("Tool", RESOURCE_ENDPOINTS.Tool, name);
}

export function useSecrets() {
  return useResourceList<Secret>("Secret", RESOURCE_ENDPOINTS.Secret);
}

export function useMemories() {
  return useResourceList<Memory>("Memory", RESOURCE_ENDPOINTS.Memory);
}

export function useAgentPolicies() {
  return useResourceList<AgentPolicy>("AgentPolicy", RESOURCE_ENDPOINTS.AgentPolicy);
}

export function useAgentRoles() {
  return useResourceList<AgentRole>("AgentRole", RESOURCE_ENDPOINTS.AgentRole);
}

export function useToolPermissions() {
  return useResourceList<ToolPermission>("ToolPermission", RESOURCE_ENDPOINTS.ToolPermission);
}

export function useTasks() {
  return useResourceList<Task>("Task", RESOURCE_ENDPOINTS.Task);
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
  return useResourceList<Worker>("Worker", RESOURCE_ENDPOINTS.Worker);
}
export function useWorker(name: string) {
  return useResourceGet<Worker>("Worker", RESOURCE_ENDPOINTS.Worker, name);
}

export function useHealthCheck() {
  return useQuery<boolean>({
    queryKey: ["healthCheck"],
    queryFn: client.healthCheck,
    refetchInterval: 10000,
  });
}

export function useCreateResource(kind: string) {
  const qc = useQueryClient();
  const ns = useNamespace();
  const path = RESOURCE_ENDPOINTS[kind as keyof typeof RESOURCE_ENDPOINTS];
  return useMutation({
    mutationFn: (body: unknown) => client.create(path, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: [kind, ns] }),
  });
}

export function useUpdateResource(kind: string) {
  const qc = useQueryClient();
  const ns = useNamespace();
  const path = RESOURCE_ENDPOINTS[kind as keyof typeof RESOURCE_ENDPOINTS];
  return useMutation({
    mutationFn: ({ name, body, rv }: { name: string; body: unknown; rv?: string }) =>
      client.update(path, name, body, rv),
    onSuccess: () => qc.invalidateQueries({ queryKey: [kind, ns] }),
  });
}

export function useDeleteResource(kind: string) {
  const qc = useQueryClient();
  const ns = useNamespace();
  const path = RESOURCE_ENDPOINTS[kind as keyof typeof RESOURCE_ENDPOINTS];
  return useMutation({
    mutationFn: (name: string) => client.del(path, name),
    onSuccess: () => qc.invalidateQueries({ queryKey: [kind, ns] }),
  });
}
