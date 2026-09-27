import type { RepoHealth } from "../api/client";

function gradeClass(grade: string) {
  switch (grade) {
    case "critical":
      return "critical";
    case "degraded":
      return "warning";
    case "healthy":
      return "success";
    default:
      return "";
  }
}

function gradeLabel(grade: string) {
  switch (grade) {
    case "critical":
      return "Critical";
    case "degraded":
      return "Degraded";
    case "healthy":
      return "Healthy";
    default:
      return grade || "Unknown";
  }
}

type Props = {
  health?: RepoHealth | null;
  /** Show numeric score next to grade. */
  showScore?: boolean;
};

export function HealthBadge({ health, showScore = true }: Props) {
  if (!health) return null;
  const label = showScore
    ? `${gradeLabel(health.grade)} ${health.score}`
    : gradeLabel(health.grade);
  const title = [
    `Score ${health.score}`,
    health.open_critical_attention ? `${health.open_critical_attention} critical attention` : null,
    health.failing_default_branch ? "failing default branch" : null,
    health.stale_open_prs ? `${health.stale_open_prs} stale open PRs` : null,
    health.ci_runs_in_window
      ? `CI fail rate ${(health.ci_fail_rate * 100).toFixed(0)}% (${health.window_days}d)`
      : null,
  ]
    .filter(Boolean)
    .join(" · ");

  return (
    <span className={`badge ${gradeClass(health.grade)}`} title={title || undefined}>
      {label}
    </span>
  );
}
