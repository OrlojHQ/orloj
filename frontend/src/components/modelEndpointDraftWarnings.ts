type ModelEndpointDraft = {
  kind?: string;
  spec?: {
    provider?: string;
    base_url?: string;
    default_model?: string;
    auth?: { secretRef?: string };
  };
};

function isLikelyLocalBaseURL(raw: string): boolean {
  const value = raw.trim();
  if (value === "") return false;
  if (value.includes("127.0.0.1") || value.includes("localhost") || value.includes("0.0.0.0")) {
    return true;
  }
  if (value.includes(".local")) return true;

  return /(\/\/)(10\.|192\.168\.|172\.(1[6-9]|2\d|3[0-1])\.)/.test(value);
}

function collectModelEndpointDraftWarnings(item: ModelEndpointDraft): string[] {
  const warnings: string[] = [];
  const provider = (item.spec?.provider ?? "").trim().toLowerCase();
  const baseURL = (item.spec?.base_url ?? "").trim().toLowerCase();
  const defaultModel = (item.spec?.default_model ?? "").trim().toLowerCase();
  const secretRef = (item.spec?.auth?.secretRef ?? "").trim();

  if (provider === "ollama" && /\/v1\/?$/.test(baseURL)) {
    warnings.push("For provider=ollama, set base_url to the server root (for example http://127.0.0.1:11434), not /v1.");
  }

  const localProvider = provider === "ollama" || (provider === "openai-compatible" && isLikelyLocalBaseURL(baseURL));
  if (localProvider && (defaultModel.startsWith("gpt-") || defaultModel.startsWith("claude-"))) {
    warnings.push("The default_model looks cloud-specific for a local/self-hosted endpoint. Verify the model id exists on your local provider.");
  }

  const requiresAuth = provider === "openai" || provider === "anthropic" || provider === "azure-openai";
  if (requiresAuth && secretRef === "") {
    warnings.push("This provider requires auth.secretRef. Create a Secret and reference it before creating the endpoint.");
  }

  return warnings;
}

export function getModelEndpointDraftWarnings(draft: string): string[] {
  try {
    const parsed = JSON.parse(draft) as ModelEndpointDraft;
    if ((parsed.kind ?? "").trim().toLowerCase() !== "modelendpoint") return [];
    return collectModelEndpointDraftWarnings(parsed);
  } catch {
    return [];
  }
}
