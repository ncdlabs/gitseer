export type Step = {
  name: string;
  status: string;
  conclusion?: string;
  number?: number;
};

export type Progress = {
  completed: number;
  total: number;
  ratio: number;
};

/** True when conclusion is a real terminal result (not empty / "unknown"). */
export function hasTerminalConclusion(conclusion?: string | null): boolean {
  const c = String(conclusion ?? "")
    .trim()
    .toLowerCase();
  return c !== "" && c !== "unknown";
}

/** Prefer terminal conclusion for badges; fall back to execution status. */
export function runStatusLabel(status?: string | null, conclusion?: string | null): string {
  if (hasTerminalConclusion(conclusion)) return String(conclusion).trim();
  const s = String(status ?? "").trim();
  return s || "—";
}

function isDone(status?: string, conclusion?: string): boolean {
  if ((status || "").toLowerCase() === "completed") return true;
  return hasTerminalConclusion(conclusion);
}

export function jobProgress(jobs: Array<{ status?: string; conclusion?: string }> | null | undefined): Progress {
  const list = jobs || [];
  const total = list.length;
  const completed = list.filter((j) => isDone(j.status, j.conclusion)).length;
  return {
    completed,
    total,
    ratio: total === 0 ? 0 : completed / total,
  };
}

export function parseSteps(steps_json?: string | null): Step[] {
  if (!steps_json || !String(steps_json).trim()) return [];
  try {
    const parsed = JSON.parse(steps_json) as unknown;
    if (!Array.isArray(parsed)) return [];
    return parsed
      .filter((s): s is Record<string, unknown> => s != null && typeof s === "object")
      .map((s) => ({
        name: typeof s.name === "string" ? s.name : "",
        status: typeof s.status === "string" ? s.status : "",
        conclusion: typeof s.conclusion === "string" ? s.conclusion : undefined,
        number: typeof s.number === "number" ? s.number : undefined,
      }))
      .filter((s) => s.name || s.status);
  } catch {
    return [];
  }
}

export function stepProgress(steps: Step[] | null | undefined): Progress & { currentName: string } {
  const list = steps || [];
  const total = list.length;
  const completed = list.filter((s) => isDone(s.status, s.conclusion)).length;
  const current = list.find((s) => !isDone(s.status, s.conclusion));
  const currentName = current?.name || list[list.length - 1]?.name || "";
  return {
    completed,
    total,
    ratio: total === 0 ? 0 : completed / total,
    currentName,
  };
}
