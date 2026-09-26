import { FormEvent, useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useLocation } from "react-router-dom";
import {
  api,
  type ForgeStatusRow,
  type GitSeerSettings,
  type SettingsResponse,
  type SystemStatus,
} from "../api/client";
import { InstanceSettingsPanel } from "../components/InstanceSettingsPanel";

type SettingsTab = "preferences" | "integration" | "status";

const SETTINGS_TABS: { id: SettingsTab; label: string }[] = [
  { id: "preferences", label: "Preferences" },
  { id: "integration", label: "Integration" },
  { id: "status", label: "Status" },
];

function tabFromHash(hash: string): SettingsTab {
  const id = hash.replace(/^#/, "");
  if (id === "integration" || id === "status" || id === "preferences") return id;
  return "preferences";
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

function statusRows(status: SystemStatus | undefined): { label: string; value: string }[] {
  if (!status) return [];
  const rows: { label: string; value: string }[] = [
    { label: "GitSeer Version", value: String(status.version ?? "—") },
    { label: "Instance Name", value: String(status.ui_name ?? "—") },
    { label: "Bootstrap Auth", value: status.bootstrap_auth ? "enabled" : "disabled" },
    { label: "Setup Completed", value: yesNo(status.setup_completed) },
    { label: "OAuth Redirect URI", value: String(status.oauth_redirect_uri ?? "—") },
    { label: "Path Prefix", value: String(status.path_prefix || "/") },
  ];

  const forges = Array.isArray(status.forges) ? (status.forges as ForgeStatusRow[]) : [];
  if (forges.length > 0) {
    for (const f of forges) {
      const name = forgeStatusLabel(f, forges);
      rows.push(
        { label: `${name} URL`, value: String(f.url || "—") },
        { label: `${name} Connected`, value: yesNo(f.connected) },
        { label: `${name} Credentials`, value: f.configured ? "configured" : "missing" },
        { label: `${name} Version`, value: String(f.version ?? "—") },
        { label: `${name} Webhook HMAC`, value: f.webhook_hmac ? "configured" : "missing" },
      );
      if ((f.forge_type || "").toLowerCase() === "gitea") {
        rows.push({
          label: `${name} OAuth`,
          value: f.oauth_configured ? "configured" : "missing",
        });
      }
    }
  } else {
    rows.push(
      { label: "Gitea Connected", value: status.instance_connected ? "yes" : "no" },
      { label: "Gitea Version", value: String(status.gitea_version ?? "—") },
      { label: "Gitea Credentials", value: status.gitea_configured ? "configured" : "missing" },
      { label: "OAuth", value: status.oauth_enabled ? "enabled" : "disabled" },
      { label: "Webhook HMAC", value: status.webhook_hmac ? "configured" : "missing" },
      {
        label: "GitHub Credentials",
        value: status.github_configured ? "configured" : "missing",
      },
      {
        label: "GitHub Webhook HMAC",
        value: status.github_webhook_hmac ? "configured" : "missing",
      },
    );
  }
  return rows;
}

export function SettingsPage() {
  const queryClient = useQueryClient();
  const location = useLocation();
  const settingsQuery = useQuery({ queryKey: ["settings"], queryFn: api.settings });

  const [tab, setTab] = useState<SettingsTab>(() =>
    typeof window !== "undefined" ? tabFromHash(window.location.hash) : "preferences",
  );
  const [draft, setDraft] = useState<GitSeerSettings>(emptySettings());
  const [baseline, setBaseline] = useState<GitSeerSettings>(emptySettings());
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [savedFlash, setSavedFlash] = useState(false);

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

  return (
    <>
      <div className="topbar">
        <div className="page-header">
          <h1>Settings</h1>
          <p className="muted">
            {editable
              ? "Configure instance behavior. Changes apply immediately."
              : "Instance configuration (read-only). Bootstrap admins can edit and save."}
          </p>
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
            <form
              className="panel panel--padded settings-form"
              id="settings-panel-preferences"
              role="tabpanel"
              aria-labelledby="settings-tab-preferences"
              onSubmit={onSubmit}
            >
              <fieldset className="settings-form__section" disabled={!editable || saving}>
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
                    Public URL where forges can reach GitSeer for webhooks and Gitea OAuth. Maps to{" "}
                    <code className="mono">server.external_url</code>.
                  </p>
                </div>
              </fieldset>

              <fieldset className="settings-form__section" disabled={!editable || saving}>
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

              <fieldset className="settings-form__section" disabled={!editable || saving}>
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

              <fieldset className="settings-form__section" disabled={!editable || saving}>
                <legend>Retention</legend>
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
                <p className="settings-form__hint">0 disables purge for that category. Next scheduled retention job uses these values.</p>
              </fieldset>

              {error && <p className="error" role="alert">{error}</p>}
              {savedFlash && !dirty && <p className="settings-form__saved">Saved.</p>}

              {editable && (
                <div className="settings-form__actions">
                  <button className="btn" type="button" onClick={cancel} disabled={!dirty || saving}>
                    Cancel
                  </button>
                  <button className="btn primary" type="submit" disabled={!dirty || saving}>
                    {saving ? "Saving…" : "Save Changes"}
                  </button>
                </div>
              )}
            </form>
          )}

          {tab === "integration" && <InstanceSettingsPanel editable={editable} />}

          {tab === "status" && (
            <div
              className="panel panel--padded"
              id="settings-panel-status"
              role="tabpanel"
              aria-labelledby="settings-tab-status"
            >
              <h2 className="settings-status__title">Integration Status</h2>
              <dl className="settings-status">
                {status.map((row) => (
                  <div className="settings-status__row" key={row.label}>
                    <dt>{row.label}</dt>
                    <dd className="mono">{row.value}</dd>
                  </div>
                ))}
              </dl>
            </div>
          )}
        </>
      )}
    </>
  );
}
