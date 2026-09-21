import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { api, type ActiveRunItem, type Job } from "../api/client";
import { useIsTerminalTheme } from "../hooks/useIsTerminalTheme";
import { asciiBar } from "../lib/asciiGraphics";
import { jobProgress, parseSteps, stepProgress } from "../lib/actionProgress";

function ProgressBar({ ratio, label }: { ratio: number; label: string }) {
  const pct = Math.max(0, Math.min(100, Math.round(ratio * 100)));
  const terminal = useIsTerminalTheme();
  if (terminal) {
    return (
      <pre className="progress-bar progress-bar--ascii mono" role="progressbar" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100} aria-label={label}>
        {asciiBar(ratio)} {pct}%
      </pre>
    );
  }
  return (
    <div className="progress-bar" role="progressbar" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100} aria-label={label}>
      <div className="progress-bar__fill" style={{ width: `${pct}%` }} />
    </div>
  );
}

function statusLabel(item: ActiveRunItem): string {
  return item.run.conclusion || item.run.status || "—";
}

function runningJobWithSteps(jobs: Job[]): { job: Job; steps: ReturnType<typeof parseSteps> } | null {
  for (const job of jobs) {
    if ((job.status || "").toLowerCase() !== "running") continue;
    const steps = parseSteps(job.steps_json);
    if (steps.length > 0) return { job, steps };
  }
  return null;
}

function openInOpener(path: string) {
  const base = window.__LENS_BASE__ || "";
  const url = `${base}${path}`;
  if (window.opener && !window.opener.closed) {
    try {
      window.opener.location.assign(url);
      window.opener.focus();
      return true;
    } catch {
      /* cross-origin or blocked — fall through */
    }
  }
  return false;
}

function ActiveRunRow({
  item,
  onNavigate,
  preferOpener,
}: {
  item: ActiveRunItem;
  onNavigate?: () => void;
  preferOpener?: boolean;
}) {
  const jobs = item.jobs || [];
  const jp = jobProgress(jobs);
  const stepSource = runningJobWithSteps(jobs);
  const sp = stepSource ? stepProgress(stepSource.steps) : null;
  const to = `/pipelines/${item.run.id}`;

  return (
    <Link
      className="actions-flyout__item"
      to={to}
      onClick={(event) => {
        if (preferOpener && openInOpener(to)) {
          event.preventDefault();
        }
        onNavigate?.();
      }}
    >
      <div className="actions-flyout__item-top">
        <span className="actions-flyout__repo">{item.run.repo_full || "—"}</span>
        <span className={`badge ${statusLabel(item)}`}>{statusLabel(item)}</span>
      </div>
      <span className="actions-flyout__run-name">{item.run.name}</span>
      {jp.total > 0 && (
        <div className="actions-flyout__progress">
          <div className="actions-flyout__progress-meta">
            <span>Jobs</span>
            <span>
              {jp.completed}/{jp.total}
            </span>
          </div>
          <ProgressBar ratio={jp.ratio} label={`Jobs ${jp.completed} of ${jp.total}`} />
        </div>
      )}
      {sp && sp.total > 0 && (
        <div className="actions-flyout__progress">
          <div className="actions-flyout__progress-meta">
            <span className="actions-flyout__step-name">{sp.currentName || "Steps"}</span>
            <span>
              {sp.completed}/{sp.total}
            </span>
          </div>
          <ProgressBar ratio={sp.ratio} label={`Steps ${sp.completed} of ${sp.total}`} />
        </div>
      )}
    </Link>
  );
}

export type ActiveActionsPanelProps = {
  headerActions?: ReactNode;
  onNavigate?: () => void;
  preferOpener?: boolean;
  className?: string;
};

export function ActiveActionsPanel({ headerActions, onNavigate, preferOpener, className }: ActiveActionsPanelProps) {
  const q = useQuery({
    queryKey: ["workflow-runs", "active"],
    queryFn: api.activeWorkflowRuns,
  });

  const total = q.data?.total ?? 0;
  const items = q.data?.items ?? [];

  return (
    <div className={className ? `actions-panel ${className}` : "actions-panel"}>
      <div className="actions-flyout__header">
        <h2 className="actions-flyout__heading">Active Actions</h2>
        <div className="actions-flyout__header-actions">
          {total > 0 && <span className="actions-flyout__count">{total}</span>}
          {headerActions}
        </div>
      </div>
      {q.isLoading && <p className="actions-flyout__empty muted">Loading…</p>}
      {q.isError && <p className="actions-flyout__empty error">{(q.error as Error).message}</p>}
      {q.isSuccess && items.length === 0 && <p className="actions-flyout__empty muted">No actions running.</p>}
      {q.isSuccess && items.length > 0 && (
        <div className="actions-flyout__list">
          {items.map((item) => (
            <ActiveRunRow key={item.run.id} item={item} onNavigate={onNavigate} preferOpener={preferOpener} />
          ))}
        </div>
      )}
    </div>
  );
}

export function useActiveActionsCount(): number {
  const q = useQuery({
    queryKey: ["workflow-runs", "active"],
    queryFn: api.activeWorkflowRuns,
  });
  return q.data?.total ?? 0;
}

const POPOUT_NAME = "lens-actions-popout";
let popoutRef: Window | null = null;

export function openActionsPopout(): Window | null {
  const base = window.__LENS_BASE__ || "";
  const url = `${window.location.origin}${base}/actions-popout`;
  if (popoutRef && !popoutRef.closed) {
    popoutRef.focus();
    return popoutRef;
  }
  const width = 360;
  const height = 520;
  const left = Math.max(0, window.screenX + 24);
  const top = Math.max(0, window.screenY + 72);
  const features = [
    "popup=yes",
    `width=${width}`,
    `height=${height}`,
    `left=${left}`,
    `top=${top}`,
    "resizable=yes",
    "scrollbars=yes",
  ].join(",");
  const win = window.open(url, POPOUT_NAME, features);
  popoutRef = win;
  return win;
}
