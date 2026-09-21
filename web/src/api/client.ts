export type LensTheme = "light" | "dark" | "system" | "gruvbox" | "terminal";

export type User = {
  id: number;
  login: string;
  display_name: string;
  is_bootstrap_admin: boolean;
  authz?: string;
  csrf_token?: string;
  /** Mapped from Gitea defaults only; omitted for custom/unknown themes. */
  theme?: LensTheme | null;
  gitea_theme?: string | null;
};

export type Repository = {
  id: number;
  external_id: number;
  owner: string;
  name: string;
  full_name: string;
  default_branch: string;
  private: boolean;
  archived: boolean;
  html_url: string;
};

export type PullRequest = {
  id: number;
  number: number;
  title: string;
  state: string;
  draft: boolean;
  author_login: string;
  ci_state?: string;
  repo_owner?: string;
  repo_name?: string;
  repo_full?: string;
  html_url: string;
};

export type WorkflowRun = {
  id: number;
  name: string;
  status: string;
  conclusion: string;
  branch: string;
  event?: string;
  actor_login: string;
  repo_full?: string;
  workflow_path?: string;
  started_at?: string;
  completed_at?: string;
  html_url: string;
};

export type Job = {
  id: number;
  name: string;
  status: string;
  conclusion: string;
  steps_json?: string;
  started_at?: string;
  completed_at?: string;
  html_url?: string;
};

export type ActiveRunItem = {
  run: WorkflowRun;
  jobs: Job[];
};

export type AttentionItem = {
  id: number;
  type: string;
  severity: string;
  title: string;
  entity_type?: string;
  entity_id?: number;
  metadata_json?: string;
  opened_at?: string;
  html_url?: string;
  repo_full?: string;
};

export type Summary = {
  repositories: number;
  open_pull_requests: number;
  attention_open: number;
  failed_runs: number;
  running_runs: number;
  days: number;
  since?: string;
};

export type DayRunBucket = {
  day: string;
  success: number;
  failure: number;
  cancelled: number;
  other: number;
};

export type DayPRBucket = {
  day: string;
  opened: number;
  merged: number;
  closed: number;
};

export type CountBucket = {
  key: string;
  count: number;
};

export type DurationStats = {
  p50_seconds: number;
  p95_seconds: number;
  sample_count: number;
};

export type StatsReport = {
  days: number;
  since?: string;
  runs_by_day: DayRunBucket[];
  run_conclusions: CountBucket[];
  run_duration: DurationStats | null;
  prs_by_day: DayPRBucket[];
  pr_ci_states: CountBucket[];
  attention_by_severity: CountBucket[];
  attention_by_type: CountBucket[];
};

export type WorkflowNode = {
  job_key: string;
  name: string;
  needs: string[];
  unknown_deps?: boolean;
};

export class UnauthorizedError extends Error {
  constructor(message = "unauthorized") {
    super(message);
    this.name = "UnauthorizedError";
  }
}

let onUnauthorized: (() => void) | null = null;
let csrfToken = "";

export function setUnauthorizedHandler(fn: (() => void) | null) {
  onUnauthorized = fn;
}

export function setCsrfToken(token: string | null | undefined) {
  csrfToken = (token || "").trim();
}

function readCookie(name: string): string {
  if (typeof document === "undefined") return "";
  const parts = document.cookie.split(";");
  for (const part of parts) {
    const [k, ...rest] = part.trim().split("=");
    if (k === name) return decodeURIComponent(rest.join("="));
  }
  return "";
}

function csrfHeader(): Record<string, string> {
  const token = csrfToken || readCookie("lens_csrf");
  return token ? { "X-CSRF-Token": token } : {};
}

