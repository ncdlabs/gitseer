import { FormEvent, useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  api,
  forgeLabel,
  type InstancePublic,
  type SystemStatus,
} from "../api/client";
import { BootstrapAccessRequired } from "./BootstrapAccessRequired";
import { ForgeBadge } from "./ForgeBadge";
import { PasswordInput } from "./PasswordInput";

type Mode = "settings" | "wizard";

type Props = {
  editable: boolean;
  mode?: Mode;
  canElevate?: boolean;
  onBecomeBootstrap?: () => void;
  /** Wizard: advance after configuring (or when none selected). */
  onContinue?: () => void;
  /** Wizard: skip OAuth setup entirely. */
  onSkip?: () => void;
};

type Draft = {
  oauth_client_id: string;
  oauth_client_secret: string;
};

function emptyDraft(inst?: InstancePublic): Draft {
  return {
    oauth_client_id: inst?.oauth_client_id || "",
    oauth_client_secret: "",
  };
}

function oauthConfigured(inst: InstancePublic): boolean {
  return Boolean(inst.oauth_client_secret_configured || (inst.oauth_client_id || "").trim());
}

function supportsAutoCreate(forgeType: string): boolean {
  const ft = (forgeType || "").toLowerCase();
  return ft === "gitea" || ft === "forgejo";
}

export function oauthRedirectURI(forgeType: string, status?: SystemStatus | null): string {
  const ft = (forgeType || "").toLowerCase();
  switch (ft) {
    case "github":
      return String(status?.github_oauth_redirect_uri ?? "");
    case "gitlab":
      return String(status?.gitlab_oauth_redirect_uri ?? "");
    case "bitbucket":
      return String(status?.bitbucket_oauth_redirect_uri ?? "");
    case "forgejo":
      return String(status?.forgejo_oauth_redirect_uri ?? "");
    default:
      return String(status?.oauth_redirect_uri ?? "");
  }
}

function forgeAppHint(forgeType: string): string {
  switch ((forgeType || "").toLowerCase()) {
    case "github":
      return "Create an OAuth App under Developer settings → OAuth Apps. Register the callback URI below.";
    case "gitlab":
      return "Create an application under Preferences → Applications (or Admin → Applications). Register the callback URI below.";
    case "bitbucket":
      return "Create an OAuth consumer under Workspace settings → OAuth consumers. Register the callback URI below.";
    case "forgejo":
      return "Create an OAuth2 application under User Settings → Applications (or Site Administration → Applications).";
    default:
      return "Create an OAuth2 application under User Settings → Applications (or Site Administration → Applications).";
  }
}

function sameDraft(a: Draft, b: Draft): boolean {
  return a.oauth_client_id === b.oauth_client_id && a.oauth_client_secret === b.oauth_client_secret;
}

