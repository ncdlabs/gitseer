import { useEffect, useState } from "react";

function readIsTerminal(): boolean {
  return document.documentElement.getAttribute("data-theme") === "terminal";
}

/** True when the resolved `data-theme` on `<html>` is `terminal`. */
export function useIsTerminalTheme(): boolean {
  const [terminal, setTerminal] = useState(readIsTerminal);

  useEffect(() => {
    const root = document.documentElement;
    const sync = () => setTerminal(readIsTerminal());
    sync();
    const mo = new MutationObserver(sync);
    mo.observe(root, { attributes: true, attributeFilter: ["data-theme"] });
    return () => mo.disconnect();
  }, []);

  return terminal;
}
