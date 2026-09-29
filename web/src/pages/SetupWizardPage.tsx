import { FormEvent, useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  api,
  forgeTokenLabel,
  type ForgeType,
  type IntegrationPublic,
  type ProbeCheck,
  type TestConnectionResponse,
  type WebhookPreview,
} from "../api/client";
import { Brand } from "../components/Brand";
import { GiteaPATHelp } from "../components/GiteaPATHelp";
import { GitHubPATHelp } from "../components/GitHubPATHelp";
import { InfoTip } from "../components/InfoTip";
import { OAuthSignInPanel } from "../components/OAuthSignInPanel";
import { PasswordInput } from "../components/PasswordInput";
import {
  ConnectionCheckModal,
  WebhookConfirmModal,
  type WebhookModalPhase,
} from "../components/SetupConnectionModals";
import { DEFAULT_GITHUB_URL } from "../lib/product";

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

type Step = "secure" | "pick" | "connect" | "validate" | "signin" | "finish";

const STEPS: { id: Step; label: string; description: string }[] = [
  {
    id: "secure",
    label: "Prepare",
    description:
      "Set GitSeer's public URL so forges can reach it, and an encryption key for tokens and webhook secrets at rest.",
  },
  {
    id: "pick",
    label: "Choose Forge",
    description: "Pick the forge to connect first. You can add another later under Settings → Integration.",
  },
  {
    id: "connect",
    label: "Connect",
    description:
      "Enter the forge base URL and access credentials so GitSeer can read repos, PRs, and Actions.",
  },
  {
    id: "validate",
    label: "Validate",
    description: "Confirm connectivity and token permissions, then install the webhook.",
  },
  {
    id: "signin",
    label: "Sign In",
    description:
      "Optionally configure forge OAuth so users can sign in to GitSeer. Skip to use bootstrap admin only.",
  },
  {
    id: "finish",
    label: "Finish",
    description: "Mark setup complete. Optionally sync repositories so the catalog starts filling in.",
  },
];

type ForgePickerOption = {
  id: WizardForge;
  label: string;
  description: string;
};

const FORGE_OPTIONS: ForgePickerOption[] = [
  {
    id: "gitea",
    label: "Gitea",
    description: "Self-hosted or cloud. Sync, system webhooks, and OAuth login.",
  },
  {
    id: "github",
    label: "GitHub",
    description: "github.com or Enterprise. PAT sync and manual webhooks.",
  },
  {
    id: "gitlab",
    label: "GitLab",
    description: "GitLab.com and self-hosted. PAT sync and webhooks.",
  },
  {
    id: "bitbucket",
    label: "Bitbucket",
    description: "Bitbucket Cloud workspaces. HTTP access token sync and webhooks.",
  },
  {
    id: "forgejo",
    label: "Forgejo",
    description: "Gitea-compatible forge. Sync, webhooks, and OAuth login.",
  },
];

type Draft = {
  gitea_url: string;
  gitea_token: string;
  gitea_allow_private_network: boolean;
  gitea_webhook_secret: string;
  gitea_allow_unsigned_webhooks: boolean;
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
  const usesGiteaFields = forge === "gitea" || forge === "forgejo";
  if (usesGiteaFields) {
    const url = draft.gitea_url.trim();
    if (!url) {
      errors.gitea_url = `${forge === "forgejo" ? "Forgejo" : "Gitea"} URL is required.`;
    } else if (!isValidHTTPURL(url)) {
      errors.gitea_url = "Enter a valid http:// or https:// URL.";
    }
    if (!draft.gitea_token.trim() && !integ?.gitea_token_configured) {
      errors.gitea_token = `${forgeTokenLabel(forge)} is required.`;
    }
  } else {
    const url = draft.github_url.trim();
    const label = forge === "gitlab" ? "GitLab" : forge === "bitbucket" ? "Bitbucket" : "GitHub";
    if (!url) {
      errors.github_url = `${label} URL is required.`;
    } else if (!isValidHTTPURL(url)) {
      errors.github_url = "Enter a valid http:// or https:// URL.";
    }
    if (!draft.github_token.trim() && !integ?.github_token_configured) {
      errors.github_token = `${forgeTokenLabel(forge)} is required.`;
    }
  }
  const publicURL = draft.server_external_url.trim();
  if (!publicURL) {
    errors.server_external_url = "GitSeer public URL is required (set it in the Prepare step).";
  } else if (!isValidHTTPURL(publicURL)) {
    errors.server_external_url = "Enter a valid http:// or https:// URL.";
  }
  const order: FieldKey[] = usesGiteaFields
    ? ["gitea_url", "gitea_token", "gitea_allow_private_network", "server_external_url"]
    : ["github_url", "github_token", "github_allow_private_network", "server_external_url"];
  const firstInvalid = order.find((k) => errors[k]);
  return { ok: !firstInvalid, errors, firstInvalid };
}

