/** Block-character progress / bar helpers for the terminal theme. */

const FULL = "#";
const EMPTY = "-";

export function asciiBar(ratio: number, width = 12): string {
  const clamped = Math.max(0, Math.min(1, ratio));
  const filled = Math.round(clamped * width);
  return `[${FULL.repeat(filled)}${EMPTY.repeat(width - filled)}]`;
}

export function asciiBarFromCount(count: number, max: number, width = 16): string {
  return asciiBar(max > 0 ? count / max : 0, width);
}

export type AsciiSeries = {
  key: string;
  /** Single character used to paint this series (e.g. `#`, `*`, `+`). */
  glyph: string;
};

/**
 * Render a stacked daily series as a compact ANSI/ASCII grid.
 * Rows are value (top = high); columns are points along x.
 */
export function asciiStackedGrid(
  data: Record<string, string | number>[],
  series: AsciiSeries[],
  height = 8,
): string[] {
  const n = data.length;
  if (n === 0 || series.length === 0) return [];

  const totals = data.map((row) =>
    series.reduce((sum, s) => sum + (Number(row[s.key]) || 0), 0),
  );
  const maxY = Math.max(1, ...totals);

  // Per column, bottom→top stack of glyphs (empty = space).
  const columns: string[][] = data.map((row) => {
    const cells: string[] = Array(height).fill(" ");
    let acc = 0;
    for (const s of series) {
      const v = Number(row[s.key]) || 0;
      if (v <= 0) continue;
      const lo = Math.floor((acc / maxY) * height);
      acc += v;
      const hi = Math.max(lo + 1, Math.round((acc / maxY) * height));
      for (let y = lo; y < Math.min(height, hi); y++) {
        cells[y] = s.glyph;
      }
    }
    return cells;
  });

  const lines: string[] = [];
  for (let row = height - 1; row >= 0; row--) {
    lines.push(columns.map((col) => col[row]).join(""));
  }
  return lines;
}

const SERIES_GLYPHS = ["#", "*", "+", "o", "x", "="];

export function seriesGlyph(index: number): string {
  return SERIES_GLYPHS[index % SERIES_GLYPHS.length];
}
