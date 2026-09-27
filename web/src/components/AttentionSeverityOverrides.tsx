import { FormEvent, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type AttentionRuleDefault, type AttentionRuleOverride } from "../api/client";

const SEVERITIES = ["critical", "warning", "waiting"] as const;

type DraftRow = {
  rule_type: string;
  default_severity: string;
  severity: string;
};

function buildDraft(defaults: AttentionRuleDefault[], overrides: AttentionRuleOverride[]): DraftRow[] {
  const map = new Map(overrides.map((o) => [o.rule_type, o.severity]));
  return defaults.map((d) => ({
    rule_type: d.rule_type,
    default_severity: d.default_severity,
    severity: map.get(d.rule_type) || d.default_severity,
  }));
}

function sameDraft(a: DraftRow[], b: DraftRow[]): boolean {
  if (a.length !== b.length) return false;
  return a.every((row, i) => row.rule_type === b[i].rule_type && row.severity === b[i].severity);
}

function typeLabel(type: string) {
  return type.replaceAll("_", " ");
}

type Props = {
  editable: boolean;
};

/** Bootstrap-admin severity override editor for Preferences → Attention. */
export function AttentionSeverityOverrides({ editable }: Props) {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["attention-rule-overrides"],
    queryFn: () => api.attentionRuleOverrides(),
  });
  const [draft, setDraft] = useState<DraftRow[]>([]);
  const [baseline, setBaseline] = useState<DraftRow[]>([]);
  const [error, setError] = useState("");
  const [savedFlash, setSavedFlash] = useState(false);

  useEffect(() => {
    if (!q.data) return;
    const next = buildDraft(q.data.defaults ?? [], q.data.overrides ?? []);
    setDraft(next);
    setBaseline(next);
  }, [q.data]);

  const dirty = !sameDraft(draft, baseline);

  const save = useMutation({
    mutationFn: async () => {
      const overrides = draft
        .filter((row) => row.severity !== row.default_severity)
        .map((row) => ({ rule_type: row.rule_type, severity: row.severity }));
      return api.updateAttentionRuleOverrides(overrides);
    },
    onSuccess: (data) => {
      setError("");
      setSavedFlash(true);
      const next = buildDraft(q.data?.defaults ?? [], data.overrides ?? []);
      setDraft(next);
      setBaseline(next);
      void qc.invalidateQueries({ queryKey: ["attention-rule-overrides"] });
    },
    onError: (err: Error) => {
      setError(err.message || "Save failed");
      setSavedFlash(false);
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!editable || !dirty || save.isPending) return;
    save.mutate();
  }

  function cancel() {
    setDraft(baseline);
    setError("");
    setSavedFlash(false);
  }

  if (q.isPending) return <p className="muted">Loading severity overrides…</p>;
  if (q.isError) return <p className="error">{(q.error as Error).message}</p>;

  return (
    <form className="settings-form settings-form--nested" onSubmit={onSubmit}>
      <p className="settings-form__hint">
        Override default severities for attention rules. Only bootstrap admins can save changes.
      </p>
      <div className="attention-severity-list">
        {draft.map((row, idx) => (
          <div className="attention-severity-row" key={row.rule_type}>
            <span className="mono attention-severity-row__type">{typeLabel(row.rule_type)}</span>
            <span className="muted attention-severity-row__default">default {row.default_severity}</span>
            <select
              aria-label={`Severity for ${typeLabel(row.rule_type)}`}
              value={row.severity}
              disabled={!editable || save.isPending}
              onChange={(e) => {
                const severity = e.target.value;
                setDraft((prev) => prev.map((r, i) => (i === idx ? { ...r, severity } : r)));
                setSavedFlash(false);
              }}
            >
              {SEVERITIES.map((s) => (
                <option key={s} value={s}>
                  {s}
                </option>
              ))}
            </select>
          </div>
        ))}
      </div>
      {error && (
        <p className="error" role="alert">
          {error}
        </p>
      )}
      {savedFlash && !dirty && <p className="settings-form__saved">Saved.</p>}
      {editable && (
        <div className="settings-form__actions">
          <button className="btn" type="button" onClick={cancel} disabled={!dirty || save.isPending}>
            Cancel
          </button>
          <button className="btn primary" type="submit" disabled={!dirty || save.isPending}>
            {save.isPending ? "Saving…" : "Save Overrides"}
          </button>
        </div>
      )}
    </form>
  );
}
