import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { api, type ActiveRunItem, type ForgeStatusRow, type Job, type User } from "../api/client";
import { useIsTerminalTheme } from "../hooks/useIsTerminalTheme";
import { resolveInstanceName, useForgeInventory } from "../hooks/useShowForgeUI";
import { asciiBar } from "../lib/asciiGraphics";
import { jobProgress, parseSteps, stepProgress } from "../lib/actionProgress";
import { ForgeBadge } from "./ForgeBadge";
import { WorkflowWriteActions } from "./WorkflowWriteActions";

function ProgressBar({ ratio, label, asciiWidth = 10 }: { ratio: number; label: string; asciiWidth?: number }) {
  const pct = Math.max(0, Math.min(100, Math.round(ratio * 100)));
  const terminal = useIsTerminalTheme();
  if (terminal) {
    return (
      <pre className="progress-bar progress-bar--ascii mono" role="progressbar" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100} aria-label={label}>
        {asciiBar(ratio, asciiWidth)} {pct}%
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

const MAIN_WINDOW_NAME = "gitseer-main";
const NAV_CHANNEL = "gitseer-navigate";

function appPath(path: string): string {
  const base = window.__GITSEER_BASE__ || "";
  return `${base}${path}`;
}

function absoluteAppURL(path: string): string {
  return `${window.location.origin}${appPath(path)}`;
}

/**
 * Drive the main GitSeer window from the actions popout.
 * Prefer opener postMessage (SPA nav), then BroadcastChannel, then a named window.
 * Never navigates the popout itself.
 */
function navigateMainWindow(path: string): boolean {
  if (window.opener && !window.opener.closed) {
    try {
      window.opener.postMessage({ type: "gitseer:navigate", path }, window.location.origin);
      window.opener.focus();
      return true;
    } catch {
      try {
        window.opener.location.assign(appPath(path));
        window.opener.focus();
        return true;
      } catch {
        /* cross-origin or blocked — fall through */
      }
    }
  }
  try {
    const bc = new BroadcastChannel(NAV_CHANNEL);
    bc.postMessage({ type: "navigate", path });
    bc.close();
    return true;
  } catch {
    /* BroadcastChannel unavailable */
  }
  const win = window.open(absoluteAppURL(path), MAIN_WINDOW_NAME);
  return win != null;
}

export { MAIN_WINDOW_NAME, NAV_CHANNEL };

function ActiveRunRow({
  item,
  onNavigate,
  preferOpener,
  showForge,
  forges,
  user,
}: {
  item: ActiveRunItem;
  onNavigate?: () => void;
  preferOpener?: boolean;
  showForge?: boolean;
  forges?: ForgeStatusRow[];
  user?: User | null;
}) {
  const jobs = item.jobs || [];
  const jp = jobProgress(jobs);
  const stepSource = runningJobWithSteps(jobs);
  const sp = stepSource ? stepProgress(stepSource.steps) : null;
  const to = `/pipelines/${item.run.id}`;

  return (
    <div className="actions-flyout__item">
      <Link
        className="actions-flyout__item-link"
        to={to}
        onClick={(event) => {
          if (preferOpener) {
            // Keep the popout on /actions-popout; open the run in the main window.
            event.preventDefault();
            navigateMainWindow(to);
          }
          onNavigate?.();
        }}
      >
        <div className="actions-flyout__item-top">
          <span className="actions-flyout__repo">{item.run.repo_full || "—"}</span>
          <span className={`badge ${statusLabel(item)}`}>{statusLabel(item)}</span>
        </div>
        <span className="actions-flyout__run-name">
          {showForge && (
            <ForgeBadge
              forgeType={item.run.forge_type}
              instanceName={resolveInstanceName(forges ?? [], {
                forgeType: item.run.forge_type,
                instanceId: item.run.instance_id,
                instanceName: item.run.instance_name,
              })}
            />
          )}
          <span className="actions-flyout__run-name-text">{item.run.name}</span>
        </span>
        {jp.total > 0 && (
          <div className="actions-flyout__progress">
            <div className="actions-flyout__progress-meta">
              <span>Jobs</span>
              <span>
                {jp.completed}/{jp.total}
              </span>
            </div>
            <ProgressBar ratio={jp.ratio} label={`Jobs ${jp.completed} of ${jp.total}`} asciiWidth={8} />
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
            <ProgressBar ratio={sp.ratio} label={`Steps ${sp.completed} of ${sp.total}`} asciiWidth={8} />
          </div>
        )}
      </Link>
      <WorkflowWriteActions
        run={item.run}
        user={user}
        compact
        className="actions-flyout__write-ops"
        onDone={onNavigate}
      />
    </div>
  );
}

export type ActiveActionsPanelProps = {
  headerActions?: ReactNode;
  onNavigate?: () => void;
  preferOpener?: boolean;
  className?: string;
};

export function ActiveActionsPanel({ headerActions, onNavigate, preferOpener, className }: ActiveActionsPanelProps) {
  const { showForge, forges } = useForgeInventory();
  const me = useQuery({ queryKey: ["me"], queryFn: api.me, retry: false });
  const q = useQuery({
    queryKey: ["workflow-runs", "active"],
    queryFn: api.activeWorkflowRuns,
    refetchInterval: 15_000,
  });

  const total = q.data?.total ?? 0;
  const items = q.data?.items ?? [];
  const user = me.data ?? null;

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
            <ActiveRunRow
              key={item.run.id}
              item={item}
              onNavigate={onNavigate}
              preferOpener={preferOpener}
              showForge={showForge}
              forges={forges}
              user={user}
            />
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
    refetchInterval: 15_000,
  });
  return q.data?.total ?? 0;
}

const POPOUT_NAME = "gitseer-actions-popout";
let popoutRef: Window | null = null;

export function openActionsPopout(): Window | null {
  const base = window.__GITSEER_BASE__ || "";
  const url = `${window.location.origin}${base}/actions-popout`;
  if (popoutRef && !popoutRef.closed) {
    popoutRef.focus();
    return popoutRef;
  }
  const width = 360;
  const height = 520;
  const left = Math.max(0, window.screenX + 24);
  const top = Math.max(0, window.screenY + 72);
  // noopener=no keeps window.opener so run clicks can target the main window.
  const features = [
    "popup=yes",
    `width=${width}`,
    `height=${height}`,
    `left=${left}`,
    `top=${top}`,
    "resizable=yes",
    "scrollbars=yes",
    "noopener=no",
    "noreferrer=no",
  ].join(",");
  const win = window.open(url, POPOUT_NAME, features);
  popoutRef = win;
  return win;
}
