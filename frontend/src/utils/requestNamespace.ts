type WithMetadata = {
  metadata?: { namespace?: string; [key: string]: unknown };
};

/** Align body metadata.namespace with the API request namespace query param. */
export function ensureRequestNamespace<T extends WithMetadata>(
  body: T,
  requestNamespace: string,
): T {
  const ns = requestNamespace.trim() || "default";
  const metadata = body.metadata ?? {};
  return {
    ...body,
    metadata: { ...metadata, namespace: ns },
  };
}

/** Inject the active namespace into a JSON create template. */
export function applyTemplateNamespace(templateJson: string, namespace: string): string {
  try {
    const doc = JSON.parse(templateJson) as WithMetadata;
    return JSON.stringify(ensureRequestNamespace(doc, namespace), null, 2);
  } catch {
    return templateJson;
  }
}