type Props = {
  onComplete: (opts?: { syncAfter?: boolean }) => void | Promise<void>;
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
  const [step, setStep] = useState<Step>("secure");
  const [forge, setForge] = useState<WizardForge | null>(null);
  const [draft, setDraft] = useState<Draft>(emptyDraft());
  const [hydrated, setHydrated] = useState(false);
  const [busy, setBusy] = useState<"save" | "test" | "webhook" | "encryption" | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [testResult, setTestResult] = useState<TestConnectionResponse | null>(null);
  const [syncAfter, setSyncAfter] = useState(true);
  const [finishing, setFinishing] = useState(false);
  const [encryptionConfigured, setEncryptionConfigured] = useState(false);
  const [encryptionSource, setEncryptionSource] = useState<string>("");
  const [encryptionDraft, setEncryptionDraft] = useState("");
  const [generatedKey, setGeneratedKey] = useState<string | null>(null);
  const [keyCopied, setKeyCopied] = useState(false);
  const [acknowledgedKey, setAcknowledgedKey] = useState(false);

  const [checkOpen, setCheckOpen] = useState(false);
  const [checkRunning, setCheckRunning] = useState(false);
  const [checkRows, setCheckRows] = useState<ProbeCheck[] | null>(null);
  const [checkError, setCheckError] = useState<string | null>(null);
  const [webhookOpen, setWebhookOpen] = useState(false);
  const [webhookPhase, setWebhookPhase] = useState<WebhookModalPhase>("confirm");
  const [webhookPreview, setWebhookPreview] = useState<WebhookPreview | null>(null);
  const [webhookError, setWebhookError] = useState<string | null>(null);
  const [canCreateWebhook, setCanCreateWebhook] = useState(false);
  const validateProbeStarted = useRef(false);

  useEffect(() => {
    if (!settingsQuery.data || hydrated) return;
    // Do not seed server_external_url from settings/status — Prepare must start empty.
    setDraft(draftFromIntegration(settingsQuery.data.integration));
    const encConfigured =
      settingsQuery.data.encryption_configured === true ||
      settingsQuery.data.status?.encryption_configured === true;
    setEncryptionConfigured(encConfigured);
    setEncryptionSource(settingsQuery.data.encryption_source || "");
    setHydrated(true);
  }, [settingsQuery.data, hydrated]);

  const stepIndex = useMemo(() => STEPS.findIndex((s) => s.id === step), [step]);
  const currentStep = STEPS[stepIndex] ?? STEPS[0];
  const integ = settingsQuery.data?.integration;
  const isBusy = busy !== null;
  const checksPassed = Boolean(testResult?.ok);
  const forgeLabel =
    forge === "github"
      ? "GitHub"
      : forge === "gitlab"
        ? "GitLab"
        : forge === "bitbucket"
          ? "Bitbucket"
          : forge === "forgejo"
            ? "Forgejo"
            : "Gitea";
  const usesGiteaFields = forge === "gitea" || forge === "forgejo";
  const usesManualWebhook =
    forge === "github" || forge === "gitlab" || forge === "bitbucket" || forge === "forgejo";

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

  function validateSecurePublicURL(): boolean {
    const publicURL = draft.server_external_url.trim();
    if (!publicURL) {
      setFieldErrors((prev) => ({
        ...prev,
        server_external_url: "GitSeer public URL is required.",
      }));
      focusField("server_external_url");
      return false;
    }
    if (!isValidHTTPURL(publicURL)) {
      setFieldErrors((prev) => ({
        ...prev,
        server_external_url: "Enter a valid http:// or https:// URL.",
      }));
      focusField("server_external_url");
      return false;
    }
    setFieldErrors((prev) => {
      if (!prev.server_external_url) return prev;
      const next = { ...prev };
      delete next.server_external_url;
      return next;
    });
    return true;
  }

  async function generateEncryptionKey() {
    if (isBusy || encryptionConfigured) return;
    if (!validateSecurePublicURL()) return;
    setBusy("encryption");
    setError(null);
    setGeneratedKey(null);
    setKeyCopied(false);
    setAcknowledgedKey(false);
    try {
      await savePublicURL();
      const res = await api.setEncryptionKey({ generate: true });
      setEncryptionConfigured(true);
      setEncryptionSource(res.source || "file");
      if (res.encryption_key) {
        setGeneratedKey(res.encryption_key);
      }
      await queryClient.invalidateQueries({ queryKey: ["settings"] });
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to generate encryption key");
    } finally {
      setBusy(null);
    }
  }

  async function saveEncryptionKey() {
    if (isBusy || encryptionConfigured) return;
    if (!validateSecurePublicURL()) return;
    const key = encryptionDraft.trim();
    if (key.length < 16) {
      setError("Encryption key must be at least 16 characters.");
      return;
    }
    setBusy("encryption");
    setError(null);
    try {
      await savePublicURL();
      const res = await api.setEncryptionKey({ encryption_key: key });
      setEncryptionConfigured(true);
      setEncryptionSource(res.source || "file");
      setEncryptionDraft("");
      setGeneratedKey(null);
      setAcknowledgedKey(true);
      await queryClient.invalidateQueries({ queryKey: ["settings"] });
      setStep("pick");
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to save encryption key");
    } finally {
      setBusy(null);
    }
  }

  async function copyGeneratedKey() {
    if (!generatedKey) return;
    try {
      await navigator.clipboard.writeText(generatedKey);
      setKeyCopied(true);
    } catch {
      setKeyCopied(false);
      setError("Could not copy to clipboard — select and copy the key manually.");
    }
  }

  async function continueAfterEncryption() {
    if (isBusy) return;
    if (!validateSecurePublicURL()) return;
    if (!encryptionConfigured) {
      setError("Generate or save an encryption key before continuing.");
      return;
    }
    if (generatedKey && !acknowledgedKey) {
      setError("Confirm you have saved the encryption key before continuing.");
      return;
    }
    setBusy("save");
    setError(null);
    try {
      await savePublicURL();
      setGeneratedKey(null);
      setStep("pick");
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to save public URL");
    } finally {
      setBusy(null);
    }
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
    if (result.firstInvalid === "server_external_url") {
      setStep("secure");
      // focus after paint on Prepare step
      queueMicrotask(() => focusField("server_external_url"));
      return false;
    }
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
    if (forge === "gitlab" || forge === "bitbucket") {
      const body: {
        forge_type: ForgeType;
        base_url?: string;
        token?: string;
        allow_private_network?: boolean;
      } = {
        forge_type: forge,
        base_url: draft.github_url.trim() || undefined,
        allow_private_network: draft.github_allow_private_network,
      };
      if (draft.github_token) body.token = draft.github_token;
      return body;
    }
    if (forge === "forgejo") {
      const body: {
        forge_type: ForgeType;
        base_url?: string;
        token?: string;
        allow_private_network?: boolean;
        gitea_url?: string;
        gitea_token?: string;
        gitea_allow_private_network?: boolean;
      } = {
        forge_type: "forgejo",
        base_url: draft.gitea_url.trim() || undefined,
        gitea_url: draft.gitea_url.trim() || undefined,
        allow_private_network: draft.gitea_allow_private_network,
        gitea_allow_private_network: draft.gitea_allow_private_network,
      };
      if (draft.gitea_token) {
        body.token = draft.gitea_token;
        body.gitea_token = draft.gitea_token;
      }
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
    if (usesManualWebhook) {
      // Manual webhook forges: persist credentials + generated secret first.
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

  function goToSignInStep() {
    setWebhookOpen(false);
    setWebhookPhase("confirm");
    setCheckOpen(false);
    setStep("signin");
  }

  async function finishWebhook(create: boolean) {
    if (busy === "webhook") return;
    setBusy("webhook");
    setWebhookError(null);
    try {
      if (forge === "gitlab" || forge === "bitbucket" || forge === "forgejo") {
        const secret =
          typeof crypto !== "undefined" && "getRandomValues" in crypto
            ? Array.from(crypto.getRandomValues(new Uint8Array(24)))
                .map((b) => b.toString(16).padStart(2, "0"))
                .join("")
            : `whsec_${Date.now()}`;
        const baseURL =
          forge === "forgejo" ? draft.gitea_url.trim() : draft.github_url.trim();
        const token = forge === "forgejo" ? draft.gitea_token : draft.github_token;
        const allowPrivate =
          forge === "forgejo"
            ? draft.gitea_allow_private_network
            : draft.github_allow_private_network;
        const created = await api.createInstance({
          forge_type: forge,
          name: forgeLabel,
          base_url: baseURL,
          token: token || undefined,
          webhook_secret: secret,
          allow_private_network: allowPrivate,
        });
        // Forgejo can auto-ensure system hooks (Gitea-compatible). GitLab/Bitbucket are manual.
        if (forge === "forgejo" && created?.id) {
          try {
            await api.ensureWebhook(created.id);
          } catch {
            /* manual fallback below */
          }
        }
        setWebhookPreview({
          ...(typeof testResult?.webhook_preview === "object" && testResult?.webhook_preview
            ? testResult.webhook_preview
            : { active: true, events: [], config: {} }),
          config: {
            ...(testResult?.webhook_preview?.config || {}),
            secret,
            url: testResult?.webhook_preview?.config?.url || "",
          },
        } as WebhookPreview);
        await queryClient.invalidateQueries({ queryKey: ["settings"] });
        await queryClient.invalidateQueries({ queryKey: ["instances"] });
        setCheckOpen(false);
        setCanCreateWebhook(false);
        setWebhookPhase("manual");
        setWebhookOpen(true);
        return;
      }
      const res = await api.createWebhook({ ...connectionBody(), create });
      const secret = res.webhook?.config?.secret ?? "";
      applyWebhookResult(secret, res.integration);
      if (res.webhook) setWebhookPreview(res.webhook);
      await queryClient.invalidateQueries({ queryKey: ["settings"] });
      await queryClient.invalidateQueries({ queryKey: ["instances"] });
      setCheckOpen(false);
      if (forge === "github" || res.manual) {
        setCanCreateWebhook(false);
        setWebhookPhase("manual");
        setWebhookOpen(true);
      } else if (create) {
        goToSignInStep();
      } else {
        setWebhookPhase("manual");
        setWebhookOpen(true);
      }
    } catch (err) {
      setWebhookError(err instanceof Error ? err.message : "webhook setup failed");
      if (usesManualWebhook) {
        setWebhookOpen(true);
        setWebhookPhase("manual");
      }
    } finally {
      setBusy(null);
    }
  }

  function continueAfterWebhook() {
    goToSignInStep();
  }

  function backFromWebhook() {
    setWebhookOpen(false);
    setWebhookPhase("confirm");
    setWebhookError(null);
    setStep("validate");
  }

  async function finish() {
    if (finishing) return;
    setFinishing(true);
    setError(null);
    try {
      // Mark setup done and leave the wizard immediately. Optional catalog sync
      // runs in the main app — awaiting it here left the UI stuck on Finishing
      // for the entire (often long) sync-repos call.
      await api.completeSetup();
      await onComplete({ syncAfter });
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
      await onComplete();
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
    if (step === "pick") {
      setStep("secure");
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
          {step === "secure" && (
            <div className="settings-form__section">
              <fieldset disabled={isBusy}>
                <legend>GitSeer Public URL</legend>
                <div className="settings-form__field">
                  <input
                    id="wiz_server_external_url"
                    type="text"
                    inputMode="url"
                    value={draft.server_external_url}
                    onChange={(e) => setField("server_external_url", e.target.value)}
                    placeholder="GitSeer public URL (https://gitseer.example.com)"
                    aria-label="GitSeer Public URL"
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
                      Where forges send webhooks and OAuth callbacks.
                    </p>
                  )}
                </div>
              </fieldset>

              <fieldset disabled={isBusy}>
                <legend>Encryption Key</legend>
                {encryptionConfigured && !generatedKey ? (
                  <>
                    <p className="settings-form__saved" role="status">
                      ✓ Encryption key is configured
                      {encryptionSource === "config"
                        ? " from the environment or config file."
                        : " and stored for this install."}
                    </p>
                    <p className="muted">
                      Forge tokens and webhook secrets are encrypted before they are stored.
                    </p>
                  </>
                ) : generatedKey ? (
                  <>
                    <p className="muted">
                      Save this key somewhere safe. GitSeer wrote it to disk for this install; you need
                      the same value to recover secrets on another host.
                    </p>
                    <div className="settings-form__field">
                      <div className="settings-form__inline">
                        <PasswordInput
                          id="wiz_generated_encryption_key"
                          value={generatedKey}
                          onChange={() => undefined}
                          readOnly
                          aria-label="Generated Encryption Key"
                          autoComplete="off"
                        />
                        <button className="btn" type="button" onClick={() => void copyGeneratedKey()}>
                          {keyCopied ? "Copied" : "Copy Key"}
                        </button>
                      </div>
                    </div>
                  </>
                ) : (
                  <>
                    <p className="muted">
                      Generate or paste a key (min 16 characters). Required before storing forge
                      credentials.
                    </p>
                    <div className="settings-form__field">
                      <div className="settings-form__inline">
                        <PasswordInput
                          id="wiz_encryption_key"
                          value={encryptionDraft}
                          onChange={(v) => {
                            setEncryptionDraft(v);
                            setError(null);
                          }}
                          placeholder="Paste Encryption Key (optional)"
                          aria-label="Encryption Key"
                          autoComplete="off"
                        />
                        <button
                          className="btn"
                          type="button"
                          onClick={() => void generateEncryptionKey()}
                          disabled={isBusy}
                        >
                          {busy === "encryption" || busy === "save" ? "Working…" : "Generate Key"}
                        </button>
                      </div>
                    </div>
                    <div className="settings-form__actions">
                      <button
                        className="btn primary"
                        type="button"
                        onClick={() => void saveEncryptionKey()}
                        disabled={isBusy || encryptionDraft.trim().length < 16}
                      >
                        {busy === "encryption" || busy === "save" ? "Saving…" : "Save Key"}
                      </button>
                    </div>
                  </>
                )}
              </fieldset>

              {error && (
                <p className="error" role="alert">
                  {error}
                </p>
              )}

              {(encryptionConfigured || generatedKey) && (
                <div className="settings-form__actions">
                  {generatedKey && (
                    <label className="settings-form__check settings-form__actions-back">
                      <input
                        type="checkbox"
                        checked={acknowledgedKey}
                        onChange={(e) => {
                          setAcknowledgedKey(e.target.checked);
                          setError(null);
                        }}
                      />
                      I Have Saved This Encryption Key
                    </label>
                  )}
                  <button
                    className="btn primary"
                    type="button"
                    onClick={() => void continueAfterEncryption()}
                    disabled={isBusy || (Boolean(generatedKey) && !acknowledgedKey)}
                  >
                    {busy === "save" ? "Saving…" : "Continue"}
                  </button>
                </div>
              )}
            </div>
          )}

          {step === "pick" && (
            <div className="settings-form__section">
              <div className="forge-picker" role="list">
                {FORGE_OPTIONS.map((opt) => {
                  return (
                    <button
                      key={opt.id}
                      type="button"
                      role="listitem"
                      className="forge-picker__card"
                      onClick={() => selectForge(opt.id)}
                    >
                      <span className="forge-picker__title">{opt.label}</span>
                      <span className="forge-picker__desc muted">{opt.description}</span>
                    </button>
                  );
                })}
              </div>
            </div>
          )}

          {step === "connect" && usesGiteaFields && forge && (
            <form onSubmit={advance} noValidate>
              <fieldset className="settings-form__section" disabled={isBusy}>
                <legend>Connect {forgeLabel}</legend>
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
                        placeholder={
                          forge === "forgejo"
                            ? "Forgejo URL (https://forgejo.example.com)"
                            : "Gitea URL (https://git.example.com)"
                        }
                        aria-label={`${forgeLabel} URL`}
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
                            Lets GitSeer call {forgeLabel} on private or lab addresses (10.x, 192.168.x, localhost, and
                            similar). Off by default to block SSRF. Enable when {forgeLabel} is only reachable on a
                            private network.
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
                            ? `${forgeTokenLabel(forge)} (leave blank to keep)`
                            : `${forgeTokenLabel(forge)} (required)`
                        }
                        aria-label={forgeTokenLabel(forge)}
                        autoComplete="new-password"
                        required={!integ?.gitea_token_configured}
                        aria-invalid={fieldErrors.gitea_token ? true : undefined}
                        aria-describedby={
                          fieldErrors.gitea_token
                            ? "wiz_gitea_token_error"
                            : integ?.gitea_token_configured
                              ? "wiz_gitea_token_hint"
                              : undefined
                        }
                      />
                      <GiteaPATHelp baseURL={draft.gitea_url} />
                      {fieldErrors.gitea_token ? (
                        <p id="wiz_gitea_token_error" className="settings-form__error" role="alert">
                          {fieldErrors.gitea_token}
                        </p>
                      ) : integ?.gitea_token_configured ? (
                        <p id="wiz_gitea_token_hint" className="settings-form__hint">
                          Token is configured. Leave blank to keep it.
                        </p>
                      ) : null}
                    </div>
                  </div>
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

          {step === "connect" && !usesGiteaFields && forge && (
            <form onSubmit={advance} noValidate>
              <fieldset className="settings-form__section" disabled={isBusy}>
                <legend>Connect {forgeLabel}</legend>
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
                        placeholder={
                          forge === "gitlab"
                            ? "GitLab URL (https://gitlab.com)"
                            : forge === "bitbucket"
                              ? "Bitbucket URL (https://bitbucket.org)"
                              : "GitHub URL (https://github.com)"
                        }
                        aria-label={`${forgeLabel} URL`}
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
                          {forge === "gitlab"
                            ? "Use https://gitlab.com or your self-hosted GitLab URL."
                            : forge === "bitbucket"
                              ? "Use https://bitbucket.org for Bitbucket Cloud."
                              : "Use https://github.com or your GitHub Enterprise host. API roots are normalized automatically."}
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
                            Lets GitSeer call {forgeLabel} on private or lab addresses. Off by default to block
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
                            ? `${forgeTokenLabel(forge)} (leave blank to keep)`
                            : `${forgeTokenLabel(forge)} (required)`
                        }
                        aria-label={forgeTokenLabel(forge)}
                        autoComplete="new-password"
                        required={!integ?.github_token_configured}
                        aria-invalid={fieldErrors.github_token ? true : undefined}
                        aria-describedby={
                          fieldErrors.github_token
                            ? "wiz_github_token_error"
                            : integ?.github_token_configured
                              ? "wiz_github_token_hint"
                              : undefined
                        }
                      />
                      {forge === "github" ? <GitHubPATHelp baseURL={draft.github_url} /> : null}
                      {fieldErrors.github_token ? (
                        <p id="wiz_github_token_error" className="settings-form__error" role="alert">
                          {fieldErrors.github_token}
                        </p>
                      ) : integ?.github_token_configured ? (
                        <p id="wiz_github_token_hint" className="settings-form__hint">
                          Token is configured. Leave blank to keep it.
                        </p>
                      ) : null}
                    </div>
                  </div>
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
              <p className="muted">
                {checkRunning || busy === "test"
                  ? "Running connectivity and permission checks…"
                  : checksPassed
                    ? usesManualWebhook
                      ? "Checks passed. Continue for manual webhook instructions."
                      : "Checks passed. Continue to install the webhook."
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

          {step === "signin" && (
            <div className="settings-form__section">
              <OAuthSignInPanel
                editable
                mode="wizard"
                onContinue={() => setStep("finish")}
                onSkip={() => setStep("finish")}
              />
              <div className="settings-form__actions">
                <button
                  className="btn settings-form__actions-back"
                  type="button"
                  onClick={back}
                  disabled={isBusy}
                >
                  Back
                </button>
              </div>
            </div>
          )}

          {step === "finish" && (
            <div className="settings-form__section">
              <p className="muted">
                Mark setup complete and open the console. Optionally start a repository sync in the
                background afterward. You can add another forge later under Settings → Integration.
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
        manualOnly={usesManualWebhook}
        onBack={backFromWebhook}
        onManual={() => void finishWebhook(false)}
        onCreate={() => void finishWebhook(true)}
        onContinue={continueAfterWebhook}
      />
    </div>
  );
}
