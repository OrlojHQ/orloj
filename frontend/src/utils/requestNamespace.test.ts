import { applyTemplateNamespace, ensureRequestNamespace } from "./requestNamespace";

function assert(condition: boolean, message: string) {
  if (!condition) throw new Error(message);
}

function testEnsureRequestNamespace() {
  const body = { metadata: { name: "a", namespace: "default" }, spec: {} };
  assert(
    JSON.stringify(ensureRequestNamespace(body, "team-a")) ===
      JSON.stringify({ metadata: { name: "a", namespace: "team-a" }, spec: {} }),
    "expected namespace rewrite",
  );
}

function testEnsureRequestNamespaceCreatesMetadata() {
  const out = ensureRequestNamespace({ spec: {} } as { metadata?: { namespace?: string }; spec: Record<string, never> }, "prod");
  assert(out.metadata?.namespace === "prod" && out.spec !== undefined, "expected metadata creation");
}

function testEnsureRequestNamespaceDefault() {
  assert(
    JSON.stringify(ensureRequestNamespace({ metadata: { name: "x" } }, "  ")) ===
      JSON.stringify({ metadata: { name: "x", namespace: "default" } }),
    "expected default namespace",
  );
}

function testApplyTemplateNamespace() {
  const tpl = JSON.stringify({ metadata: { name: "a", namespace: "default" } });
  assert(
    JSON.parse(applyTemplateNamespace(tpl, "staging")).metadata.namespace === "staging",
    "expected template namespace rewrite",
  );
}

function run() {
  testEnsureRequestNamespace();
  testEnsureRequestNamespaceCreatesMetadata();
  testEnsureRequestNamespaceDefault();
  testApplyTemplateNamespace();
}

run();
