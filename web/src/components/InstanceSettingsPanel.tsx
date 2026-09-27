import { FormEvent, useEffect, useId, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  api,
  forgeLabel,
  type ForgeType,
  type InstancePatch,
  type InstancePublic,
} from "../api/client";
import { DEFAULT_GITHUB_URL } from "../lib/product";
import { ConfirmDialog } from "./ConfirmDialog";
import { ForgeBadge } from "./ForgeBadge";
import { GiteaPATHelp } from "./GiteaPATHelp";
import { GitHubPATHelp } from "./GitHubPATHelp";
import { InfoTip } from "./InfoTip";
import { PasswordInput } from "./PasswordInput";

/** 32-byte hex secret — matches server GenerateWebhookSecret. */
function generateWebhookSecret(): string {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}

type DialogMode = "closed" | "pick" | "create" | "edit";

type InstanceDraft = {
  forge_type: ForgeType;
  name: string;
  base_url: string;
  token: string;
  webhook_secret: string;
  oauth_client_id: string;
  oauth_client_secret: string;
  allow_private_network: boolean;
  allow_unsigned_webhooks: boolean;
};

function emptyDraft(forgeType: ForgeType = "gitea"): InstanceDraft {
  return {
    forge_type: forgeType,
    name: "",
    base_url: forgeType === "github" ? DEFAULT_GITHUB_URL : "",
    token: "",
    webhook_secret: "",
    oauth_client_id: "",
    oauth_client_secret: "",
    allow_private_network: false,
    allow_unsigned_webhooks: false,
  };
}

function draftFromPublic(inst: InstancePublic): InstanceDraft {
  return {
    forge_type: (["gitea", "github", "gitlab", "bitbucket", "forgejo"].includes(
      (inst.forge_type || "").toLowerCase(),
    )
      ? (inst.forge_type as ForgeType)
      : "gitea") as ForgeType,
    name: inst.name || "",
    base_url: inst.base_url || "",
    token: "",
    webhook_secret: "",
    oauth_client_id: inst.oauth_client_id || "",
    oauth_client_secret: "",
    allow_private_network: inst.allow_private_network,
    allow_unsigned_webhooks: inst.allow_unsigned_webhooks,
  };
}

function sameDraft(a: InstanceDraft, b: InstanceDraft): boolean {
  return (
    a.forge_type === b.forge_type &&
    a.name === b.name &&
    a.base_url === b.base_url &&
    a.token === b.token &&
    a.webhook_secret === b.webhook_secret &&
    a.oauth_client_id === b.oauth_client_id &&
    a.oauth_client_secret === b.oauth_client_secret &&
    a.allow_private_network === b.allow_private_network &&
    a.allow_unsigned_webhooks === b.allow_unsigned_webhooks
  );
}

function webhookPath(forgeType: string, id: number): string {
  const ft = (forgeType || "gitea").toLowerCase();
  const allowed = new Set(["gitea", "github", "gitlab", "bitbucket", "forgejo"]);
  const pathType = allowed.has(ft) ? ft : "gitea";
  return `/api/webhooks/${pathType}/${id}`;
}

function configuredFlags(inst: InstancePublic): string[] {
  const flags: string[] = [];
  if (inst.token_configured) flags.push("Token");
  if (inst.webhook_secret_configured) flags.push("Webhook");
  if (inst.oauth_client_secret_configured || (inst.oauth_client_id || "").trim()) {
    flags.push("OAuth");
  }
  return flags;
}

type Props = {
  editable: boolean;
};

