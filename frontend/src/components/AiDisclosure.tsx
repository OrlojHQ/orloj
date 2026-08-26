import type { AiDisclosureKind, AiDisclosureLocale } from "../compliance/aiDisclosureLocale";
import { aiDisclosureCopy, browserAiDisclosureLocale } from "../compliance/aiDisclosureLocale";

interface AiDisclosureProps {
  kind: AiDisclosureKind;
  provider?: string | null;
  modelId?: string | null;
  locale?: AiDisclosureLocale;
  compact?: boolean;
  className?: string;
  accessibleLabel?: string;
  testId?: string;
}

const SAFE_METADATA_PATTERN = /^[\p{L}\p{N}][\p{L}\p{N}._+/@:-]{0,119}$/u;
const SENSITIVE_METADATA_PATTERN = /(?:^|[-_.])(secret|token|password|credential|api[-_]?key)(?:$|[-_.])/i;

export function normalizeAiAttributionValue(value: string | null | undefined): string | undefined {
  const normalized = value?.trim();
  if (!normalized || normalized.includes("://") || !SAFE_METADATA_PATTERN.test(normalized)) return undefined;
  if (SENSITIVE_METADATA_PATTERN.test(normalized)) return undefined;
  return normalized;
}

export function AiDisclosure({
  kind,
  provider,
  modelId,
  locale = browserAiDisclosureLocale(),
  compact = false,
  className,
  accessibleLabel,
  testId,
}: AiDisclosureProps) {
  const safeProvider = normalizeAiAttributionValue(provider);
  const safeModel = safeProvider ? normalizeAiAttributionValue(modelId) : undefined;
  const label = aiDisclosureCopy[locale][kind];
  const attribution = safeProvider ? `${safeProvider}${safeModel ? `/${safeModel}` : ""}` : undefined;

  return (
    <span
      className={["ai-disclosure", compact ? "ai-disclosure--compact" : "", className ?? ""].filter(Boolean).join(" ")}
      role="note"
      aria-label={accessibleLabel ?? (attribution ? `${label} · ${attribution}` : label)}
      data-testid={testId}
      data-compliance="eu-ai-act-art-50"
      data-ai-disclosure={kind}
      {...(safeProvider
        ? { "data-ai-provider": safeProvider, ...(safeModel ? { "data-ai-model": safeModel } : {}) }
        : { "data-ai-attribution": "provider-agnostic" })}
    >
      <span>{label}</span>
      {attribution ? <span className="ai-disclosure__attribution" aria-hidden="true">· {attribution}</span> : null}
    </span>
  );
}
