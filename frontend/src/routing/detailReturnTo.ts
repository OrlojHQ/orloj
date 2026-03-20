import type { Location } from "react-router-dom";

/** Set via `navigate(path, { state: { returnTo } })` when opening a detail from another screen (e.g. agent system graph). */
export type DetailLocationState = {
  returnTo?: string;
};

export function readDetailReturnTo(location: Location): string | undefined {
  const s = location.state as DetailLocationState | null | undefined;
  const v = s?.returnTo;
  if (typeof v !== "string" || !v.startsWith("/") || v.startsWith("//")) return undefined;
  return v;
}
