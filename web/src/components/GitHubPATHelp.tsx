import { useEffect, useId, useRef, useState } from "react";
import { githubFineGrainedPATCreateURL, githubPATCreateURL } from "../lib/github";
import { Glyph } from "./Glyph";

type Props = {
  /** Forge base URL (github.com, api.github.com, or GHE host). */
  baseURL: string;
  /** Raise backdrop above an already-open settings dialog. */
  nested?: boolean;
};

const CLASSIC_PERMS = [
  { code: "repo", detail: "sync repositories, PRs, statuses, and Actions" },
  { code: "admin:repo_hook", detail: "manage repository webhooks" },
  { code: "admin:org_hook", detail: "manage organization webhooks" },
] as const;

const FINE_GRAINED_PERMS = [
  { code: "Contents", detail: "Read — sync workflow files and repository content" },
  { code: "Pull requests", detail: "Read — sync pull requests" },
  { code: "Actions", detail: "Read — sync workflow runs and jobs" },
  { code: "Commit statuses", detail: "Read — sync commit statuses" },
  { code: "Webhooks", detail: "Read and write — manage repository webhooks" },
  {
    code: "Organization → Webhooks",
    detail: "Read and write — manage organization webhooks (when the owner is an org)",
  },
] as const;

export function GitHubPATHelp({ baseURL, nested = false }: Props) {
  const [open, setOpen] = useState(false);
  const titleId = useId();
  const ctaRef = useRef<HTMLAnchorElement>(null);
  const classicURL = githubPATCreateURL(baseURL);
  const fineGrainedURL = githubFineGrainedPATCreateURL(baseURL);

  useEffect(() => {
    if (!open) return;
    ctaRef.current?.focus();
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") setOpen(false);
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open]);

  return (
    <>
      <div className="field-help field-help--two-col">
        <div className="field-help-block">
          <div className="field-help-row">
            <a
              href={classicURL}
              target="_blank"
              rel="noreferrer"
              className="field-help-row__link"
            >
              Open GitHub PAT Settings (classic)
            </a>
            <button
              type="button"
              className="info-tip"
              aria-label="How to create a GitHub PAT"
              aria-haspopup="dialog"
              aria-expanded={open}
              onClick={() => setOpen(true)}
            >
              <Glyph name="info" className="info-tip__icon" />
            </button>
          </div>
          <ul className="field-help__perms settings-form__hint">
            {CLASSIC_PERMS.map((p) => (
              <li key={p.code}>
                <code>{p.code}</code> — {p.detail}
              </li>
            ))}
          </ul>
        </div>

        <div className="field-help-block">
          <div className="field-help-row">
            <a
              href={fineGrainedURL}
              target="_blank"
              rel="noreferrer"
              className="field-help-row__link"
            >
              Open GitHub PAT Settings (fine-grained)
            </a>
          </div>
          <ul className="field-help__perms settings-form__hint">
            {FINE_GRAINED_PERMS.map((p) => (
              <li key={p.code}>
                <strong>{p.code}</strong> — {p.detail}
              </li>
            ))}
          </ul>
        </div>
      </div>

      {open && (
        <div
          className={`modal-backdrop${nested ? " modal-backdrop--stack" : ""}`}
          role="presentation"
          onClick={(e) => {
            if (e.target === e.currentTarget) setOpen(false);
          }}
        >
          <div className="modal modal--wide" role="dialog" aria-modal="true" aria-labelledby={titleId}>
            <header className="modal__header">
              <h2 id={titleId}>Create a GitHub PAT</h2>
              <p className="muted">
                GitSeer uses a personal access token to sync data and manage webhook delivery. Paste
                the token into the field after you create it.
              </p>
            </header>

            <div className="pat-help">
              <ol className="pat-help__steps">
                <li>
                  Open{" "}
                  <a href={classicURL} target="_blank" rel="noreferrer">
                    classic PAT settings
                  </a>{" "}
                  or{" "}
                  <a href={fineGrainedURL} target="_blank" rel="noreferrer">
                    fine-grained PAT settings
                  </a>
                  .
                </li>
                <li>
                  Create a token named something like GitSeer and enable the scopes or repository
                  permissions listed under each settings link above.
                </li>
                <li>
                  Leave other scopes off (including <code>workflow</code> / Workflows write). GitSeer
                  shows a webhook payload to add under Organization or Repository → Settings →
                  Webhooks; the hook permissions are required if you create or manage those hooks
                  with this token.
                </li>
                <li>Generate the token, copy it once, and paste it into GitSeer.</li>
              </ol>
              <p className="settings-form__hint pat-help__note">
                The service PAT is for sync and bootstrap-admin forge calls. For per-user ACL and
                write ops, configure a GitHub OAuth App on the instance (callback{" "}
                <code>/api/v1/auth/github/callback</code>) or grant repos under Settings → Access.
                Prefer an organization webhook so one delivery covers all repos.
              </p>
            </div>

            <div className="modal__actions">
              <button
                className="btn modal__actions-back"
                type="button"
                onClick={() => setOpen(false)}
              >
                Close
              </button>
              <a
                className="btn primary"
                href={classicURL}
                target="_blank"
                rel="noreferrer"
                ref={ctaRef}
              >
                Open GitHub PAT Settings (classic)
              </a>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
