import { FormEvent, useEffect, useState } from "react";
import { api, type UIConfig } from "../api/client";
import { Brand } from "../components/Brand";
import { PasswordInput } from "../components/PasswordInput";

type Props = { onLoggedIn: () => void };

type ConfigState = "loading" | "ready" | "error";

export function LoginPage({ onLoggedIn }: Props) {
  const [username, setUsername] = useState("bootstrap");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [ui, setUI] = useState<UIConfig>({});
  const [configState, setConfigState] = useState<ConfigState>("loading");

  useEffect(() => {
    api
      .uiConfig()
      .then((cfg) => {
        setUI(cfg);
        if (cfg.bootstrap_username) {
          setUsername(cfg.bootstrap_username);
        }
        setConfigState("ready");
      })
      .catch(() => {
        setUI({ base_path: window.__GITSEER_BASE__ || "" });
        setConfigState("error");
      });
  }, []);

  const unclaimed = configState === "ready" && ui.bootstrap_unclaimed === true;
  const showBootstrapLogin = configState === "ready" && ui.bootstrap_login_enabled === true;
  const warnBootstrapName = username.trim().toLowerCase() === "bootstrap";

  async function submitClaim(e: FormEvent) {
    e.preventDefault();
    setLoading(true);
    setError(null);
    if (password !== confirm) {
      setError("Passwords do not match");
      setLoading(false);
      return;
    }
    try {
      await api.claimBootstrap(username.trim(), password);
      onLoggedIn();
    } catch (err) {
      setError(err instanceof Error ? err.message : "claim failed");
    } finally {
      setLoading(false);
    }
  }

  async function submitLogin(e: FormEvent) {
    e.preventDefault();
    setLoading(true);
    setError(null);
    try {
      await api.login(username.trim(), password);
      onLoggedIn();
    } catch (err) {
      setError(err instanceof Error ? err.message : "login failed");
    } finally {
      setLoading(false);
    }
  }

  const basePath = window.__GITSEER_BASE__ || ui.base_path || "";
  const oauthHref = `${basePath}/api/v1/auth/login`;
  const githubOAuthHref = `${basePath}/api/v1/auth/github/login`;
  const gitlabOAuthHref = `${basePath}/api/v1/auth/gitlab/login`;
  const bitbucketOAuthHref = `${basePath}/api/v1/auth/bitbucket/login`;
  const forgejoOAuthHref = `${basePath}/api/v1/auth/forgejo/login`;
  const showOAuth = configState === "ready" && ui.oauth_enabled === true;
  const showGitHubOAuth = configState === "ready" && ui.github_oauth_enabled === true;
  const showGitLabOAuth = configState === "ready" && ui.gitlab_oauth_enabled === true;
  const showBitbucketOAuth = configState === "ready" && ui.bitbucket_oauth_enabled === true;
  const showForgejoOAuth = configState === "ready" && ui.forgejo_oauth_enabled === true;
  const showForgeOAuth =
    showOAuth || showGitHubOAuth || showGitLabOAuth || showBitbucketOAuth || showForgejoOAuth;

  return (
    <div className="login">
      <div className="login__compose">
        <div className="login__brand">
          <Brand variant="logo" className="login__logo" />
          <p className="muted">Pipelines, repository status, and real-time activity across your Git platforms.</p>
        </div>

        <div className="login__actions">
          {configState === "loading" && <p className="muted">Loading sign-in options…</p>}
          {configState === "error" && <p className="error">Could not load sign-in configuration.</p>}

          {unclaimed && (
            <form className="login__bootstrap" onSubmit={submitClaim}>
              <h2 className="login__claim-title">Claim Bootstrap</h2>
              <p className="muted login__claim-hint">
                Set the bootstrap admin username and password. This is the first step after install.
              </p>
              <input
                id="claim-username"
                type="text"
                autoComplete="username"
                placeholder="Username"
                aria-label="Bootstrap username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                required
              />
              {warnBootstrapName && (
                <p className="muted login__claim-warn">
                  Prefer a unique username instead of keeping &quot;bootstrap&quot;.
                </p>
              )}
              <PasswordInput
                id="claim-password"
                autoComplete="new-password"
                placeholder="Password (min 8 characters)"
                aria-label="Bootstrap password"
                value={password}
                onChange={setPassword}
                required
              />
              <PasswordInput
                id="claim-confirm"
                autoComplete="new-password"
                placeholder="Confirm password"
                aria-label="Confirm bootstrap password"
                value={confirm}
                onChange={setConfirm}
                required
              />
              {error && <p className="error">{error}</p>}
              <button
                className="btn primary"
                disabled={loading || !username.trim() || !password || !confirm}
                type="submit"
              >
                {loading ? "Claiming…" : "Claim Bootstrap"}
              </button>
            </form>
          )}

          {!unclaimed && (
            <>
              {showOAuth && (
                <a className="btn primary" href={oauthHref}>
                  Continue with Gitea
                </a>
              )}
              {showForgejoOAuth && (
                <a className={`btn${!showOAuth ? " primary" : ""}`} href={forgejoOAuthHref}>
                  Continue with Forgejo
                </a>
              )}
              {showGitHubOAuth && (
                <a className={`btn${!showOAuth && !showForgejoOAuth ? " primary" : ""}`} href={githubOAuthHref}>
                  Continue with GitHub
                </a>
              )}
              {showGitLabOAuth && (
                <a
                  className={`btn${!showOAuth && !showForgejoOAuth && !showGitHubOAuth ? " primary" : ""}`}
                  href={gitlabOAuthHref}
                >
                  Continue with GitLab
                </a>
              )}
              {showBitbucketOAuth && (
                <a
                  className={`btn${
                    !showOAuth && !showForgejoOAuth && !showGitHubOAuth && !showGitLabOAuth ? " primary" : ""
                  }`}
                  href={bitbucketOAuthHref}
                >
                  Continue with Bitbucket
                </a>
              )}

              {showBootstrapLogin && (
                <>
                  {showForgeOAuth && <div className="login__divider">or bootstrap</div>}
                  <form className="login__bootstrap" onSubmit={submitLogin}>
                    <input
                      id="login-username"
                      type="text"
                      autoComplete="username"
                      placeholder="Username"
                      aria-label="Bootstrap username"
                      value={username}
                      onChange={(e) => setUsername(e.target.value)}
                      required
                    />
                    <PasswordInput
                      id="password"
                      autoComplete="current-password"
                      placeholder="Bootstrap password"
                      aria-label="Bootstrap password"
                      value={password}
                      onChange={setPassword}
                      required
                    />
                    {error && <p className="error">{error}</p>}
                    <button className="btn" disabled={loading || !username.trim() || !password} type="submit">
                      {loading ? "Signing In…" : "Bootstrap Sign-In"}
                    </button>
                  </form>
                </>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  );
}
