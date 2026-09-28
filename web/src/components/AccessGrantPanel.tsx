import { useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type ACLUser, type InstancePublic, type Repository } from "../api/client";
import { BootstrapAccessRequired } from "./BootstrapAccessRequired";

type Props = {
  editable: boolean;
  canElevate?: boolean;
  onBecomeBootstrap?: () => void;
};

/** Bootstrap-admin fallback to grant GitHub (or other) repo ACL when users lack per-user OAuth tokens. */
export function AccessGrantPanel({ editable, canElevate, onBecomeBootstrap }: Props) {
  const queryClient = useQueryClient();
  const usersQuery = useQuery({
    queryKey: ["acl-users"],
    queryFn: api.users,
    enabled: editable,
  });
  const instancesQuery = useQuery({
    queryKey: ["instances"],
    queryFn: api.instances,
    enabled: editable,
  });
  const reposQuery = useQuery({
    queryKey: ["repositories", "acl-grant"],
    queryFn: () => api.repositories("", "all"),
    enabled: editable,
  });

  const [selectedUserId, setSelectedUserId] = useState<number | "">("");
  const [selectedInstanceId, setSelectedInstanceId] = useState<number | "">("");
  const [selectedRepoIds, setSelectedRepoIds] = useState<number[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [hint, setHint] = useState<string | null>(null);

  const users = (usersQuery.data?.users || []).filter((u) => !u.is_bootstrap_admin);
  const instances = instancesQuery.data?.items || [];
  const githubInstances = instances.filter((i) => (i.forge_type || "").toLowerCase() === "github");

  const reposForInstance = useMemo(() => {
    const items = reposQuery.data?.items || [];
    if (!selectedInstanceId) return [] as Repository[];
    return items.filter((r) => r.instance_id === selectedInstanceId);
  }, [reposQuery.data, selectedInstanceId]);

  async function loadAccess(userId: number, instanceId: number) {
    setError(null);
    setHint(null);
    setLoaded(false);
    try {
      const access = await api.userAccess(userId, instanceId);
      setSelectedRepoIds(access.repo_ids || []);
      setLoaded(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to load access");
      setSelectedRepoIds([]);
    }
  }

  function onSelectUser(user: ACLUser | undefined) {
    const id = user?.id ?? "";
    setSelectedUserId(id);
    setLoaded(false);
    setSelectedRepoIds([]);
    if (typeof id === "number" && typeof selectedInstanceId === "number") {
      void loadAccess(id, selectedInstanceId);
    }
  }

  function onSelectInstance(inst: InstancePublic | undefined) {
    const id = inst?.id ?? "";
    setSelectedInstanceId(id);
    setLoaded(false);
    setSelectedRepoIds([]);
    if (typeof selectedUserId === "number" && typeof id === "number") {
      void loadAccess(selectedUserId, id);
    }
  }

  function toggleRepo(repoId: number) {
    setSelectedRepoIds((prev) =>
      prev.includes(repoId) ? prev.filter((id) => id !== repoId) : [...prev, repoId],
    );
  }

  async function save() {
    if (typeof selectedUserId !== "number" || typeof selectedInstanceId !== "number") return;
    setSaving(true);
    setError(null);
    setHint(null);
    try {
      await api.putUserAccess(selectedUserId, selectedInstanceId, selectedRepoIds);
      setHint("Repository access saved. Gitea OAuth refresh will not clear these grants.");
      await queryClient.invalidateQueries({ queryKey: ["acl-users"] });
    } catch (err) {
      setError(err instanceof Error ? err.message : "save failed");
    } finally {
      setSaving(false);
    }
  }

  if (!editable) {
    return (
      <div id="settings-panel-access" role="tabpanel" aria-labelledby="settings-tab-access">
        <BootstrapAccessRequired
          message="Granting repository access requires bootstrap admin access."
          canElevate={canElevate}
          onBecomeBootstrap={onBecomeBootstrap}
        />
      </div>
    );
  }

  return (
    <div className="panel panel--padded settings-form" id="settings-panel-access" role="tabpanel" aria-labelledby="settings-tab-access">
      <fieldset className="settings-form__section" disabled={saving}>
        <legend>Manual Repository Access</legend>
        <p className="settings-form__hint">
          Use this when GitHub OAuth is not configured (service PAT sync only). Grant selected GitHub
          repositories to an ordinary user. Prefer <strong>Continue with GitHub</strong> /{" "}
          <strong>Link GitHub</strong> when an OAuth App is available so ACL follows the user&apos;s
          token automatically.
        </p>

        <div className="settings-form__field">
          <select
            aria-label="User"
            value={selectedUserId === "" ? "" : String(selectedUserId)}
            onChange={(e) => {
              const id = e.target.value ? Number(e.target.value) : "";
              onSelectUser(users.find((u) => u.id === id));
            }}
          >
            <option value="">Select user…</option>
            {users.map((u) => (
              <option key={u.id} value={u.id}>
                {u.login}
                {u.has_github ? " (GitHub linked)" : ""}
                {u.has_gitea && !u.has_github ? " (Gitea)" : ""}
              </option>
            ))}
          </select>
        </div>

        <div className="settings-form__field">
          <select
            aria-label="Forge Instance"
            value={selectedInstanceId === "" ? "" : String(selectedInstanceId)}
            onChange={(e) => {
              const id = e.target.value ? Number(e.target.value) : "";
              onSelectInstance(instances.find((i) => i.id === id));
            }}
          >
            <option value="">Select instance…</option>
            {(githubInstances.length ? githubInstances : instances).map((inst) => (
              <option key={inst.id} value={inst.id}>
                {(inst.forge_type || "gitea").toUpperCase()} — {inst.name || inst.base_url || `#${inst.id}`}
              </option>
            ))}
          </select>
          {githubInstances.length === 0 && (
            <p className="settings-form__hint">No GitHub instance yet; other forge instances are listed.</p>
          )}
        </div>

        {typeof selectedUserId === "number" && typeof selectedInstanceId === "number" && (
          <div className="settings-form__field">
            <p className="settings-form__hint">
              {loaded
                ? `${selectedRepoIds.length} of ${reposForInstance.length} repositories selected`
                : "Loading current grants…"}
            </p>
            <ul className="access-grant__repos" style={{ listStyle: "none", padding: 0, margin: 0 }}>
              {reposForInstance.map((repo) => {
                const checked = selectedRepoIds.includes(repo.id);
                return (
                  <li key={repo.id}>
                    <label className="settings-form__check">
                      <input
                        type="checkbox"
                        checked={checked}
                        onChange={() => toggleRepo(repo.id)}
                        disabled={!loaded}
                      />
                      <span className="settings-form__check-text mono">{repo.full_name}</span>
                    </label>
                  </li>
                );
              })}
            </ul>
            {reposForInstance.length === 0 && loaded && (
              <p className="muted">No synced repositories on this instance. Run Sync Now first.</p>
            )}
          </div>
        )}

        {error && (
          <p className="error" role="alert">
            {error}
          </p>
        )}
        {hint && <p className="muted">{hint}</p>}

        <div className="modal__actions" style={{ justifyContent: "flex-end" }}>
          <button
            className="btn primary"
            type="button"
            disabled={
              saving ||
              !loaded ||
              typeof selectedUserId !== "number" ||
              typeof selectedInstanceId !== "number"
            }
            onClick={() => void save()}
          >
            {saving ? "Saving…" : "Save Access"}
          </button>
        </div>
      </fieldset>
    </div>
  );
}
