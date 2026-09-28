export type GitSeerTheme = "light" | "dark" | "system" | "gruvbox" | "terminal";

export type User = {
  id: number;
  login: string;
  display_name: string;
  is_bootstrap_admin: boolean;
  is_bootstrap_permanent?: boolean;
  bootstrap_elevated_until?: string | null;
  bootstrap_login_enabled?: boolean;
  can_elevate_bootstrap?: boolean;
  authz?: string;
  csrf_token?: string;
  /** Mapped from Gitea defaults only; omitted for custom/unknown themes. */
  theme?: GitSeerTheme | null;
  gitea_theme?: string | null;
  has_gitea?: boolean;
  has_github?: boolean;
  has_gitlab?: boolean;
  has_bitbucket?: boolean;
  github_oauth_enabled?: boolean;
  gitlab_oauth_enabled?: boolean;
  bitbucket_oauth_enabled?: boolean;
  forgejo_oauth_enabled?: boolean;
};

export type ACLUser = {
  id: number;
  login: string;
  display_name: string;
  email?: string;
  is_bootstrap_admin: boolean;
  has_gitea?: boolean;
  has_github?: boolean;
  has_gitlab?: boolean;
  has_bitbucket?: boolean;
  gitea_instance_id?: number;
  github_instance_id?: number;
  gitlab_instance_id?: number;
  bitbucket_instance_id?: number;
  token_instance_ids?: number[];
};

export type ForgeType = "gitea" | "github" | "gitlab" | "bitbucket" | "forgejo";

export type Repository = {
  id: number;
  external_id: number;
  node_id?: string;
  owner: string;
  name: string;
  full_name: string;
  default_branch: string;
  private: boolean;
  archived: boolean;
  html_url: string;
  forge_type?: ForgeType | string;
  instance_id?: number;
  instance_name?: string;
  health?: RepoHealth;
};

export type RepoHealth = {
  repo_id: number;
  score: number;
  grade: "healthy" | "degraded" | "critical" | string;
  open_critical_attention: number;
  failing_default_branch: boolean;
  failing_default_branch_run_id?: number;
  stale_open_prs: number;
  ci_fail_rate: number;
  ci_runs_in_window: number;
  ci_failures_in_window: number;
  window_days: number;
  stale_pr_days: number;
};

export type FailureCluster = {
  repo_id: number;
  workflow_path: string;
  job_name: string;
  failure_count: number;
  last_failed_at?: string;
  sample_run_id?: number;
  sample_job_id?: number;
};

export type AttentionLogSnippet = {
  attention_id: number;
  job_id: number;
  job_name: string;
  snippet: string;
  truncated: boolean;
  bytes: number;
  stored: boolean;
};

export type PullRequest = {
  id: number;
  number: number;
  title: string;
  state: string;
  draft: boolean;
  author_login: string;
  external_id?: number;
  node_id?: string;
  ci_state?: string;
  review_state?: string;
  repo_owner?: string;
  repo_name?: string;
  repo_full?: string;
  html_url: string;
  forge_type?: ForgeType | string;
  instance_id?: number;
  instance_name?: string;
};

export type WorkflowRun = {
  id: number;
  name: string;
  status: string;
  conclusion: string;
  branch: string;
  event?: string;
  actor_login: string;
  external_id?: number;
  node_id?: string;
  repo_full?: string;
  workflow_path?: string;
  started_at?: string;
  completed_at?: string;
  html_url: string;
  forge_type?: ForgeType | string;
  instance_id?: number;
  instance_name?: string;
};

export type Job = {
  id: number;
  name: string;
  status: string;
  conclusion: string;
  external_id?: number;
  node_id?: string;
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
  fingerprint?: string;
  opened_at?: string;
  html_url?: string;
  repo_full?: string;
  forge_type?: ForgeType | string;
  instance_id?: number;
  instance_name?: string;
};

export type AttentionMuteUntil = "24h" | "7d" | "resolved";

export type InboxReason = "author" | "requested_reviewer" | "failing_ci" | "blocked_on_me";

export type InboxItem = {
  kind: "attention" | "pull_request" | string;
  reasons: InboxReason[] | string[];
  attention?: AttentionItem;
  pull_request?: PullRequest;
};

