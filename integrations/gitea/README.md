# Gitea custom UI integration

`lens install-ui` writes marker-bounded snippets into:

- `custom/templates/custom/extra_links.tmpl`
- `custom/templates/custom/extra_tabs.tmpl`

Markers:

- `<!-- BEGIN GITEA-LENS -->` … `<!-- END GITEA-LENS -->`
- `<!-- BEGIN GITEA-LENS-TABS -->` … `<!-- END GITEA-LENS-TABS -->`

Admin content outside markers is preserved. `uninstall-ui` removes only marker blocks.

Lens links use `target="_blank"` so clicking them from Gitea opens Lens in a new tab and leaves the current Gitea page.

Gitea caches templates in memory — after `install-ui` / `uninstall-ui`, restart Gitea (e.g. `kubectl -n gitea rollout restart deployment/gitea`) before verifying.
