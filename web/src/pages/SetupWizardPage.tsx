import { FormEvent, useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  api,
  type ForgeType,
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
import { DEFAULT_GITHUB_URL, PRODUCT_NAME } from "../lib/product";

type WizardForge = ForgeType;

type FieldKey =
  | "gitea_url"
  | "gitea_token"
  | "gitea_allow_private_network"
  | "github_url"
  | "github_token"
  | "github_allow_private_network"
  | "server_external_url";
type FieldErrors = Partial<Record<FieldKey, string>>;

const FIELD_INPUT_IDS: Record<FieldKey, string> = {
  gitea_url: "wiz_gitea_url",
  gitea_token: "wiz_gitea_token",
  gitea_allow_private_network: "wiz_allow_private_network",
  github_url: "wiz_github_url",
  github_token: "wiz_github_token",
  github_allow_private_network: "wiz_github_allow_private_network",
  server_external_url: "wiz_server_external_url",
};

function isValidHTTPURL(raw: string): boolean {
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

type Step = "pick" | "connect" | "validate" | "finish";

const STEPS: { id: Step; label: string; description: string }[] = [
  {
    id: "pick",
    label: "Choose Forge",
    description: "Pick the forge to connect first. You can add another later under Settings → Integration.",
  },
  {
    id: "connect",
    label: "Connect",
    description: "Point GitSeer at your forge and set the public URL used for webhooks (and Gitea OAuth).",
  },
  {
    id: "validate",
    label: "Validate",
    description: "GitSeer checks connectivity and token permissions, then walks you through webhook setup.",
  },
  {
    id: "finish",
    label: "Finish",
    description: "Complete setup and optionally sync repositories so GitSeer can start indexing forge data.",
  },
];

type ForgePickerOption = {
  id: WizardForge | "gitlab" | "bitbucket";
  label: string;
  description: string;
  comingSoon?: boolean;
};

const FORGE_OPTIONS: ForgePickerOption[] = [
  {
    id: "gitea",
    label: "Gitea",
    description: "Self-hosted or cloud Gitea. Sync, system webhooks, and OAuth login.",
  },
  {
    id: "github",
    label: "GitHub",
    description: "github.com or GitHub Enterprise. Service PAT sync and manual webhooks.",
  },
  {
    id: "gitlab",
    label: "GitLab",
    description: "GitLab.com and self-hosted.",
    comingSoon: true,
  },
  {
    id: "bitbucket",
    label: "Bitbucket",
    description: "Bitbucket Cloud and Data Center.",
    comingSoon: true,
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
  github_url: string;
  github_token: string;
  github_allow_private_network: boolean;
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
    github_url: DEFAULT_GITHUB_URL,
    github_token: "",
    github_allow_private_network: false,
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
    github_url: integ.github_url || DEFAULT_GITHUB_URL,
    github_token: "",
    github_allow_private_network: integ.github_allow_private_network,
    server_external_url: serverExternalURL,
  };
}

function validateConnectFields(
  forge: WizardForge,
  draft: Draft,
  integ: IntegrationPublic | undefined,
): { ok: boolean; errors: FieldErrors; firstInvalid?: FieldKey } {
  const errors: FieldErrors = {};
  if (forge === "gitea") {
    const url = draft.gitea_url.trim();
    if (!url) {
      errors.gitea_url = "Gitea URL is required.";
    } else if (!isValidHTTPURL(url)) {
      errors.gitea_url = "Enter a valid http:// or https:// URL.";
    }
    if (!draft.gitea_token.trim() && !integ?.gitea_token_configured) {
      errors.gitea_token = "Service token is required.";
    }
  } else {
    const url = draft.github_url.trim();
    if (!url) {
      errors.github_url = "GitHub URL is required.";
    } else if (!isValidHTTPURL(url)) {
      errors.github_url = "Enter a valid http:// or https:// URL.";
    }
    if (!draft.github_token.trim() && !integ?.github_token_configured) {
      errors.github_token = "Personal access token is required.";
    }
  }
  const publicURL = draft.server_external_url.trim();
  if (!publicURL) {
    errors.server_external_url = "Lens public URL is required.";
  } else if (!isValidHTTPURL(publicURL)) {
    errors.server_external_url = "Enter a valid http:// or https:// URL.";
  }
  const order: FieldKey[] =
    forge === "gitea"
      ? ["gitea_url", "gitea_token", "server_external_url", "gitea_allow_private_network"]
      : ["github_url", "github_token", "server_external_url", "github_allow_private_network"];
  const firstInvalid = order.find((k) => errors[k]);
  return { ok: !firstInvalid, errors, firstInvalid };
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
  const [step, setStep] = useState<Step>("pick");
  const [forge, setForge] = useState<WizardForge | null>(null);
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
  const forgeLabel = forge === "github" ? "GitHub" : "Gitea";

  function focusField(key: FieldKey) {
    const el = document.getElementById(FIELD_INPUT_IDS[key]);
    if (el instanceof HTMLElement) el.focus();
  }

  function setField<K extends keyof Draft>(key: K, value: Draft[K]) {
    setDraft((prev) => ({ ...prev, [key]: value }));
    setError(null);
    setTestResult(null);
    if (
      key === "gitea_url" ||
      key === "gitea_token" ||
      key === "github_url" ||
      key === "github_token" ||
      key === "server_external_url"
    ) {
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
        void onForgeURLBlur("gitea", true);
      }
    }
    if (key === "github_allow_private_network") {
      setFieldErrors((prev) => {
        if (!prev.github_allow_private_network && !prev.github_url) return prev;
        const next = { ...prev };
        delete next.github_allow_private_network;
        if (value === true) delete next.github_url;
        return next;
      });
      if (value === true && draft.github_url.trim()) {
        void onForgeURLBlur("github", true);
      }
    }
  }

  function selectForge(next: WizardForge) {
    setForge(next);
    setError(null);
    setFieldErrors({});
    setTestResult(null);
    setStep("connect");
  }

  async function onForgeURLBlur(which: WizardForge, allowPrivateOverride?: boolean) {
    if (isBusy) return;
    if (which === "gitea") {
      const raw = draft.gitea_url.trim();
      if (!raw) return;
      const allowPrivate = allowPrivateOverride ?? draft.gitea_allow_private_network;
      setFieldErrors((prev) => {
        const next = { ...prev };
        delete next.gitea_url;
        delete next.gitea_allow_private_network;
        return next;
      });
      try {
        const res = await api.checkGiteaURL({
          forge_type: "gitea",
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
      return;
    }

    const raw = draft.github_url.trim();
    if (!raw) return;
    const allowPrivate = allowPrivateOverride ?? draft.github_allow_private_network;
    setFieldErrors((prev) => {
      const next = { ...prev };
      delete next.github_url;
      delete next.github_allow_private_network;
      return next;
    });
    try {
      const res = await api.checkGiteaURL({
        forge_type: "github",
        github_url: raw,
        github_allow_private_network: allowPrivate,
      });
      if (res.github_url && res.github_url !== draft.github_url.trim()) {
        setDraft((prev) => ({ ...prev, github_url: res.github_url! }));
        setTestResult(null);
      }
      if (res.ok) return;
      if (res.code === "private_network" || isPrivateNetworkMessage(res.error || res.detail || "")) {
        setFieldErrors((prev) => ({
          ...prev,
          github_allow_private_network:
            res.error ||
            res.detail ||
            'This is a private address. To continue, check "Allow Private Network Addresses".',
        }));
        focusField("github_allow_private_network");
        return;
      }
      setFieldErrors((prev) => ({
        ...prev,
        github_url: res.error || res.detail || "Could not validate GitHub URL.",
      }));
      focusField("github_url");
    } catch (err) {
      setFieldErrors((prev) => ({
        ...prev,
        github_url: err instanceof Error ? err.message : "Could not validate GitHub URL.",
      }));
    }
  }

  function runConnectValidation(): boolean {
    if (!forge) return false;
    const result = validateConnectFields(forge, draft, integ);
    const errors: FieldErrors = { ...result.errors };
    if (forge === "gitea" && fieldErrors.gitea_allow_private_network && !draft.gitea_allow_private_network) {
      errors.gitea_allow_private_network = fieldErrors.gitea_allow_private_network;
    }
    if (
      forge === "github" &&
      fieldErrors.github_allow_private_network &&
      !draft.github_allow_private_network
    ) {
      errors.github_allow_private_network = fieldErrors.github_allow_private_network;
    }
    setFieldErrors(errors);
    if (result.firstInvalid) focusField(result.firstInvalid);
    return !Object.keys(errors).length;
  }

  function connectionBody() {
    if (forge === "github") {
      const body: {
        forge_type: ForgeType;
        github_url?: string;
        github_token?: string;
        github_allow_private_network?: boolean;
      } = {
        forge_type: "github",
        github_url: draft.github_url.trim() || undefined,
        github_allow_private_network: draft.github_allow_private_network,
      };
      if (draft.github_token) body.github_token = draft.github_token;
      return body;
    }
    const body: {
      forge_type: ForgeType;
      gitea_url?: string;
      gitea_token?: string;
      gitea_allow_private_network?: boolean;
    } = {
      forge_type: "gitea",
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
      gitea_webhook_secret: forge === "gitea" ? secret : prev.gitea_webhook_secret,
      github_token: prev.github_token,
      oauth_client_secret: prev.oauth_client_secret,
    }));
  }

  async function advance(e: FormEvent) {
    e.preventDefault();
    if (isBusy || step !== "connect") return;
    setError(null);
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
    if (forge === "github") {
      // GitHub webhooks are always manual; persist credentials + generated secret first.
      void finishWebhook(false);
      return;
    }
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
        const urlKey = forge === "github" ? "github_url" : "gitea_url";
        setFieldErrors((prev) => ({ ...prev, [urlKey]: msg }));
        setStep("connect");
        focusField(urlKey);
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
        name: PRODUCT_NAME,
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
      if (forge === "github" || res.manual) {
        setCanCreateWebhook(false);
        setWebhookPhase("manual");
        setWebhookOpen(true);
      } else if (create) {
        setWebhookOpen(false);
        setWebhookPhase("confirm");
        openOAuthModal();
      } else {
        setWebhookPhase("manual");
        setWebhookOpen(true);
      }
    } catch (err) {
      setWebhookError(err instanceof Error ? err.message : "webhook setup failed");
      if (forge === "github") {
        setWebhookOpen(true);
        setWebhookPhase("manual");
      }
    } finally {
      setBusy(null);
    }
  }

  function continueAfterWebhook() {
    setWebhookOpen(false);
    setWebhookPhase("confirm");
    setCheckOpen(false);
    if (forge === "github") {
      setStep("finish");
      return;
    }
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
      github_token: prev.github_token,
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
    if (step === "connect") {
      setForge(null);
      setStep("pick");
      return;
    }
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

  const forgeURL = forge === "github" ? draft.github_url : draft.gitea_url;

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
          {step === "pick" && (
            <div className="settings-form__section">
              <div className="forge-picker" role="list">
                {FORGE_OPTIONS.map((opt) => {
                  const disabled = Boolean(opt.comingSoon);
                  return (
                    <button
                      key={opt.id}
                      type="button"
                      role="listitem"
                      className={`forge-picker__card${disabled ? " is-disabled" : ""}`}
                      disabled={disabled}
                      onClick={() => {
                        if (opt.id === "gitea" || opt.id === "github") selectForge(opt.id);
                      }}
                    >
                      <span className="forge-picker__title">{opt.label}</span>
                      {opt.comingSoon ? (
                        <span className="forge-picker__badge">Coming Soon</span>
                      ) : null}
                      <span className="forge-picker__desc muted">{opt.description}</span>
                    </button>
                  );
                })}
              </div>
            </div>
          )}

          {step === "connect" && forge === "gitea" && (
            <form onSubmit={advance} noValidate>
              <fieldset className="settings-form__section" disabled={isBusy}>
                <legend>Connect Gitea</legend>
                <div className="setup-wizard__connect-row">
                  <div className="setup-wizard__connect-col">
                    <div className="settings-form__field">
                      <input
                        id="wiz_gitea_url"
                        type="text"
                        inputMode="url"
                        value={draft.gitea_url}
                        onChange={(e) => setField("gitea_url", e.target.value)}
                        onBlur={() => void onForgeURLBlur("gitea")}
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
                            Lets GitSeer call Gitea on private or lab addresses (10.x, 192.168.x, localhost, and
                            similar). Off by default to block SSRF. Enable when Gitea is only reachable on a private
                            network.
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
                      Public URL where Gitea can reach GitSeer (webhooks and OAuth callback).
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
                <button className="btn settings-form__actions-back" type="button" onClick={back} disabled={isBusy}>
                  Back
                </button>
                <button className="btn primary" type="submit" disabled={isBusy}>
                  {busy === "save" ? "Saving…" : "Continue"}
                </button>
              </div>
            </form>
          )}

          {step === "connect" && forge === "github" && (
            <form onSubmit={advance} noValidate>
              <fieldset className="settings-form__section" disabled={isBusy}>
                <legend>Connect GitHub</legend>
                <div className="setup-wizard__connect-row">
                  <div className="setup-wizard__connect-col">
                    <div className="settings-form__field">
                      <input
                        id="wiz_github_url"
                        type="text"
                        inputMode="url"
                        value={draft.github_url}
                        onChange={(e) => setField("github_url", e.target.value)}
                        onBlur={() => void onForgeURLBlur("github")}
                        placeholder="GitHub URL (https://github.com)"
                        aria-label="GitHub URL"
                        autoComplete="off"
                        required
                        aria-invalid={fieldErrors.github_url ? true : undefined}
                        aria-describedby={
                          fieldErrors.github_url ? "wiz_github_url_error" : "wiz_github_url_hint"
                        }
                      />
                      {fieldErrors.github_url ? (
                        <p id="wiz_github_url_error" className="settings-form__error" role="alert">
                          {fieldErrors.github_url}
                        </p>
                      ) : (
                        <p id="wiz_github_url_hint" className="settings-form__hint">
                          Use https://github.com or your GitHub Enterprise host. API roots are normalized automatically.
                        </p>
                      )}
                    </div>
                    <div className="settings-form__field">
                      <label
                        className="settings-form__check"
                        htmlFor="wiz_github_allow_private_network"
                      >
                        <input
                          id="wiz_github_allow_private_network"
                          type="checkbox"
                          checked={draft.github_allow_private_network}
                          onChange={(e) => setField("github_allow_private_network", e.target.checked)}
                          aria-invalid={fieldErrors.github_allow_private_network ? true : undefined}
                          aria-describedby={
                            fieldErrors.github_allow_private_network
                              ? "wiz_github_allow_private_network_error"
                              : undefined
                          }
                        />
                        <span className="settings-form__check-text">
                          Allow Private Network Addresses
                          <InfoTip label="About private network addresses">
                            Lets GitSeer call GitHub Enterprise on private or lab addresses. Off by default to block
                            SSRF.
                          </InfoTip>
                        </span>
                      </label>
                      {fieldErrors.github_allow_private_network && (
                        <p
                          id="wiz_github_allow_private_network_error"
                          className="settings-form__error"
                          role="alert"
                        >
                          {fieldErrors.github_allow_private_network}
                        </p>
                      )}
                    </div>
                  </div>
                  <div className="setup-wizard__connect-col">
                    <div className="settings-form__field">
                      <PasswordInput
                        id="wiz_github_token"
                        value={draft.github_token}
                        onChange={(value) => setField("github_token", value)}
                        placeholder={
                          integ?.github_token_configured
                            ? "Personal access token (leave blank to keep)"
                            : "Personal access token (required)"
                        }
                        aria-label="Personal access token"
                        autoComplete="new-password"
                        required={!integ?.github_token_configured}
                        aria-invalid={fieldErrors.github_token ? true : undefined}
                        aria-describedby={
                          fieldErrors.github_token ? "wiz_github_token_error" : "wiz_github_token_hint"
                        }
                      />
                      {fieldErrors.github_token ? (
                        <p id="wiz_github_token_error" className="settings-form__error" role="alert">
                          {fieldErrors.github_token}
                        </p>
                      ) : (
                        <p id="wiz_github_token_hint" className="settings-form__hint">
                          {integ?.github_token_configured
                            ? "Token is configured. Leave blank to keep it."
                            : "Fine-grained or classic PAT with repo and Actions read access. No GitHub OAuth login in this release."}
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
                        : "wiz_server_external_url_hint_gh"
                    }
                  />
                  {fieldErrors.server_external_url ? (
                    <p id="wiz_server_external_url_error" className="settings-form__error" role="alert">
                      {fieldErrors.server_external_url}
                    </p>
                  ) : (
                    <p id="wiz_server_external_url_hint_gh" className="settings-form__hint">
                      Public URL where GitHub can reach GitSeer for webhooks (
                      <code className="mono">/api/webhooks/github/&#123;instanceID&#125;</code>).
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
                <button className="btn settings-form__actions-back" type="button" onClick={back} disabled={isBusy}>
                  Back
                </button>
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
                    ? forge === "github"
                      ? "Required checks passed. Continue for manual webhook instructions."
                      : "Required checks passed. Continue to install the webhook and set up OAuth."
                    : "Fix any failed checks, then retry."}
              </p>
              {checksPassed && (
                <p className="settings-form__saved" role="status">
                  ✓ Checks passed — {forgeLabel} {testResult?.version}
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
                    {busy === "webhook" ? "Preparing…" : "Continue"}
                  </button>
                )}
              </div>
            </div>
          )}

          {step === "finish" && (
            <div className="settings-form__section">
              <h2 className="settings-status__title">Finish Setup</h2>
              <p className="muted">
                Mark setup complete. You can add another forge later under Settings → Integration.
              </p>
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
        giteaURL={forgeURL}
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
        forgeLabel={forgeLabel}
        manualOnly={forge === "github"}
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