export type SavedFilterQuery = {
  q?: string;
  severity?: string;
  type?: string;
  forge_type?: string;
  instance_id?: number;
  reason?: InboxReason | string;
  page?: "attention" | "inbox" | "pull-requests" | "dashboard" | string;
  org_id?: number;
  owner?: string;
  team?: string;
};

export type SavedFilter = {
  id: number;
  user_id: number;
  name: string;
  query: SavedFilterQuery;
  created_at?: string;
  updated_at?: string;
};

export type AttentionRuleOverride = {
  rule_type: string;
  severity: string;
  updated_at?: string;
};

export type AttentionRuleDefault = {
  rule_type: string;
  default_severity: string;
};

export type Summary = {
  repositories: number;
  open_pull_requests: number;
  attention_open: number;
  failed_runs: number;
  running_runs: number;
  days: number;
  since?: string;
  org_id?: number;
  owner?: string;
  team?: string;
};

export type DashboardScope = {
  org_id?: number;
  owner?: string;
  team?: string;
  forge_type?: string;
  instance_id?: number;
};

export type RunnerUtilizationRow = {
  runner_name: string;
  runner_id?: number;
  busy_jobs: number;
  queued_jobs: number;
  completed_jobs: number;
  failed_jobs: number;
  instance_id?: number;
  instance_name?: string;
  forge_type?: string;
};

export type RunnerUtilizationReport = {
  days: number;
  since?: string;
  source: string;
  degraded: boolean;
  degraded_reason?: string;
  runners_api_capable: boolean;
  items: RunnerUtilizationRow[];
};

export type FlakyJob = {
  repo_id: number;
  repo_full?: string;
  workflow_path: string;
  job_name: string;
  failure_count: number;
  success_count: number;
  flip_count: number;
  last_failed_at?: string;
  last_success_at?: string;
  sample_run_id?: number;
};

export type ReleaseRun = {
  id: number;
  repo_id: number;
  repo_full: string;
  name: string;
  workflow_path: string;
  event: string;
  branch: string;
  status: string;
  conclusion: string;
  html_url?: string;
  started_at?: string;
  completed_at?: string;
  forge_type?: string;
  instance_id?: number;
  attention_open: boolean;
};

export type WallboardToken = {
  id: number;
  name: string;
  token_prefix: string;
  created_by_user_id?: number;
  created_at?: string;
  last_used_at?: string;
  revoked_at?: string;
};

export type WallboardSnapshot = {
  generated_at: string;
  summary: Summary;
  attention: AttentionItem[];
  attention_by_severity: CountBucket[];
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

export type StatsSection = "core" | "trends" | "duration" | "all";

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
  const token = csrfToken || readCookie("gitseer_csrf");
  return token ? { "X-CSRF-Token": token } : {};
}

/** Allow only http(s) absolute URLs (or same-origin relative paths). */
export function safeExternalHref(url?: string | null): string | undefined {
  if (!url) return undefined;
  const trimmed = url.trim();
  if (!trimmed) return undefined;
  // Reject protocol-relative URLs (//evil.com/...) which URL() would resolve against the page origin.
  if (trimmed.startsWith("//")) return undefined;
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
  const base = (typeof window !== "undefined" && window.__GITSEER_BASE__) || "";
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
  github_oauth_enabled?: boolean;
  gitlab_oauth_enabled?: boolean;
  bitbucket_oauth_enabled?: boolean;
  forgejo_oauth_enabled?: boolean;
  bootstrap_enabled?: boolean;
  bootstrap_unclaimed?: boolean;
  bootstrap_username?: string;
  bootstrap_login_enabled?: boolean;
  allow_skip_setup?: boolean;
  csrf_token?: string;
};

export type GitSeerSettings = {
  instance_name: string;
  sync_history_days: number;
  attention_long_running_after: string;
  retention_runs_days: number;
  retention_webhooks_days: number;
  retention_attention_days: number;
  server_external_url: string;
};

export type NotificationSettings = {
  enabled: boolean;
  min_severity: string;
  immediate_enabled: boolean;
  digest_enabled: boolean;
  digest_hour_utc: number;
  smtp_enabled: boolean;
  smtp_host: string;
  smtp_port: number;
  smtp_tls_mode: string;
  smtp_from: string;
  smtp_to: string;
  smtp_username: string;
  smtp_password_configured: boolean;
  slack_enabled: boolean;
  slack_webhook_configured: boolean;
  discord_enabled: boolean;
  discord_webhook_configured: boolean;
  webhook_enabled: boolean;
  webhook_url_configured: boolean;
  incident_enabled: boolean;
  incident_webhook_configured: boolean;
};

