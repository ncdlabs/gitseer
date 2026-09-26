/** Web UI URL to manage / create Gitea access tokens, derived from the configured base. */
export function giteaPATSettingsURL(rawBaseURL: string): string {
  const fallback = "/user/settings/applications";
  const trimmed = rawBaseURL.trim();
  if (!trimmed) return fallback;
  let u: URL;
  try {
    u = new URL(trimmed.includes("://") ? trimmed : `https://${trimmed}`);
  } catch {
    return fallback;
  }
  if (!u.hostname) return fallback;
  return `${u.protocol}//${u.host}/user/settings/applications`;
}
