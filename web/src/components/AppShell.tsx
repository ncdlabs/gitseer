import { useEffect, useId, useRef, useState, type PropsWithChildren } from "react";
import { Link, NavLink } from "react-router-dom";
import type { User } from "../api/client";
import { useIsTerminalTheme } from "../hooks/useIsTerminalTheme";
import type { Theme } from "../hooks/useTheme";
import { ActionsStatusFlyout } from "./ActionsStatusFlyout";
import { Brand } from "./Brand";
import { Glyph, type ShellIconName } from "./Glyph";
import { HeaderSearch } from "./HeaderSearch";
import { ThemePicker } from "./ThemePicker";

type Props = {
  user: User;
  theme: Theme;
  onTheme: (t: Theme) => void;
  onLogout: () => void;
  onSync?: () => void;
  syncing?: boolean;
};

const navigation: Array<{ to: string; label: string; icon: ShellIconName; end?: boolean }> = [
  { to: "/", label: "Dashboard", icon: "dashboard", end: true },
  { to: "/attention", label: "Attention", icon: "attention" },
  { to: "/pull-requests", label: "Pull Requests", icon: "pullRequests" },
  { to: "/pipelines", label: "Pipelines", icon: "pipelines" },
  { to: "/repositories", label: "Repositories", icon: "repositories" },
];

export function AppShell({ user, theme, onTheme, onLogout, onSync, syncing, children }: PropsWithChildren<Props>) {
  const [isRailCollapsed, setRailCollapsed] = useState(false);
  const [userMenuOpen, setUserMenuOpen] = useState(false);
  const userMenuRef = useRef<HTMLDivElement>(null);
  const userMenuId = useId();
  const initials = user.login.slice(0, 2).toUpperCase();
  const terminal = useIsTerminalTheme();

  useEffect(() => {
    if (!userMenuOpen) return;
    const onPointerDown = (event: MouseEvent) => {
      if (!userMenuRef.current?.contains(event.target as Node)) {
        setUserMenuOpen(false);
      }
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") setUserMenuOpen(false);
    };
    document.addEventListener("mousedown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [userMenuOpen]);

  return (
    <div className={`app-shell${isRailCollapsed ? " app-shell--rail-collapsed" : ""}`}>
      <aside className="sidebar" aria-label="Main navigation">
        <Link className="brand" to="/" aria-label="Gitea Lens dashboard">
          {isRailCollapsed ? (
            <Brand variant="mark" className="brand__img brand__img--mark" />
          ) : (
            <>
              <Brand variant="mark" className="brand__img brand__img--mark" alt="" />
              <span className="brand__wordmark" aria-hidden="true">
                <span className="brand__wordmark-gitea">Gitea</span>{" "}
                <span className="brand__wordmark-lens">Lens</span>
              </span>
            </>
          )}
        </Link>
        <button className="rail-toggle" type="button" onClick={() => setRailCollapsed((value) => !value)} aria-label={isRailCollapsed ? "Expand navigation" : "Collapse navigation"} title={isRailCollapsed ? "Expand navigation" : "Collapse navigation"}>
          <Glyph name="menu" />
        </button>
        <nav className="nav">
          <span className="nav__label">{terminal ? "// workspace" : "Workspace"}</span>
          {navigation.map(({ to, label, icon, end }) => (
            <NavLink key={to} to={to} end={end} title={label}>
              <Glyph name={icon} />
              <span>{label}</span>
            </NavLink>
          ))}
        </nav>
        <div className="sidebar__footer">
          <ActionsStatusFlyout />
          <div className={`user-menu${userMenuOpen ? " is-open" : ""}`} ref={userMenuRef}>
            {userMenuOpen && (
              <div className="user-menu__flyout" id={userMenuId} role="menu" aria-label="Account menu">
                <div className="user-menu__section">
                  <span className="user-menu__section-label" id={`${userMenuId}-themes`}>Themes</span>
                  <ThemePicker theme={theme} onTheme={onTheme} />
                </div>
                <div className="user-menu__divider" role="separator" />
                <NavLink
                  className="user-menu__item"
                  to="/settings"
                  role="menuitem"
                  onClick={() => setUserMenuOpen(false)}
                >
                  <Glyph name="settings" />
                  <span>Settings</span>
                </NavLink>
                <button className="user-menu__item user-menu__item--danger" type="button" role="menuitem" onClick={onLogout}>
                  <Glyph name="logout" />
                  <span>Log Out</span>
                </button>
              </div>
            )}
            <button
              className="user-menu__trigger"
              type="button"
              aria-haspopup="menu"
              aria-expanded={userMenuOpen}
              aria-controls={userMenuOpen ? userMenuId : undefined}
              title={user.login}
              onClick={() => setUserMenuOpen((open) => !open)}
            >
              <span className="user-chip__avatar">{terminal ? `[${initials}]` : initials}</span>
              <span className="user-menu__name">{user.login}</span>
              <span className="user-menu__chevron"><Glyph name="chevron" /></span>
            </button>
          </div>
        </div>
      </aside>
      <main className="content">
        <header className="app-header">
          <div className="app-header__actions">
            {user.is_bootstrap_admin && onSync && (
              <button
                className={`app-header__sync${syncing ? " is-syncing" : ""}`}
                disabled={syncing}
                onClick={onSync}
                type="button"
                aria-label={syncing ? "Syncing from Gitea" : "Sync Now"}
                title={
                  syncing
                    ? "Syncing repositories, pull requests, and workflow runs from Gitea…"
                    : "Sync repositories, pull requests, and workflow runs from Gitea"
                }
              >
                <Glyph name="sync" />
              </button>
            )}
            <HeaderSearch />
          </div>
        </header>
        <div className="page-content">{children}</div>
      </main>
    </div>
  );
}