export type NotificationSettingsPatch = {
  enabled?: boolean;
  min_severity?: string;
  immediate_enabled?: boolean;
  digest_enabled?: boolean;
  digest_hour_utc?: number;
  smtp_enabled?: boolean;
  smtp_host?: string;
  smtp_port?: number;
  smtp_tls_mode?: string;
  smtp_from?: string;
  smtp_to?: string;
  smtp_username?: string;
  smtp_password?: string;
  clear_smtp_password?: boolean;
  slack_enabled?: boolean;
  slack_webhook_url?: string;
  clear_slack_webhook?: boolean;
  discord_enabled?: boolean;
  discord_webhook_url?: string;
  clear_discord_webhook?: boolean;
  webhook_enabled?: boolean;
  webhook_url?: string;
  clear_webhook_url?: boolean;
  incident_enabled?: boolean;
  incident_webhook_url?: string;
  clear_incident_webhook?: boolean;
};

export type AlertPrefs = {
  browser_enabled: boolean;
  push_enabled: boolean;
  min_severity: string;
  push_configured: boolean;
  vapid_public_key?: string;
  subscription_count: number;
};

export type AlertPrefsPatch = {
  browser_enabled?: boolean;
  push_enabled?: boolean;
  min_severity?: string;
};

export type IntegrationPublic = {
  gitea_url: string;
  gitea_token_configured: boolean;
  gitea_webhook_secret_configured: boolean;
  gitea_allow_private_network: boolean;
  gitea_allow_unsigned_webhooks: boolean;
  oauth_client_id: string;
  oauth_client_secret_configured: boolean;
  github_url: string;
  github_token_configured: boolean;
  github_webhook_secret_configured: boolean;
  github_allow_private_network: boolean;
  github_allow_unsigned_webhooks: boolean;
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
  apply_github?: boolean;
  github_url?: string;
  github_token?: string;
  github_webhook_secret?: string;
  clear_github_token?: boolean;
  clear_github_webhook_secret?: boolean;
  github_allow_private_network?: boolean;
  github_allow_unsigned_webhooks?: boolean;
};

export type UpdateSettingsBody = Partial<GitSeerSettings> & {
  integration?: IntegrationPatch;
  setup_completed?: boolean;
};

export type ForgeStatusRow = {
  forge_type: string;
  name?: string;
  url?: string;
  configured?: boolean;
  connected?: boolean;
  webhook_hmac?: boolean;
  oauth_configured?: boolean;
  allow_private_network?: boolean;
  allow_unsigned?: boolean;
  version?: string;
  instance_id?: number;
  token_configured?: boolean;
  webhook_secret_configured?: boolean;
  capabilities?: unknown;
  sync_phase?: string;
  sync_last_success_at?: string;
  sync_last_error?: string;
  lease_holder?: string;
  lease_expires_at?: string;
  lease_active?: boolean;
  webhook_stats_24h?: {
    ok?: number;
    failed?: number;
    pending?: number;
    processing?: number;
    total?: number;
  };
  webhook_last_at?: string;
  webhook_last_error?: string;
  webhook_last_status?: string;
  webhook_last_event?: string;
  webhook_verified?: boolean;
  webhook_verified_at?: string;
  webhook_ensure_at?: string;
  webhook_ensure_error?: string;
  webhook_verify_pending?: boolean;
  ops_checklist?: OpsChecklistItem[];
  capability_matrix?: OpsChecklistItem[];
  active_actions_hint?: string;
};

export type OpsChecklistItem = {
  id: string;
  label: string;
  status: "pass" | "warn" | "fail" | "unknown" | string;
  detail?: string;
};

export type InstancePublic = {
  id: number;
  name: string;
  forge_type: string;
  base_url: string;
  version?: string;
  token_configured: boolean;
  webhook_secret_configured: boolean;
  oauth_client_id?: string;
  oauth_client_secret_configured?: boolean;
  allow_private_network: boolean;
  allow_unsigned_webhooks: boolean;
  created_at?: string;
  updated_at?: string;
};

