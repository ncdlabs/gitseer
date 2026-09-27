import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState, type ReactNode } from "react";
import { api, type User, type WorkflowRun } from "../api/client";
import { ConfirmDialog } from "./ConfirmDialog";
import { IconTipButton } from "./IconTipButton";

function isInFlight(status?: string): boolean {
  const s = (status || "").toLowerCase();
  return s === "queued" || s === "waiting" || s === "running";
}

function servicePatWarning(user?: User | null, forgeType?: string): string {
  if (!user?.is_bootstrap_admin) return "";
  if ((forgeType || "").toLowerCase() === "github") {
    return " This uses the GitHub service PAT (GitHub OAuth login is not available yet).";
  }
  return " This uses the instance service token, not a personal OAuth token.";
}

export type WorkflowWriteActionsProps = {
  run: WorkflowRun;
  user?: User | null;
  /** Compact layout for flyout rows. */
  compact?: boolean;
  className?: string;
  onDone?: () => void;
};

export function WorkflowWriteActions({ run, user, compact, className, onDone }: WorkflowWriteActionsProps) {
  const queryClient = useQueryClient();
  const [confirm, setConfirm] = useState<"rerun" | "cancel" | null>(null);
  const [error, setError] = useState<string | null>(null);

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["workflow-runs"] });
    void queryClient.invalidateQueries({ queryKey: ["run", run.id] });
    void queryClient.invalidateQueries({ queryKey: ["runs"] });
    onDone?.();
  };

  const rerun = useMutation({
    mutationFn: () => api.rerunWorkflowRun(run.id),
    onSuccess: () => {
      setConfirm(null);
      setError(null);
      invalidate();
    },
    onError: (err: Error) => setError(err.message || "Rerun failed"),
  });

  const cancel = useMutation({
    mutationFn: () => api.cancelWorkflowRun(run.id),
    onSuccess: () => {
      setConfirm(null);
      setError(null);
      invalidate();
    },
    onError: (err: Error) => setError(err.message || "Cancel failed"),
  });

  const busy = rerun.isPending || cancel.isPending;
  const inFlight = isInFlight(run.status);
  const canRerun = !inFlight;
  const canCancel = inFlight;
  if (!canRerun && !canCancel) return null;

  const warn = servicePatWarning(user, run.forge_type);
  const rootClass = [
    "workflow-write-actions",
    compact ? "workflow-write-actions--compact" : "",
    canCancel ? "workflow-write-actions--cancel" : "",
    className || "",
  ]
    .filter(Boolean)
    .join(" ");

  let dialogTitle = "";
  let dialogMessage: ReactNode = null;
  let confirmLabel = "Confirm";
  let danger = false;
  if (confirm === "rerun") {
    dialogTitle = "Rerun Workflow";
    dialogMessage = (
      <>
        Rerun <strong>{run.name || "this workflow"}</strong>
        {run.repo_full ? ` in ${run.repo_full}` : ""} on the forge?
        {warn}
      </>
    );
    confirmLabel = "Rerun Workflow";
  } else if (confirm === "cancel") {
    dialogTitle = "Cancel Workflow";
    dialogMessage = (
      <>
        Cancel the in-progress run <strong>{run.name || "this workflow"}</strong>
        {run.repo_full ? ` in ${run.repo_full}` : ""} on the forge?
        {warn}
      </>
    );
    confirmLabel = "Cancel Workflow";
    danger = true;
  }

  return (
    <div className={rootClass}>
      {canRerun && (
        <IconTipButton
          label="Rerun Workflow"
          icon="rerun"
          compact={compact}
          disabled={busy}
          onClick={(e) => {
            e.preventDefault();
            e.stopPropagation();
            setError(null);
            setConfirm("rerun");
          }}
        />
      )}
      {canCancel && (
        <IconTipButton
          label="Cancel Workflow"
          icon="cancel"
          danger
          compact={compact}
          disabled={busy}
          onClick={(e) => {
            e.preventDefault();
            e.stopPropagation();
            setError(null);
            setConfirm("cancel");
          }}
        />
      )}
      {error && <p className="error workflow-write-actions__error">{error}</p>}
      <ConfirmDialog
        open={confirm != null}
        title={dialogTitle}
        message={dialogMessage}
        confirmLabel={confirmLabel}
        danger={danger}
        busy={busy}
        onConfirm={() => {
          if (confirm === "rerun") rerun.mutate();
          else if (confirm === "cancel") cancel.mutate();
        }}
        onCancel={() => {
          if (!busy) setConfirm(null);
        }}
      />
    </div>
  );
}
