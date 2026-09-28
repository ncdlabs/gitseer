import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";

/** Path prefix from server-injected <meta name="gitseer-base"> (CSP-safe; no inline script). */
function applyGitSeerBase() {
  const content = document.querySelector('meta[name="gitseer-base"]')?.getAttribute("content");
  window.__GITSEER_BASE__ = content ?? "";
}

applyGitSeerBase();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