export type InstancePatch = {
  forge_type?: string;
  name?: string;
  base_url?: string;
  token?: string;
  webhook_secret?: string;
  clear_token?: boolean;
  clear_webhook_secret?: boolean;
  oauth_client_id?: string;
  oauth_client_secret?: string;
  clear_oauth_client_secret?: boolean;
  allow_private_network?: boolean;
  allow_unsigned_webhooks?: boolean;
};

export type SystemStatus = {
  version?: string;
  ui_name?: string;
  gitea_configured?: boolean;
  github_configured?: boolean;
  bootstrap_auth?: boolean;
  oauth_enabled?: boolean;
  path_prefix?: string;
  instance_connected?: boolean;
  webhook_hmac?: boolean;
  github_webhook_hmac?: boolean;
  setup_completed?: boolean;
  encryption_configured?: boolean;
  encryption_source?: string;
  encryption_healthy?: boolean;
  encryption_error?: string;
  oauth_redirect_uri?: string;
  server_external_url?: string;
  gitea_version?: string;
  github_version?: string;
  forges?: ForgeStatusRow[];
  webhook_stats_24h?: ForgeStatusRow["webhook_stats_24h"];
  active_actions_hint?: string;
  active_actions_in_flight?: number;
  storage?: StorageStatus;
  [key: string]: unknown;
};

export type StorageStatus = {
  driver?: string;
  bytes?: number;
  bytes_human?: string;
  method?: string;
  estimate?: boolean;
  level?: "ok" | "warn" | "critical" | "unknown" | string;
  warn_bytes?: number;
  critical_bytes?: number;
  warn_human?: string;
  critical_human?: string;
  path?: string;
  error?: string;
};

export type PurgeRetentionResponse = {
  ok: boolean;
  stats: Record<string, number>;
  windows: {
    runs_days: number;
    webhooks_days: number;
    attention_days: number;
  };
  storage?: StorageStatus;
};

export type SettingsResponse = {
  editable: boolean;
  settings: GitSeerSettings;
  integration: IntegrationPublic;
  setup_completed: boolean;
  encryption_configured?: boolean;
  encryption_source?: string;
  status: SystemStatus;
};

export type EncryptionStatus = {
  configured: boolean;
  source?: string;
};

export type SetEncryptionResponse = {
  configured: boolean;
  source?: string;
  generated?: boolean;
  /** Present only when generate=true — copy and store securely; never returned again. */
  encryption_key?: string;
};

export type CompleteSetupResponse = {
  setup_completed: boolean;
  status: SystemStatus;
};

export type CheckGiteaURLBody = {
  forge_type?: ForgeType | string;
  gitea_url?: string;
  gitea_allow_private_network?: boolean;
  github_url?: string;
  github_allow_private_network?: boolean;
};

export type CheckGiteaURLResponse = {
  ok: boolean;
  forge_type?: string;
  gitea_url?: string;
  github_url?: string;
  private?: boolean;
  private_ip?: string;
  code?: "invalid" | "unreachable" | "private_network" | string;
  error?: string;
  detail?: string;
};

export type TestConnectionBody = {
  forge_type?: ForgeType | string;
  gitea_url?: string;
  gitea_token?: string;
  gitea_allow_private_network?: boolean;
  github_url?: string;
  github_token?: string;
  github_allow_private_network?: boolean;
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
  type?: string;
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
  forge_type?: string;
  version: string;
  login?: string;
  is_admin?: boolean;
  checks: ProbeCheck[];
  capabilities?: unknown;
  can_create_webhook?: boolean;
  can_create_oauth?: boolean;
  webhook_preview?: WebhookPreview;
  oauth_app_preview?: OAuthAppPreview;
  manual_webhook?: boolean;
};

export type CreateWebhookBody = TestConnectionBody & {
  create: boolean;
};

