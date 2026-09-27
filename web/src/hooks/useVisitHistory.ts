import { useEffect, useState } from "react";
import { useLocation } from "react-router-dom";

export type VisitEntry = {
  pathname: string;
  search: string;
  hash: string;
};

const STORAGE_KEY = "gitseer-visit-history";

function visitKey(entry: VisitEntry): string {
  return `${entry.pathname}${entry.search}${entry.hash}`;
}

function readStored(): VisitEntry[] {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw) as unknown;
    if (!Array.isArray(parsed)) return [];
    return parsed.filter(
      (item): item is VisitEntry =>
        !!item &&
        typeof item === "object" &&
        typeof (item as VisitEntry).pathname === "string" &&
        typeof (item as VisitEntry).search === "string" &&
        typeof (item as VisitEntry).hash === "string",
    );
  } catch {
    return [];
  }
}

function writeStored(entries: VisitEntry[]) {
  try {
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(entries));
  } catch {
    /* quota / private mode */
  }
}

/** Returns up to `maxPrevious` pages visited before the current location. */
export function useVisitHistory(maxPrevious = 5): VisitEntry[] {
  const location = useLocation();
  const [history, setHistory] = useState<VisitEntry[]>(readStored);

  useEffect(() => {
    const current: VisitEntry = {
      pathname: location.pathname,
      search: location.search,
      hash: location.hash,
    };
    setHistory((prev) => {
      const last = prev[prev.length - 1];
      if (last && visitKey(last) === visitKey(current)) return prev;
      const next = [...prev, current].slice(-(maxPrevious + 1));
      writeStored(next);
      return next;
    });
  }, [location.pathname, location.search, location.hash, maxPrevious]);

  if (history.length <= 1) return [];
  return history.slice(0, -1).slice(-maxPrevious);
}

export function labelForVisit(entry: VisitEntry): string {
  const { pathname, search, hash } = entry;

  if (pathname === "/") return "Dashboard";
  if (pathname === "/inbox") return "Inbox";
  if (pathname === "/attention") return "Attention";
  if (pathname === "/pull-requests") return "Pull Requests";
  if (pathname === "/pipelines") return "Pipelines";
  if (pathname === "/repositories") return "Repositories";
  if (pathname === "/settings") {
    const tab = hash.replace(/^#/, "");
    if (tab === "preferences") return "Settings · Preferences";
    if (tab === "integration") return "Settings · Integration";
    if (tab === "access") return "Settings · Access";
    if (tab === "notifications") return "Settings · Notifications";
    if (tab === "status") return "Settings · Status";
    return "Settings";
  }

  const repoMatch = pathname.match(/^\/repositories\/([^/]+)\/([^/]+)\/?$/);
  if (repoMatch) {
    return `${decodeURIComponent(repoMatch[1])}/${decodeURIComponent(repoMatch[2])}`;
  }

  const pipelineMatch = pathname.match(/^\/pipelines\/([^/]+)\/?$/);
  if (pipelineMatch) {
    const id = decodeURIComponent(pipelineMatch[1]);
    return id.length > 10 ? `Run ${id.slice(0, 8)}…` : `Run ${id}`;
  }

  const base = pathname.replace(/^\//, "") || "Page";
  if (search) return `${base}${search}`;
  return base;
}

export function hrefForVisit(entry: VisitEntry): string {
  return `${entry.pathname}${entry.search}${entry.hash}`;
}