/** Allow only http(s) absolute URLs (or same-origin relative paths). */
export function safeExternalHref(url?: string | null): string | undefined {
  if (!url) return undefined;
  const trimmed = url.trim();
  if (!trimmed) return undefined;
  try {
    const base = typeof window !== "undefined" ? window.location.origin : "http://localhost";
    const u = new URL(trimmed, base);
    if (u.protocol === "http:" || u.protocol === "https:") return u.href;
  } catch {
    /* ignore */
  }
  return undefined;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const base = (typeof window !== "undefined" && window.__LENS_BASE__) || "";
  const headers = new Headers(init?.headers);
  if (init?.body != null && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  const method = (init?.method || "GET").toUpperCase();
  if (method !== "GET" && method !== "HEAD" && method !== "OPTIONS") {
    for (const [k, v] of Object.entries(csrfHeader())) {
      if (!headers.has(k)) headers.set(k, v);
    }
  }
  const res = await fetch(`${base}${path}`, {
    ...init,
    credentials: "include",
    headers,
  });
  if (res.status === 401) {
    onUnauthorized?.();
    throw new UnauthorizedError();
  }
  if (!res.ok) {
    let msg = `Request failed (${res.status})`;
    try {
      const body = await res.json();
      if (body?.error) msg = String(body.error);
    } catch {
      /* ignore */
    }
    throw new Error(msg);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}

export type UIConfig = {
  base_path?: string;
  oauth_enabled?: boolean;
  bootstrap_enabled?: boolean;
  allow_skip_setup?: boolean;
  /** Present only when allow_skip_setup is true (local npm start). */
  dev_bootstrap_password?: string;
  csrf_token?: string;
};

export type LensSettings = {
  instance_name: string;
  sync_history_days: number;
  attention_long_running_after: string;
  retention_runs_days: number;
  retention_webhooks_days: number;
  retention_attention_days: number;
  server_external_url: string;
};

export type IntegrationPublic = {
  gitea_url: string;
  gitea_token_configured: boolean;
  gitea_webhook_secret_configured: boolean;
  gitea_allow_private_network: boolean;
  gitea_allow_unsigned_webhooks: boolean;
  oauth_client_id: string;
  oauth_client_secret_configured: boolean;
};

export type IntegrationPatch = {
  gitea_url: string;
  gitea_token?: string;
  gitea_webhook_secret?: string;
  clear_gitea_token?: boolean;
  clear_gitea_webhook_secret?: boolean;
  gitea_allow_private_network: boolean;
  gitea_allow_unsigned_webhooks: boolean;
  oauth_client_id: string;
  oauth_client_secret?: string;
  clear_oauth_client_secret?: boolean;
};

export type UpdateSettingsBody = Partial<LensSettings> & {
  integration?: IntegrationPatch;
  setup_completed?: boolean;
};

export type SettingsResponse = {
  editable: boolean;
  settings: LensSettings;
  integration: IntegrationPublic;
  setup_completed: boolean;
  status: Record<string, unknown>;
};

export type CheckGiteaURLBody = {
  gitea_url: string;
  gitea_allow_private_network?: boolean;
};

export type CheckGiteaURLResponse = {
  ok: boolean;
  gitea_url?: string;
  private?: boolean;
  private_ip?: string;
  code?: "invalid" | "unreachable" | "private_network" | string;
  error?: string;
};

export type TestConnectionBody = {
  gitea_url?: string;
  gitea_token?: string;
  gitea_allow_private_network?: boolean;
};

export type ProbeCheckStatus = "ok" | "fail" | "warn" | "skip" | "pending" | "running";

export type ProbeCheck = {
  id: string;
  group?: "connectivity" | "permissions" | string;
  label: string;
  status: ProbeCheckStatus;
  detail?: string;
};

export type WebhookPreview = {
  type: string;
  active: boolean;
  events: string[];
  config: Record<string, string>;
};

export type OAuthAppPreview = {
  name: string;
  redirect_uri: string;
  confidential_client: boolean;
  gitea_settings_path: string;
  gitea_admin_apps_path: string;
};

export type TestConnectionResponse = {
  ok: boolean;
  version: string;
  login?: string;
  is_admin?: boolean;
  checks: ProbeCheck[];
  capabilities?: unknown;
  can_create_webhook?: boolean;
  can_create_oauth?: boolean;
  webhook_preview?: WebhookPreview;
  oauth_app_preview?: OAuthAppPreview;
};

export type CreateWebhookBody = TestConnectionBody & {
  create: boolean;
};

export type CreateWebhookResponse = {
  ok: boolean;
  created?: boolean;
  updated?: boolean;
  manual?: boolean;
  hook_id?: number;
  webhook: WebhookPreview;
  integration: IntegrationPublic;
  delivery_url: string;
};

export type CreateOAuthBody = TestConnectionBody & {
  create: boolean;
  oauth_client_id?: string;
  oauth_client_secret?: string;
};

export type CreateOAuthResponse = {
  ok: boolean;
  created?: boolean;
  updated?: boolean;
  manual?: boolean;
  redirect_uri: string;
  oauth_app: OAuthAppPreview;
  client_id: string;
  integration: IntegrationPublic;
};

export type CompleteSetupResponse = {
  setup_completed: boolean;
  status: Record<string, unknown>;
};

export const api = {
  uiConfig: async () => {
    const cfg = await request<UIConfig>("/api/v1/ui-config");
    if (cfg.csrf_token) setCsrfToken(cfg.csrf_token);
    return cfg;
  },
  me: async () => {
    const base = (typeof window !== "undefined" && window.__LENS_BASE__) || "";
    const res = await fetch(`${base}/api/v1/auth/me`, { credentials: "include" });
    if (!res.ok) {
      let msg = res.statusText;
      try {
        const body = await res.json();
        msg = body.error || msg;
      } catch {
        /* ignore */
      }
      throw new Error(msg);
    }
    const body = (await res.json()) as User & { authenticated?: boolean; csrf_token?: string };
    if (body.csrf_token) setCsrfToken(body.csrf_token);
    if (body.authenticated === false || body.id == null) {
      return null;
    }
    return body;
  },
  login: async (password: string) => {
    const out = await request<{ user: User; csrf_token?: string }>("/api/v1/auth/bootstrap/login", {
      method: "POST",
      body: JSON.stringify({ password }),
    });
    if (out.csrf_token) setCsrfToken(out.csrf_token);
    return out;
  },
  logout: () => request<{ status: string }>("/api/v1/auth/logout", { method: "POST" }),
  summary: (days = 0) => request<Summary>(`/api/v1/summary?days=${days}`),
  stats: (days = 0) => request<StatsReport>(`/api/v1/stats?days=${days}`),
  repositories: (q = "") =>
    request<{ items: Repository[]; total: number }>(`/api/v1/repositories?limit=100&q=${encodeURIComponent(q)}`),
  pullRequests: (q = "") =>
    request<{ items: PullRequest[]; total: number }>(
      `/api/v1/pull-requests?state=open&limit=100&q=${encodeURIComponent(q)}`,
    ),
  workflowRuns: (q = "") =>
    request<{ items: WorkflowRun[]; total: number }>(
      `/api/v1/workflow-runs?limit=100&q=${encodeURIComponent(q)}`,
    ),
  activeWorkflowRuns: () =>
    request<{ items: ActiveRunItem[]; total: number }>("/api/v1/workflow-runs/active"),
  workflowRun: (id: number) =>
    request<{ run: WorkflowRun; jobs: Job[]; graph: WorkflowNode[] | null }>(`/api/v1/workflow-runs/${id}`),
  attention: (q = "") =>
    request<{ items: AttentionItem[]; total: number }>(
      `/api/v1/attention?limit=100&q=${encodeURIComponent(q)}`,
    ),
  syncRepos: () => request<unknown>("/api/v1/setup/sync-repos", { method: "POST" }),
  systemStatus: () => request<Record<string, unknown>>("/api/v1/system/status"),
  settings: () => request<SettingsResponse>("/api/v1/settings"),
  updateSettings: (body: UpdateSettingsBody) =>
    request<SettingsResponse>("/api/v1/settings", {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  testConnection: (body?: TestConnectionBody) =>
    request<TestConnectionResponse>("/api/v1/setup/test-connection", {
      method: "POST",
      body: JSON.stringify(body ?? {}),
    }),
  checkGiteaURL: async (body: CheckGiteaURLBody) => {
    const base = (typeof window !== "undefined" && window.__LENS_BASE__) || "";
    const headers = new Headers({ "Content-Type": "application/json" });
    for (const [k, v] of Object.entries(csrfHeader())) {
      headers.set(k, v);
    }
    const res = await fetch(`${base}/api/v1/setup/check-gitea-url`, {
      method: "POST",
      credentials: "include",
      headers,
      body: JSON.stringify(body),
    });
    if (res.status === 401) {
      onUnauthorized?.();
      throw new UnauthorizedError();
    }
    let data: CheckGiteaURLResponse = { ok: false, error: `Request failed (${res.status})` };
    try {
      data = (await res.json()) as CheckGiteaURLResponse;
    } catch {
      /* ignore */
    }
    return data;
  },
  createWebhook: (body: CreateWebhookBody) =>
    request<CreateWebhookResponse>("/api/v1/setup/create-webhook", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  createOAuth: (body: CreateOAuthBody) =>
    request<CreateOAuthResponse>("/api/v1/setup/create-oauth", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  completeSetup: () =>
    request<CompleteSetupResponse>("/api/v1/setup/complete", { method: "POST" }),
  jobLogs: async (id: number) => {
    const base = (typeof window !== "undefined" && window.__LENS_BASE__) || "";
    const res = await fetch(`${base}/api/v1/jobs/${id}/logs`, { credentials: "include" });
    if (res.status === 401) {
      onUnauthorized?.();
      throw new UnauthorizedError();
    }
    if (!res.ok) throw new Error("failed to load logs");
    return res.text();
  },
};

export function repoLabel(item: { repo_full?: string; full_name?: string }) {
  return item.repo_full || item.full_name || "";
}

export function ciLabel(state?: string) {
  switch ((state || "").toLowerCase()) {
    case "success":
      return "pass";
    case "failure":
      return "fail";
    case "pending":
      return "pending";
    case "cancelled":
    case "canceled":
      return "cancelled";
    case "":
      return "—";
    default:
      return state || "—";
  }
}

export function ciBadgeClass(state?: string) {
  switch ((state || "").toLowerCase()) {
    case "success":
      return "success";
    case "failure":
      return "failure";
    case "pending":
      return "pending";
    case "cancelled":
    case "canceled":
      return "cancelled";
    default:
      return "";
  }
}
