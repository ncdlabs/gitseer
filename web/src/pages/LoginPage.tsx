import { FormEvent, useEffect, useState } from "react";
import { api } from "../api/client";

type Props = { onLoggedIn: () => void };

type UIConfig = {
  base_path?: string;
  oauth_enabled?: boolean;
  bootstrap_enabled?: boolean;
  csrf_token?: string;
};

type ConfigState = "loading" | "ready" | "error";

export function LoginPage({ onLoggedIn }: Props) {
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [ui, setUI] = useState<UIConfig>({});
  const [configState, setConfigState] = useState<ConfigState>("loading");

  useEffect(() => {
    api
      .uiConfig()
      .then((cfg) => {
        setUI(cfg);
        setConfigState("ready");
      })
      .catch(() => {
        setUI({ base_path: window.__LENS_BASE__ || "" });
        setConfigState("error");
      });
  }, []);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setLoading(true);
    setError(null);
    try {
      await api.login(password);
      onLoggedIn();
    } catch (err) {
      setError(err instanceof Error ? err.message : "login failed");
    } finally {
      setLoading(false);
    }
  }

  const basePath = window.__LENS_BASE__ || ui.base_path || "";
  const oauthHref = `${basePath}/api/v1/auth/login`;
  const showOAuth = configState === "ready" && ui.oauth_enabled === true;
  const showBootstrap = configState === "ready" && ui.bootstrap_enabled === true;

  return (
    <div className="login">
      <div className="login__compose">
        <div className="login__brand">
          <h1>
            Gitea <span className="brand__mark">Lens</span>
          </h1>
          <p className="muted">CI/CD and PR operations for your Gitea instance.</p>
        </div>

        <div className="login__actions">
          {configState === "loading" && <p className="muted">Loading sign-in options…</p>}
          {configState === "error" && <p className="error">Could not load sign-in configuration.</p>}

          {showOAuth && (
            <a className="btn primary" href={oauthHref}>
              Continue with Gitea
            </a>
          )}

          {showBootstrap && (
            <>
              {showOAuth && <div className="login__divider">or bootstrap</div>}
              <form className="login__bootstrap" onSubmit={submit}>
                <input
                  id="password"
                  type="password"
                  autoComplete="current-password"
                  placeholder="Bootstrap password"
                  aria-label="Bootstrap password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                />
                {error && <p className="error">{error}</p>}
                <button className="btn" disabled={loading || !password} type="submit">
                  {loading ? "Signing in…" : "Bootstrap sign-in"}
                </button>
              </form>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
