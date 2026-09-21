export type ChartSeries = {
  key: string;
  label: string;
  /** CSS color value (prefer theme vars, e.g. `var(--ok)`). */
  color: string;
};

type Row = Record<string, string | number>;

type Props = {
  title: string;
  description?: string;
  data: Row[];
  xKey: string;
  series: ChartSeries[];
  empty?: string;
};

const W = 400;
const H = 160;
const PAD = { top: 12, right: 8, bottom: 24, left: 8 };

function formatXLabel(value: string): string {
  // Expect ISO date YYYY-MM-DD; show MM-DD for density.
  if (/^\d{4}-\d{2}-\d{2}/.test(value)) return value.slice(5, 10);
  return value;
}

function buildStackedPaths(
  data: Row[],
  series: ChartSeries[],
): { areas: { key: string; d: string; color: string }[]; maxY: number } {
  const n = data.length;
  if (n === 0) return { areas: [], maxY: 0 };

  const totals = data.map((row) =>
    series.reduce((sum, s) => sum + (Number(row[s.key]) || 0), 0),
  );
  const maxY = Math.max(1, ...totals);
  const innerW = W - PAD.left - PAD.right;
  const innerH = H - PAD.top - PAD.bottom;
  const xAt = (i: number) =>
    PAD.left + (n === 1 ? innerW / 2 : (i / (n - 1)) * innerW);
  const yAt = (v: number) => PAD.top + innerH - (v / maxY) * innerH;

  // Cumulative stacks bottom → top per series.
  const floors: number[][] = series.map(() => new Array(n).fill(0));
  const ceilings: number[][] = series.map(() => new Array(n).fill(0));
  for (let i = 0; i < n; i++) {
    let acc = 0;
    for (let s = 0; s < series.length; s++) {
      floors[s][i] = acc;
      acc += Number(data[i][series[s].key]) || 0;
      ceilings[s][i] = acc;
    }
  }

  const areas = series.map((s, si) => {
    const top: string[] = [];
    const bottom: string[] = [];
    for (let i = 0; i < n; i++) {
      top.push(`${i === 0 ? "M" : "L"}${xAt(i).toFixed(2)},${yAt(ceilings[si][i]).toFixed(2)}`);
    }
    for (let i = n - 1; i >= 0; i--) {
      bottom.push(`L${xAt(i).toFixed(2)},${yAt(floors[si][i]).toFixed(2)}`);
    }
    return { key: s.key, color: s.color, d: `${top.join(" ")} ${bottom.join(" ")} Z` };
  });

  return { areas, maxY };
}

export function StackedAreaChart({
  title,
  description,
  data,
  xKey,
  series,
  empty = "No data in this range.",
}: Props) {
  const descId = `${title.replace(/\s+/g, "-").toLowerCase()}-desc`;
  const hasData =
    data.length > 0 &&
    data.some((row) => series.some((s) => (Number(row[s.key]) || 0) > 0));

  if (!hasData) {
    return (
      <div className="chart">
        <div className="chart__header">
          <h3 className="chart__title">{title}</h3>
        </div>
        <p className="chart__empty muted">{empty}</p>
      </div>
    );
  }

  const { areas } = buildStackedPaths(data, series);
  const labelEvery = Math.max(1, Math.ceil(data.length / 7));

  return (
    <div className="chart">
      <div className="chart__header">
        <h3 className="chart__title">{title}</h3>
        <ul className="chart__legend" aria-hidden="true">
          {series.map((s) => (
            <li key={s.key} className="chart__legend-item">
              <span className="chart__swatch" style={{ background: s.color }} />
              {s.label}
            </li>
          ))}
        </ul>
      </div>
      <svg
        className="chart__svg"
        viewBox={`0 0 ${W} ${H}`}
        role="img"
        aria-labelledby={descId}
      >
        <title>{title}</title>
        <desc id={descId}>
          {description ||
            `${title}: stacked daily counts for ${series.map((s) => s.label).join(", ")}.`}
        </desc>
        {areas.map((a) => (
          <path key={a.key} d={a.d} fill={a.color} fillOpacity={0.72} stroke="none" />
        ))}
        {data.map((row, i) =>
          i % labelEvery === 0 || i === data.length - 1 ? (
            <text
              key={String(row[xKey])}
              x={
                PAD.left +
                (data.length === 1
                  ? (W - PAD.left - PAD.right) / 2
                  : (i / (data.length - 1)) * (W - PAD.left - PAD.right))
              }
              y={H - 6}
              textAnchor="middle"
              className="chart__axis-label"
            >
              {formatXLabel(String(row[xKey]))}
            </text>
          ) : null,
        )}
      </svg>
      <details className="chart__fallback">
        <summary>Data table</summary>
        <table>
          <caption className="visually-hidden">{title}</caption>
          <thead>
            <tr>
              <th scope="col">Date</th>
              {series.map((s) => (
                <th key={s.key} scope="col">
                  {s.label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {data.map((row) => (
              <tr key={String(row[xKey])}>
                <th scope="row">{String(row[xKey])}</th>
                {series.map((s) => (
                  <td key={s.key}>{Number(row[s.key]) || 0}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </details>
    </div>
  );
}
