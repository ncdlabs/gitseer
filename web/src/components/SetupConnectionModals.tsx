import { useEffect, useId, useMemo, useRef } from "react";
import type { ProbeCheck, ProbeCheckStatus, WebhookPreview } from "../api/client";

type ProbeGroup = "connectivity" | "permissions";

const GROUP_ORDER: { id: ProbeGroup; title: string }[] = [
  { id: "connectivity", title: "Connectivity" },
  { id: "permissions", title: "Permissions" },
];

const CONNECTIVITY_IDS = new Set(["reachability", "authenticate", "webhook_url"]);

function pendingChecks(giteaURL: string): ProbeCheck[] {
  const url = giteaURL.trim() || "Gitea";
  return [
    { id: "reachability", group: "connectivity", label: `Can reach ${url}`, status: "pending" },
    { id: "authenticate", group: "connectivity", label: "Authenticate with token", status: "pending" },
    { id: "webhook_url", group: "connectivity", label: "Lens public URL", status: "pending" },
    { id: "admin", group: "permissions", label: "Site administrator", status: "pending" },
    { id: "list_repos", group: "permissions", label: "List repositories", status: "pending" },
    { id: "list_orgs", group: "permissions", label: "List organizations", status: "pending" },
    { id: "pull_requests", group: "permissions", label: "Read pull requests", status: "pending" },
    { id: "actions", group: "permissions", label: "Read Actions runs", status: "pending" },
    { id: "commit_status", group: "permissions", label: "Read commit status", status: "pending" },
    { id: "system_hooks", group: "permissions", label: "Manage system webhooks", status: "pending" },
  ];
}

function resolveGroup(check: ProbeCheck): ProbeGroup {
  if (check.group === "connectivity" || check.group === "permissions") return check.group;
  return CONNECTIVITY_IDS.has(check.id) ? "connectivity" : "permissions";
}

function StatusIcon({ status }: { status: ProbeCheckStatus }) {
  if (status === "ok") {
    return (
      <svg className="probe-list__icon" viewBox="0 0 16 16" aria-hidden="true">
        <path
          d="M3.5 8.5 6.5 11.5 12.5 4.5"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </svg>
    );
  }
  if (status === "fail") {
    return (
      <svg className="probe-list__icon" viewBox="0 0 16 16" aria-hidden="true">
        <path
          d="M4 4 12 12M12 4 4 12"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
        />
      </svg>
    );
  }
  const glyph =
    status === "warn" ? "!" : status === "skip" ? "–" : status === "running" ? "…" : "";
  return <span aria-hidden="true">{glyph}</span>;
}

type CheckModalProps = {
  open: boolean;
  running: boolean;
  checks: ProbeCheck[] | null;
  giteaURL?: string;
  error: string | null;
  onClose: () => void;
  onContinue: () => void;
  canContinue: boolean;
};

export function ConnectionCheckModal({
  open,
  running,
  checks,
  giteaURL = "",
  error,
  onClose,
  onContinue,
  canContinue,
}: CheckModalProps) {
  const titleId = useId();
  const closeRef = useRef<HTMLButtonElement>(null);
  const rows = useMemo(() => {
    const base =
      checks && checks.length > 0
        ? checks
        : pendingChecks(giteaURL).map((c) => ({
            ...c,
            status: (running ? "running" : "pending") as ProbeCheckStatus,
          }));
    return base;
  }, [checks, giteaURL, running]);

  const grouped = useMemo(() => {
    const map: Record<ProbeGroup, ProbeCheck[]> = { connectivity: [], permissions: [] };
    for (const c of rows) {
      map[resolveGroup(c)].push(c);
    }
    return map;
  }, [rows]);

  useEffect(() => {
    if (!open) return;
    closeRef.current?.focus();
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape" && !running) onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, running, onClose]);

  if (!open) return null;

  return (
    <div className="modal-backdrop" role="presentation">
      <div className="modal modal--probe" role="dialog" aria-modal="true" aria-labelledby={titleId}>
        <header className="modal__header">
          <h2 id={titleId}>Connection Checks</h2>
          <p className="muted">
            {running
              ? "Probing Gitea connectivity and token permissions…"
              : canContinue
                ? "All required checks passed."
                : "Fix failed checks, then try again."}
          </p>
        </header>
        <div className="probe-groups" aria-live="polite">
          {GROUP_ORDER.map((g) => {
            const items = grouped[g.id];
            if (items.length === 0) return null;
            return (
              <section key={g.id} className="probe-group">
                <h3 className="probe-group__title">{g.title}</h3>
                <ul className="probe-list">
                  {items.map((c) => (
                    <li key={c.id} className={`probe-list__item probe-list__item--${c.status}`}>
                      <span className="probe-list__glyph">
                        <StatusIcon status={c.status} />
                      </span>
                      <div className="probe-list__body">
                        <span className="probe-list__label">{c.label}</span>
                        {c.detail ? <span className="probe-list__detail muted">{c.detail}</span> : null}
                      </div>
                      <span className="visually-hidden">{c.status}</span>
                    </li>
                  ))}
                </ul>
              </section>
            );
          })}
        </div>
        {error && (
          <p className="error" role="alert">
            {error}
          </p>
        )}
        <div className="modal__actions">
          <button className="btn" type="button" ref={closeRef} onClick={onClose} disabled={running}>
            {canContinue ? "Cancel" : "Close"}
          </button>
          {canContinue && (
            <button className="btn primary" type="button" onClick={onContinue} disabled={running}>
              Continue
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

type WebhookModalProps = {
  open: boolean;
  preview: WebhookPreview | null;
  busy: boolean;
  error: string | null;
  onCancel: () => void;
  onManual: () => void;
  onCreate: () => void;
  canCreate: boolean;
};

export function WebhookConfirmModal({
  open,
  preview,
  busy,
  error,
  onCancel,
  onManual,
  onCreate,
  canCreate,
}: WebhookModalProps) {
  const titleId = useId();
  const payload = preview ? JSON.stringify(preview, null, 2) : "";

  useEffect(() => {
    if (!open) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape" && !busy) onCancel();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, busy, onCancel]);

  if (!open || !preview) return null;

  return (
    <div className="modal-backdrop" role="presentation">
      <div className="modal modal--wide" role="dialog" aria-modal="true" aria-labelledby={titleId}>
        <header className="modal__header">
          <h2 id={titleId}>Install system webhook?</h2>
          <p className="muted">
            Gitea will POST live pull-request and Actions events to Lens. Without it, Lens relies on
            periodic sync only.
          </p>
        </header>
        <pre className="webhook-preview mono" tabIndex={0} aria-label="Webhook payload">
          {payload}
        </pre>
        <p className="settings-form__hint">
          System hook on your Gitea instance. Secret is generated when you confirm.
        </p>
        {error && (
          <p className="error" role="alert">
            {error}
          </p>
        )}
        <div className="modal__actions">
          <button className="btn" type="button" onClick={onManual} disabled={busy}>
            I&apos;ll add it in Gitea
          </button>
          <button className="btn" type="button" onClick={onCancel} disabled={busy}>
            Cancel
          </button>
          <button
            className="btn primary"
            type="button"
            onClick={onCreate}
            disabled={busy || !canCreate}
            title={canCreate ? undefined : "Admin system-hooks permission required"}
          >
            {busy ? "Creating…" : "Create webhook"}
          </button>
        </div>
      </div>
    </div>
  );
}
