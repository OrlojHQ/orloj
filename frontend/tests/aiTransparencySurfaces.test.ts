import { describe, expect, test } from "bun:test";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { resolve } from "node:path";
import {
  ORLOJ_AI_CENSUS_PATTERNS,
  aiTransparencySurfaces,
  validateAiTransparencyRegistry,
} from "../src/compliance/aiTransparencySurfaces";

const sourceRoot = resolve(import.meta.dir, "../src");

function productionSources(dir = sourceRoot, prefix = ""): Map<string, string> {
  const files = new Map<string, string>();
  for (const name of readdirSync(dir)) {
    const absolute = resolve(dir, name);
    const relative = prefix ? `${prefix}/${name}` : name;
    if (statSync(absolute).isDirectory()) {
      for (const [child, source] of productionSources(absolute, relative)) files.set(child, source);
    } else if (/\.(ts|tsx)$/.test(name) && !name.includes(".test.") && relative !== "compliance/aiTransparencySurfaces.ts") {
      files.set(relative, readFileSync(absolute, "utf8"));
    }
  }
  return files;
}

function findUnregistered(files: ReadonlyMap<string, string>): string[] {
  const registered = new Set<string>(aiTransparencySurfaces.flatMap((surface) => [...surface.sourceFiles]));
  return [...files]
    .filter(([, source]) => ORLOJ_AI_CENSUS_PATTERNS.some((pattern) => pattern.test(source)))
    .map(([file]) => file)
    .filter((file) => !registered.has(file))
    .sort();
}

describe("Orloj AI transparency registry", () => {
  test("is internally valid and classifies every required candidate", () => {
    expect(validateAiTransparencyRegistry()).toEqual([]);
    const registered = new Set<string>(aiTransparencySurfaces.flatMap((surface) => [...surface.sourceFiles]));
    const candidates = [
      "pages/TaskDetail.tsx",
      "components/TraceView.tsx",
      "components/LogViewer.tsx",
      "pages/EvalRunDetail.tsx",
      "pages/SessionDetail.tsx",
      "components/SessionTimeline.tsx",
    ];
    expect(candidates.filter((file) => !registered.has(file))).toEqual([]);
  });

  test("fails a synthetic unregistered AI output branch", () => {
    const synthetic = new Map(productionSources());
    synthetic.set("pages/UnregisteredAiOutput.tsx", "export const output = task.status?.output;");
    expect(findUnregistered(synthetic)).toContain("pages/UnregisteredAiOutput.tsx");
  });

  test("has no unregistered production branch in the deterministic census", () => {
    expect(findUnregistered(productionSources())).toEqual([]);
  });

  test("ties every included row to a production disclosure placement", () => {
    const files = productionSources();
    for (const surface of aiTransparencySurfaces.filter((entry) => entry.decision === "included")) {
      expect(
        surface.sourceFiles.some((file) => files.get(file)?.includes("AiDisclosure")),
      ).toBe(true);
    }
  });
});
