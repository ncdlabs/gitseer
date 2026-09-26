/** Web UI URL to create a classic PAT, derived from the configured GitHub / GHE base. */
export function githubPATCreateURL(rawBaseURL: string): string {
  // repo = sync inventory; admin:repo_hook / admin:org_hook = create/manage delivery hooks.
  const scopes = "repo,admin:repo_hook,admin:org_hook";
  const fallback = `https://github.com/settings/tokens/new?description=GitSeer&scopes=${scopes}`;
  const origin = githubWebOrigin(rawBaseURL);
  if (!origin) return fallback;
  return `${origin}/settings/tokens/new?description=GitSeer&scopes=${scopes}`;
}

/** Web UI URL to create a fine-grained PAT with GitSeer-oriented permissions prefilled. */
export function githubFineGrainedPATCreateURL(rawBaseURL: string): string {
  const params = new URLSearchParams({
    name: "GitSeer",
    contents: "read",
    pull_requests: "read",
    actions: "read",
    statuses: "read",
    repository_hooks: "write",
    organization_hooks: "write",
  });
  const fallback = `https://github.com/settings/personal-access-tokens/new?${params}`;
  const origin = githubWebOrigin(rawBaseURL);
  if (!origin) return fallback;
  return `${origin}/settings/personal-access-tokens/new?${params}`;
}

/**
 * Entry for organization webhook settings (Settings → Webhooks).
 * Without a known org slug, opens the organizations list on the forge host.
 */
export function githubOrgWebhookSettingsURL(rawBaseURL: string): string {
  const origin = githubWebOrigin(rawBaseURL) || "https://github.com";
  return `${origin}/settings/organizations`;
}

/**
 * Entry for repository webhook settings (Settings → Webhooks).
 * Without a known owner/repo, opens the repositories list on the forge host.
 */
export function githubRepoWebhookSettingsURL(rawBaseURL: string): string {
  const origin = githubWebOrigin(rawBaseURL) || "https://github.com";
  return `${origin}/settings/repositories`;
}

/** github.com / GHE web origin, or null when the input cannot be parsed. */
function githubWebOrigin(rawBaseURL: string): string | null {
  const trimmed = rawBaseURL.trim() || "https://github.com";
  let u: URL;
  try {
    u = new URL(trimmed.includes("://") ? trimmed : `https://${trimmed}`);
  } catch {
    return null;
  }
  if (!u.hostname) return null;

  const host = u.hostname.toLowerCase();
  if (host === "github.com" || host === "www.github.com" || host === "api.github.com") {
    return "https://github.com";
  }

  return `${u.protocol}//${u.host}`;
}
