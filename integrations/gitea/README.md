# Gitea custom UI integration

`gitseer install-ui` writes a marker-bounded snippet into:

- `custom/templates/custom/extra_links.tmpl` (header navbar: Issues, Pull Requests, Milestones, Explore, GitSeer)

Header-only by design — no repo-tab (`extra_tabs.tmpl`) snippet. `install-ui` strips any
previously-installed `<!-- BEGIN GITSEER-TABS -->` … `<!-- END GITSEER-TABS -->` block.

Markers:

- `<!-- BEGIN GITSEER -->` … `<!-- END GITSEER -->`
- `<!-- BEGIN GITSEER-TABS -->` … `<!-- END GITSEER-TABS -->` (legacy cleanup only)

Admin content outside markers is preserved. `uninstall-ui` removes only marker blocks.
`install-ui` strips every existing marker pair before writing, so re-runs cannot double-insert.

GitSeer links use `target="_blank"` so clicking them from Gitea opens GitSeer in a new tab and leaves the current Gitea page.

Gitea caches templates in memory — after `install-ui` / `uninstall-ui`, restart Gitea (e.g. `kubectl -n gitea rollout restart deployment/gitea`) before verifying.
