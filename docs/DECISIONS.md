# Decisions

Durable architecture and release decisions live here (not in task annotations).

## 2026-10-08 — Multi-arch GHCR images via prebuilt binaries

**Decision:** Public container images are a multi-arch OCI index of
**linux/amd64 + linux/arm64**, packaged from host cross-compiled release
binaries (`CGO_ENABLED=0 GOOS=linux GOARCH=amd64|arm64 go build`) using
`deploy/docker/Containerfile.runtime` (BuildKit `TARGETARCH` selects
`gitseer_linux_amd64` / `gitseer_linux_arm64` from the staged build context).

**Why:** In-image `go build` under QEMU is unreliable here (toolchain
crashes), and Buildx provenance/SBOM attestations add `unknown/unknown`
manifests to the OCI index, which pollutes GHCR's OS/Arch UI. The SBOM
still ships as a GitHub Release asset.

**Consequences:**
- `provenance: false` and `sbom: false` on the release workflow's
  `docker/build-push-action` steps.
- Lab single-arch builds can pass `--build-arg BINARY_NAME=gitseer` after
  placing a `gitseer` binary in the build context.
- README / docs no longer require `--platform linux/amd64` on arm64 hosts.

**Status:** Code + docs complete and verified locally (multi-arch image
build + run smoke test). Publishing to GHCR is a public-facing action cut
by the maintainer (new tag or `workflow_dispatch`).
