import { FormEvent, useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useLocation } from "react-router-dom";
import {
  api,
  type ForgeStatusRow,
  type GitSeerSettings,
  type OpsChecklistItem,
  type SettingsResponse,
  type StorageStatus,
  type SystemStatus,
} from "../api/client";
import { InstanceSettingsPanel } from "../components/InstanceSettingsPanel";
import { OAuthSignInPanel } from "../components/OAuthSignInPanel";
import { AttentionSeverityOverrides } from "../components/AttentionSeverityOverrides";
import { AccessGrantPanel } from "../components/AccessGrantPanel";
import { NotificationsPanel } from "../components/NotificationsPanel";
import { BrowserAlertsPanel } from "../components/BrowserAlertsPanel";
import { WallboardTokensPanel } from "../components/WallboardTokensPanel";
import { BootstrapPanel } from "../components/BootstrapPanel";
import { BecomeBootstrapDialog } from "../components/BecomeBootstrapDialog";
import { BootstrapAccessRequired } from "../components/BootstrapAccessRequired";
import { ConfirmDialog } from "../components/ConfirmDialog";

type SettingsTab =
  | "preferences"
  | "integration"
  | "signin"
  | "access"
  | "notifications"
  | "status"
  | "bootstrap";

const SETTINGS_TABS: { id: SettingsTab; label: string }[] = [
  { id: "status", label: "Status" },
  { id: "notifications", label: "Notifications" },
  { id: "bootstrap", label: "Bootstrap" },
  { id: "preferences", label: "Preferences" },
  { id: "integration", label: "Integration" },
  { id: "signin", label: "Sign In" },
  { id: "access", label: "Access" },
];

/** Lab: short windows for ephemeral environments. */
const LAB_PRESET: Pick<
  GitSeerSettings,
  "sync_history_days" | "retention_runs_days" | "retention_webhooks_days" | "retention_attention_days"
> = {
  sync_history_days: 7,
  retention_runs_days: 14,
  retention_webhooks_days: 7,
  retention_attention_days: 30,
};

/** Prod: package defaults (history 30d; retention 90/30/180). */
const PROD_PRESET: Pick<
  GitSeerSettings,
  "sync_history_days" | "retention_runs_days" | "retention_webhooks_days" | "retention_attention_days"
> = {
  sync_history_days: 30,
  retention_runs_days: 90,
  retention_webhooks_days: 30,
  retention_attention_days: 180,
};

