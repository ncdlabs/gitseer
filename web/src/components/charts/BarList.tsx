export type BarListItem = {
  key: string;
  count: number;
  label?: string;
};

type Props = {
  title: string;
  items: BarListItem[];
  empty?: string;
  /** Optional CSS color for a bar; defaults to accent. */
  colorForKey?: (key: string) => string;
};

function humanize(key: string): string {
  const k = (key || "").trim();
  if (!k) return "unknown";
  return k.replace(/[_-]+/g, " ");
}

export function BarList({
  title,
  items,
  empty = "No data.",
  colorForKey,
}: Props) {
  const sorted = [...items].sort((a, b) => b.count - a.count);
  const max = Math.max(1, ...sorted.map((i) => i.count));
  const total = sorted.reduce((sum, i) => sum + i.count, 0);
  const descId = `${title.replace(/\s+/g, "-").toLowerCase()}-bars-desc`;

  if (sorted.length === 0 || total === 0) {
    return (
      <div className="chart">
        <div className="chart__header">
          <h3 className="chart__title">{title}</h3>
        </div>
        <p className="chart__empty muted">{empty}</p>
      </div>
    );
  }

  return (
    <div className="chart" role="img" aria-labelledby={descId}>
      <div className="chart__header">
        <h3 className="chart__title" id={descId}>
          {title}
        </h3>
      </div>
      <ul className="bar-list">
        {sorted.map((item) => {
          const pct = Math.round((item.count / max) * 100);
          const share = total > 0 ? Math.round((item.count / total) * 100) : 0;
          const label = item.label || humanize(item.key);
          const color = colorForKey?.(item.key) || "var(--accent)";
          return (
            <li key={item.key} className="bar-list__row">
              <div className="bar-list__meta">
                <span className="bar-list__label">{label}</span>
                <span className="bar-list__count mono">
                  {item.count}
                  <span className="muted"> · {share}%</span>
                </span>
              </div>
              <div className="bar-list__track" aria-hidden="true">
                <div
                  className="bar-list__fill"
                  style={{ width: `${pct}%`, background: color }}
                />
              </div>
            </li>
          );
        })}
      </ul>
      <details className="chart__fallback">
        <summary>Data table</summary>
        <table>
          <caption className="visually-hidden">{title}</caption>
          <thead>
            <tr>
              <th scope="col">Category</th>
              <th scope="col">Count</th>
            </tr>
          </thead>
          <tbody>
            {sorted.map((item) => (
              <tr key={item.key}>
                <th scope="row">{item.label || humanize(item.key)}</th>
                <td>{item.count}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </details>
    </div>
  );
}
