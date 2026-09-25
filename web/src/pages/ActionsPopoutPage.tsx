import { useEffect } from "react";
import { ActiveActionsPanel } from "../components/ActiveActionsPanel";

import { PRODUCT_NAME } from "../lib/product";

export function ActionsPopoutPage() {
  useEffect(() => {
    const prev = document.title;
    document.title = `Active Actions · ${PRODUCT_NAME}`;
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
