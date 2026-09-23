import { QueryClient, QueryClientProvider, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { api, setUnauthorizedHandler, type User } from "./api/client";
import { AppShell } from "./components/AppShell";
import { useTheme } from "./hooks/useTheme";
import { AttentionPage } from "./pages/AttentionPage";
import { ActionsPopoutPage } from "./pages/ActionsPopoutPage";
import { LoginPage } from "./pages/LoginPage";
import { DashboardPage } from "./pages/DashboardPage";
import { PipelineDetailPage, PipelinesPage } from "./pages/PipelinesPage";
import { SettingsPage } from "./pages/SettingsPage";
import { SetupWizardPage } from "./pages/SetupWizardPage";
import { PullRequestsPage, RepositoriesPage } from "./pages/RepositoriesPage";
import "./styles/app.css";

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
    const base = window.__LENS_BASE__ || "";
    const es = new EventSource(`${base}/api/v1/events`);
    const onMessage = (event: MessageEvent) => {
      let type = "";
      try {
        const parsed = JSON.parse(String(event.data ?? "")) as { type?: string };
        type = typeof parsed?.type === "string" ? parsed.type : "";
      } catch {
        /* unparseable → default invalidation */
      }
      if (type === "workflow_run" || type === "workflow_job") {
        invalidateKeys(queryClient, ["workflow-runs", "runs", "run", "summary", "stats", "attention"]);
        return;
      }
      if (type === "pull_request") {
        invalidateKeys(queryClient, ["prs", "summary", "stats", "attention"]);
        return;
      }
      invalidateKeys(queryClient, [
        "workflow-runs",
        "runs",
        "run",
        "summary",
        "stats",
        "attention",
        "prs",
        "repositories",
      ]);
    };
    es.addEventListener("message", onMessage);
    es.onerror = () => {
      /* browser reconnects; avoid tight loops */
    };
    return () => {
      es.removeEventListener("message", onMessage);
      es.close();
    };
  }, [queryClient]);
}

function AuthenticatedApp({ user, onLogout }: { user: User; onLogout: () => void }) {
  const giteaTheme =
    user.theme === "light" || user.theme === "dark" || user.theme === "system" ? user.theme : null;
  const { theme, setTheme } = useTheme(giteaTheme);
  const queryClient = useQueryClient();
  const [syncing, setSyncing] = useState(false);
  useRealtimeInvalidation();

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
      alert(err instanceof Error ? err.message : "sync failed");
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
    <Routes>
      <Route path="/actions-popout" element={<ActionsPopoutPage />} />
      <Route
        path="*"
        element={
          <AppShell user={user} theme={theme} onTheme={setTheme} onLogout={onLogout} onSync={syncNow} syncing={syncing}>
            <Routes>
              <Route path="/" element={<DashboardPage />} />
              <Route path="/attention" element={<AttentionPage />} />
              <Route path="/repositories" element={<RepositoriesPage />} />
              <Route path="/pull-requests" element={<PullRequestsPage />} />
              <Route path="/pipelines" element={<PipelinesPage />} />
              <Route path="/pipelines/:id" element={<PipelineDetailPage />} />
              <Route path="/settings" element={<SettingsPage />} />
              <Route path="/setup" element={<Navigate to="/" replace />} />
              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
          </AppShell>
        }
      />
    </Routes>
  );
}

function Root() {
  const queryClient = useQueryClient();
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

  if (me.isLoading) return <div className="loading">Loading…</div>;
  if (me.isError) return <div className="error">Could not check session.</div>;
  if (!me.data) {
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
    __LENS_BASE__?: string;
  }
}

export default function App() {
  const basename = window.__LENS_BASE__ || "";
  return (
    <QueryClientProvider client={qc}>
      <BrowserRouter basename={basename || undefined}>
        <Root />
      </BrowserRouter>
    </QueryClientProvider>
  );
}
