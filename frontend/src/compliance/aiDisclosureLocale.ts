export const AI_DISCLOSURE_LOCALES = ["en", "de", "es", "fr", "ko", "pt", "zh"] as const;

export type AiDisclosureLocale = (typeof AI_DISCLOSURE_LOCALES)[number];
export type AiDisclosureKind = "ai-interaction" | "generated-output" | "ai-assisted-analysis" | "ai-translation";

export const aiDisclosureCopy: Record<AiDisclosureLocale, Record<AiDisclosureKind, string>> = {
  en: {
    "ai-interaction": "AI interaction",
    "generated-output": "AI-generated",
    "ai-assisted-analysis": "AI-assisted analysis",
    "ai-translation": "AI translation",
  },
  de: {
    "ai-interaction": "KI-Interaktion",
    "generated-output": "KI-generiert",
    "ai-assisted-analysis": "KI-gestützte Analyse",
    "ai-translation": "KI-Übersetzung",
  },
  es: {
    "ai-interaction": "Interacción con IA",
    "generated-output": "Generado por IA",
    "ai-assisted-analysis": "Análisis asistido por IA",
    "ai-translation": "Traducción con IA",
  },
  fr: {
    "ai-interaction": "Interaction avec l’IA",
    "generated-output": "Généré par l’IA",
    "ai-assisted-analysis": "Analyse assistée par l’IA",
    "ai-translation": "Traduction par l’IA",
  },
  ko: {
    "ai-interaction": "AI 상호작용",
    "generated-output": "AI 생성",
    "ai-assisted-analysis": "AI 지원 분석",
    "ai-translation": "AI 번역",
  },
  pt: {
    "ai-interaction": "Interação com IA",
    "generated-output": "Gerado por IA",
    "ai-assisted-analysis": "Análise assistida por IA",
    "ai-translation": "Tradução por IA",
  },
  zh: {
    "ai-interaction": "AI 交互",
    "generated-output": "AI 生成",
    "ai-assisted-analysis": "AI 辅助分析",
    "ai-translation": "AI 翻译",
  },
};

export function resolveAiDisclosureLocale(language?: string | null): AiDisclosureLocale {
  const primary = language?.trim().toLowerCase().split(/[-_]/, 1)[0] ?? "";
  return AI_DISCLOSURE_LOCALES.includes(primary as AiDisclosureLocale)
    ? primary as AiDisclosureLocale
    : "en";
}

export function browserAiDisclosureLocale(): AiDisclosureLocale {
  if (typeof document !== "undefined" && document.documentElement.lang) {
    return resolveAiDisclosureLocale(document.documentElement.lang);
  }
  return resolveAiDisclosureLocale(typeof navigator === "undefined" ? undefined : navigator.language);
}