function tabFromHash(hash: string): SettingsTab {
  const id = hash.replace(/^#/, "");
  if (
    id === "integration" ||
    id === "signin" ||
    id === "status" ||
    id === "preferences" ||
    id === "access" ||
    id === "notifications" ||
    id === "bootstrap"
  ) {
    return id;
  }
  return "status";
}

function emptySettings(): GitSeerSettings {
  return {
    instance_name: "",
    sync_history_days: 30,
    attention_long_running_after: "2h",
    retention_runs_days: 90,
    retention_webhooks_days: 30,
    retention_attention_days: 180,
    server_external_url: "",
  };
}

function sameSettings(a: GitSeerSettings, b: GitSeerSettings): boolean {
  return (
    a.instance_name === b.instance_name &&
    a.sync_history_days === b.sync_history_days &&
    a.attention_long_running_after === b.attention_long_running_after &&
    a.retention_runs_days === b.retention_runs_days &&
    a.retention_webhooks_days === b.retention_webhooks_days &&
    a.retention_attention_days === b.retention_attention_days &&
    a.server_external_url === b.server_external_url
  );
}

function forgeDisplayName(ft: string): string {
  switch ((ft || "").toLowerCase()) {
    case "github":
      return "GitHub";
    case "gitea":
      return "Gitea";
    default:
      return ft || "Forge";
  }
}

/** Label forges by name / instance_id when multiple of the same type. */
function forgeStatusLabel(f: ForgeStatusRow, forges: ForgeStatusRow[]): string {
  const typeLabel = forgeDisplayName(f.forge_type);
  const sameType = forges.filter(
    (x) => (x.forge_type || "").toLowerCase() === (f.forge_type || "").toLowerCase(),
  );
  if (sameType.length <= 1) {
    return (f.name || "").trim() || typeLabel;
  }
  const name = (f.name || "").trim();
  if (name) return `${typeLabel} (${name})`;
  if (f.instance_id != null) return `${typeLabel} (#${f.instance_id})`;
  return typeLabel;
}

function yesNo(v: unknown): string {
  return v ? "yes" : "no";
}

function checklistTone(status: string): string {
  switch (status) {
    case "pass":
      return "pass";
    case "fail":
      return "fail";
    case "warn":
      return "warn";
    default:
      return "unknown";
  }
}

function storageLabel(storage?: StorageStatus): string {
  if (!storage) return "—";
  if (storage.error || storage.level === "unknown") return "unavailable";
  const size = storage.bytes_human || (storage.bytes != null ? `${storage.bytes} B` : "—");
  const driver = storage.driver || "db";
  const est = storage.estimate ? " (estimate)" : "";
  const level = storage.level && storage.level !== "ok" ? ` · ${storage.level}` : "";
  const warn =
    storage.warn_human || storage.critical_human
      ? ` · warn ${storage.warn_human || "—"} / critical ${storage.critical_human || "—"}`
      : "";
  return `${size}${est} (${driver})${level}${warn}`;
}

function statusRows(status: SystemStatus | undefined): { label: string; value: string }[] {
  if (!status) return [];
  const rows: { label: string; value: string }[] = [
    { label: "GitSeer Version", value: String(status.version ?? "—") },
    { label: "Instance Name", value: String(status.ui_name ?? "—") },
    { label: "Bootstrap Auth", value: status.bootstrap_auth ? "enabled" : "disabled" },
    { label: "Setup Completed", value: yesNo(status.setup_completed) },
    {
      label: "Encryption",
      value: status.encryption_healthy
        ? `healthy (${status.encryption_source || "configured"})`
        : status.encryption_configured
          ? `unhealthy${status.encryption_error ? `: ${status.encryption_error}` : ""}`
          : "not configured",
    },
    { label: "Database Size", value: storageLabel(status.storage) },
    { label: "OAuth Redirect URI", value: String(status.oauth_redirect_uri ?? "—") },
    { label: "Path Prefix", value: String(status.path_prefix || "/") },
  ];
  if (status.active_actions_hint) {
    rows.push({ label: "Active Actions", value: String(status.active_actions_hint) });
  }
  return rows;
}

export function SettingsPage() {
  const queryClient = useQueryClient();
  const location = useLocation();
  const settingsQuery = useQuery({ queryKey: ["settings"], queryFn: api.settings });
  const meQuery = useQuery({ queryKey: ["me"], queryFn: api.me, retry: false });

  const elevatedUntil = meQuery.data?.bootstrap_elevated_until || null;
  const elevationActive =
    !!elevatedUntil && !Number.isNaN(Date.parse(elevatedUntil)) && Date.parse(elevatedUntil) > Date.now();
  const canElevate = !!meQuery.data?.can_elevate_bootstrap;

  const [tab, setTab] = useState<SettingsTab>(() =>
    typeof window !== "undefined" ? tabFromHash(window.location.hash) : "preferences",
  );
  const [draft, setDraft] = useState<GitSeerSettings>(emptySettings());
  const [baseline, setBaseline] = useState<GitSeerSettings>(emptySettings());
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [savedFlash, setSavedFlash] = useState(false);
  const [opsBusy, setOpsBusy] = useState<string | null>(null);
  const [opsMessage, setOpsMessage] = useState<string | null>(null);
  const [purgeOpen, setPurgeOpen] = useState(false);
  const [purgeBusy, setPurgeBusy] = useState(false);
  const [purgeMessage, setPurgeMessage] = useState<string | null>(null);
  const [elevateOpen, setElevateOpen] = useState(false);
  const [grantMessage, setGrantMessage] = useState<string | null>(null);

  function openBecomeBootstrap() {
    setElevateOpen(true);
  }

  useEffect(() => {
    if (!elevationActive || !elevatedUntil) return;
    const ms = Date.parse(elevatedUntil) - Date.now();
    if (ms <= 0) {
      void queryClient.invalidateQueries({ queryKey: ["me"] });
      void queryClient.invalidateQueries({ queryKey: ["settings"] });
      return;
    }
    const id = window.setTimeout(() => {
      void queryClient.invalidateQueries({ queryKey: ["me"] });
      void queryClient.invalidateQueries({ queryKey: ["settings"] });
    }, ms + 250);
    return () => window.clearTimeout(id);
  }, [elevationActive, elevatedUntil, queryClient]);

  useEffect(() => {
    setTab(tabFromHash(location.hash));
  }, [location.hash]);

  useEffect(() => {
    function onHashChange() {
      setTab(tabFromHash(window.location.hash));
    }
    window.addEventListener("hashchange", onHashChange);
    return () => window.removeEventListener("hashchange", onHashChange);
  }, []);

  useEffect(() => {
    if (!settingsQuery.data) return;
    setDraft(settingsQuery.data.settings);
    setBaseline(settingsQuery.data.settings);
    setError(null);
  }, [settingsQuery.data]);

  function selectTab(next: SettingsTab) {
    setTab(next);
    const nextHash = `#${next}`;
    if (window.location.hash !== nextHash) {
      window.history.replaceState(null, "", `${window.location.pathname}${window.location.search}${nextHash}`);
    }
  }

  useEffect(() => {
    function onBecome() {
      openBecomeBootstrap();
    }
    window.addEventListener("gitseer:become-bootstrap", onBecome);
    return () => window.removeEventListener("gitseer:become-bootstrap", onBecome);
  }, []);

  const editable = settingsQuery.data?.editable === true;
  const dirty = useMemo(() => !sameSettings(draft, baseline), [draft, baseline]);
  const status = statusRows(settingsQuery.data?.status);

  function setField<K extends keyof GitSeerSettings>(key: K, value: GitSeerSettings[K]) {
    setDraft((prev) => ({ ...prev, [key]: value }));
    setSavedFlash(false);
  }

  function cancel() {
    setDraft(baseline);
    setError(null);
    setSavedFlash(false);
  }

  function applyPreset(preset: typeof LAB_PRESET) {
    if (!editable || saving) return;
    setDraft((prev) => ({ ...prev, ...preset }));
    setSavedFlash(false);
  }

  async function runPurgeNow() {
    if (!editable || purgeBusy) return;
    setPurgeBusy(true);
    setPurgeMessage(null);
    try {
      const res = await api.purgeRetention();
      const parts = Object.entries(res.stats || {})
        .map(([k, v]) => `${k}: ${v}`)
        .join(" · ");
      setPurgeMessage(parts ? `Purged ${parts}.` : "Purge complete (nothing aged out).");
      setPurgeOpen(false);
      await queryClient.invalidateQueries({ queryKey: ["settings"] });
    } catch (err) {
      setPurgeMessage(err instanceof Error ? err.message : "Purge failed");
    } finally {
      setPurgeBusy(false);
    }
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!editable || !dirty || saving) return;
    setSaving(true);
    setError(null);
    try {
      const res: SettingsResponse = await api.updateSettings(draft);
      setDraft(res.settings);
      setBaseline(res.settings);
      setSavedFlash(true);
      await queryClient.invalidateQueries({ queryKey: ["settings"] });
    } catch (err) {
      setError(err instanceof Error ? err.message : "save failed");
    } finally {
      setSaving(false);
    }
  }

  async function runEnsureWebhook(instanceId: number) {
    if (!editable || opsBusy) return;
    setOpsBusy(`ensure-${instanceId}`);
    setOpsMessage(null);
    try {
      const res = await api.ensureWebhook(instanceId);
      setOpsMessage(res.hint || (res.ok ? "Webhook ensured." : "Webhook ensure finished."));
      await queryClient.invalidateQueries({ queryKey: ["settings"] });
    } catch (err) {
      setOpsMessage(err instanceof Error ? err.message : "Ensure Webhook failed");
    } finally {
      setOpsBusy(null);
    }
  }

  async function runVerifyWebhook(instanceId: number, confirm: boolean) {
    if (!editable || opsBusy) return;
    setOpsBusy(`verify-${instanceId}-${confirm ? "confirm" : "arm"}`);
    setOpsMessage(null);
    try {
      const res = await api.verifyWebhook(instanceId, { confirm });
      setOpsMessage(
        res.hint ||
          (res.verified
            ? "Webhook delivery verified."
            : "Verification armed — send a Ping from the forge, then Confirm Delivery."),
      );
      await queryClient.invalidateQueries({ queryKey: ["settings"] });
    } catch (err) {
      setOpsMessage(err instanceof Error ? err.message : "Verify Delivery failed");
    } finally {
      setOpsBusy(null);
    }
  }

  async function runSyncNow() {
    if (!editable || opsBusy) return;
    setOpsBusy("sync");
    setOpsMessage(null);
    try {
      await api.syncRepos();
      setOpsMessage("Sync started.");
      await queryClient.invalidateQueries({ queryKey: ["settings"] });
    } catch (err) {
      setOpsMessage(err instanceof Error ? err.message : "Sync Now failed");
    } finally {
      setOpsBusy(null);
    }
  }

  const forges = Array.isArray(settingsQuery.data?.status?.forges)
    ? (settingsQuery.data!.status.forges as ForgeStatusRow[])
    : [];

  function renderChecklist(items: OpsChecklistItem[] | undefined) {
    if (!items || items.length === 0) return null;
    return (
      <ul className="ops-checklist">
        {items.map((item) => (
          <li key={item.id} className={`ops-checklist__item ops-checklist__item--${checklistTone(item.status)}`}>
            <span className="ops-checklist__status" aria-hidden>
              {item.status === "pass" ? "✓" : item.status === "fail" ? "✗" : "!"}
            </span>
            <div>
              <div className="ops-checklist__label">{item.label}</div>
              {item.detail ? <div className="ops-checklist__detail muted">{item.detail}</div> : null}
            </div>
          </li>
        ))}
      </ul>
    );
  }

  return (
    <>
      <div className="topbar">
        <div className="page-header">
          <h1>Settings</h1>
          <p className="muted">
            {editable
              ? "Configure instance behavior. Changes apply immediately."
              : "Sections that change instance configuration require bootstrap admin access."}
          </p>
          {grantMessage && (
            <p className="settings-form__saved" role="status">
              {grantMessage}
            </p>
          )}
          {elevationActive && elevatedUntil && (
            <p className="muted" role="status">
              Bootstrap access active until {new Date(elevatedUntil).toLocaleTimeString()}.
            </p>
          )}
        </div>
      </div>

      {settingsQuery.isLoading && <div className="loading">Loading…</div>}
      {settingsQuery.isError && <div className="error">{(settingsQuery.error as Error).message}</div>}

      {settingsQuery.data && (
        <>
          <div
            className="settings-tabs"
            role="tablist"
            aria-label="Settings sections"
            onKeyDown={(e) => {
              const idx = SETTINGS_TABS.findIndex((item) => item.id === tab);
              if (idx < 0) return;
              if (e.key === "ArrowRight" || e.key === "ArrowLeft") {
                e.preventDefault();
                const delta = e.key === "ArrowRight" ? 1 : -1;
                const next = SETTINGS_TABS[(idx + delta + SETTINGS_TABS.length) % SETTINGS_TABS.length];
                selectTab(next.id);
                document.getElementById(`settings-tab-${next.id}`)?.focus();
              } else if (e.key === "Home") {
                e.preventDefault();
                selectTab(SETTINGS_TABS[0].id);
                document.getElementById(`settings-tab-${SETTINGS_TABS[0].id}`)?.focus();
              } else if (e.key === "End") {
                e.preventDefault();
                const last = SETTINGS_TABS[SETTINGS_TABS.length - 1];
                selectTab(last.id);
                document.getElementById(`settings-tab-${last.id}`)?.focus();
              }
            }}
          >
            {SETTINGS_TABS.map((item) => {
              const selected = tab === item.id;
              const dirtyMark = item.id === "preferences" && dirty;
              return (
                <button
                  key={item.id}
                  type="button"
                  role="tab"
                  id={`settings-tab-${item.id}`}
                  className={`settings-tabs__btn${selected ? " is-active" : ""}`}
                  aria-selected={selected}
                  aria-controls={`settings-panel-${item.id}`}
                  tabIndex={selected ? 0 : -1}
                  onClick={() => selectTab(item.id)}
                >
                  {item.label}
                  {dirtyMark ? <span className="settings-tabs__dirty" aria-label="Unsaved changes" /> : null}
                </button>
              );
            })}
          </div>

          {tab === "preferences" && (
            !editable ? (
              <div id="settings-panel-preferences" role="tabpanel" aria-labelledby="settings-tab-preferences">
                <BootstrapAccessRequired
                  message="Editing preferences requires bootstrap admin access."
                  canElevate={canElevate}
                  onBecomeBootstrap={openBecomeBootstrap}
                />
              </div>
            ) : (
            <>
            <form
              className="panel panel--padded settings-form"
              id="settings-panel-preferences"
              role="tabpanel"
              aria-labelledby="settings-tab-preferences"
              onSubmit={onSubmit}
            >
              <fieldset className="settings-form__section" disabled={saving}>
                <legend>Instance</legend>
                <div className="settings-form__field">
                  <input
                    id="instance_name"
                    type="text"
                    maxLength={100}
                    value={draft.instance_name}
                    onChange={(e) => setField("instance_name", e.target.value)}
                    placeholder="Display name"
                    aria-label="Display name"
                    autoComplete="off"
                  />
                  <p className="settings-form__hint">Shown in status and used when labeling connected forges.</p>
                </div>
                <div className="settings-form__field">
                  <input
                    id="server_external_url"
                    type="text"
                    inputMode="url"
                    value={draft.server_external_url}
                    onChange={(e) => setField("server_external_url", e.target.value)}
                    placeholder="GitSeer public URL (https://gitseer.example.com)"
                    aria-label="GitSeer public URL"
                    autoComplete="off"
                  />
                  <p className="settings-form__hint">
                    Public URL where forges can reach GitSeer for webhooks and OAuth. Maps to{" "}
                    <code className="mono">server.external_url</code>.
                  </p>
                </div>
              </fieldset>

              <fieldset className="settings-form__section" disabled={saving}>
                <legend>Sync</legend>
                <div className="settings-form__field">
                  <input
                    id="sync_history_days"
                    type="number"
                    min={1}
                    max={365}
                    value={draft.sync_history_days}
                    onChange={(e) => setField("sync_history_days", Number(e.target.value))}
                    placeholder="History depth (days)"
                    aria-label="History depth (days)"
                  />
                  <p className="settings-form__hint">How far back to import workflow runs on sync (1–365).</p>
                </div>
              </fieldset>

              <fieldset className="settings-form__section" disabled={saving}>
                <legend>Attention</legend>
                <div className="settings-form__field">
                  <input
                    id="attention_long_running_after"
                    type="text"
                    value={draft.attention_long_running_after}
                    onChange={(e) => setField("attention_long_running_after", e.target.value)}
                    placeholder="Long-running after (e.g. 2h)"
                    aria-label="Long-running after"
                    autoComplete="off"
                  />
                  <p className="settings-form__hint">Duration like 2h or 90m (15m–168h). Incomplete runs older than this raise attention.</p>
                </div>
              </fieldset>

              <fieldset className="settings-form__section" disabled={saving}>
                <legend>Retention</legend>
                <div className="settings-form__presets">
                  <button
                    className="btn"
                    type="button"
                    disabled={saving}
                    onClick={() => applyPreset(LAB_PRESET)}
                  >
                    Apply Lab Preset
                  </button>
                  <button
                    className="btn"
                    type="button"
                    disabled={saving}
                    onClick={() => applyPreset(PROD_PRESET)}
                  >
                    Apply Prod Preset
                  </button>
                </div>
                <p className="settings-form__hint">
                  Lab: history 7d · runs 14d · webhooks 7d · attention 30d. Prod: history 30d · runs 90d ·
                  webhooks 30d · attention 180d. Apply updates the form; click Save Changes to persist.
                </p>
                <div className="settings-form__grid">
                  <div className="settings-form__field">
                    <label htmlFor="retention_runs_days">Workflow runs (days)</label>
                    <input
                      id="retention_runs_days"
                      type="number"
                      min={0}
                      max={3650}
                      value={draft.retention_runs_days}
                      onChange={(e) => setField("retention_runs_days", Number(e.target.value))}
                    />
                  </div>
                  <div className="settings-form__field">
                    <label htmlFor="retention_webhooks_days">Webhook events (days)</label>
                    <input
                      id="retention_webhooks_days"
                      type="number"
                      min={0}
                      max={3650}
                      value={draft.retention_webhooks_days}
                      onChange={(e) => setField("retention_webhooks_days", Number(e.target.value))}
                    />
                  </div>
                  <div className="settings-form__field">
                    <label htmlFor="retention_attention_days">Attention history (days)</label>
                    <input
                      id="retention_attention_days"
                      type="number"
                      min={0}
                      max={3650}
                      value={draft.retention_attention_days}
                      onChange={(e) => setField("retention_attention_days", Number(e.target.value))}
                    />
                  </div>
                </div>
                <p className="settings-form__hint">
                  0 disables purge for that category. Next scheduled retention job (~6h) uses these values.
                </p>
                <div className="settings-form__inline">
                  <button
                    className="btn"
                    type="button"
                    disabled={purgeBusy}
                    onClick={() => setPurgeOpen(true)}
                  >
                    Purge Now
                  </button>
                </div>
                {purgeMessage && (
                  <p className="settings-form__hint" role="status">
                    {purgeMessage}
                  </p>
                )}
              </fieldset>

              {error && <p className="error" role="alert">{error}</p>}
              {savedFlash && !dirty && <p className="settings-form__saved">Saved.</p>}

              <div className="settings-form__actions">
                <button className="btn" type="button" onClick={cancel} disabled={!dirty || saving}>
                  Cancel
                </button>
                <button className="btn primary" type="submit" disabled={!dirty || saving}>
                  {saving ? "Saving…" : "Save Changes"}
                </button>
              </div>
            </form>
            <div className="panel panel--padded settings-severity-panel">
              <fieldset className="settings-form__section">
                <legend>Attention Severity Overrides</legend>
                <AttentionSeverityOverrides editable={editable} />
              </fieldset>
            </div>
            </>
            )
          )}

          {tab === "integration" && (
            <InstanceSettingsPanel
              editable={editable}
              canElevate={canElevate}
              onBecomeBootstrap={openBecomeBootstrap}
            />
          )}
          {tab === "signin" && (
            <OAuthSignInPanel
              editable={editable}
              canElevate={canElevate}
              onBecomeBootstrap={openBecomeBootstrap}
            />
          )}
          {tab === "access" && (
            <AccessGrantPanel
              editable={editable}
              canElevate={canElevate}
              onBecomeBootstrap={openBecomeBootstrap}
            />
          )}
          {tab === "bootstrap" &&
            (elevationActive && elevatedUntil ? (
              <BootstrapPanel elevatedUntil={elevatedUntil} />
            ) : (
              <div id="settings-panel-bootstrap" role="tabpanel" aria-labelledby="settings-tab-bootstrap">
                <BootstrapAccessRequired
                  message="Resetting the bootstrap password requires an elevated bootstrap session."
                  canElevate={canElevate}
                  onBecomeBootstrap={openBecomeBootstrap}
                />
              </div>
            ))}
          {tab === "notifications" && (
            <div
              id="settings-panel-notifications"
              role="tabpanel"
              aria-labelledby="settings-tab-notifications"
            >
              <BrowserAlertsPanel />
              <NotificationsPanel
                editable={editable}
                canElevate={canElevate}
                onBecomeBootstrap={openBecomeBootstrap}
              />
            </div>
          )}

          {tab === "status" && (
            <div id="settings-panel-status" role="tabpanel" aria-labelledby="settings-tab-status">
            <div className="panel panel--padded">
              <h2 className="settings-status__title">Integration Status</h2>
              <dl className="settings-status">
                {status.map((row) => (
                  <div className="settings-status__row" key={row.label}>
                    <dt>{row.label}</dt>
                    <dd className="mono">{row.value}</dd>
                  </div>
                ))}
              </dl>

              {settingsQuery.data?.status?.storage &&
                (settingsQuery.data.status.storage.level === "warn" ||
                  settingsQuery.data.status.storage.level === "critical") && (
                  <p className="error" role="status">
                    Database size is {settingsQuery.data.status.storage.level}
                    {settingsQuery.data.status.storage.bytes_human
                      ? ` (${settingsQuery.data.status.storage.bytes_human})`
                      : ""}
                    . Consider shortening retention or running Purge Now under Preferences.
                  </p>
                )}

              {opsMessage && (
                <p className="settings-form__hint" role="status">
                  {opsMessage}
                </p>
              )}

              {forges.length === 0 && (
                <p className="muted">No forge instances configured yet. Add one under Integration.</p>
              )}

              {forges.map((f) => {
                const name = forgeStatusLabel(f, forges);
                const id = f.instance_id ?? 0;
                const stats = f.webhook_stats_24h;
                return (
                  <section key={`${f.forge_type}-${id}`} className="ops-forge">
                    <div className="ops-forge__header">
                      <h3 className="settings-status__title">{name}</h3>
                      {editable && id > 0 && (
                        <div className="ops-forge__actions">
                          <button
                            className="btn"
                            type="button"
                            disabled={opsBusy != null}
                            onClick={() => runEnsureWebhook(id)}
                          >
                            {opsBusy === `ensure-${id}` ? "Ensuring…" : "Ensure Webhook"}
                          </button>
                          <button
                            className="btn"
                            type="button"
                            disabled={opsBusy != null}
                            onClick={() => runVerifyWebhook(id, false)}
                          >
                            {opsBusy === `verify-${id}-arm` ? "Arming…" : "Verify Delivery"}
                          </button>
                          {f.webhook_verify_pending && (
                            <button
                              className="btn primary"
                              type="button"
                              disabled={opsBusy != null}
                              onClick={() => runVerifyWebhook(id, true)}
                            >
                              {opsBusy === `verify-${id}-confirm` ? "Confirming…" : "Confirm Delivery"}
                            </button>
                          )}
                          <button
                            className="btn"
                            type="button"
                            disabled={opsBusy != null}
                            onClick={() => runSyncNow()}
                          >
                            {opsBusy === "sync" ? "Syncing…" : "Sync Now"}
                          </button>
                        </div>
                      )}
                    </div>
                    <dl className="settings-status">
                      <div className="settings-status__row">
                        <dt>URL</dt>
                        <dd className="mono">{f.url || "—"}</dd>
                      </div>
                      <div className="settings-status__row">
                        <dt>Connected</dt>
                        <dd className="mono">{yesNo(f.connected)}{f.version ? ` (${f.version})` : ""}</dd>
                      </div>
                      <div className="settings-status__row">
                        <dt>Sync Phase</dt>
                        <dd className="mono">
                          {f.sync_phase || "—"}
                          {f.sync_last_error ? ` · ${f.sync_last_error}` : ""}
                        </dd>
                      </div>
                      <div className="settings-status__row">
                        <dt>Sync Lease</dt>
                        <dd className="mono">
                          {f.lease_holder
                            ? `${f.lease_holder}${f.lease_active ? " (active)" : " (expired)"}`
                            : "—"}
                        </dd>
                      </div>
                      <div className="settings-status__row">
                        <dt>Webhooks (24h)</dt>
                        <dd className="mono">
                          {stats
                            ? `ok ${stats.ok ?? 0} · failed ${stats.failed ?? 0} · pending ${stats.pending ?? 0} · total ${stats.total ?? 0}`
                            : "—"}
                        </dd>
                      </div>
                      <div className="settings-status__row">
                        <dt>Last Webhook</dt>
                        <dd className="mono">
                          {f.webhook_last_at || "—"}
                          {f.webhook_last_event ? ` · ${f.webhook_last_event}` : ""}
                          {f.webhook_last_error ? ` · ${f.webhook_last_error}` : ""}
                        </dd>
                      </div>
                      {f.active_actions_hint ? (
                        <div className="settings-status__row">
                          <dt>Active Actions</dt>
                          <dd>{f.active_actions_hint}</dd>
                        </div>
                      ) : null}
                    </dl>
                    <h4 className="ops-forge__subtitle">Ops Checklist</h4>
                    {renderChecklist(f.ops_checklist)}
                    <h4 className="ops-forge__subtitle">Capability Matrix</h4>
                    {renderChecklist(f.capability_matrix)}
                  </section>
                );
              })}
            </div>
            {editable ? <WallboardTokensPanel /> : null}
            </div>
          )}
        </>
      )}

      <ConfirmDialog
        open={purgeOpen}
        title="Purge Retention Now"
        message="Delete aged workflow runs, webhook events, and resolved attention using the currently saved retention windows (not unsaved draft values). This cannot be undone."
        confirmLabel="Purge Now"
        danger
        busy={purgeBusy}
        onConfirm={() => void runPurgeNow()}
        onCancel={() => {
          if (!purgeBusy) setPurgeOpen(false);
        }}
      />
      <BecomeBootstrapDialog
        open={elevateOpen}
        onClose={() => setElevateOpen(false)}
        onGranted={(_until, message) => {
          setGrantMessage(message);
          selectTab("bootstrap");
        }}
      />
    </>
  );
}
