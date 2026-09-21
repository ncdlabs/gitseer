import type { ReactNode, SVGProps } from "react";
import { useIsTerminalTheme } from "../hooks/useIsTerminalTheme";

export type ShellIconName =
  | "dashboard"
  | "attention"
  | "pullRequests"
  | "pipelines"
  | "repositories"
  | "settings"
  | "menu"
  | "search"
  | "submit"
  | "sync"
  | "logout"
  | "chevron"
  | "popOut"
  | "filter"
  | "table"
  | "cards"
  | "expandAll"
  | "collapseAll"
  | "info"
  | "eye"
  | "eyeOff"
  | "ok"
  | "fail"
  | "warn"
  | "skip"
  | "running"
  | "pending";

const ASCII: Record<ShellIconName, string> = {
  dashboard: "[#]",
  attention: "[!]",
  pullRequests: "[PR]",
  pipelines: "[|>]",
  repositories: "[R]",
  settings: "[=]",
  menu: "[=]",
  search: "[?]",
  submit: "->",
  sync: "[~]",
  logout: "[>]",
  chevron: "^",
  popOut: "[^]",
  filter: "[/]",
  table: "[T]",
  cards: "[::]",
  expandAll: "[vv]",
  collapseAll: "[^^]",
  info: "(?)",
  eye: "[o]",
  eyeOff: "[-]",
  ok: "[+]",
  fail: "[x]",
  warn: "[!]",
  skip: "[-]",
  running: "[~]",
  pending: "[.]",
};

type SvgCommon = SVGProps<SVGSVGElement>;

function svgCommon(extra?: SvgCommon): SvgCommon {
  return {
    fill: "none",
    stroke: "currentColor",
    strokeWidth: 1.8,
    strokeLinecap: "round",
    strokeLinejoin: "round",
    "aria-hidden": true,
    ...extra,
  };
}

