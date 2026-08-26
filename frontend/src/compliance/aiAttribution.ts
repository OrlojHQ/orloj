import type { Agent, ModelEndpoint } from "../api/types";

export interface AiAttribution {
  provider: string;
  modelId?: string;
}

/**
 * Resolve only the public provider/model fields from one unambiguous
 * agent→model_ref→endpoint chain. Secret refs, endpoint URLs, and endpoint
 * options are intentionally outside this helper's return type.
 */
export function resolveAgentAiAttribution(
  agentName: string | null | undefined,
  agents: readonly Agent[],
  endpoints: readonly ModelEndpoint[],
): AiAttribution | undefined {
  const normalizedAgentName = agentName?.trim();
  if (!normalizedAgentName || normalizedAgentName.toLowerCase() === "system") return undefined;

  const matchingAgents = agents.filter((agent) => agent.metadata.name === normalizedAgentName);
  if (matchingAgents.length !== 1) return undefined;
  const agent = matchingAgents[0];
  const modelRef = agent.spec.model_ref?.trim();
  if (!modelRef) return undefined;

  const matchingEndpoints = endpoints.filter((endpoint) => (
    endpoint.metadata.name === modelRef
    && (!agent.metadata.namespace || !endpoint.metadata.namespace || endpoint.metadata.namespace === agent.metadata.namespace)
  ));
  if (matchingEndpoints.length !== 1) return undefined;

  const endpoint = matchingEndpoints[0];
  const provider = endpoint.spec.provider?.trim();
  const modelId = endpoint.spec.default_model?.trim();
  if (!provider) return undefined;
  return modelId ? { provider, modelId } : { provider };
}