export function InstanceSettingsPanel({ editable }: Props) {
  const queryClient = useQueryClient();
  const titleId = useId();
  const instancesQuery = useQuery({
    queryKey: ["instances"],
    queryFn: api.instances,
  });

  const [mode, setMode] = useState<DialogMode>("closed");
  const [editing, setEditing] = useState<InstancePublic | null>(null);
  const [draft, setDraft] = useState<InstanceDraft>(emptyDraft());
  const [baseline, setBaseline] = useState<InstanceDraft>(emptyDraft());
  const [saving, setSaving] = useState(false);
  const [removingId, setRemovingId] = useState<number | null>(null);
  const [pendingRemove, setPendingRemove] = useState<InstancePublic | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [listError, setListError] = useState<string | null>(null);
  const [actionHint, setActionHint] = useState<string | null>(null);

  const dirty = useMemo(() => !sameDraft(draft, baseline), [draft, baseline]);
  const isGitea = draft.forge_type === "gitea" || draft.forge_type === "forgejo";
  const isGitHub = draft.forge_type === "github";
  const isGitLab = draft.forge_type === "gitlab";
  const isBitbucket = draft.forge_type === "bitbucket";
  const showsOAuth = isGitea || isGitLab;
  const dialogOpen = mode === "pick" || mode === "create" || mode === "edit";

  useEffect(() => {
    if (!dialogOpen) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape" && !saving) {
        setMode("closed");
        setEditing(null);
        setError(null);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [dialogOpen, saving]);

  function setField<K extends keyof InstanceDraft>(key: K, value: InstanceDraft[K]) {
    setDraft((prev) => ({ ...prev, [key]: value }));
  }

  function closeDialog() {
    setMode("closed");
    setEditing(null);
    setError(null);
    setDraft(emptyDraft());
    setBaseline(emptyDraft());
  }

  function openAdd() {
    setEditing(null);
    setError(null);
    setMode("pick");
  }

  function pickForge(ft: ForgeType) {
    const next = emptyDraft(ft);
    next.webhook_secret = generateWebhookSecret();
    setDraft(next);
    setBaseline({ ...next });
    setMode("create");
  }

  function openEdit(inst: InstancePublic) {
    const next = draftFromPublic(inst);
    setEditing(inst);
    setDraft(next);
    setBaseline(next);
    setError(null);
    setMode("edit");
  }

  async function onSave(e: FormEvent) {
    e.preventDefault();
    if (!editable || !dirty || saving) return;
    setSaving(true);
    setError(null);
    try {
      const patch: InstancePatch = {
        forge_type: draft.forge_type,
        name: draft.name.trim(),
        base_url: draft.base_url.trim(),
        token: draft.token,
        webhook_secret: draft.webhook_secret,
        oauth_client_id: draft.oauth_client_id.trim(),
        oauth_client_secret: draft.oauth_client_secret,
        allow_private_network: draft.allow_private_network,
        allow_unsigned_webhooks: draft.allow_unsigned_webhooks,
      };
      const hadToken = draft.token.trim() !== "";
      if (mode === "edit" && editing) {
        await api.updateInstance(editing.id, patch);
      } else {
        await api.createInstance(patch);
      }
      // Match setup-wizard behavior: a new/updated token should land in inventory
      // without requiring a manual header Sync (background reconcile can lag).
      if (hadToken) {
        try {
          await api.syncRepos();
        } catch {
          // Save succeeded; sync can still be retried from the header.
        }
      }
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["instances"] }),
        queryClient.invalidateQueries({ queryKey: ["settings"] }),
        queryClient.invalidateQueries({ queryKey: ["system-status"] }),
        queryClient.invalidateQueries({ queryKey: ["repositories"] }),
        queryClient.invalidateQueries({ queryKey: ["prs"] }),
        queryClient.invalidateQueries({ queryKey: ["runs"] }),
        queryClient.invalidateQueries({ queryKey: ["attention"] }),
        queryClient.invalidateQueries({ queryKey: ["summary"] }),
      ]);
      closeDialog();
    } catch (err) {
      setError(err instanceof Error ? err.message : "save failed");
    } finally {
      setSaving(false);
    }
  }

  async function onRemove(inst: InstancePublic) {
    if (!editable || removingId != null) return;
    setPendingRemove(inst);
  }

  async function confirmRemove() {
    const inst = pendingRemove;
    if (!inst || !editable || removingId != null) return;
    setRemovingId(inst.id);
    setListError(null);
    try {
      await api.deleteInstance(inst.id);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["instances"] }),
        queryClient.invalidateQueries({ queryKey: ["settings"] }),
        queryClient.invalidateQueries({ queryKey: ["system-status"] }),
      ]);
      setPendingRemove(null);
    } catch (err) {
      setListError(err instanceof Error ? err.message : "remove failed");
      setPendingRemove(null);
    } finally {
      setRemovingId(null);
    }
  }

  const items = instancesQuery.data?.items ?? [];
  const pendingRemoveLabel = pendingRemove
    ? pendingRemove.name || forgeLabel(pendingRemove.forge_type)
    : "";

  return (
    <div
      className="panel panel--padded settings-form"
      id="settings-panel-integration"
      role="tabpanel"
      aria-labelledby="settings-tab-integration"
    >
      <div className="instance-settings__header">
        <div>
          <h2 className="settings-status__title">Forge Instances</h2>
          <p className="muted">
            Add one or more Gitea or GitHub forges. Webhooks use a per-instance path.
          </p>
        </div>
        {editable && (
          <button className="btn primary" type="button" onClick={openAdd}>
            Add Forge
          </button>
        )}
      </div>

      {instancesQuery.isLoading && <div className="loading">Loading instances…</div>}
      {instancesQuery.isError && (
        <div className="error">{(instancesQuery.error as Error).message}</div>
      )}
      {listError && (
        <p className="error" role="alert">
          {listError}
        </p>
      )}
      {actionHint && (
        <p className="settings-form__hint" role="status">
          {actionHint}
        </p>
      )}

      {!instancesQuery.isLoading && !instancesQuery.isError && items.length === 0 && (
        <div className="empty">No forge instances configured yet.</div>
      )}

      {items.length > 0 && (
        <ul className="instance-card-list">
          {items.map((inst) => {
            const flags = configuredFlags(inst);
            const path = webhookPath(inst.forge_type, inst.id);
            return (
              <li key={inst.id} className="instance-card">
                <div className="instance-card__top">
                  <ForgeBadge forgeType={inst.forge_type} instanceName={inst.name} />
                  <span className="instance-card__name">{inst.name || forgeLabel(inst.forge_type)}</span>
                </div>
                <div className="instance-card__url mono">{inst.base_url || "—"}</div>
                <div className="instance-card__meta muted">
                  {flags.length > 0 ? (
                    <span>Configured: {flags.join(" · ")}</span>
                  ) : (
                    <span>Credentials missing</span>
                  )}
                </div>
                <div className="instance-card__webhook">
                  <span className="muted">Webhook path</span>
                  <code className="mono">{path}</code>
                </div>
                {editable && (
                  <div className="instance-card__actions">
                    <button className="btn" type="button" onClick={() => openEdit(inst)}>
                      Edit
                    </button>
                    <button
                      className="btn"
                      type="button"
                      onClick={async () => {
                        setListError(null);
                        setActionHint(null);
                        try {
                          const res = await api.ensureWebhook(inst.id);
                          setActionHint(res.hint || (res.ok ? "Webhook ensured." : "Ensure finished."));
                          await queryClient.invalidateQueries({ queryKey: ["settings"] });
                        } catch (err) {
                          setListError(err instanceof Error ? err.message : "Ensure Webhook failed");
                        }
                      }}
                    >
                      Ensure Webhook
                    </button>
                    <button
                      className="btn"
                      type="button"
                      onClick={async () => {
                        setListError(null);
                        setActionHint(null);
                        try {
                          const res = await api.verifyWebhook(inst.id);
                          setActionHint(res.hint || "Verification armed.");
                          await queryClient.invalidateQueries({ queryKey: ["settings"] });
                        } catch (err) {
                          setListError(err instanceof Error ? err.message : "Verify Delivery failed");
                        }
                      }}
                    >
                      Verify Delivery
                    </button>
                    {(inst.forge_type || "gitea").toLowerCase() === "gitea" && (
                      <button
                        className="btn"
                        type="button"
                        onClick={async () => {
                          setListError(null);
                          setActionHint(null);
                          try {
                            const { blob, filename } = await api.downloadGiteaUISnippets(inst.id, "zip");
                            const url = URL.createObjectURL(blob);
                            const a = document.createElement("a");
                            a.href = url;
                            a.download = filename;
                            document.body.appendChild(a);
                            a.click();
                            a.remove();
                            URL.revokeObjectURL(url);
                            setActionHint(
                              "Downloaded Gitea UI snippets. Extract under Gitea custom/ or use gitseer install-ui on the Gitea host.",
                            );
                          } catch (err) {
                            setListError(
                              err instanceof Error ? err.message : "Download Gitea UI Snippets failed",
                            );
                          }
                        }}
                      >
                        Download Gitea UI Snippets
                      </button>
                    )}
                    <button
                      className="btn"
                      type="button"
                      onClick={() => onRemove(inst)}
                      disabled={removingId === inst.id}
                    >
                      {removingId === inst.id ? "Removing…" : "Remove"}
                    </button>
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      )}

      {dialogOpen && (
        <div className="modal-backdrop" role="presentation">
          <div
            className="modal modal--integration"
            role="dialog"
            aria-modal="true"
            aria-labelledby={titleId}
          >
            {mode === "pick" ? (
              <>
                <header className="modal__header">
                  <h2 id={titleId}>Add Forge</h2>
                  <p className="muted">Choose a forge type to connect.</p>
                </header>
                <div className="forge-pick">
                  <button className="forge-pick__btn" type="button" onClick={() => pickForge("gitea")}>
                    <ForgeBadge forgeType="gitea" />
                    <span>Gitea</span>
                  </button>
                  <button className="forge-pick__btn" type="button" onClick={() => pickForge("github")}>
                    <ForgeBadge forgeType="github" />
                    <span>GitHub</span>
                  </button>
                  <button className="forge-pick__btn" type="button" onClick={() => pickForge("gitlab")}>
                    <ForgeBadge forgeType="gitlab" />
                    <span>GitLab</span>
                  </button>
                  <button className="forge-pick__btn" type="button" onClick={() => pickForge("bitbucket")}>
                    <ForgeBadge forgeType="bitbucket" />
                    <span>Bitbucket</span>
                  </button>
                  <button className="forge-pick__btn" type="button" onClick={() => pickForge("forgejo")}>
                    <ForgeBadge forgeType="forgejo" />
                    <span>Forgejo</span>
                  </button>
                </div>
                <div className="modal__actions">
                  <button className="btn" type="button" onClick={closeDialog}>
                    Cancel
                  </button>
                </div>
              </>
            ) : (
              <form onSubmit={onSave}>
                <header className="modal__header">
                  <h2 id={titleId}>
                    {mode === "edit" ? "Edit Forge" : `Add ${forgeLabel(draft.forge_type)}`}
                  </h2>
                  {mode === "edit" && (
                    <p className="muted">Leave secret fields blank to keep existing values.</p>
                  )}
                </header>

                <fieldset className="settings-form__section" disabled={saving}>
                  <div className="settings-form__field">
                    <input
                      id="inst_name"
                      type="text"
                      value={draft.name}
                      onChange={(e) => setField("name", e.target.value)}
                      placeholder="Display name"
                      aria-label="Display name"
                      autoComplete="off"
                      required
                    />
                  </div>
                  <div className="settings-form__field">
                    <input
                      id="inst_base_url"
                      type="url"
                      value={draft.base_url}
                      onChange={(e) => setField("base_url", e.target.value)}
                      placeholder={
                        isGitea
                          ? draft.forge_type === "forgejo"
                            ? "Forgejo URL (https://forgejo.example.com)"
                            : "Gitea URL (https://git.example.com)"
                          : isGitLab
                            ? "GitLab URL (https://gitlab.com)"
                            : isBitbucket
                              ? "Bitbucket URL (https://bitbucket.org)"
                              : `GitHub URL (${DEFAULT_GITHUB_URL})`
                      }
                      aria-label={`${forgeLabel(draft.forge_type)} URL`}
                      autoComplete="off"
                      required
                    />
                  </div>
                  <div className="settings-form__field">
                    <PasswordInput
                      id="inst_token"
                      value={draft.token}
                      onChange={(value) => setField("token", value)}
                      placeholder={
                        isGitHub
                          ? mode === "edit" && editing?.token_configured
                            ? "GitHub PAT (leave blank to keep)"
                            : "GitHub PAT"
                          : mode === "edit" && editing?.token_configured
                            ? "Service token (leave blank to keep)"
                            : "Service token"
                      }
                      aria-label={isGitHub ? "GitHub PAT" : "Service token"}
                      autoComplete="new-password"
                    />
                    {isGitea ? (
                      <GiteaPATHelp baseURL={draft.base_url} nested />
                    ) : isGitHub ? (
                      <GitHubPATHelp baseURL={draft.base_url} nested />
                    ) : null}
                    {mode === "edit" && editing?.token_configured ? (
                      <p className="settings-form__hint">Token is configured. Leave blank to keep it.</p>
                    ) : null}
                  </div>
                  <div className="settings-form__field">
                    <PasswordInput
                      id="inst_webhook_secret"
                      value={draft.webhook_secret}
                      onChange={(value) => setField("webhook_secret", value)}
                      placeholder={
                        mode === "edit" && editing?.webhook_secret_configured
                          ? "Webhook HMAC secret (leave blank to keep)"
                          : "Webhook HMAC secret"
                      }
                      aria-label="Webhook HMAC secret"
                      autoComplete="new-password"
                    />
                    {mode === "edit" && editing?.webhook_secret_configured ? (
                      <p className="settings-form__hint settings-form__hint--stack">
                        <span>Secret is configured. Leave blank to keep it.</span>
                        <span>
                          Payload URL path:{" "}
                          <code className="mono">{webhookPath(draft.forge_type, editing.id)}</code>
                        </span>
                      </p>
                    ) : isGitea ? (
                      <p className="settings-form__hint settings-form__hint--stack">
                        <span>
                          Shared secret {forgeLabel(draft.forge_type)} uses to sign webhook deliveries (HMAC).
                        </span>
                        <span>
                          Paste the same value into the forge webhook Secret field.
                        </span>
                        <span>
                          {mode === "edit" && editing ? (
                            <>
                              Payload URL path:{" "}
                              <code className="mono">
                                {webhookPath(draft.forge_type, editing.id)}
                              </code>
                            </>
                          ) : (
                            "Payload URL path is assigned after you save (numeric instance id in the path)."
                          )}
                        </span>
                      </p>
                    ) : (
                      <p className="settings-form__hint settings-form__hint--stack">
                        <span>
                          Shared secret {forgeLabel(draft.forge_type)} uses to verify webhook deliveries
                          {isGitLab ? " (token / optional HMAC)" : " (HMAC)"}.
                        </span>
                        <span>
                          Paste the same value into the forge webhook secret field.
                        </span>
                        <span>
                          {mode === "edit" && editing ? (
                            <>
                              Payload URL path:{" "}
                              <code className="mono">
                                {webhookPath(draft.forge_type, editing.id)}
                              </code>
                            </>
                          ) : (
                            "Payload URL path is assigned after you save (numeric instance id in the path)."
                          )}
                        </span>
                      </p>
                    )}
                  </div>
                  {showsOAuth && (
                    <>
                      <div className="settings-form__field">
                        <input
                          id="inst_oauth_client_id"
                          type="text"
                          value={draft.oauth_client_id}
                          onChange={(e) => setField("oauth_client_id", e.target.value)}
                          placeholder="OAuth client ID"
                          aria-label="OAuth client ID"
                          autoComplete="off"
                        />
                      </div>
                      <div className="settings-form__field">
                        <input
                          id="inst_oauth_client_secret"
                          type="password"
                          value={draft.oauth_client_secret}
                          onChange={(e) => setField("oauth_client_secret", e.target.value)}
                          placeholder={
                            mode === "edit" && editing?.oauth_client_secret_configured
                              ? "OAuth client secret (leave blank to keep)"
                              : "OAuth client secret"
                          }
                          aria-label="OAuth client secret"
                          autoComplete="new-password"
                        />
                        <p className="settings-form__hint">
                          {isGitHub
                            ? mode === "edit" && editing?.oauth_client_secret_configured
                              ? "Secret is configured. Leave blank to keep it. Callback: /api/v1/auth/github/callback"
                              : "From the GitHub OAuth App. Register callback {external}/api/v1/auth/github/callback."
                            : mode === "edit" && editing?.oauth_client_secret_configured
                              ? "Secret is configured. Leave blank to keep it."
                              : "From the Gitea OAuth application."}
                        </p>
                      </div>
                    </>
                  )}
                  <label className="settings-form__check">
                    <input
                      type="checkbox"
                      checked={draft.allow_private_network}
                      onChange={(e) => setField("allow_private_network", e.target.checked)}
                    />
                    <span className="settings-form__check-text">
                      Allow Private Network Addresses
                      <InfoTip label="About private network addresses">
                        Lets GitSeer call this forge on private or lab addresses. Off by default to
                        block SSRF.
                      </InfoTip>
                    </span>
                  </label>
                  <label className="settings-form__check">
                    <input
                      type="checkbox"
                      checked={draft.allow_unsigned_webhooks}
                      onChange={(e) => setField("allow_unsigned_webhooks", e.target.checked)}
                    />
                    Allow Unsigned Webhooks (Not Recommended)
                  </label>
                </fieldset>

                {error && (
                  <p className="error" role="alert">
                    {error}
                  </p>
                )}

                <div className="modal__actions">
                  {mode === "create" && (
                    <p className="muted modal__actions-note">
                      Secrets are stored encrypted. Webhook path is assigned after create.
                    </p>
                  )}
                  <button className="btn" type="button" onClick={closeDialog} disabled={saving}>
                    Cancel
                  </button>
                  <button className="btn primary" type="submit" disabled={!dirty || saving}>
                    {saving ? "Saving…" : "Save Changes"}
                  </button>
                </div>
              </form>
            )}
          </div>
        </div>
      )}

      <ConfirmDialog
        open={pendingRemove != null}
        title="Remove Forge"
        message={
          <>
            Remove forge “{pendingRemoveLabel}”? This permanently deletes synced
            repositories, pull requests, runs, and ACL rows for this instance.
          </>
        }
        confirmLabel="Remove"
        danger
        busy={removingId != null}
        onConfirm={() => {
          void confirmRemove();
        }}
        onCancel={() => {
          if (removingId == null) setPendingRemove(null);
        }}
      />
    </div>
  );
}