function SvgIcon({ name, className }: { name: ShellIconName; className?: string }) {
  const common = svgCommon({ className: className || "shell-icon", viewBox: "0 0 24 24" });
  const paths: Record<ShellIconName, ReactNode> = {
    dashboard: (
      <>
        <rect x="3" y="3" width="7" height="7" rx="1" />
        <rect x="14" y="3" width="7" height="7" rx="1" />
        <rect x="3" y="14" width="7" height="7" rx="1" />
        <rect x="14" y="14" width="7" height="7" rx="1" />
      </>
    ),
    attention: (
      <>
        <path d="M12 3 3.7 19h16.6L12 3Z" />
        <path d="M12 9v4M12 16h.01" />
      </>
    ),
    pullRequests: (
      <>
        <circle cx="6" cy="5" r="2" />
        <circle cx="18" cy="19" r="2" />
        <path d="M6 7v10a2 2 0 0 0 2 2h8M14 5h4v8" />
        <path d="m13 9 5-4-5-4" />
      </>
    ),
    pipelines: (
      <>
        <rect x="3" y="4" width="7" height="6" rx="1" />
        <rect x="14" y="14" width="7" height="6" rx="1" />
        <path d="M10 7h3a2 2 0 0 1 2 2v5" />
      </>
    ),
    repositories: (
      <>
        <path d="M4 5.5A2.5 2.5 0 0 1 6.5 3H20v15.5A2.5 2.5 0 0 0 17.5 16H4V5.5Z" />
        <path d="M4 16v2.5A2.5 2.5 0 0 0 6.5 21H20M8 7h8" />
      </>
    ),
    settings: (
      <>
        <circle cx="12" cy="12" r="3" />
        <path d="M19.4 15a1.7 1.7 0 0 0 .34 1.88l.06.06-2.14 2.14-.06-.06a1.7 1.7 0 0 0-1.88-.34 1.7 1.7 0 0 0-1.03 1.56V20.3h-3.03v-.08A1.7 1.7 0 0 0 10.63 18.7a1.7 1.7 0 0 0-1.88.34l-.06.06-2.14-2.14.06-.06A1.7 1.7 0 0 0 6.95 15a1.7 1.7 0 0 0-1.56-1.03H5.3v-3.03h.08A1.7 1.7 0 0 0 6.9 9.91a1.7 1.7 0 0 0-.34-1.88L6.5 7.97l2.14-2.14.06.06a1.7 1.7 0 0 0 1.88.34 1.7 1.7 0 0 0 1.03-1.56V4.6h3.03v.08a1.7 1.7 0 0 0 1.03 1.56 1.7 1.7 0 0 0 1.88-.34l.06-.06 2.14 2.14-.06.06a1.7 1.7 0 0 0-.34 1.88 1.7 1.7 0 0 0 1.56 1.03h.08v3.03h-.08A1.7 1.7 0 0 0 19.4 15Z" />
      </>
    ),
    menu: <path d="M4 7h16M4 12h16M4 17h16" />,
    search: (
      <>
        <circle cx="11" cy="11" r="6" />
        <path d="m20 20-4.2-4.2" />
      </>
    ),
    submit: (
      <>
        <path d="M5 12h14" />
        <path d="m13 6 6 6-6 6" />
      </>
    ),
    sync: (
      <>
        <path d="M20 7v5h-5" />
        <path d="M4 17v-5h5" />
        <path d="M6.3 9A7 7 0 0 1 18 7M17.7 15A7 7 0 0 1 6 17" />
      </>
    ),
    logout: (
      <>
        <path d="M10 5H5v14h5" />
        <path d="m14 8 4 4-4 4M18 12H9" />
      </>
    ),
    chevron: <path d="m8 14 4-4 4 4" />,
    popOut: (
      <>
        <path d="M14 4h6v6" />
        <path d="M10 14 20 4" />
        <path d="M20 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1h5" />
      </>
    ),
    filter: <path d="M4 5h16l-6.5 7.5V19l-3 1.5v-8L4 5z" strokeWidth={1.75} />,
    table: (
      <>
        <rect x="3" y="4" width="18" height="16" rx="2" strokeWidth={1.75} />
        <path d="M3 10h18M3 16h18M9 4v16" strokeWidth={1.75} />
      </>
    ),
    cards: (
      <>
        <rect x="3" y="3" width="8" height="8" rx="1.5" strokeWidth={1.75} />
        <rect x="13" y="3" width="8" height="8" rx="1.5" strokeWidth={1.75} />
        <rect x="3" y="13" width="8" height="8" rx="1.5" strokeWidth={1.75} />
        <rect x="13" y="13" width="8" height="8" rx="1.5" strokeWidth={1.75} />
      </>
    ),
    expandAll: (
      <>
        <path d="m7 8 5 5 5-5" strokeWidth={1.75} />
        <path d="m7 13 5 5 5-5" strokeWidth={1.75} />
      </>
    ),
    collapseAll: (
      <>
        <path d="m7 11 5-5 5 5" strokeWidth={1.75} />
        <path d="m7 16 5-5 5 5" strokeWidth={1.75} />
      </>
    ),
    info: (
      <>
        <circle cx="12" cy="12" r="9" strokeWidth={1.5} />
        <path d="M12 11v5M12 8h.01" strokeWidth={1.75} />
      </>
    ),
    eye: (
      <>
        <path
          strokeWidth={1.75}
          d="M2 12c1.5-3.6 5-7 10-7s8.5 3.4 10 7c-1.5 3.6-5 7-10 7s-8.5-3.4-10-7z"
        />
        <circle cx="12" cy="12" r="3" strokeWidth={1.75} />
      </>
    ),
    eyeOff: (
      <path
        strokeWidth={1.75}
        d="M3 3l18 18M10.6 10.6a2 2 0 002.8 2.8M9.9 5.1A9.8 9.8 0 0112 5c5 0 8.5 3.4 10 7-.4 1-1 2-1.8 2.9M6.1 6.1C4.2 7.4 2.8 9.1 2 12c1.5 3.6 5 7 10 7a9.7 9.7 0 005.1-1.4"
      />
    ),
    ok: <path d="M5 12.5 9.5 17 19 7" strokeWidth={2} />,
    fail: <path d="M6 6l12 12M18 6 6 18" strokeWidth={2} />,
    warn: <path d="M12 8v4M12 16h.01" strokeWidth={2} />,
    skip: <path d="M6 12h12" strokeWidth={2} />,
    running: <path d="M5 12h14M12 5v14" strokeWidth={2} />,
    pending: <circle cx="12" cy="12" r="2" strokeWidth={2} />,
  };

  return <svg {...common}>{paths[name]}</svg>;
}

type Props = {
  name: ShellIconName;
  className?: string;
};

/** SVG icon that switches to an ANSI/ASCII glyph under the terminal theme. */
export function Glyph({ name, className }: Props) {
  const terminal = useIsTerminalTheme();
  if (terminal) {
    return (
      <span className={className ? `ascii-glyph ${className}` : "ascii-glyph"} aria-hidden="true">
        {ASCII[name]}
      </span>
    );
  }
  return <SvgIcon name={name} className={className} />;
}
