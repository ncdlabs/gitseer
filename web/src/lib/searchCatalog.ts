import type { Theme } from "../hooks/useTheme";
import { THEME_OPTIONS } from "../hooks/useTheme";

export type SearchCatalogKind = "page" | "command";

export type SearchCatalogItem = {
  id: string;
  kind: SearchCatalogKind;
  label: string;
  keywords: string[];
  /** In-app path for pages / settings deep-links. */
  to?: string;
  /** Command action id handled by the shell. */
  action?:
    | "sync"
    | "open-actions"
    | "popout-actions"
    | "logout"
    | { type: "theme"; theme: Theme };
};

const PAGES: SearchCatalogItem[] = [
  { id: "page-dashboard", kind: "page", label: "Dashboard", to: "/", keywords: ["home", "summary", "dashboard"] },
  {
    id: "page-attention",
    kind: "page",
    label: "Attention",
    to: "/attention",
    keywords: ["attention", "alerts", "queue", "critical"],
  },
  {
    id: "page-inbox",
    kind: "page",
    label: "Inbox",
    to: "/inbox",
    keywords: ["inbox", "personal", "mine", "review", "my prs"],
  },
  {
    id: "page-pull-requests",
    kind: "page",
    label: "Pull Requests",
    to: "/pull-requests",
    keywords: ["pr", "prs", "pull", "requests", "merge"],
  },
  {
    id: "page-pipelines",
    kind: "page",
    label: "Pipelines",
    to: "/pipelines",
    keywords: ["pipelines", "actions", "workflows", "ci", "runs"],
  },
  {
    id: "page-repositories",
    kind: "page",
    label: "Repositories",
    to: "/repositories",
    keywords: ["repos", "repositories", "code"],
  },
  {
    id: "page-settings",
    kind: "page",
    label: "Settings",
    to: "/settings",
    keywords: ["settings", "preferences", "config"],
  },
];

const COMMANDS: SearchCatalogItem[] = [
  {
    id: "cmd-sync",
    kind: "command",
    label: "Sync Now",
    action: "sync",
    keywords: ["sync", "refresh", "reconcile", "pull"],
  },
  {
    id: "cmd-open-actions",
    kind: "command",
    label: "Open Active Actions",
    action: "open-actions",
    keywords: ["actions", "active", "running", "flyout"],
  },
  {
    id: "cmd-popout-actions",
    kind: "command",
    label: "Pop Out Active Actions",
    action: "popout-actions",
    keywords: ["actions", "popout", "pop out", "window"],
  },
  {
    id: "cmd-inbox-failing",
    kind: "command",
    label: "Inbox · Failing CI",
    to: "/inbox?reason=failing_ci",
    keywords: ["inbox", "ci", "failure", "failing", "my"],
  },
  {
    id: "cmd-inbox-blocked",
    kind: "command",
    label: "Inbox · Blocked On Me",
    to: "/inbox?reason=blocked_on_me",
    keywords: ["inbox", "blocked", "changes requested", "conflict"],
  },
  {
    id: "cmd-inbox-reviews",
    kind: "command",
    label: "Inbox · Review Requests",
    to: "/inbox?reason=requested_reviewer",
    keywords: ["inbox", "review", "requested", "reviewer"],
  },
  {
    id: "cmd-settings-preferences",
    kind: "command",
    label: "Settings · Preferences",
    to: "/settings#preferences",
    keywords: ["settings", "preferences", "retention", "sync history"],
  },
  {
    id: "cmd-settings-integration",
    kind: "command",
    label: "Settings · Integration",
    to: "/settings#integration",
    keywords: ["settings", "integration", "forge", "gitea", "github", "instances"],
  },
  {
    id: "cmd-settings-access",
    kind: "command",
    label: "Settings · Access",
    to: "/settings#access",
    keywords: ["settings", "access", "acl", "grant", "users", "github"],
  },
  {
    id: "cmd-settings-notifications",
    kind: "command",
    label: "Settings · Notifications",
    to: "/settings#notifications",
    keywords: ["settings", "notifications", "smtp", "slack", "discord", "webhook", "digest"],
  },
  {
    id: "cmd-settings-status",
    kind: "command",
    label: "Settings · Status",
    to: "/settings#status",
    keywords: ["settings", "status", "health", "connection"],
  },
  {
    id: "cmd-settings-bootstrap",
    kind: "command",
    label: "Settings · Bootstrap",
    to: "/settings#bootstrap",
    keywords: ["settings", "bootstrap", "password", "elevate", "sudo"],
  },
  ...THEME_OPTIONS.map(
    (opt): SearchCatalogItem => ({
      id: `cmd-theme-${opt.id}`,
      kind: "command",
      label: `Theme · ${opt.label}`,
      action: { type: "theme", theme: opt.id },
      keywords: ["theme", "appearance", opt.id, opt.label.toLowerCase()],
    }),
  ),
  {
    id: "cmd-logout",
    kind: "command",
    label: "Log Out",
    action: "logout",
    keywords: ["logout", "sign out", "exit"],
  },
];

export const SEARCH_CATALOG: SearchCatalogItem[] = [...COMMANDS, ...PAGES];

function normalize(s: string): string {
  return s.trim().toLowerCase();
}

/** Filter pages/commands by substring match on label or keywords. Empty query shows a short default set. */
export function filterSearchCatalog(query: string, opts?: { includeSync?: boolean }): SearchCatalogItem[] {
  const q = normalize(query);
  const includeSync = opts?.includeSync !== false;
  const pool = SEARCH_CATALOG.filter((item) => {
    if (!includeSync && item.action === "sync") return false;
    return true;
  });
  if (!q) {
    return pool.filter((item) => item.kind === "page").slice(0, 6);
  }
  return pool.filter((item) => {
    if (normalize(item.label).includes(q)) return true;
    return item.keywords.some((kw) => normalize(kw).includes(q));
  });
}
