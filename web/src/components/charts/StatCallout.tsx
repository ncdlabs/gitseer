type Props = {
  title: string;
  p50Seconds: number | null | undefined;
  p95Seconds: number | null | undefined;
  sampleCount: number;
  empty?: string;
};

export function formatDuration(seconds: number | null | undefined): string {
  if (seconds == null || !Number.isFinite(seconds) || seconds < 0) return "—";
  if (seconds < 60) return `${Math.round(seconds)}s`;
  const m = Math.floor(seconds / 60);
  const s = Math.round(seconds % 60);
  if (m < 60) return s > 0 ? `${m}m ${s}s` : `${m}m`;
  const h = Math.floor(m / 60);
  const rm = m % 60;
  return rm > 0 ? `${h}h ${rm}m` : `${h}h`;
}

export function StatCallout({
  title,
  p50Seconds,
  p95Seconds,
  sampleCount,
  empty = "No completed runs with duration in this range.",
}: Props) {
  const hasSamples = sampleCount > 0 && p50Seconds != null && p95Seconds != null;

  return (
    <div className="stat-callout" role="img" aria-label={title}>
      <div className="stat-callout__title">{title}</div>
      {hasSamples ? (
        <>
          <div className="stat-callout__figures">
            <div className="stat-callout__figure">
              <div className="stat-callout__label">p50</div>
              <div className="stat-callout__value">{formatDuration(p50Seconds)}</div>
            </div>
            <div className="stat-callout__figure">
              <div className="stat-callout__label">p95</div>
              <div className="stat-callout__value">{formatDuration(p95Seconds)}</div>
            </div>
          </div>
          <div className="stat-callout__meta muted">
            {sampleCount.toLocaleString()} completed run{sampleCount === 1 ? "" : "s"}
          </div>
          <table className="visually-hidden">
            <caption>{title}</caption>
            <tbody>
              <tr>
                <th scope="row">p50</th>
                <td>{formatDuration(p50Seconds)}</td>
              </tr>
              <tr>
                <th scope="row">p95</th>
                <td>{formatDuration(p95Seconds)}</td>
              </tr>
              <tr>
                <th scope="row">Samples</th>
                <td>{sampleCount}</td>
              </tr>
            </tbody>
          </table>
        </>
      ) : (
        <p className="stat-callout__empty muted">{empty}</p>
      )}
    </div>
  );
}
