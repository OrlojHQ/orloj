import { describe, expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { AI_DISCLOSURE_LOCALES, aiDisclosureCopy, resolveAiDisclosureLocale } from "../src/compliance/aiDisclosureLocale";
import { AiDisclosure, normalizeAiAttributionValue } from "../src/components/AiDisclosure";

describe("AiDisclosure", () => {
  test("renders exact visible, semantic, and machine-readable metadata", () => {
    const html = renderToStaticMarkup(
      <AiDisclosure kind="generated-output" provider="anthropic" modelId="claude-opus-4-1" locale="en" />,
    );

    expect(html).toContain('role="note"');
    expect(html).toContain('aria-label="AI-generated · anthropic/claude-opus-4-1"');
    expect(html).toContain('data-compliance="eu-ai-act-art-50"');
    expect(html).toContain('data-ai-disclosure="generated-output"');
    expect(html).toContain('data-ai-provider="anthropic"');
    expect(html).toContain('data-ai-model="claude-opus-4-1"');
    expect(html).not.toContain("data-ai-attribution");
  });

  test("uses provider-agnostic metadata and never guesses Mistral", () => {
    const html = renderToStaticMarkup(<AiDisclosure kind="ai-assisted-analysis" locale="en" />);
    expect(html).toContain('data-ai-attribution="provider-agnostic"');
    expect(html).not.toContain("data-ai-provider");
    expect(html.toLowerCase()).not.toContain("mistral");
  });

  test("rejects URL and secret-like values", () => {
    const html = renderToStaticMarkup(
      <AiDisclosure kind="ai-interaction" provider="https://provider.example/v1" modelId="api-key-secret" locale="en" />,
    );
    expect(html).toContain('data-ai-attribution="provider-agnostic"');
    expect(html).not.toContain("provider.example");
    expect(html).not.toContain("api-key-secret");
    expect(normalizeAiAttributionValue("credential-id")).toBeUndefined();
  });

  test("declares non-empty copy for every supported locale and falls back to English", () => {
    for (const locale of AI_DISCLOSURE_LOCALES) {
      for (const value of Object.values(aiDisclosureCopy[locale])) expect(value.trim().length).toBeGreaterThan(0);
      const html = renderToStaticMarkup(<AiDisclosure kind="generated-output" locale={locale} />);
      expect(html).toContain(aiDisclosureCopy[locale]["generated-output"]);
    }
    expect(resolveAiDisclosureLocale("pt-BR")).toBe("pt");
    expect(resolveAiDisclosureLocale("zh-Hant-TW")).toBe("zh");
    expect(resolveAiDisclosureLocale("unsupported-Latn")).toBe("en");
  });
});
