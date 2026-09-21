import { useEffect } from "react";
import { ActiveActionsPanel } from "../components/ActiveActionsPanel";

export function ActionsPopoutPage() {
  useEffect(() => {
    const prev = document.title;
    document.title = "Active Actions · Gitea Lens";
    return () => {
      document.title = prev;
    };
  }, []);

  return (
    <div className="actions-popout">
      <ActiveActionsPanel className="actions-popout__body" preferOpener />
    </div>
  );
}
