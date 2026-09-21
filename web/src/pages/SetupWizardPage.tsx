import { FormEvent, useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  api,
  type IntegrationPatch,
  type IntegrationPublic,
  type ProbeCheck,
  type TestConnectionResponse,
  type WebhookPreview,
} from "../api/client";
import { InfoTip } from "../components/InfoTip";
import { PasswordInput } from "../components/PasswordInput";
import { ConnectionCheckModal, WebhookConfirmModal } from "../components/SetupConnectionModals";

type FieldKey =
  | "gitea_url"
  | "gitea_token"
  | "gitea_webhook_secret"
  | "gitea_allow_private_network"
  | "server_external_url";
type FieldErrors = Partial<Record<FieldKey, string>>;

const FIELD_INPUT_IDS: Record<FieldKey, string> = {
  gitea_url: "wiz_gitea_url",
  gitea_token: "wiz_gitea_token",
  gitea_webhook_secret: "wiz_webhook_secret",
  gitea_allow_private_network: "wiz_allow_private_network",
  server_external_url: "wiz_server_external_url",
};

function isValidGiteaURL(raw: string): boolean {
  try {
    const u = new URL(raw.trim());
    return (u.protocol === "http:" || u.protocol === "https:") && Boolean(u.host);
  } catch {
    return false;
  }
}

function isPrivateNetworkMessage(msg: string): boolean {
  return (
    msg.includes("Allow Private Network Addresses") ||
    /private network address/i.test(msg)
  );
}

function validateConnectFields(
  draft: Draft,
  tokenConfigured: boolean,
): { ok: boolean; errors: FieldErrors; firstInvalid?: FieldKey } {
  const errors: FieldErrors = {};
  const url = draft.gitea_url.trim();
  if (!url) {
    errors.gitea_url = "Gitea URL is required.";
  } else if (!isValidGiteaURL(url)) {
    errors.gitea_url = "Enter a valid http:// or https:// URL.";
  }
  if (!draft.gitea_token.trim() && !tokenConfigured) {
    errors.gitea_token = "Service token is required.";
  }
  const publicURL = draft.server_external_url.trim();
  if (!publicURL) {
    errors.server_external_url = "Lens public URL is required.";
  } else if (!isValidGiteaURL(publicURL)) {
    errors.server_external_url = "Enter a valid http:// or https:// URL.";
  }
  const order: FieldKey[] = [
    "gitea_url",
    "gitea_token",
    "server_external_url",
    "gitea_allow_private_network",
  ];
  const firstInvalid = order.find((k) => errors[k]);
  return { ok: !firstInvalid, errors, firstInvalid };
}

function validateWebhookFields(
  draft: Draft,
  secretConfigured: boolean,
): { ok: boolean; errors: FieldErrors; firstInvalid?: FieldKey } {
  const errors: FieldErrors = {};
  if (!draft.gitea_webhook_secret.trim() && !secretConfigured && !draft.gitea_allow_unsigned_webhooks) {
    errors.gitea_webhook_secret = "Provide a webhook HMAC secret, or allow unsigned webhooks.";
  }
  return {
    ok: !errors.gitea_webhook_secret,
    errors,
    firstInvalid: errors.gitea_webhook_secret ? "gitea_webhook_secret" : undefined,
  };
}

type Step = "connect" | "webhooks" | "oauth" | "validate" | "finish";

const STEPS: { id: Step; label: string; description: string }[] = [
  {
    id: "connect",
    label: "Connect",
    description:
      "Point Lens at your Gitea instance and set the public Lens URL Gitea will use for webhooks and OAuth callbacks.",
  },
  {
    id: "webhooks",
    label: "Webhooks",
    description:
      "Configure the shared HMAC secret so Gitea can deliver live workflow and pull-request events to Lens securely.",
  },
  {
    id: "oauth",
    label: "OAuth",
    description:
      "Add Gitea OAuth application credentials so users can sign in with their Gitea accounts instead of only the bootstrap admin.",
  },
  {
    id: "validate",
    label: "Validate",
    description:
      "Test that Lens can reach Gitea with the saved URL and token before you mark setup complete.",
  },
  {
    id: "finish",
    label: "Finish",
    description:
      "Complete setup and optionally sync repositories so Lens can start indexing your forge data.",
  },
];

