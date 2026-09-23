import { FormEvent, useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  api,
  type IntegrationPatch,
  type IntegrationPublic,
  type LensSettings,
  type SettingsResponse,
} from "../api/client";
import { InfoTip } from "../components/InfoTip";
import { PasswordInput } from "../components/PasswordInput";

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

function emptySettings(): LensSettings {
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

function emptyIntegration(): IntegrationPublic {
  return {
    gitea_url: "",
    gitea_token_configured: false,
    gitea_webhook_secret_configured: false,
    gitea_allow_private_network: false,
    gitea_allow_unsigned_webhooks: false,
    oauth_client_id: "",
    oauth_client_secret_configured: false,
  };
}

type IntegrationDraft = {
  gitea_url: string;
  gitea_token: string;
  gitea_webhook_secret: string;
  gitea_allow_private_network: boolean;
  gitea_allow_unsigned_webhooks: boolean;
  oauth_client_id: string;
  oauth_client_secret: string;
};

function draftFromPublic(integ: IntegrationPublic): IntegrationDraft {
  return {
    gitea_url: integ.gitea_url || "",
    gitea_token: "",
    gitea_webhook_secret: "",
    gitea_allow_private_network: integ.gitea_allow_private_network,
    gitea_allow_unsigned_webhooks: integ.gitea_allow_unsigned_webhooks,
    oauth_client_id: integ.oauth_client_id || "",
    oauth_client_secret: "",
  };
}

function sameSettings(a: LensSettings, b: LensSettings): boolean {
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

function sameIntegrationDraft(a: IntegrationDraft, b: IntegrationDraft): boolean {
  return (
    a.gitea_url === b.gitea_url &&
    a.gitea_token === b.gitea_token &&
    a.gitea_webhook_secret === b.gitea_webhook_secret &&
    a.gitea_allow_private_network === b.gitea_allow_private_network &&
    a.gitea_allow_unsigned_webhooks === b.gitea_allow_unsigned_webhooks &&
    a.oauth_client_id === b.oauth_client_id &&
    a.oauth_client_secret === b.oauth_client_secret
  );
}

function statusRows(status: Record<string, unknown> | undefined): { label: string; value: string }[] {
  if (!status) return [];
  const rows: { label: string; value: string }[] = [
    { label: "Lens version", value: String(status.version ?? "—") },
    { label: "Instance name", value: String(status.ui_name ?? "—") },
    { label: "Gitea connected", value: status.instance_connected ? "yes" : "no" },
    { label: "Gitea version", value: String(status.gitea_version ?? "—") },
    { label: "Gitea credentials", value: status.gitea_configured ? "configured" : "missing" },
    { label: "OAuth", value: status.oauth_enabled ? "enabled" : "disabled" },
    { label: "Bootstrap auth", value: status.bootstrap_auth ? "enabled" : "disabled" },
    { label: "Webhook HMAC", value: status.webhook_hmac ? "configured" : "missing" },
    { label: "Setup completed", value: status.setup_completed ? "yes" : "no" },
    { label: "OAuth redirect URI", value: String(status.oauth_redirect_uri ?? "—") },
    { label: "Path prefix", value: String(status.path_prefix || "/") },
  ];
  return rows;
}

export function SettingsPage() {
  const queryClient = useQueryClient();
  const settingsQuery = useQuery({ queryKey: ["settings"], queryFn: api.settings });

  const [tab, setTab] = useState<SettingsTab>(() =>
    typeof window !== "undefined" ? tabFromHash(window.location.hash) : "preferences",
  );
  const [draft, setDraft] = useState<LensSettings>(emptySettings());
  const [baseline, setBaseline] = useState<LensSettings>(emptySettings());
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [savedFlash, setSavedFlash] = useState(false);

  const [integDraft, setIntegDraft] = useState<IntegrationDraft>(draftFromPublic(emptyIntegration()));
  const [integBaseline, setIntegBaseline] = useState<IntegrationDraft>(draftFromPublic(emptyIntegration()));
  const [integMeta, setIntegMeta] = useState<IntegrationPublic>(emptyIntegration());
  const [integSaving, setIntegSaving] = useState(false);
  const [integError, setIntegError] = useState<string | null>(null);
  const [integSavedFlash, setIntegSavedFlash] = useState(false);

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
    const integ = settingsQuery.data.integration ?? emptyIntegration();
    const next = draftFromPublic(integ);
    setIntegDraft(next);
    setIntegBaseline(next);
    setIntegMeta(integ);
    setIntegError(null);
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
  const integDirty = useMemo(
    () => !sameIntegrationDraft(integDraft, integBaseline),
    [integDraft, integBaseline],
  );
  const status = statusRows(settingsQuery.data?.status);

  function setField<K extends keyof LensSettings>(key: K, value: LensSettings[K]) {
    setDraft((prev) => ({ ...prev, [key]: value }));
    setSavedFlash(false);
  }

  function setIntegField<K extends keyof IntegrationDraft>(key: K, value: IntegrationDraft[K]) {
    setIntegDraft((prev) => ({ ...prev, [key]: value }));
    setIntegSavedFlash(false);
  }

  function cancel() {
    setDraft(baseline);
    setError(null);
    setSavedFlash(false);
  }

  function cancelIntegration() {
    setIntegDraft(integBaseline);
    setIntegError(null);
    setIntegSavedFlash(false);
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

  async function onSubmitIntegration(e: FormEvent) {
    e.preventDefault();
    if (!editable || !integDirty || integSaving) return;
    setIntegSaving(true);
    setIntegError(null);
    try {
      const patch: IntegrationPatch = {
        gitea_url: integDraft.gitea_url.trim(),
        gitea_token: integDraft.gitea_token,
        gitea_webhook_secret: integDraft.gitea_webhook_secret,
        gitea_allow_private_network: integDraft.gitea_allow_private_network,
        gitea_allow_unsigned_webhooks: integDraft.gitea_allow_unsigned_webhooks,
        oauth_client_id: integDraft.oauth_client_id.trim(),
        oauth_client_secret: integDraft.oauth_client_secret,
      };
      const res: SettingsResponse = await api.updateSettings({ integration: patch });
      const integ = res.integration ?? emptyIntegration();
      const next = draftFromPublic(integ);
      setIntegDraft(next);
      setIntegBaseline(next);
      setIntegMeta(integ);
      setIntegSavedFlash(true);
      await queryClient.invalidateQueries({ queryKey: ["settings"] });
    } catch (err) {
      setIntegError(err instanceof Error ? err.message : "save failed");
    } finally {
      setIntegSaving(false);
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
              const dirtyMark =
                (item.id === "preferences" && dirty) || (item.id === "integration" && integDirty);
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
                  <p className="settings-form__hint">Shown in status and used when labeling the connected Gitea instance.</p>
                </div>
                <div className="settings-form__field">
                  <input
                    id="server_external_url"
                    type="text"
                    inputMode="url"
                    value={draft.server_external_url}
                    onChange={(e) => setField("server_external_url", e.target.value)}
                    placeholder="Lens public URL (https://lens.example.com)"
                    aria-label="Lens public URL"
                    autoComplete="off"
                  />
                  <p className="settings-form__hint">
                    Public URL where Gitea can reach Lens for webhooks and OAuth. Maps to{" "}
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

          {tab === "integration" && (
            <form
              className="panel panel--padded settings-form"
              id="settings-panel-integration"
              role="tabpanel"
              aria-labelledby="settings-tab-integration"
              onSubmit={onSubmitIntegration}
            >
              <fieldset className="settings-form__section" disabled={!editable || integSaving}>
                <legend>Gitea</legend>
                <div className="settings-form__field">
                  <input
                    id="integ_gitea_url"
                    type="url"
                    value={integDraft.gitea_url}
                    onChange={(e) => setIntegField("gitea_url", e.target.value)}
                    placeholder="Gitea URL (https://git.example.com)"
                    aria-label="Gitea URL"
                    autoComplete="off"
                  />
                </div>
                <div className="settings-form__field">
                  <PasswordInput
                    id="integ_gitea_token"
                    value={integDraft.gitea_token}
                    onChange={(value) => setIntegField("gitea_token", value)}
                    placeholder={
                      integMeta.gitea_token_configured ? "Service token (leave blank to keep)" : "Service token"
                    }
                    aria-label="Service token"
                    autoComplete="new-password"
                  />
                  <p className="settings-form__hint">
                    {integMeta.gitea_token_configured
                      ? "Token is configured. Leave blank to keep it."
                      : "Personal access token or app token used for sync and API calls."}
                  </p>
                </div>
                <div className="settings-form__field">
                  <input
                    id="integ_webhook_secret"
                    type="password"
                    value={integDraft.gitea_webhook_secret}
                    onChange={(e) => setIntegField("gitea_webhook_secret", e.target.value)}
                    placeholder={
                      integMeta.gitea_webhook_secret_configured
                        ? "Webhook HMAC secret (leave blank to keep)"
                        : "Webhook HMAC secret"
                    }
                    aria-label="Webhook HMAC secret"
                    autoComplete="new-password"
                  />
                  <p className="settings-form__hint">
                    {integMeta.gitea_webhook_secret_configured
                      ? "Secret is configured. Leave blank to keep it."
                      : "Must match the Gitea system webhook secret unless unsigned webhooks are allowed."}
                  </p>
                </div>
                <div className="settings-form__field">
                  <input
                    id="integ_oauth_client_id"
                    type="text"
                    value={integDraft.oauth_client_id}
                    onChange={(e) => setIntegField("oauth_client_id", e.target.value)}
                    placeholder="OAuth client ID"
                    aria-label="OAuth client ID"
                    autoComplete="off"
                  />
                </div>
                <div className="settings-form__field">
                  <input
                    id="integ_oauth_client_secret"
                    type="password"
                    value={integDraft.oauth_client_secret}
                    onChange={(e) => setIntegField("oauth_client_secret", e.target.value)}
                    placeholder={
                      integMeta.oauth_client_secret_configured
                        ? "OAuth client secret (leave blank to keep)"
                        : "OAuth client secret"
                    }
                    aria-label="OAuth client secret"
                    autoComplete="new-password"
                  />
                  <p className="settings-form__hint">
                    {integMeta.oauth_client_secret_configured
                      ? "Secret is configured. Leave blank to keep it."
                      : "From the Gitea OAuth application."}
                  </p>
                </div>
                <label className="settings-form__check">
                  <input
                    type="checkbox"
                    checked={integDraft.gitea_allow_private_network}
                    onChange={(e) => setIntegField("gitea_allow_private_network", e.target.checked)}
                  />
                  <span className="settings-form__check-text">
                    Allow Private Network Addresses
                    <InfoTip label="About private network addresses">
                      Lets Lens call Gitea on private or lab addresses (10.x, 192.168.x, localhost, and similar). Off by
                      default to block SSRF. Enable when Gitea is only reachable on a private network.
                    </InfoTip>
                  </span>
                </label>
                <label className="settings-form__check">
                  <input
                    type="checkbox"
                    checked={integDraft.gitea_allow_unsigned_webhooks}
                    onChange={(e) => setIntegField("gitea_allow_unsigned_webhooks", e.target.checked)}
                  />
                  Allow Unsigned Webhooks (Not Recommended)
                </label>
              </fieldset>

              {integError && (
                <p className="error" role="alert">
                  {integError}
                </p>
              )}
              {integSavedFlash && !integDirty && <p className="settings-form__saved">Integration saved.</p>}

              {editable && (
                <div className="settings-form__actions">
                  <button className="btn" type="button" onClick={cancelIntegration} disabled={!integDirty || integSaving}>
                    Cancel
                  </button>
                  <button className="btn primary" type="submit" disabled={!integDirty || integSaving}>
                    {integSaving ? "Saving…" : "Save Changes"}
                  </button>
                </div>
              )}
            </form>
          )}

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
