import type { PropsWithChildren } from "react";
import { Link, NavLink } from "react-router-dom";
import type { User } from "../api/client";
import type { Theme } from "../hooks/useTheme";
import { ThemePicker } from "./ThemePicker";

type Props = {
  user: User;
  theme: Theme;
  onTheme: (t: Theme) => void;
  onLogout: () => void;
  onSync?: () => void;
  syncing?: boolean;
};

export function AppShell({ user, theme, onTheme, onLogout, onSync, syncing, children }: PropsWithChildren<Props>) {
  return (
    <div className="app-shell">
      <aside className="sidebar">
        <Link className="brand" to="/" aria-label="Gitea Lens dashboard">
          <span className="brand__name">Gitea</span>
          <span className="brand__mark">Lens</span>
        </Link>
        <nav className="nav">
          <NavLink to="/" end>Dashboard</NavLink>
          <NavLink to="/attention">Attention</NavLink>
          <NavLink to="/pull-requests">Pull Requests</NavLink>
          <NavLink to="/pipelines">Pipelines</NavLink>
          <NavLink to="/repositories">Repositories</NavLink>
          <NavLink to="/settings">Settings</NavLink>
        </nav>
        <div className="sidebar__footer">
          <ThemePicker theme={theme} onTheme={onTheme} />
          {user.is_bootstrap_admin && onSync && (
            <button className="btn primary" disabled={syncing} onClick={onSync} type="button">
              {syncing ? "Syncing…" : "Sync now"}
            </button>
          )}
          <button className="btn" onClick={onLogout} type="button">
            Log out ({user.login})
          </button>
        </div>
      </aside>
      <main className="content">{children}</main>
    </div>
  );
}