type Draft = {
  gitea_url: string;
  gitea_token: string;
  gitea_allow_private_network: boolean;
  gitea_webhook_secret: string;
  gitea_allow_unsigned_webhooks: boolean;
  oauth_client_id: string;
  oauth_client_secret: string;
  server_external_url: string;
};

function emptyDraft(): Draft {
  return {
    gitea_url: "",
    gitea_token: "",
    gitea_allow_private_network: false,
    gitea_webhook_secret: "",
    gitea_allow_unsigned_webhooks: false,
    oauth_client_id: "",
    oauth_client_secret: "",
    server_external_url: "",
  };
}

function draftFromIntegration(
  integ: IntegrationPublic | undefined,
  serverExternalURL = "",
): Draft {
  if (!integ) {
    return { ...emptyDraft(), server_external_url: serverExternalURL };
  }
  return {
    gitea_url: integ.gitea_url || "",
    gitea_token: "",
    gitea_allow_private_network: integ.gitea_allow_private_network,
    gitea_webhook_secret: "",
    gitea_allow_unsigned_webhooks: integ.gitea_allow_unsigned_webhooks,
    oauth_client_id: integ.oauth_client_id || "",
    oauth_client_secret: "",
    server_external_url: serverExternalURL,
  };
}

function toPatch(draft: Draft): IntegrationPatch {
  return {
    gitea_url: draft.gitea_url.trim(),
    gitea_token: draft.gitea_token,
    gitea_webhook_secret: draft.gitea_webhook_secret,
    gitea_allow_private_network: draft.gitea_allow_private_network,
    gitea_allow_unsigned_webhooks: draft.gitea_allow_unsigned_webhooks,
    oauth_client_id: draft.oauth_client_id.trim(),
    oauth_client_secret: draft.oauth_client_secret,
  };
}

type Props = {
  onComplete: () => void;
  onLogout: () => void;
  login: string;
};

