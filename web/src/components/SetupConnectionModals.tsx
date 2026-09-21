import { useEffect, useId, useMemo, useRef } from "react";
import type { OAuthAppPreview, ProbeCheck, ProbeCheckStatus, WebhookPreview } from "../api/client";

type ProbeGroup = "connectivity" | "permissions";

const GROUP_ORDER: { id: ProbeGroup; title: string }[] = [
  { id: "connectivity", title: "Connectivity" },
  { id: "permissions", title: "Permissions" },
];

const CONNECTIVITY_IDS = new Set(["reachability", "authenticate"]);

function pendingChecks(giteaURL: string): ProbeCheck[] {
  const url = giteaURL.trim() || "Gitea";
  return [
    { id: "reachability", group: "connectivity", label: `Can reach ${url}`, status: "pending" },
    { id: "authenticate", group: "connectivity", label: "Authenticate with token", status: "pending" },
    { id: "admin", group: "permissions", label: "Site administrator", status: "pending" },
    { id: "list_repos", group: "permissions", label: "List repositories", status: "pending" },
    { id: "list_orgs", group: "permissions", label: "List organizations", status: "pending" },
    { id: "pull_requests", group: "permissions", label: "Read pull requests", status: "pending" },
    { id: "actions", group: "permissions", label: "Read Actions runs", status: "pending" },
    { id: "commit_status", group: "permissions", label: "Read commit status", status: "pending" },
    { id: "system_hooks", group: "permissions", label: "Manage system webhooks", status: "pending" },
    { id: "oauth_apps", group: "permissions", label: "Manage OAuth applications", status: "pending" },
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
  closeLabel?: string;
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
  closeLabel = "Return to Form",
}: CheckModalProps) {
  const titleId = useId();
  const actionRef = useRef<HTMLButtonElement>(null);
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
    actionRef.current?.focus();
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
          {canContinue ? (
            <button
              className="btn primary"
              type="button"
              ref={actionRef}
              onClick={onContinue}
              disabled={running}
            >
              Continue
            </button>
          ) : (
            <button
              className="btn primary"
              type="button"
              ref={actionRef}
              onClick={onClose}
              disabled={running}
            >
              {closeLabel}
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

export type WebhookModalPhase = "confirm" | "manual";

type WebhookModalProps = {
  open: boolean;
  phase: WebhookModalPhase;
  preview: WebhookPreview | null;
  busy: boolean;
  error: string | null;
  onBack: () => void;
  onManual: () => void;
  onCreate: () => void;
  onContinue: () => void;
  canCreate: boolean;
};

export function WebhookConfirmModal({
  open,
  phase,
  preview,
  busy,
  error,
  onBack,
  onManual,
  onCreate,
  onContinue,
  canCreate,
}: WebhookModalProps) {
  const titleId = useId();
  const payload = preview ? JSON.stringify(preview, null, 2) : "";
  const isManual = phase === "manual";

  useEffect(() => {
    if (!open) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape" && !busy) onBack();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, busy, onBack]);

  if (!open || !preview) return null;

  return (
    <div className="modal-backdrop" role="presentation">
      <div className="modal modal--wide" role="dialog" aria-modal="true" aria-labelledby={titleId}>
        <header className="modal__header">
          <h2 id={titleId}>{isManual ? "Add Webhook in Gitea" : "Install System Webhook?"}</h2>
          <p className="muted">
            {isManual
              ? "Lens generated and stored an HMAC secret. Create a system webhook in Gitea using the payload below (Site Administration → Webhooks)."
              : "Gitea will POST live pull-request and Actions events to Lens. Without it, Lens relies on periodic sync only."}
          </p>
        </header>
        <pre className="webhook-preview mono" tabIndex={0} aria-label="Webhook payload">
          {payload}
        </pre>
        {!isManual && (
          <p className="settings-form__hint">
            System hook on your Gitea instance. Secret is generated when you confirm.
          </p>
        )}
        {error && (
          <p className="error" role="alert">
            {error}
          </p>
        )}
        <div className="modal__actions">
          <button className="btn modal__actions-back" type="button" onClick={onBack} disabled={busy}>
            Back
          </button>
          {isManual ? (
            <button className="btn primary" type="button" onClick={onContinue} disabled={busy}>
              Continue
            </button>
          ) : (
            <>
              <button className="btn" type="button" onClick={onManual} disabled={busy}>
                {busy ? "Preparing…" : <>I&apos;ll Add It in Gitea</>}
              </button>
              <button
                className="btn primary"
                type="button"
                onClick={onCreate}
                disabled={busy || !canCreate}
                title={canCreate ? undefined : "Admin system-hooks permission required"}
              >
                {busy ? "Working…" : "Create Webhook"}
              </button>
            </>
          )}
        </div>
      </div>
    </div>
  );
}

export type OAuthModalPhase = "confirm" | "manual";

type OAuthModalProps = {
  open: boolean;
  phase: OAuthModalPhase;
  preview: OAuthAppPreview | null;
  giteaURL: string;
  busy: boolean;
  error: string | null;
  clientId: string;
  clientSecret: string;
  onClientIdChange: (value: string) => void;
  onClientSecretChange: (value: string) => void;
  onBack: () => void;
  onManual: () => void;
  onCreate: () => void;
  onSkip: () => void;
  onContinue: () => void;
  canCreate: boolean;
};

function giteaAbsolute(base: string, path: string): string {
  const root = base.trim().replace(/\/$/, "");
  if (!root) return path;
  return `${root}${path.startsWith("/") ? path : `/${path}`}`;
}

export function OAuthConfirmModal({
  open,
  phase,
  preview,
  giteaURL,
  busy,
  error,
  clientId,
  clientSecret,
  onClientIdChange,
  onClientSecretChange,
  onBack,
  onManual,
  onCreate,
  onSkip,
  onContinue,
  canCreate,
}: OAuthModalProps) {
  const titleId = useId();
  const isManual = phase === "manual";
  const redirectURI = preview?.redirect_uri ?? "";
  const userAppsURL = preview
    ? giteaAbsolute(giteaURL, preview.gitea_settings_path)
    : "";
  const adminAppsURL = preview
    ? giteaAbsolute(giteaURL, preview.gitea_admin_apps_path)
    : "";
  const canContinueManual = Boolean(clientId.trim() && clientSecret.trim());

  useEffect(() => {
    if (!open) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape" && !busy) onBack();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, busy, onBack]);

  if (!open || !preview) return null;

  return (
    <div className="modal-backdrop" role="presentation">
      <div className="modal modal--wide" role="dialog" aria-modal="true" aria-labelledby={titleId}>
        <header className="modal__header">
          <h2 id={titleId}>{isManual ? "Add OAuth App in Gitea" : "Set Up Gitea OAuth?"}</h2>
          <p className="muted">
            {isManual
              ? "Create an OAuth2 application in Gitea, then paste the client ID and secret below. Bootstrap admin login still works without OAuth."
              : "OAuth lets users sign in with their Gitea accounts. You can create the app automatically, configure it yourself, or skip and stay on bootstrap admin only."}
          </p>
        </header>

        {isManual ? (
          <div className="oauth-manual">
            <ol className="oauth-manual__steps">
              <li>
                In Gitea, open{" "}
                <a href={userAppsURL} target="_blank" rel="noreferrer">
                  User Settings → Applications
                </a>
                {adminAppsURL ? (
                  <>
                    {" "}
                    (site admins can also use{" "}
                    <a href={adminAppsURL} target="_blank" rel="noreferrer">
                      Site Administration → Applications
                    </a>
                    ).
                  </>
                ) : null}
              </li>
              <li>
                Create a new OAuth2 application named <strong>{preview.name}</strong>.
              </li>
              <li>
                Set the redirect URI to exactly:
                <code className="mono setup-wizard__redirect oauth-manual__redirect">{redirectURI}</code>
              </li>
              <li>Enable confidential client, create the application, then copy the client ID and secret.</li>
            </ol>
            <div className="settings-form__field">
              <input
                type="text"
                value={clientId}
                onChange={(e) => onClientIdChange(e.target.value)}
                placeholder="OAuth client ID"
                aria-label="OAuth client ID"
                autoComplete="off"
                disabled={busy}
              />
            </div>
            <div className="settings-form__field">
              <input
                type="password"
                value={clientSecret}
                onChange={(e) => onClientSecretChange(e.target.value)}
                placeholder="OAuth client secret"
                aria-label="OAuth client secret"
                autoComplete="new-password"
                disabled={busy}
              />
            </div>
          </div>
        ) : (
          <div className="oauth-confirm-summary">
            <dl className="oauth-confirm-summary__list">
              <div>
                <dt>Application Name</dt>
                <dd>{preview.name}</dd>
              </div>
              <div>
                <dt>Redirect URI</dt>
                <dd>
                  <code className="mono setup-wizard__redirect">{redirectURI}</code>
                </dd>
              </div>
              <div>
                <dt>Confidential Client</dt>
                <dd>{preview.confidential_client ? "Yes" : "No"}</dd>
              </div>
            </dl>
          </div>
        )}

        {error && (
          <p className="error" role="alert">
            {error}
          </p>
        )}
        <div className="modal__actions">
          <button className="btn modal__actions-back" type="button" onClick={onBack} disabled={busy}>
            Back
          </button>
          {isManual ? (
            <button
              className="btn primary"
              type="button"
              onClick={onContinue}
              disabled={busy || !canContinueManual}
            >
              {busy ? "Saving…" : "Continue"}
            </button>
          ) : (
            <>
              <button className="btn" type="button" onClick={onSkip} disabled={busy}>
                Skip OAuth
              </button>
              <button className="btn" type="button" onClick={onManual} disabled={busy}>
                I&apos;ll Configure It
              </button>
              <button
                className="btn primary"
                type="button"
                onClick={onCreate}
                disabled={busy || !canCreate}
                title={canCreate ? undefined : "OAuth application API permission required"}
              >
                {busy ? "Creating…" : "Create OAuth App"}
              </button>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
