// The harness list comes from the host (built-in and user manifests); nothing
// here is hard-coded. main.tsx loads it once before the first render.
export type HarnessId = string;

export type HarnessCue = {
  state: "working" | "idle" | "input";
  source: "title" | "last-rows" | "last-text-rows";
  rows?: number;
  pattern: string;
  ignoreCase?: boolean;
};

export type HarnessInfo = {
  id: HarnessId;
  name: string;
  short: string;
  executable: string;
  cues?: HarnessCue[];
  activity?: string;
  phoneAnswer?: string;
  brand: { mark: string; text?: string; color?: string };
  builtin: boolean;
  path?: string;
  hash?: string;
  trusted?: boolean;
  resolvedPath?: string;
  resumeVerified?: boolean;
};

export type HarnessLoadError = { path: string; error: string };

let registry: HarnessInfo[] = [];
const compiled = new Map<HarnessCue, RegExp | null>();

export function setHarnesses(items: HarnessInfo[]) {
  registry = items.map((item) => ({ ...item, trusted: item.trusted ?? item.builtin }));
  compiled.clear();
}

export const getHarnesses = (): ReadonlyArray<HarnessInfo> => registry;
export const harnessIds = (): HarnessId[] => registry.map((harness) => harness.id);
export const findHarness = (id: string) => registry.find((harness) => harness.id === id);
export const harnessName = (id: string) => findHarness(id)?.name ?? id;
export const harnessShort = (id: string) => findHarness(id)?.short ?? id;
export const isHarness = (id: string) => findHarness(id) !== undefined;
export const hasActivityReader = (id: string) => !!findHarness(id)?.activity;

// Patterns were validated by the host; one this engine cannot compile is skipped.
export function cuePattern(cue: HarnessCue): RegExp | null {
  let pattern = compiled.get(cue);
  if (pattern === undefined) {
    try {
      pattern = new RegExp(cue.pattern, cue.ignoreCase ? "iu" : "u");
    } catch {
      pattern = null;
    }
    compiled.set(cue, pattern);
  }
  return pattern;
}
