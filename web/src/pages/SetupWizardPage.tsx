import { FormEvent, useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  api,
  type IntegrationPublic,
  type OAuthAppPreview,
  type ProbeCheck,
  type TestConnectionResponse,
  type WebhookPreview,
} from "../api/client";
import { Brand } from "../components/Brand";
import { InfoTip } from "../components/InfoTip";
import { PasswordInput } from "../components/PasswordInput";
import {
  ConnectionCheckModal,
  OAuthConfirmModal,
  WebhookConfirmModal,
  type OAuthModalPhase,
  type WebhookModalPhase,
} from "../components/SetupConnectionModals";

type FieldKey =
  | "gitea_url"
  | "gitea_token"
  | "gitea_allow_private_network"
  | "server_external_url";
type FieldErrors = Partial<Record<FieldKey, string>>;

const FIELD_INPUT_IDS: Record<FieldKey, string> = {
  gitea_url: "wiz_gitea_url",
  gitea_token: "wiz_gitea_token",
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

type Step = "connect" | "validate" | "finish";

const STEPS: { id: Step; label: string; description: string }[] = [
  {
    id: "connect",
    label: "Connect",
    description:
      "Point Lens at your Gitea instance and set the public Lens URL Gitea will use for webhooks and OAuth callbacks.",
  },
  {
    id: "validate",
    label: "Validate",
    description:
      "Lens checks connectivity and token permissions, then walks you through the system webhook and OAuth app.",
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

type Props = {
  onComplete: () => void;
  onLogout: () => void;
};

function WizardTopBar({
  allowSkip,
  skipping,
  onSkip,
  onLogout,
}: {
  allowSkip: boolean;
  skipping: boolean;
  onSkip: () => void;
  onLogout: () => void;
}) {
  return (
    <div className="setup-wizard__top">
      <Brand variant="logo" className="setup-wizard__logo" />
      <div className="setup-wizard__top-actions">
        {allowSkip && (
          <button
            className="setup-wizard__logout"
            type="button"
            onClick={onSkip}
            disabled={skipping}
          >
            {skipping ? "Skipping…" : "Skip Setup"}
          </button>
        )}
        <button className="setup-wizard__logout" type="button" onClick={onLogout} disabled={skipping}>
          Log Out
        </button>
      </div>
    </div>
  );
}

export function SetupWizardPage({ onComplete, onLogout }: Props) {
  const queryClient = useQueryClient();
  const settingsQuery = useQuery({ queryKey: ["settings"], queryFn: api.settings });
  const uiConfigQuery = useQuery({ queryKey: ["ui-config"], queryFn: api.uiConfig });
  const allowSkipSetup = uiConfigQuery.data?.allow_skip_setup === true;
  const [step, setStep] = useState<Step>("connect");
  const [draft, setDraft] = useState<Draft>(emptyDraft());
  const [hydrated, setHydrated] = useState(false);
  const [busy, setBusy] = useState<"save" | "test" | "webhook" | "oauth" | null>(null);
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
  const [webhookPhase, setWebhookPhase] = useState<WebhookModalPhase>("confirm");
  const [webhookPreview, setWebhookPreview] = useState<WebhookPreview | null>(null);
  const [webhookError, setWebhookError] = useState<string | null>(null);
  const [canCreateWebhook, setCanCreateWebhook] = useState(false);
  const [oauthOpen, setOauthOpen] = useState(false);
  const [oauthPhase, setOauthPhase] = useState<OAuthModalPhase>("confirm");
  const [oauthPreview, setOauthPreview] = useState<OAuthAppPreview | null>(null);
  const [oauthError, setOauthError] = useState<string | null>(null);
  const [canCreateOAuth, setCanCreateOAuth] = useState(false);
  const [oauthClientId, setOauthClientId] = useState("");
  const [oauthClientSecret, setOauthClientSecret] = useState("");
  const validateProbeStarted = useRef(false);

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
    if (key === "gitea_url" || key === "gitea_token" || key === "server_external_url") {
      const field = key as FieldKey;
      setFieldErrors((prev) => {
        if (!prev[field]) return prev;
        const next = { ...prev };
        delete next[field];
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
    if (step !== "connect") return;
    if (!runConnectValidation()) return;
    setBusy("save");
    try {
      await savePublicURL();
      setTestResult(null);
      setCheckRows(null);
      setCheckError(null);
      validateProbeStarted.current = false;
      setStep("validate");
    } catch (err) {
      setError(err instanceof Error ? err.message : "save failed");
    } finally {
      setBusy(null);
    }
  }

  function openWebhookFlow() {
    if (!testResult?.ok || !testResult.webhook_preview) return;
    setCheckOpen(false);
    setWebhookPreview(testResult.webhook_preview);
    setCanCreateWebhook(Boolean(testResult.can_create_webhook));
    setWebhookError(null);
    setWebhookPhase("confirm");
    setWebhookOpen(true);
  }

  async function runTest() {
    if (isBusy) return;
    setError(null);
    if (!runConnectValidation()) {
      setStep("connect");
      return;
    }

    setBusy("test");
    setCheckRunning(true);
    setCheckOpen(true);
    setTestResult(null);
    setCheckError(null);
    setCheckRows(null);
    try {
      const res = await api.testConnection(connectionBody());
      setCheckRows(res.checks ?? []);
      setTestResult(res);
      if (!res.ok) {
        setCheckError("One or more required checks failed.");
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : "connection test failed";
      setCheckOpen(false);
      if (isPrivateNetworkMessage(msg)) {
        setFieldErrors((prev) => ({ ...prev, gitea_url: msg }));
        setStep("connect");
        focusField("gitea_url");
      } else {
        setError(msg);
      }
    } finally {
      setCheckRunning(false);
      setBusy(null);
    }
  }

  useEffect(() => {
    if (step !== "validate") {
      validateProbeStarted.current = false;
      return;
    }
    if (validateProbeStarted.current || checksPassed) return;
    validateProbeStarted.current = true;
    void runTest();
    // Probe once per visit to validate; runTest reads latest draft/state.
    // eslint-disable-next-line react-hooks/exhaustive-deps -- once per validate entry
  }, [step]);

  function openOAuthModal() {
    setOauthPreview(
      testResult?.oauth_app_preview ?? {
        name: "Gitea Lens",
        redirect_uri: String(settingsQuery.data?.status?.oauth_redirect_uri ?? ""),
        confidential_client: true,
        gitea_settings_path: "/user/settings/applications",
        gitea_admin_apps_path: "/admin/applications",
      },
    );
    setCanCreateOAuth(Boolean(testResult?.can_create_oauth));
    setOauthError(null);
    setOauthPhase("confirm");
    setOauthClientId("");
    setOauthClientSecret("");
    setOauthOpen(true);
  }

  async function finishWebhook(create: boolean) {
    if (busy === "webhook") return;
    setBusy("webhook");
    setWebhookError(null);
    try {
      const res = await api.createWebhook({ ...connectionBody(), create });
      const secret = res.webhook?.config?.secret ?? "";
      applyWebhookResult(secret, res.integration);
      if (res.webhook) setWebhookPreview(res.webhook);
      await queryClient.invalidateQueries({ queryKey: ["settings"] });
      setCheckOpen(false);
      if (create) {
        setWebhookOpen(false);
        setWebhookPhase("confirm");
        openOAuthModal();
      } else {
        // Secret stored; show copy payload so the admin can add the hook in Gitea.
        setWebhookPhase("manual");
      }
    } catch (err) {
      setWebhookError(err instanceof Error ? err.message : "webhook setup failed");
    } finally {
      setBusy(null);
    }
  }

  function continueAfterWebhook() {
    setWebhookOpen(false);
    setWebhookPhase("confirm");
    setCheckOpen(false);
    openOAuthModal();
  }

  function backFromWebhook() {
    setWebhookOpen(false);
    setWebhookPhase("confirm");
    setWebhookError(null);
    setStep("validate");
  }

  function applyOAuthResult(clientId: string, integPublic: IntegrationPublic) {
    setDraft((prev) => ({
      ...draftFromIntegration(integPublic, prev.server_external_url),
      gitea_token: prev.gitea_token,
      gitea_webhook_secret: prev.gitea_webhook_secret,
      oauth_client_id: clientId,
      oauth_client_secret: "",
    }));
  }

  async function finishOAuth(create: boolean) {
    if (busy === "oauth") return;
    setBusy("oauth");
    setOauthError(null);
    try {
      const res = await api.createOAuth({
        ...connectionBody(),
        create,
        oauth_client_id: create ? undefined : oauthClientId.trim(),
        oauth_client_secret: create ? undefined : oauthClientSecret,
      });
      applyOAuthResult(res.client_id, res.integration);
      await queryClient.invalidateQueries({ queryKey: ["settings"] });
      setOauthOpen(false);
      setOauthPhase("confirm");
      setStep("finish");
    } catch (err) {
      setOauthError(err instanceof Error ? err.message : "oauth setup failed");
    } finally {
      setBusy(null);
    }
  }

  function skipOAuth() {
    setOauthOpen(false);
    setOauthPhase("confirm");
    setOauthError(null);
    setStep("finish");
  }

  function backFromOAuth() {
    setOauthOpen(false);
    setOauthPhase("confirm");
    setOauthError(null);
    setWebhookPhase("confirm");
    setWebhookOpen(true);
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
          setError(
            err instanceof Error
              ? `Setup complete, but sync failed: ${err.message}`
              : "Setup complete, but sync failed.",
          );
          await queryClient.invalidateQueries({ queryKey: ["settings"] });
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

  async function skipSetup() {
    if (!allowSkipSetup || finishing) return;
    setFinishing(true);
    setError(null);
    try {
      await api.completeSetup();
      await queryClient.invalidateQueries({ queryKey: ["settings"] });
      onComplete();
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to skip setup");
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
        <WizardTopBar
          allowSkip={allowSkipSetup}
          skipping={finishing}
          onSkip={() => void skipSetup()}
          onLogout={onLogout}
        />
        <div className="setup-wizard__dialog" role="dialog" aria-modal="true" aria-busy="true">
          <div className="loading">Loading setup…</div>
        </div>
      </div>
    );
  }

  if (settingsQuery.isError) {
    return (
      <div className="setup-wizard">
        <WizardTopBar
          allowSkip={allowSkipSetup}
          skipping={finishing}
          onSkip={() => void skipSetup()}
          onLogout={onLogout}
        />
        <div className="setup-wizard__dialog" role="dialog" aria-modal="true" aria-label="Setup error">
          <div className="error">{(settingsQuery.error as Error).message}</div>
        </div>
      </div>
    );
  }

  return (
    <div className="setup-wizard">
      <WizardTopBar
        allowSkip={allowSkipSetup}
        skipping={finishing}
        onSkip={() => void skipSetup()}
        onLogout={onLogout}
      />
      <div
        className="setup-wizard__dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="setup-wizard-title"
      >
      <header className="setup-wizard__header">
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
        {step === "connect" && (
          <form onSubmit={advance} noValidate>
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
                      Public URL where Gitea can reach Lens (webhooks and OAuth callback).
                    </p>
                  )}
                </div>
            </fieldset>

            {error && (
              <p className="error" role="alert">
                {error}
              </p>
            )}

            <div className="settings-form__actions">
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
              {checkRunning || busy === "test"
                ? "Running connectivity and permission checks…"
                : checksPassed
                  ? "Required checks passed. Continue to install the webhook and set up OAuth."
                  : "Fix any failed checks, then retry."}
            </p>
            {checksPassed && (
              <p className="settings-form__saved" role="status">
                ✓ Checks passed — Gitea {testResult?.version}
                {testResult?.login ? ` as ${testResult.login}` : ""}
              </p>
            )}
            {error && (
              <p className="error" role="alert">
                {error}
              </p>
            )}
            <div className="settings-form__actions">
              <button className="btn settings-form__actions-back" type="button" onClick={back} disabled={isBusy}>
                Back
              </button>
              {!checksPassed ? (
                <button
                  className="btn primary"
                  type="button"
                  onClick={() => {
                    validateProbeStarted.current = true;
                    void runTest();
                  }}
                  disabled={isBusy}
                >
                  {busy === "test" ? "Checking…" : "Retry Checks"}
                </button>
              ) : (
                <button
                  className="btn primary"
                  type="button"
                  onClick={openWebhookFlow}
                  disabled={isBusy || !testResult?.webhook_preview}
                >
                  Continue
                </button>
              )}
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
              <button
                className="btn settings-form__actions-back"
                type="button"
                onClick={back}
                disabled={finishing}
              >
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
        closeLabel={checksPassed ? undefined : "Close"}
        onClose={() => setCheckOpen(false)}
        onContinue={openWebhookFlow}
      />

      <WebhookConfirmModal
        open={webhookOpen}
        phase={webhookPhase}
        preview={webhookPreview}
        busy={busy === "webhook"}
        error={webhookError}
        canCreate={canCreateWebhook}
        onBack={backFromWebhook}
        onManual={() => void finishWebhook(false)}
        onCreate={() => void finishWebhook(true)}
        onContinue={continueAfterWebhook}
      />

      <OAuthConfirmModal
        open={oauthOpen}
        phase={oauthPhase}
        preview={oauthPreview}
        giteaURL={draft.gitea_url}
        busy={busy === "oauth"}
        error={oauthError}
        clientId={oauthClientId}
        clientSecret={oauthClientSecret}
        onClientIdChange={setOauthClientId}
        onClientSecretChange={setOauthClientSecret}
        onBack={backFromOAuth}
        onManual={() => {
          setOauthError(null);
          setOauthPhase("manual");
        }}
        onCreate={() => void finishOAuth(true)}
        onSkip={skipOAuth}
        onContinue={() => void finishOAuth(false)}
        canCreate={canCreateOAuth}
      />
    </div>
  );
}