export type CreateWebhookResponse = {
  ok: boolean;
  forge_type?: string;
  created?: boolean;
  updated?: boolean;
  manual?: boolean;
  hook_id?: number;
  webhook: WebhookPreview;
  integration: IntegrationPublic;
  delivery_url: string;
  instance_id?: number;
  hint?: string;
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

export type Organization = {
  id: number;
  instance_id: number;
  external_id: number;
  name: string;
  full_name: string;
  avatar_url?: string;
  forge_type?: string;
  instance_name?: string;
};

export type SearchResult = {
  repositories: Repository[];
  organizations: Organization[];
  pull_requests: PullRequest[];
  workflow_runs: WorkflowRun[];
  attention: AttentionItem[];
};

/** Encode inventory filter: "all" | forge type | "instance:{id}". */
export function forgeListParam(filter?: string): string {
  const raw = (filter || "").trim();
  if (!raw || raw === "all") return "";
  if (raw.startsWith("instance:")) {
    const id = raw.slice("instance:".length);
    if (!id || Number(id) <= 0) return "";
    return `&instance_id=${encodeURIComponent(id)}`;
  }
  return `&forge_type=${encodeURIComponent(raw.toLowerCase())}`;
}

export const api = {
  uiConfig: async () => {
    const cfg = await request<UIConfig>("/api/v1/ui-config");
    if (cfg.csrf_token) setCsrfToken(cfg.csrf_token);
    return cfg;
  },
  me: async () => {
    const base = (typeof window !== "undefined" && window.__GITSEER_BASE__) || "";
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
  login: async (username: string, password: string) => {
    const out = await request<{ user: User; csrf_token?: string }>("/api/v1/auth/bootstrap/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    });
    if (out.csrf_token) setCsrfToken(out.csrf_token);
    return out;
  },
  claimBootstrap: async (username: string, password: string) => {
    const out = await request<{ user: User; csrf_token?: string }>("/api/v1/auth/bootstrap/claim", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    });
    if (out.csrf_token) setCsrfToken(out.csrf_token);
    return out;
  },
  elevateBootstrap: async (password: string) => {
    return request<{ ok: boolean; bootstrap_elevated_until: string; message: string }>(
      "/api/v1/auth/bootstrap/elevate",
      {
        method: "POST",
        body: JSON.stringify({ password }),
      },
    );
  },
  setBootstrapPassword: async (currentPassword: string, newPassword: string) => {
    return request<{ status: string }>("/api/v1/auth/bootstrap/password", {
      method: "POST",
      body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
    });
  },
  logout: () => request<{ status: string }>("/api/v1/auth/logout", { method: "POST" }),
  users: () => request<{ users: ACLUser[] }>("/api/v1/users"),
  userAccess: (userId: number, instanceId: number) =>
    request<{ user_id: number; instance_id: number; repo_ids: number[] }>(
      `/api/v1/users/${userId}/access?instance_id=${instanceId}`,
    ),
  putUserAccess: (userId: number, instanceId: number, repoIds: number[]) =>
    request<{ user_id: number; instance_id: number; repo_ids: number[]; forge_type?: string }>(
      `/api/v1/users/${userId}/access`,
      {
        method: "PUT",
        body: JSON.stringify({ instance_id: instanceId, repo_ids: repoIds }),
      },
    ),
  summary: (days = 0, scope?: DashboardScope) => {
    const params = new URLSearchParams({ days: String(days) });
    if (scope?.org_id && scope.org_id > 0) params.set("org_id", String(scope.org_id));
    if (scope?.owner) params.set("owner", scope.owner);
    if (scope?.team) params.set("team", scope.team);
    if (scope?.instance_id && scope.instance_id > 0) params.set("instance_id", String(scope.instance_id));
    else if (scope?.forge_type) params.set("forge_type", scope.forge_type);
    return request<Summary>(`/api/v1/summary?${params}`);
  },
  stats: (days = 0, section?: StatsSection, scope?: DashboardScope) => {
    const params = new URLSearchParams({ days: String(days) });
    if (section) params.set("section", section);
    if (scope?.org_id && scope.org_id > 0) params.set("org_id", String(scope.org_id));
    if (scope?.owner) params.set("owner", scope.owner);
    if (scope?.team) params.set("team", scope.team);
    if (scope?.instance_id && scope.instance_id > 0) params.set("instance_id", String(scope.instance_id));
    else if (scope?.forge_type) params.set("forge_type", scope.forge_type);
    return request<StatsReport>(`/api/v1/stats?${params}`);
  },
  organizations: (q = "") =>
    request<{ items: Organization[] }>(
      `/api/v1/organizations?limit=100&q=${encodeURIComponent(q)}`,
    ),
  runnerUtilization: (days = 7) =>
    request<RunnerUtilizationReport>(`/api/v1/runners/utilization?days=${days}`),
  flakyJobs: (days = 14) => request<{ items: FlakyJob[]; days: number }>(`/api/v1/flaky-jobs?days=${days}`),
  releases: (days = 30) => request<{ items: ReleaseRun[]; days: number }>(`/api/v1/releases?days=${days}`),
  wallboardTokens: (all = false) =>
    request<{ items: WallboardToken[] }>(`/api/v1/wallboard/tokens${all ? "?all=1" : ""}`),
  createWallboardToken: (name: string) =>
    request<{ token: WallboardToken; secret: string; warning: string }>("/api/v1/wallboard/tokens", {
      method: "POST",
      body: JSON.stringify({ name }),
    }),
  revokeWallboardToken: (id: number) =>
    request<{ ok: boolean }>(`/api/v1/wallboard/tokens/${id}`, { method: "DELETE" }),
  wallboardSnapshot: (token: string) => {
    const base = (typeof window !== "undefined" && window.__GITSEER_BASE__) || "";
    return fetch(`${base}/api/v1/wallboard/snapshot`, {
      headers: { Authorization: `Bearer ${token}` },
    }).then(async (res) => {
      if (!res.ok) {
        let msg = res.statusText;
        try {
          const body = await res.json();
          msg = body.error || msg;
        } catch {
          /* ignore */
        }
        throw new Error(msg || "wallboard failed");
      }
      return res.json() as Promise<WallboardSnapshot>;
    });
  },
  systemStatus: () => request<SystemStatus>("/api/v1/system/status"),
  notificationSettings: () =>
    request<{ settings: NotificationSettings }>("/api/v1/notifications/settings"),
  updateNotificationSettings: (body: NotificationSettingsPatch) =>
    request<{ settings: NotificationSettings }>("/api/v1/notifications/settings", {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  sendTestNotification: () =>
    request<{ queued: number }>("/api/v1/notifications/test", { method: "POST" }),
  alertPrefs: () => request<{ prefs: AlertPrefs }>("/api/v1/alerts/prefs"),
  updateAlertPrefs: (body: AlertPrefsPatch) =>
    request<{ prefs: AlertPrefs }>("/api/v1/alerts/prefs", {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  pushSubscribe: (body: { endpoint: string; keys: { p256dh: string; auth: string } }) =>
    request<{ id: number }>("/api/v1/alerts/push/subscribe", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  pushUnsubscribe: (body: { endpoint: string }) =>
    request<{ ok: boolean }>("/api/v1/alerts/push/unsubscribe", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  sendTestBrowserAlert: () =>
    request<{ sent: number }>("/api/v1/alerts/push/test", { method: "POST" }),
  purgeRetention: () =>
    request<PurgeRetentionResponse>("/api/v1/admin/purge-retention", { method: "POST" }),
  downloadGiteaUISnippets: async (instanceId: number, format: "zip" | "text" = "zip") => {
    const base = (typeof window !== "undefined" && window.__GITSEER_BASE__) || "";
    const res = await fetch(
      `${base}/api/v1/instances/${instanceId}/gitea-ui-snippets?format=${encodeURIComponent(format)}`,
      { credentials: "include" },
    );
    if (!res.ok) {
      let msg = res.statusText;
      try {
        const body = await res.json();
        msg = body.error || msg;
      } catch {
        /* ignore */
      }
      throw new Error(msg || "download failed");
    }
    const blob = await res.blob();
    const cd = res.headers.get("Content-Disposition") || "";
    const match = /filename="?([^";]+)"?/i.exec(cd);
    const filename =
      match?.[1] ||
      (format === "text" ? `gitseer-gitea-ui-${instanceId}.txt` : `gitseer-gitea-ui-${instanceId}.zip`);
    return { blob, filename };
  },
  repositories: (q = "", forgeType: string = "all") =>
    request<{ items: Repository[]; total: number }>(
      `/api/v1/repositories?limit=100&q=${encodeURIComponent(q)}${forgeListParam(forgeType)}`,
    ),
  repository: (owner: string, repo: string, instanceId?: number) => {
    const q = instanceId && instanceId > 0 ? `?instance_id=${instanceId}` : "";
    return request<Repository>(
      `/api/v1/repositories/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}${q}`,
    );
  },
  repositoryHealth: (owner: string, repo: string, instanceId?: number, days = 7) => {
    const params = new URLSearchParams();
    params.set("days", String(days));
    if (instanceId && instanceId > 0) params.set("instance_id", String(instanceId));
    return request<{ health: RepoHealth }>(
      `/api/v1/repositories/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/health?${params}`,
    );
  },
  repositoryFailureClusters: (owner: string, repo: string, instanceId?: number, days = 7) => {
    const params = new URLSearchParams();
    params.set("days", String(days));
    if (instanceId && instanceId > 0) params.set("instance_id", String(instanceId));
    return request<{ items: FailureCluster[]; days: number }>(
      `/api/v1/repositories/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/failure-clusters?${params}`,
    );
  },
  repositoryFlakyJobs: (owner: string, repo: string, instanceId?: number, days = 14) => {
    const params = new URLSearchParams();
    params.set("days", String(days));
    if (instanceId && instanceId > 0) params.set("instance_id", String(instanceId));
    return request<{ items: FlakyJob[]; days: number; repo_id: number }>(
      `/api/v1/repositories/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/flaky-jobs?${params}`,
    );
  },
  attentionLogSnippet: (id: number, maxBytes?: number) => {
    const q = maxBytes && maxBytes > 0 ? `?max_bytes=${maxBytes}` : "";
    return request<AttentionLogSnippet>(`/api/v1/attention/${id}/log-snippet${q}`);
  },
  pullRequests: (q = "", forgeType: string = "all") =>
    request<{ items: PullRequest[]; total: number }>(
      `/api/v1/pull-requests?state=open&limit=100&q=${encodeURIComponent(q)}${forgeListParam(forgeType)}`,
    ),
  workflowRuns: (q = "", forgeType: string = "all") =>
    request<{ items: WorkflowRun[]; total: number }>(
      `/api/v1/workflow-runs?limit=100&q=${encodeURIComponent(q)}${forgeListParam(forgeType)}`,
    ),
  activeWorkflowRuns: () =>
    request<{ items: ActiveRunItem[]; total: number }>("/api/v1/workflow-runs/active"),
  workflowRun: (id: number) =>
    request<{ run: WorkflowRun; jobs: Job[]; graph: WorkflowNode[] | null; graph_error?: string }>(
      `/api/v1/workflow-runs/${id}`,
    ),
  rerunWorkflowRun: (id: number) =>
    request<{ ok: boolean; operation: string; run_id: number; used_service_pat?: boolean }>(
      `/api/v1/workflow-runs/${id}/rerun`,
      { method: "POST" },
    ),
  cancelWorkflowRun: (id: number) =>
    request<{ ok: boolean; operation: string; run_id: number; used_service_pat?: boolean }>(
      `/api/v1/workflow-runs/${id}/cancel`,
      { method: "POST" },
    ),
  attention: (q = "", forgeType: string = "all") =>
    request<{ items: AttentionItem[]; total: number }>(
      `/api/v1/attention?limit=100&q=${encodeURIComponent(q)}${forgeListParam(forgeType)}`,
    ),
  inbox: (opts?: { q?: string; reason?: string; forgeType?: string }) => {
    const q = opts?.q ?? "";
    const reason = opts?.reason ? `&reason=${encodeURIComponent(opts.reason)}` : "";
    return request<{ items: InboxItem[]; total: number }>(
      `/api/v1/inbox?limit=100&q=${encodeURIComponent(q)}${reason}${forgeListParam(opts?.forgeType ?? "all")}`,
    );
  },
  savedFilters: () => request<{ items: SavedFilter[] }>("/api/v1/saved-filters"),
  createSavedFilter: (name: string, query: SavedFilterQuery) =>
    request<SavedFilter>("/api/v1/saved-filters", {
      method: "POST",
      body: JSON.stringify({ name, query }),
    }),
  updateSavedFilter: (id: number, name: string, query: SavedFilterQuery) =>
    request<SavedFilter>(`/api/v1/saved-filters/${id}`, {
      method: "PUT",
      body: JSON.stringify({ name, query }),
    }),
  deleteSavedFilter: (id: number) =>
    request<{ ok: boolean }>(`/api/v1/saved-filters/${id}`, { method: "DELETE" }),
  muteAttention: (id: number, until: AttentionMuteUntil, reason = "") =>
    request<{ mute: { id: number } }>(`/api/v1/attention/${id}/mute`, {
      method: "POST",
      body: JSON.stringify({ until, reason }),
    }),
  unmuteAttention: (id: number) =>
    request<{ ok: boolean }>(`/api/v1/attention/${id}/mute`, { method: "DELETE" }),
  attentionRuleOverrides: () =>
    request<{
      overrides: AttentionRuleOverride[];
      defaults: AttentionRuleDefault[];
      editable: boolean;
    }>("/api/v1/attention/rule-overrides"),
  updateAttentionRuleOverrides: (overrides: AttentionRuleOverride[]) =>
    request<{ overrides: AttentionRuleOverride[] }>("/api/v1/attention/rule-overrides", {
      method: "PUT",
      body: JSON.stringify({ overrides }),
    }),
  search: (q: string) =>
    request<SearchResult>(`/api/v1/search?q=${encodeURIComponent(q)}`),
  syncRepos: () => request<unknown>("/api/v1/setup/sync-repos", { method: "POST" }),
  encryptionStatus: () => request<EncryptionStatus>("/api/v1/setup/encryption"),
  setEncryptionKey: (body: { generate?: boolean; encryption_key?: string }) =>
    request<SetEncryptionResponse>("/api/v1/setup/encryption", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  settings: () => request<SettingsResponse>("/api/v1/settings"),
  updateSettings: (body: UpdateSettingsBody) =>
    request<SettingsResponse>("/api/v1/settings", {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  instances: () => request<{ items: InstancePublic[] }>("/api/v1/instances"),
  createInstance: (body: InstancePatch) =>
    request<InstancePublic>("/api/v1/instances", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  updateInstance: (id: number, body: InstancePatch) =>
    request<InstancePublic>(`/api/v1/instances/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  deleteInstance: (id: number) =>
    request<void>(`/api/v1/instances/${id}`, { method: "DELETE" }),
  ensureWebhook: (id: number, body?: { org?: string; repo?: string }) =>
    request<{
      ok: boolean;
      created?: boolean;
      updated?: boolean;
      manual?: boolean;
      delivery_url?: string;
      hint?: string;
      hook_id?: number;
    }>(`/api/v1/instances/${id}/ensure-webhook`, {
      method: "POST",
      body: JSON.stringify(body ?? {}),
    }),
  verifyWebhook: (id: number, body?: { confirm?: boolean }) =>
    request<{
      ok: boolean;
      verified?: boolean;
      pending?: boolean;
      mode?: string;
      delivery_url?: string;
      hint?: string;
    }>(`/api/v1/instances/${id}/verify-webhook`, {
      method: "POST",
      body: JSON.stringify(body ?? {}),
    }),
  testConnection: (body?: TestConnectionBody) =>
    request<TestConnectionResponse>("/api/v1/setup/test-connection", {
      method: "POST",
      body: JSON.stringify(body ?? {}),
    }),
  checkGiteaURL: async (body: CheckGiteaURLBody) => {
    const base = (typeof window !== "undefined" && window.__GITSEER_BASE__) || "";
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
    const base = (typeof window !== "undefined" && window.__GITSEER_BASE__) || "";
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

export function forgeLabel(forgeType?: string | null): string {
  switch ((forgeType || "").toLowerCase()) {
    case "github":
      return "GitHub";
    case "gitea":
      return "Gitea";
    case "gitlab":
      return "GitLab";
    case "bitbucket":
      return "Bitbucket";
    case "forgejo":
      return "Forgejo";
    default:
      return forgeType ? forgeType : "Forge";
  }
}

export function openOnForgeLabel(forgeType?: string | null): string {
  const name = forgeLabel(forgeType);
  if (name === "Forge") return "Open on Forge →";
  return `Open on ${name} →`;
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

export function reviewLabel(state?: string) {
  switch ((state || "").toLowerCase()) {
    case "approved":
      return "approved";
    case "changes_requested":
      return "changes requested";
    case "":
      return "";
    default:
      return state || "";
  }
}

export function reviewBadgeClass(state?: string) {
  switch ((state || "").toLowerCase()) {
    case "approved":
      return "success";
    case "changes_requested":
      return "failure";
    default:
      return "";
  }
}
