import { getModelEndpointDraftWarnings } from "./modelEndpointDraftWarnings";

function assert(condition: boolean, message: string) {
  if (!condition) throw new Error(message);
}

function includesSnippet(items: string[], snippet: string): boolean {
  return items.some((item) => item.includes(snippet));
}

function testOllamaV1Warning() {
  const warnings = getModelEndpointDraftWarnings(JSON.stringify({
    kind: "ModelEndpoint",
    spec: { provider: "ollama", base_url: "http://127.0.0.1:11434/v1", default_model: "llama3.1" },
  }));
  assert(includesSnippet(warnings, "not /v1"), "expected ollama /v1 warning");
}

function testOpenAICompatibleAllowsMissingAuth() {
  const warnings = getModelEndpointDraftWarnings(JSON.stringify({
    kind: "ModelEndpoint",
    spec: { provider: "openai-compatible", base_url: "http://127.0.0.1:11434/v1", default_model: "llama3.1" },
  }));
  assert(!includesSnippet(warnings, "requires auth.secretRef"), "did not expect auth.secretRef warning for openai-compatible");
}

function testOpenAIRequiresAuthWarning() {
  const warnings = getModelEndpointDraftWarnings(JSON.stringify({
    kind: "ModelEndpoint",
    spec: { provider: "openai", base_url: "https://api.openai.com/v1", default_model: "gpt-4o" },
  }));
  assert(includesSnippet(warnings, "requires auth.secretRef"), "expected auth.secretRef warning for openai");
}

function testLocalProviderCloudModelWarning() {
  const warnings = getModelEndpointDraftWarnings(JSON.stringify({
    kind: "ModelEndpoint",
    spec: { provider: "openai-compatible", base_url: "http://localhost:11434/v1", default_model: "gpt-4o" },
  }));
  assert(includesSnippet(warnings, "cloud-specific"), "expected cloud-model warning for local endpoint");
}

function testInvalidDraftReturnsNoWarnings() {
  const warnings = getModelEndpointDraftWarnings("{ not json");
  assert(warnings.length === 0, "expected invalid JSON to return no warnings");
}

function testNonModelEndpointReturnsNoWarnings() {
  const warnings = getModelEndpointDraftWarnings(JSON.stringify({
    kind: "Tool",
    spec: { provider: "openai" },
  }));
  assert(warnings.length === 0, "expected non-ModelEndpoint draft to return no warnings");
}

function run() {
  testOllamaV1Warning();
  testOpenAICompatibleAllowsMissingAuth();
  testOpenAIRequiresAuthWarning();
  testLocalProviderCloudModelWarning();
  testInvalidDraftReturnsNoWarnings();
  testNonModelEndpointReturnsNoWarnings();
}

run();
