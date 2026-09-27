import { QueryClient, QueryClientProvider, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { BrowserRouter, Navigate, Route, Routes, useLocation, useNavigate } from "react-router-dom";
import { api, setUnauthorizedHandler, type User } from "./api/client";
import { AppShell } from "./components/AppShell";
import { ConfirmDialog } from "./components/ConfirmDialog";
import { MAIN_WINDOW_NAME, NAV_CHANNEL } from "./components/ActiveActionsPanel";
import { useTheme } from "./hooks/useTheme";
import { prefetchDashboardRanges } from "./lib/prefetchDashboard";
import { AttentionPage } from "./pages/AttentionPage";
import { ActionsPopoutPage } from "./pages/ActionsPopoutPage";
import { LoginPage } from "./pages/LoginPage";
import { DashboardPage } from "./pages/DashboardPage";
import { InboxPage } from "./pages/InboxPage";
import { PipelineDetailPage, PipelinesPage } from "./pages/PipelinesPage";
import { SettingsPage } from "./pages/SettingsPage";
import { SetupWizardPage } from "./pages/SetupWizardPage";
import { PullRequestsPage, RepositoriesPage } from "./pages/RepositoriesPage";
import { RepositoryDetailPage } from "./pages/RepositoryDetailPage";
import { WallboardPage } from "./pages/WallboardPage";
import "./styles/app.css";

const DASHBOARD_REWARM_MS = 1500;

const qc = new QueryClient({
  defaultOptions: {
    queries: {
      retry: (count, err) => {
        if (err instanceof Error && err.name === "UnauthorizedError") return false;
        return count < 1;
      },
    },
  },
});

function invalidateKeys(queryClient: QueryClient, keys: string[]) {
  for (const key of keys) {
    void queryClient.invalidateQueries({ queryKey: [key] });
  }
}

function useRealtimeInvalidation() {
  const queryClient = useQueryClient();
  useEffect(() => {
    const base = window.__GITSEER_BASE__ || "";
    const es = new EventSource(`${base}/api/v1/events`);
    let rewarmTimer: ReturnType<typeof setTimeout> | undefined;
    const scheduleDashboardRewarm = () => {
      if (rewarmTimer) clearTimeout(rewarmTimer);
      rewarmTimer = setTimeout(() => {
        void prefetchDashboardRanges(queryClient);
      }, DASHBOARD_REWARM_MS);
    };
    const onMessage = (event: MessageEvent) => {
      let type = "";
      try {
        const parsed = JSON.parse(String(event.data ?? "")) as { type?: string };
        type = typeof parsed?.type === "string" ? parsed.type : "";
      } catch {
        /* unparseable → default invalidation */
      }
      if (type === "workflow_run" || type === "workflow_job") {
        invalidateKeys(queryClient, ["workflow-runs", "runs", "run", "summary", "stats", "attention", "inbox"]);
        scheduleDashboardRewarm();
        return;
      }
      if (type === "pull_request") {
        invalidateKeys(queryClient, ["prs", "summary", "stats", "attention", "inbox"]);
        scheduleDashboardRewarm();
        return;
      }
      invalidateKeys(queryClient, [
        "workflow-runs",
        "runs",
        "run",
        "summary",
        "stats",
        "attention",
        "inbox",
        "prs",
        "repositories",
      ]);
      scheduleDashboardRewarm();
    };
    es.addEventListener("message", onMessage);
    es.onerror = () => {
      /* browser reconnects; avoid tight loops */
    };
    return () => {
      if (rewarmTimer) clearTimeout(rewarmTimer);
      es.removeEventListener("message", onMessage);
      es.close();
    };
  }, [queryClient]);
}

function useActionsPopoutNavigation() {
  const navigate = useNavigate();
  const location = useLocation();
  const isPopout = location.pathname === "/actions-popout";

  useEffect(() => {
    if (isPopout) return;
    if (!window.name) {
      window.name = MAIN_WINDOW_NAME;
    }
    const go = (path: unknown) => {
      if (typeof path !== "string" || !path.startsWith("/")) return;
      navigate(path);
      window.focus();
    };
    const onMessage = (event: MessageEvent) => {
      if (event.origin !== window.location.origin) return;
      if (event.data?.type !== "gitseer:navigate") return;
      go(event.data.path);
    };
    window.addEventListener("message", onMessage);
    let bc: BroadcastChannel | undefined;
    try {
      bc = new BroadcastChannel(NAV_CHANNEL);
      bc.onmessage = (event) => {
        if (event.data?.type !== "navigate") return;
        go(event.data.path);
      };
    } catch {
      /* BroadcastChannel unavailable */
    }
    return () => {
      window.removeEventListener("message", onMessage);
      bc?.close();
    };
  }, [isPopout, navigate]);
}

function AuthenticatedApp({ user, onLogout }: { user: User; onLogout: () => void }) {
  const giteaTheme =
    user.theme === "light" || user.theme === "dark" || user.theme === "system" ? user.theme : null;
  const { theme, setTheme } = useTheme(giteaTheme);
  const queryClient = useQueryClient();
  const [syncing, setSyncing] = useState(false);
  const [syncError, setSyncError] = useState<string | null>(null);
  useRealtimeInvalidation();
  useActionsPopoutNavigation();

  const settingsQuery = useQuery({
    queryKey: ["settings"],
    queryFn: api.settings,
    enabled: user.is_bootstrap_admin,
  });

  const setupGateLoading = user.is_bootstrap_admin && settingsQuery.isLoading;
  const needsSetup =
    user.is_bootstrap_admin && settingsQuery.isSuccess && settingsQuery.data.setup_completed === false;

  async function syncNow() {
    setSyncing(true);
    try {
      await api.syncRepos();
      await queryClient.invalidateQueries();
    } catch (err) {
      setSyncError(err instanceof Error ? err.message : "Sync failed.");
    } finally {
      setSyncing(false);
    }
  }

  if (setupGateLoading) {
    return <div className="loading">Loading…</div>;
  }

  if (user.is_bootstrap_admin && settingsQuery.isError) {
    return <div className="error">Could not load setup status.</div>;
  }

  if (needsSetup) {
    return (
      <Routes>
        <Route
          path="/setup"
          element={
            <SetupWizardPage
              onLogout={onLogout}
              onComplete={() => {
                void queryClient.invalidateQueries({ queryKey: ["settings"] });
              }}
            />
          }
        />
        <Route path="*" element={<Navigate to="/setup" replace />} />
      </Routes>
    );
  }

  return (
    <>
      <Routes>
        <Route path="/actions-popout" element={<ActionsPopoutPage />} />
        <Route
          path="*"
          element={
            <AppShell user={user} theme={theme} onTheme={setTheme} onLogout={onLogout} onSync={syncNow} syncing={syncing}>
              <Routes>
                <Route path="/" element={<DashboardPage />} />
                <Route path="/inbox" element={<InboxPage />} />
                <Route path="/attention" element={<AttentionPage />} />
                <Route path="/repositories" element={<RepositoriesPage />} />
                <Route path="/repositories/:owner/:repo" element={<RepositoryDetailPage />} />
                <Route path="/pull-requests" element={<PullRequestsPage />} />
                <Route path="/pipelines" element={<PipelinesPage />} />
                <Route path="/pipelines/:id" element={<PipelineDetailPage />} />
                <Route path="/settings" element={<SettingsPage />} />
                <Route path="/wallboard" element={<WallboardPage />} />
                <Route path="/setup" element={<Navigate to="/" replace />} />
                <Route path="*" element={<Navigate to="/" replace />} />
              </Routes>
            </AppShell>
          }
        />
      </Routes>
      <ConfirmDialog
        open={syncError != null}
        title="Sync Failed"
        message={syncError ?? ""}
        cancelLabel={null}
        onConfirm={() => setSyncError(null)}
        onCancel={() => setSyncError(null)}
      />
    </>
  );
}

function Root() {
  const queryClient = useQueryClient();
  const location = useLocation();
  const me = useQuery({
    queryKey: ["me"],
    queryFn: api.me,
    retry: false,
  });

  useEffect(() => {
    setUnauthorizedHandler(() => {
      queryClient.clear();
    });
    return () => setUnauthorizedHandler(null);
  }, [queryClient]);

  const onWallboard = location.pathname === "/wallboard" || location.pathname.endsWith("/wallboard");

  if (me.isLoading && !onWallboard) return <div className="loading">Loading…</div>;
  if (me.isError && !onWallboard) return <div className="error">Could not check session.</div>;
  if (!me.data) {
    if (onWallboard) {
      return <WallboardPage />;
    }
    return (
      <LoginPage
        onLoggedIn={() => {
          queryClient.clear();
          void me.refetch();
        }}
      />
    );
  }
  return (
    <AuthenticatedApp
      user={me.data}
      onLogout={async () => {
        await api.logout();
        queryClient.clear();
        void me.refetch();
      }}
    />
  );
}

declare global {
  interface Window {
    __GITSEER_BASE__?: string;
  }
}

export default function App() {
  const basename = window.__GITSEER_BASE__ || "";
  return (
    <QueryClientProvider client={qc}>
      <BrowserRouter basename={basename || undefined}>
        <Root />
      </BrowserRouter>
    </QueryClientProvider>
  );
}