export function OAuthSignInPanel({
  editable,
  mode = "settings",
  canElevate,
  onBecomeBootstrap,
  onContinue,
  onSkip,
}: Props) {
  const queryClient = useQueryClient();
  const isWizard = mode === "wizard";

  const instancesQuery = useQuery({
    queryKey: ["instances"],
    queryFn: api.instances,
    enabled: editable,
  });
  const settingsQuery = useQuery({
    queryKey: ["settings"],
    queryFn: api.settings,
    enabled: editable,
  });

  const items = instancesQuery.data?.items ?? [];
  const status = settingsQuery.data?.status;

  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [drafts, setDrafts] = useState<Record<number, Draft>>({});
  const [baselines, setBaselines] = useState<Record<number, Draft>>({});
  const [savingId, setSavingId] = useState<number | null>(null);
  const [creatingId, setCreatingId] = useState<number | null>(null);
  const [savingAll, setSavingAll] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [hint, setHint] = useState<string | null>(null);
  const [copiedId, setCopiedId] = useState<number | null>(null);
  const [selectionReady, setSelectionReady] = useState(false);

  useEffect(() => {
    if (!items.length) {
      setDrafts({});
      setBaselines({});
      return;
    }
    setDrafts((prev) => {
      const next: Record<number, Draft> = {};
      for (const inst of items) {
        next[inst.id] = prev[inst.id] ?? emptyDraft(inst);
      }
      return next;
    });
    setBaselines((prev) => {
      const next: Record<number, Draft> = {};
      for (const inst of items) {
        // Keep local baseline if we already have a draft in progress for this id.
        next[inst.id] = prev[inst.id] ?? emptyDraft(inst);
      }
      return next;
    });
  }, [items]);

  useEffect(() => {
    if (!isWizard || selectionReady || !items.length) return;
    const initial = new Set<number>();
    for (const inst of items) {
      if (!oauthConfigured(inst)) initial.add(inst.id);
    }
    // If everything already has OAuth, select all so the admin can review/update.
    if (initial.size === 0) {
      for (const inst of items) initial.add(inst.id);
    }
    setSelected(initial);
    setSelectionReady(true);
  }, [isWizard, items, selectionReady]);

  const dirtyIds = useMemo(() => {
    const out = new Set<number>();
    for (const inst of items) {
      const d = drafts[inst.id];
      const b = baselines[inst.id];
      if (d && b && !sameDraft(d, b)) out.add(inst.id);
    }
    return out;
  }, [items, drafts, baselines]);

  function setDraftField(id: number, key: keyof Draft, value: string) {
    setDrafts((prev) => ({
      ...prev,
      [id]: { ...(prev[id] ?? emptyDraft()), [key]: value },
    }));
    setError(null);
    setHint(null);
  }

  function toggleSelected(id: number) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  async function saveInstance(inst: InstancePublic): Promise<void> {
    const draft = drafts[inst.id] ?? emptyDraft(inst);
    await api.updateInstance(inst.id, {
      oauth_client_id: draft.oauth_client_id.trim(),
      oauth_client_secret: draft.oauth_client_secret,
    });
    setBaselines((prev) => ({
      ...prev,
      [inst.id]: { oauth_client_id: draft.oauth_client_id.trim(), oauth_client_secret: "" },
    }));
    setDrafts((prev) => ({
      ...prev,
      [inst.id]: { oauth_client_id: draft.oauth_client_id.trim(), oauth_client_secret: "" },
    }));
  }

  async function onSaveOne(e: FormEvent, inst: InstancePublic) {
    e.preventDefault();
    if (!editable || savingId != null || creatingId != null) return;
    if (!dirtyIds.has(inst.id)) return;
    setSavingId(inst.id);
    setError(null);
    setHint(null);
    try {
      await saveInstance(inst);
      await invalidate();
      setHint(`Saved OAuth for ${inst.name || forgeLabel(inst.forge_type)}.`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "save failed");
    } finally {
      setSavingId(null);
    }
  }

  async function onSaveSelected() {
    if (!editable || savingAll || creatingId != null) return;
    const targets = items.filter((inst) => selected.has(inst.id) && dirtyIds.has(inst.id));
    if (targets.length === 0) {
      onContinue?.();
      return;
    }
    setSavingAll(true);
    setError(null);
    setHint(null);
    try {
      for (const inst of targets) {
        await saveInstance(inst);
      }
      await invalidate();
      setHint(
        targets.length === 1
          ? `Saved OAuth for ${targets[0].name || forgeLabel(targets[0].forge_type)}.`
          : `Saved OAuth for ${targets.length} forges.`,
      );
      onContinue?.();
    } catch (err) {
      setError(err instanceof Error ? err.message : "save failed");
    } finally {
      setSavingAll(false);
    }
  }

  async function onCreateApp(inst: InstancePublic) {
    if (!editable || !supportsAutoCreate(inst.forge_type) || creatingId != null || savingId != null) {
      return;
    }
    setCreatingId(inst.id);
    setError(null);
    setHint(null);
    try {
      const res = await api.createOAuth({
        forge_type: inst.forge_type,
        gitea_url: inst.base_url,
        gitea_allow_private_network: inst.allow_private_network,
        create: true,
      });
      await api.updateInstance(inst.id, {
        oauth_client_id: res.client_id,
        oauth_client_secret: res.client_secret || "",
      });
      setDrafts((prev) => ({
        ...prev,
        [inst.id]: { oauth_client_id: res.client_id, oauth_client_secret: "" },
      }));
      setBaselines((prev) => ({
        ...prev,
        [inst.id]: { oauth_client_id: res.client_id, oauth_client_secret: "" },
      }));
      await invalidate();
      setHint(
        res.created
          ? `Created OAuth app for ${inst.name || forgeLabel(inst.forge_type)}.`
          : `Updated OAuth app for ${inst.name || forgeLabel(inst.forge_type)}.`,
      );
    } catch (err) {
      setError(err instanceof Error ? err.message : "create OAuth app failed");
    } finally {
      setCreatingId(null);
    }
  }

  async function invalidate() {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["instances"] }),
      queryClient.invalidateQueries({ queryKey: ["settings"] }),
      queryClient.invalidateQueries({ queryKey: ["system-status"] }),
      queryClient.invalidateQueries({ queryKey: ["ui-config"] }),
      queryClient.invalidateQueries({ queryKey: ["me"] }),
    ]);
  }

  async function copyRedirect(inst: InstancePublic) {
    const uri = oauthRedirectURI(inst.forge_type, status);
    if (!uri) return;
    try {
      await navigator.clipboard.writeText(uri);
      setCopiedId(inst.id);
      window.setTimeout(() => setCopiedId((cur) => (cur === inst.id ? null : cur)), 2000);
    } catch {
      setError("Could not copy redirect URI.");
    }
  }

  if (!editable) {
    return (
      <div id="settings-panel-signin" role="tabpanel" aria-labelledby="settings-tab-signin">
        <BootstrapAccessRequired
          message="OAuth sign-in setup requires bootstrap admin access."
          canElevate={canElevate}
          onBecomeBootstrap={onBecomeBootstrap}
        />
      </div>
    );
  }

  const busy = savingId != null || creatingId != null || savingAll;

  return (
    <div
      className={`panel panel--padded settings-form${isWizard ? " oauth-signin--wizard" : ""}`}
      id={isWizard ? undefined : "settings-panel-signin"}
      role={isWizard ? undefined : "tabpanel"}
      aria-labelledby={isWizard ? undefined : "settings-tab-signin"}
    >
      <div className="instance-settings__header">
        <div>
          {!isWizard && <h2 className="settings-status__title">Sign In</h2>}
          <p className="muted">
            Configure forge OAuth apps so users can sign in to GitSeer. Forge inventory sync still uses
            each instance&apos;s access token under Integration.
          </p>
        </div>
      </div>

      {instancesQuery.isLoading && <div className="loading">Loading forges…</div>}
      {instancesQuery.isError && (
        <div className="error">{(instancesQuery.error as Error).message}</div>
      )}

      {!instancesQuery.isLoading && !instancesQuery.isError && items.length === 0 && (
        <div className="empty">
          No forge instances yet. Connect a forge under Settings → Integration first.
        </div>
      )}

      {error && (
        <p className="error" role="alert">
          {error}
        </p>
      )}
      {hint && (
        <p className="settings-form__hint" role="status">
          {hint}
        </p>
      )}

      {items.length > 0 && (
        <ul className="instance-card-list oauth-signin__list">
          {items.map((inst) => {
            const draft = drafts[inst.id] ?? emptyDraft(inst);
            const configured = oauthConfigured(inst);
            const redirect = oauthRedirectURI(inst.forge_type, status);
            const dirty = dirtyIds.has(inst.id);
            const isSelected = !isWizard || selected.has(inst.id);
            const showForm = isSelected;
            const autoCreate = supportsAutoCreate(inst.forge_type);

            return (
              <li key={inst.id} className={`instance-card${isSelected ? "" : " instance-card--dim"}`}>
                <div className="instance-card__top">
                  {isWizard && (
                    <label className="settings-form__check oauth-signin__select">
                      <input
                        type="checkbox"
                        checked={selected.has(inst.id)}
                        onChange={() => toggleSelected(inst.id)}
                        disabled={busy}
                        aria-label={`Configure OAuth for ${inst.name || forgeLabel(inst.forge_type)}`}
                      />
                    </label>
                  )}
                  <ForgeBadge forgeType={inst.forge_type} instanceName={inst.name} />
                  <span className="instance-card__name">{inst.name || forgeLabel(inst.forge_type)}</span>
                  {configured ? (
                    <span className="badge">OAuth Configured</span>
                  ) : (
                    <span className="badge">OAuth Not Set</span>
                  )}
                </div>
                <div className="instance-card__url mono">{inst.base_url || "—"}</div>

                {showForm && (
                  <form
                    className="oauth-signin__form"
                    onSubmit={(e) => void onSaveOne(e, inst)}
                    noValidate
                  >
                    <div className="settings-form__field">
                      <div className="oauth-signin__redirect">
                        <span className="muted">Callback URI</span>
                        <code className="mono oauth-signin__redirect-uri">
                          {redirect || "Set GitSeer public URL under Preferences first."}
                        </code>
                        {redirect ? (
                          <button
                            className="btn"
                            type="button"
                            onClick={() => void copyRedirect(inst)}
                            disabled={busy}
                          >
                            {copiedId === inst.id ? "Copied" : "Copy"}
                          </button>
                        ) : null}
                      </div>
                      <p className="settings-form__hint">{forgeAppHint(inst.forge_type)}</p>
                    </div>

                    <div className="settings-form__field">
                      <input
                        id={`oauth_client_id_${inst.id}`}
                        type="text"
                        value={draft.oauth_client_id}
                        onChange={(e) => setDraftField(inst.id, "oauth_client_id", e.target.value)}
                        placeholder="OAuth Client ID"
                        aria-label={`OAuth Client ID for ${forgeLabel(inst.forge_type)}`}
                        autoComplete="off"
                        disabled={busy}
                      />
                    </div>
                    <div className="settings-form__field">
                      <PasswordInput
                        id={`oauth_client_secret_${inst.id}`}
                        value={draft.oauth_client_secret}
                        onChange={(value) => setDraftField(inst.id, "oauth_client_secret", value)}
                        placeholder={
                          inst.oauth_client_secret_configured
                            ? "OAuth Client Secret (leave blank to keep)"
                            : "OAuth Client Secret"
                        }
                        aria-label={`OAuth Client Secret for ${forgeLabel(inst.forge_type)}`}
                        autoComplete="new-password"
                        disabled={busy}
                      />
                      {inst.oauth_client_secret_configured ? (
                        <p className="settings-form__hint">
                          Secret is configured. Leave blank to keep it.
                        </p>
                      ) : null}
                    </div>

                    <div className="instance-card__actions">
                      {autoCreate && (
                        <button
                          className="btn"
                          type="button"
                          onClick={() => void onCreateApp(inst)}
                          disabled={busy || !redirect}
                          title={
                            redirect
                              ? undefined
                              : "Set GitSeer public URL under Preferences before creating an OAuth app"
                          }
                        >
                          {creatingId === inst.id ? "Creating…" : "Create OAuth App"}
                        </button>
                      )}
                      {!isWizard && (
                        <button
                          className="btn primary"
                          type="submit"
                          disabled={!dirty || busy}
                        >
                          {savingId === inst.id ? "Saving…" : "Save Changes"}
                        </button>
                      )}
                    </div>
                  </form>
                )}
              </li>
            );
          })}
        </ul>
      )}

      {isWizard && items.length > 0 && (
        <div className="settings-form__actions oauth-signin__wizard-actions">
          {onSkip && (
            <button className="btn" type="button" onClick={onSkip} disabled={busy}>
              Skip Sign In
            </button>
          )}
          <button
            className="btn primary"
            type="button"
            onClick={() => void onSaveSelected()}
            disabled={busy}
          >
            {savingAll
              ? "Saving…"
              : dirtyIds.size > 0 && [...dirtyIds].some((id) => selected.has(id))
                ? "Save And Continue"
                : "Continue"}
          </button>
        </div>
      )}
    </div>
  );
}