export function SetupWizardPage({ onComplete, onLogout, login }: Props) {
  const queryClient = useQueryClient();
  const settingsQuery = useQuery({ queryKey: ["settings"], queryFn: api.settings });
  const [step, setStep] = useState<Step>("connect");
  const [draft, setDraft] = useState<Draft>(emptyDraft());
  const [hydrated, setHydrated] = useState(false);
  const [busy, setBusy] = useState<"save" | "test" | "webhook" | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [testResult, setTestResult] = useState<TestConnectionResponse | null>(null);
  const [syncAfter, setSyncAfter] = useState(true);
  const [finishing, setFinishing] = useState(false);

  const [checkOpen, setCheckOpen] = useState(false);
  const [checkRunning, setCheckRunning] = useState(false);
  const [checkRows, setCheckRows] = useState<ProbeCheck[] | null>(null);
  const [checkError, setCheckError] = useState<string | null>(null);
  const [webhookOpen, setWebhookOpen] = useState(false);
  const [webhookPreview, setWebhookPreview] = useState<WebhookPreview | null>(null);
  const [webhookError, setWebhookError] = useState<string | null>(null);
  const [canCreateWebhook, setCanCreateWebhook] = useState(false);

  useEffect(() => {
    if (!settingsQuery.data || hydrated) return;
    const publicURL =
      settingsQuery.data.settings?.server_external_url ||
      String(settingsQuery.data.status?.server_external_url ?? "");
    setDraft(draftFromIntegration(settingsQuery.data.integration, publicURL));
    setHydrated(true);
  }, [settingsQuery.data, hydrated]);

  const stepIndex = useMemo(() => STEPS.findIndex((s) => s.id === step), [step]);
  const currentStep = STEPS[stepIndex] ?? STEPS[0];
  const redirectURI = String(settingsQuery.data?.status?.oauth_redirect_uri ?? "—");
  const integ = settingsQuery.data?.integration;
  const isBusy = busy !== null;
  const checksPassed = Boolean(testResult?.ok);

  function focusField(key: FieldKey) {
    const el = document.getElementById(FIELD_INPUT_IDS[key]);
    if (el instanceof HTMLElement) el.focus();
  }

  function setField<K extends keyof Draft>(key: K, value: Draft[K]) {
    setDraft((prev) => ({ ...prev, [key]: value }));
    setError(null);
    setTestResult(null);
    if (key === "gitea_url" || key === "gitea_token" || key === "gitea_webhook_secret" || key === "server_external_url") {
      const field = key as FieldKey;
      setFieldErrors((prev) => {
        if (!prev[field]) return prev;
        const next = { ...prev };
        delete next[field];
        return next;
      });
    }
    if (key === "gitea_allow_unsigned_webhooks") {
      setFieldErrors((prev) => {
        if (!prev.gitea_webhook_secret) return prev;
        const next = { ...prev };
        delete next.gitea_webhook_secret;
        return next;
      });
    }
    if (key === "gitea_allow_private_network") {
      setFieldErrors((prev) => {
        if (!prev.gitea_allow_private_network && !prev.gitea_url) return prev;
        const next = { ...prev };
        delete next.gitea_allow_private_network;
        if (value === true) delete next.gitea_url;
        return next;
      });
      if (value === true && draft.gitea_url.trim()) {
        void onGiteaURLBlur(true);
      }
    }
  }

  async function onGiteaURLBlur(allowPrivateOverride?: boolean) {
    const raw = draft.gitea_url.trim();
    if (!raw || isBusy) return;
    const allowPrivate = allowPrivateOverride ?? draft.gitea_allow_private_network;
    setFieldErrors((prev) => {
      const next = { ...prev };
      delete next.gitea_url;
      delete next.gitea_allow_private_network;
      return next;
    });
    try {
      const res = await api.checkGiteaURL({
        gitea_url: raw,
        gitea_allow_private_network: allowPrivate,
      });
      if (res.gitea_url && res.gitea_url !== draft.gitea_url.trim()) {
        setDraft((prev) => ({ ...prev, gitea_url: res.gitea_url! }));
        setTestResult(null);
      }
      if (res.ok) return;
      if (res.code === "private_network") {
        setFieldErrors((prev) => ({
          ...prev,
          gitea_allow_private_network:
            res.error ||
            'This is a private address. To continue, check "Allow Private Network Addresses".',
        }));
        focusField("gitea_allow_private_network");
        return;
      }
      setFieldErrors((prev) => ({
        ...prev,
        gitea_url: res.error || "Could not validate Gitea URL.",
      }));
      focusField("gitea_url");
    } catch (err) {
      setFieldErrors((prev) => ({
        ...prev,
        gitea_url: err instanceof Error ? err.message : "Could not validate Gitea URL.",
      }));
    }
  }

  function runConnectValidation(): boolean {
    const result = validateConnectFields(draft, Boolean(integ?.gitea_token_configured));
    const errors: FieldErrors = { ...result.errors };
    if (fieldErrors.gitea_allow_private_network && !draft.gitea_allow_private_network) {
      errors.gitea_allow_private_network = fieldErrors.gitea_allow_private_network;
    }
    const order: FieldKey[] = [
      "gitea_url",
      "gitea_token",
      "server_external_url",
      "gitea_allow_private_network",
    ];
    const firstInvalid = order.find((k) => errors[k]);
    setFieldErrors(errors);
    if (firstInvalid) focusField(firstInvalid);
    return !firstInvalid;
  }

  function runWebhookValidation(): boolean {
    const result = validateWebhookFields(draft, Boolean(integ?.gitea_webhook_secret_configured));
    setFieldErrors(result.errors);
    if (!result.ok && result.firstInvalid) focusField(result.firstInvalid);
    return result.ok;
  }

  function connectionBody() {
    const body: {
      gitea_url?: string;
      gitea_token?: string;
      gitea_allow_private_network?: boolean;
    } = {
      gitea_url: draft.gitea_url.trim() || undefined,
      gitea_allow_private_network: draft.gitea_allow_private_network,
    };
    if (draft.gitea_token) body.gitea_token = draft.gitea_token;
    return body;
  }

  async function saveIntegration() {
    const res = await api.updateSettings({ integration: toPatch(draft) });
    setDraft((prev) => ({
      ...draftFromIntegration(res.integration, prev.server_external_url),
      gitea_token: prev.gitea_token,
      gitea_webhook_secret: prev.gitea_webhook_secret,
      oauth_client_secret: prev.oauth_client_secret,
    }));
    await queryClient.invalidateQueries({ queryKey: ["settings"] });
    return res;
  }

  async function savePublicURL() {
    const current = settingsQuery.data?.settings;
    if (!current) {
      throw new Error("Settings are not loaded yet.");
    }
    const res = await api.updateSettings({
      ...current,
      server_external_url: draft.server_external_url.trim(),
    });
    await queryClient.invalidateQueries({ queryKey: ["settings"] });
    return res;
  }

  function applyWebhookResult(secret: string, integPublic: IntegrationPublic) {
    setDraft((prev) => ({
      ...draftFromIntegration(integPublic, prev.server_external_url),
      gitea_token: prev.gitea_token,
      gitea_webhook_secret: secret,
      oauth_client_secret: prev.oauth_client_secret,
    }));
  }

  async function advance(e: FormEvent) {
    e.preventDefault();
    if (isBusy) return;
    setError(null);
    try {
      if (step === "connect") {
        if (!runConnectValidation()) return;
        if (!checksPassed) {
          setError("Run Test Connection and pass all required checks before continuing.");
          return;
        }
        if (!testResult?.webhook_preview) {
          setError("Lens public URL is required to install the webhook.");
          return;
        }
        setWebhookPreview(testResult.webhook_preview);
        setCanCreateWebhook(Boolean(testResult.can_create_webhook));
        setWebhookError(null);
        setWebhookOpen(true);
        return;
      }
      setBusy("save");
      if (step === "webhooks") {
        if (!runWebhookValidation()) return;
        await saveIntegration();
        setStep("oauth");
      } else if (step === "oauth") {
        await saveIntegration();
        setStep("validate");
        setTestResult(null);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "save failed");
    } finally {
      setBusy(null);
    }
  }

  async function runTest() {
    if (isBusy) return;
    setError(null);
    if (!runConnectValidation()) return;

    setBusy("test");
    setTestResult(null);
    setCheckError(null);
    setCheckRows(null);
    try {
      if (step !== "connect") {
        await saveIntegration();
      } else {
        await savePublicURL();
      }
      // Run probe before opening the modal so private-network / URL errors stay on the form.
      const res = await api.testConnection(connectionBody());
      setCheckRows(res.checks ?? []);
      setTestResult(res);
      setCheckOpen(true);
      if (!res.ok) {
        setCheckError("One or more required checks failed.");
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : "connection test failed";
      if (isPrivateNetworkMessage(msg)) {
        setFieldErrors((prev) => ({ ...prev, gitea_url: msg }));
        focusField("gitea_url");
      } else {
        setError(msg);
      }
    } finally {
      setCheckRunning(false);
      setBusy(null);
    }
  }

  async function finishWebhook(create: boolean) {
    if (busy === "webhook") return;
    setBusy("webhook");
    setWebhookError(null);
    try {
      const res = await api.createWebhook({ ...connectionBody(), create });
      const secret = res.webhook?.config?.secret ?? "";
      applyWebhookResult(secret, res.integration);
      await queryClient.invalidateQueries({ queryKey: ["settings"] });
      setWebhookOpen(false);
      setCheckOpen(false);
      if (create) {
        // Secret is stored; skip manual webhooks entry.
        setStep("oauth");
      } else {
        setStep("webhooks");
      }
    } catch (err) {
      setWebhookError(err instanceof Error ? err.message : "webhook setup failed");
    } finally {
      setBusy(null);
    }
  }

  async function finish() {
    if (finishing) return;
    setFinishing(true);
    setError(null);
    try {
      await api.completeSetup();
      if (syncAfter) {
        try {
          await api.syncRepos();
        } catch (err) {
          setError(err instanceof Error ? `Setup complete, but sync failed: ${err.message}` : "Setup complete, but sync failed.");
          await queryClient.invalidateQueries({ queryKey: ["settings"] });
          onComplete();
          return;
        }
      }
      await queryClient.invalidateQueries();
      onComplete();
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to complete setup");
    } finally {
      setFinishing(false);
    }
  }

  function back() {
    setError(null);
    setFieldErrors({});
    const prev = STEPS[stepIndex - 1];
    if (prev) setStep(prev.id);
  }

  if (settingsQuery.isLoading && !hydrated) {
    return (
      <div className="setup-wizard">
        <div className="setup-wizard__dialog" role="dialog" aria-modal="true" aria-busy="true">
          <div className="loading">Loading setup…</div>
        </div>
      </div>
    );
  }

  if (settingsQuery.isError) {
    return (
      <div className="setup-wizard">
        <div className="setup-wizard__dialog" role="dialog" aria-modal="true" aria-label="Setup error">
          <div className="error">{(settingsQuery.error as Error).message}</div>
        </div>
      </div>
    );
  }

  return (
    <div className="setup-wizard">
      <div
        className="setup-wizard__dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="setup-wizard-title"
      >
      <header className="setup-wizard__header">
        <div className="setup-wizard__top">
          <p className="setup-wizard__brand">
            Gitea <span className="brand__mark">Lens</span>
          </p>
          <button className="setup-wizard__logout" type="button" onClick={onLogout}>
            Log Out ({login})
          </button>
        </div>
        <div className="setup-wizard__intro">
          <h1 id="setup-wizard-title">{currentStep.label}</h1>
          <p className="muted">{currentStep.description}</p>
        </div>
      </header>

      <ol className="setup-wizard__steps" aria-label="Setup steps">
        {STEPS.map((s, i) => {
          const done = i < stepIndex;
          const current = s.id === step;
          return (
            <li
              key={s.id}
              className={
                current
                  ? "setup-wizard__step setup-wizard__step--current"
                  : done
                    ? "setup-wizard__step setup-wizard__step--done"
                    : "setup-wizard__step"
              }
              aria-current={current ? "step" : undefined}
              aria-label={s.label}
            >
              {done ? (
                <span className="setup-wizard__step-check" aria-hidden="true">
                  ✓
                </span>
              ) : (
                <span className="setup-wizard__step-label">{s.label}</span>
              )}
            </li>
          );
        })}
      </ol>

      <div className="settings-form">
        {(step === "connect" || step === "webhooks" || step === "oauth") && (
          <form onSubmit={advance} noValidate>
            {step === "connect" && (
              <fieldset className="settings-form__section" disabled={isBusy}>
                <legend>Connect</legend>
                <div className="setup-wizard__connect-row">
                  <div className="setup-wizard__connect-col">
                    <div className="settings-form__field">
                      <input
                        id="wiz_gitea_url"
                        type="text"
                        inputMode="url"
                        value={draft.gitea_url}
                        onChange={(e) => setField("gitea_url", e.target.value)}
                        onBlur={() => void onGiteaURLBlur()}
                        placeholder="Gitea URL (https://git.example.com)"
                        aria-label="Gitea URL"
                        autoComplete="off"
                        required
                        aria-invalid={fieldErrors.gitea_url ? true : undefined}
                        aria-describedby={fieldErrors.gitea_url ? "wiz_gitea_url_error" : undefined}
                      />
                      {fieldErrors.gitea_url && (
                        <p id="wiz_gitea_url_error" className="settings-form__error" role="alert">
                          {fieldErrors.gitea_url}
                        </p>
                      )}
                    </div>
                    <div className="settings-form__field">
                      <label className="settings-form__check" htmlFor="wiz_allow_private_network">
                        <input
                          id="wiz_allow_private_network"
                          type="checkbox"
                          checked={draft.gitea_allow_private_network}
                          onChange={(e) => setField("gitea_allow_private_network", e.target.checked)}
                          aria-invalid={fieldErrors.gitea_allow_private_network ? true : undefined}
                          aria-describedby={
                            fieldErrors.gitea_allow_private_network
                              ? "wiz_allow_private_network_error"
                              : undefined
                          }
                        />
                        <span className="settings-form__check-text">
                          Allow Private Network Addresses
                          <InfoTip label="About private network addresses">
                            Lets Lens call Gitea on private or lab addresses (10.x, 192.168.x, localhost, and similar). Off
                            by default to block SSRF. Enable when Gitea is only reachable on a private network.
                          </InfoTip>
                        </span>
                      </label>
                      {fieldErrors.gitea_allow_private_network && (
                        <p id="wiz_allow_private_network_error" className="settings-form__error" role="alert">
                          {fieldErrors.gitea_allow_private_network}
                        </p>
                      )}
                    </div>
                  </div>
                  <div className="setup-wizard__connect-col">
                    <div className="settings-form__field">
                      <PasswordInput
                        id="wiz_gitea_token"
                        value={draft.gitea_token}
                        onChange={(value) => setField("gitea_token", value)}
                        placeholder={
                          integ?.gitea_token_configured
                            ? "Service token (leave blank to keep)"
                            : "Service token (required)"
                        }
                        aria-label="Service token"
                        autoComplete="new-password"
                        required={!integ?.gitea_token_configured}
                        aria-invalid={fieldErrors.gitea_token ? true : undefined}
                        aria-describedby={
                          fieldErrors.gitea_token ? "wiz_gitea_token_error" : "wiz_gitea_token_hint"
                        }
                      />
                      {fieldErrors.gitea_token ? (
                        <p id="wiz_gitea_token_error" className="settings-form__error" role="alert">
                          {fieldErrors.gitea_token}
                        </p>
                      ) : checksPassed ? (
                        <p id="wiz_gitea_token_hint" className="settings-form__saved" role="status">
                          ✓ Checks passed — Gitea {testResult?.version}
                          {testResult?.login ? ` as ${testResult.login}` : ""}
                        </p>
                      ) : (
                        <p id="wiz_gitea_token_hint" className="settings-form__hint">
                          {integ?.gitea_token_configured
                            ? "Token is configured. Leave blank to keep it."
                            : "Admin personal access token with repository and Actions read access."}
                        </p>
                      )}
                    </div>
                  </div>
                </div>
                <div className="settings-form__field">
                  <input
                    id="wiz_server_external_url"
                    type="text"
                    inputMode="url"
                    value={draft.server_external_url}
                    onChange={(e) => setField("server_external_url", e.target.value)}
                    placeholder="Lens public URL (https://lens.example.com)"
                    aria-label="Lens public URL"
                    autoComplete="off"
                    required
                    aria-invalid={fieldErrors.server_external_url ? true : undefined}
                    aria-describedby={
                      fieldErrors.server_external_url
                        ? "wiz_server_external_url_error"
                        : "wiz_server_external_url_hint"
                    }
                  />
                  {fieldErrors.server_external_url ? (
                    <p id="wiz_server_external_url_error" className="settings-form__error" role="alert">
                      {fieldErrors.server_external_url}
                    </p>
                  ) : (
                    <p id="wiz_server_external_url_hint" className="settings-form__hint">
                      Public URL where Gitea can reach Lens (webhooks and OAuth callback). Same value as{" "}
                      <code className="mono">server.external_url</code>.
                    </p>
                  )}
                </div>
              </fieldset>
            )}

            {step === "webhooks" && (
              <fieldset className="settings-form__section" disabled={isBusy}>
                <legend>Webhooks</legend>
                <div className="settings-form__field">
                  <input
                    id="wiz_webhook_secret"
                    type="password"
                    value={draft.gitea_webhook_secret}
                    onChange={(e) => setField("gitea_webhook_secret", e.target.value)}
                    placeholder={
                      integ?.gitea_webhook_secret_configured
                        ? "Webhook HMAC secret (leave blank to keep)"
                        : "Webhook HMAC secret"
                    }
                    aria-label="Webhook HMAC secret"
                    autoComplete="new-password"
                    aria-invalid={fieldErrors.gitea_webhook_secret ? true : undefined}
                    aria-describedby={
                      fieldErrors.gitea_webhook_secret
                        ? "wiz_webhook_secret_error"
                        : "wiz_webhook_secret_hint"
                    }
                  />
                  {fieldErrors.gitea_webhook_secret ? (
                    <p id="wiz_webhook_secret_error" className="settings-form__error" role="alert">
                      {fieldErrors.gitea_webhook_secret}
                    </p>
                  ) : (
                    <p id="wiz_webhook_secret_hint" className="settings-form__hint">
                      {integ?.gitea_webhook_secret_configured
                        ? "Secret is configured. Leave blank to keep it."
                        : "Must match the secret on the Gitea system webhook. Create the hook under Site Administration → Webhooks."}
                    </p>
                  )}
                </div>
                {webhookPreview && (
                  <div className="settings-form__field">
                    <span className="settings-form__label-like">Suggested hook</span>
                    <pre className="webhook-preview mono" tabIndex={0}>
                      {JSON.stringify(
                        {
                          ...webhookPreview,
                          config: {
                            ...webhookPreview.config,
                            secret: draft.gitea_webhook_secret || webhookPreview.config.secret,
                          },
                        },
                        null,
                        2,
                      )}
                    </pre>
                  </div>
                )}
                <label className="settings-form__check">
                  <input
                    type="checkbox"
                    checked={draft.gitea_allow_unsigned_webhooks}
                    onChange={(e) => setField("gitea_allow_unsigned_webhooks", e.target.checked)}
                  />
                  Allow Unsigned Webhooks (Not Recommended)
                </label>
              </fieldset>
            )}

            {step === "oauth" && (
              <fieldset className="settings-form__section" disabled={isBusy}>
                <legend>OAuth</legend>
                <div className="settings-form__field">
                  <input
                    id="wiz_oauth_client_id"
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
                    id="wiz_oauth_client_secret"
                    type="password"
                    value={draft.oauth_client_secret}
                    onChange={(e) => setField("oauth_client_secret", e.target.value)}
                    placeholder={
                      integ?.oauth_client_secret_configured
                        ? "OAuth client secret (leave blank to keep)"
                        : "OAuth client secret"
                    }
                    aria-label="OAuth client secret"
                    autoComplete="new-password"
                  />
                  <p className="settings-form__hint">
                    {integ?.oauth_client_secret_configured
                      ? "Secret is configured. Leave blank to keep it."
                      : "Create an OAuth application in Gitea and paste the credentials here."}
                  </p>
                </div>
                <div className="settings-form__field">
                  <span className="settings-form__label-like">Redirect URI</span>
                  <code className="mono setup-wizard__redirect">{redirectURI}</code>
                  <p className="settings-form__hint">Register this exact callback URL on the Gitea OAuth application.</p>
                </div>
              </fieldset>
            )}

            {error && (
              <p className="error" role="alert">
                {error}
              </p>
            )}

            <div className="settings-form__actions">
              {stepIndex > 0 && (
                <button className="btn" type="button" onClick={back} disabled={isBusy}>
                  Back
                </button>
              )}
              {step === "connect" && (
                <button className="btn" type="button" onClick={() => void runTest()} disabled={isBusy}>
                  {busy === "test" ? "Testing…" : "Test Connection"}
                </button>
              )}
              <button className="btn primary" type="submit" disabled={isBusy}>
                {busy === "save" ? "Saving…" : "Continue"}
              </button>
            </div>
          </form>
        )}

        {step === "validate" && (
          <div className="settings-form__section">
            <h2 className="settings-status__title">Validate Connection</h2>
            <p className="muted">
              Re-run permission checks against the saved URL and token, then continue to finish setup.
            </p>
            {checksPassed && (
              <p className="settings-form__saved" role="status">
                ✓ Checks passed — Gitea {testResult?.version}
              </p>
            )}
            {error && (
              <p className="error" role="alert">
                {error}
              </p>
            )}
            <div className="settings-form__actions">
              <button className="btn" type="button" onClick={back} disabled={isBusy}>
                Back
              </button>
              <button className="btn" type="button" onClick={() => void runTest()} disabled={isBusy}>
                {busy === "test" ? "Testing…" : "Test Connection"}
              </button>
              <button
                className="btn primary"
                type="button"
                onClick={() => {
                  setError(null);
                  setStep("finish");
                }}
                disabled={isBusy || !checksPassed}
              >
                Continue
              </button>
            </div>
          </div>
        )}

        {step === "finish" && (
          <div className="settings-form__section">
            <h2 className="settings-status__title">Finish Setup</h2>
            <p className="muted">Mark setup complete. You can change integration settings later under Settings.</p>
            <label className="settings-form__check">
              <input
                type="checkbox"
                checked={syncAfter}
                onChange={(e) => setSyncAfter(e.target.checked)}
                disabled={finishing}
              />
              Sync repositories after finishing
            </label>
            {error && (
              <p className="error" role="alert">
                {error}
              </p>
            )}
            <div className="settings-form__actions">
              <button className="btn" type="button" onClick={back} disabled={finishing}>
                Back
              </button>
              <button className="btn primary" type="button" onClick={() => void finish()} disabled={finishing}>
                {finishing ? "Finishing…" : "Complete Setup"}
              </button>
            </div>
          </div>
        )}
      </div>
      </div>

      <ConnectionCheckModal
        open={checkOpen}
        running={checkRunning}
        checks={checkRows}
        giteaURL={draft.gitea_url}
        error={checkError}
        canContinue={checksPassed && !checkRunning}
        onClose={() => setCheckOpen(false)}
        onContinue={() => {
          setCheckOpen(false);
          if (step === "connect" && checksPassed) {
            setWebhookPreview(testResult?.webhook_preview ?? null);
            setCanCreateWebhook(Boolean(testResult?.can_create_webhook));
            setWebhookError(null);
            setWebhookOpen(true);
          }
        }}
      />

      <WebhookConfirmModal
        open={webhookOpen}
        preview={webhookPreview}
        busy={busy === "webhook"}
        error={webhookError}
        canCreate={canCreateWebhook}
        onCancel={() => setWebhookOpen(false)}
        onManual={() => void finishWebhook(false)}
        onCreate={() => void finishWebhook(true)}
      />
    </div>
  );
}
