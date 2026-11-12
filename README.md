# kube-commander

A Midnight Commander–style terminal UI for navigating Kubernetes **pools** and their DTAP environments (Develop, Testing, Acceptance, Production).

k9s is excellent inside one context. kube-commander keeps the map of every pool and environment visible while you browse namespaces and workloads.

## Status

Read-only two-pane navigator with optional Azure AKS credential refresh.

- Left: pools and environments from your config (friendly aliases)
- Right: deployments → pods (with CPU/mem requests/limits) → pod detail → logs / shell
- `a` / `l` for Azure login and credential ensure
- No mutating Kubernetes API calls

## Requirements

- Go 1.26+ (to build from source; see `go.mod`)
- `kubectl` / a valid kubeconfig with the contexts you reference
- Linux `az` CLI and `kubelogin` (only if you use `l` / azure blocks)
- Run `az login` yourself first (in WSL/Cursor prefer `az login --use-device-code`)

## Logging

Each run writes a session log under `$XDG_STATE_HOME/kube-commander/logs` (default `~/.local/state/kube-commander/logs`; on Windows `%LOCALAPPDATA%\kube-commander\logs`). Files are named `session-YYYYMMDD-HHMMSS.log`. On startup the newest **10** session files are kept and older ones are deleted.

External command stdout/stderr (`az`, `kubelogin`) is written to that log only — it is not dumped into the TUI. Interactive `az login` redirects stdout into the session log (`-o none` / `AZURE_CORE_OUTPUT=none`); stderr stays on the terminal so device-code / error messages remain visible.

## Install from release

GitHub Actions publishes binaries for **Linux** (Debian/WSL), **macOS**, and **Windows** when a version tag is pushed.

1. Open the [latest release](https://github.com/elmosgot/kube-commander/releases/latest).
2. Download the asset for your OS/arch, for example:

```bash
# Linux amd64 (Debian, Ubuntu, WSL)
VERSION=v0.1.0   # or use the tag from the releases page
curl -fsSL -o kube-commander \
  "https://github.com/elmosgot/kube-commander/releases/download/${VERSION}/kube-commander_${VERSION}_linux_amd64"
chmod +x kube-commander
./kube-commander --version
```

Other assets: `linux_arm64`, `darwin_amd64`, `darwin_arm64`, `windows_amd64.exe`. Each release also includes `checksums.txt`.

### Cut a release (maintainers)

```bash
git tag v0.1.0
git push origin v0.1.0
```

That runs the **Release** workflow and creates a GitHub Release with the binaries.

## Auto draft pull requests

Pushing a feature branch creates a **draft PR** into `main` via [`.github/workflows/auto-pr.yml`](.github/workflows/auto-pr.yml).

**Branch naming:**

| Pattern | Example | PR title/body from |
| --- | --- | --- |
| GitHub issue | `issues/42-add-healthcheck` | GitHub issue #42 |
| Linear issue | `ELU-12-storage-adapter` | Linear issue ELU-12 |

Branches that do not match either pattern are skipped (no PR created).

**Secrets:**

| Secret | Required when |
| --- | --- |
| `GITHUB_TOKEN` | Always (provided by Actions) |
| `LINEAR_API_KEY` | Branch name contains a Linear id (`TEAM-123`) |

Add `LINEAR_API_KEY` under repo **Settings → Secrets and variables → Actions**. Use a Linear **Personal API key** (Settings → API → Personal API keys). Paste the raw key only — no `Bearer ` prefix.

**Verify:** merge this workflow to `main`, then push a branch such as `issues/1-setup` or `ELU-1-setup` and confirm a draft PR appears.

## Build from source

```bash
go build -o bin/kube-commander ./cmd/kube-commander
./bin/kube-commander
```

Or:

```bash
go run ./cmd/kube-commander
```

## Names: alias vs context

| Field | Role |
|-------|------|
| `pools[].name` | Friendly pool alias in the left pane (`pool-x`, `pool-y`) |
| `environments[].name` | Friendly DTAP alias (`develop`, `testing`, `production`) |
| `context` | Real kubeconfig context name (usually the AKS cluster name) |
| `namespace` | App namespace opened on Enter (recommended; required if you cannot list namespaces cluster-wide) |
| `azure.*` | How to create/refresh that context via `az aks get-credentials` |

Pool + environment **are** the aliases. `context` must match `kubectl config get-contexts`.

## Config

Default path: `~/.config/kube-commander/config.yaml`

Override with `--config /path/to/config.yaml`.

If the default file is missing, the bundled [`configs/example.yaml`](configs/example.yaml) is loaded (generic `pool-x` / `pool-y` sample).

```yaml
pools:
  - name: pool-x
    environments:
      - name: testing
        context: pool-x-test
        namespace: app
        azure:
          subscription: 00000000-0000-0000-0000-000000000002
          resource_group: rg-pool-x-test
          cluster: aks-pool-x-test
```

- `context` is required
- `namespace` is optional in YAML but **recommended**: without it the UI may try to list all namespaces (often Forbidden under restricted RBAC). Set it to jump straight to deployments.
- `azure` is optional; if set, `resource_group` and `cluster` are required; `subscription` is recommended
- Optional `kubeconfig` overrides the file used for that environment; default is always `$HOME/.kube/config`

Missing contexts show as **unavailable**. Press `l` (after `az login`) to fetch credentials for the selected environment. If listing namespaces is Forbidden, the UI returns to the left pane and asks you to set `environments[].namespace`.

### Azure / WSL kubeconfig

Ensure (`l`) always writes with `az aks get-credentials --file <resolved>` and then runs `kubelogin convert-kubeconfig -l azurecli --kubeconfig <resolved>` against the **same** file.

- Default resolved path: `$HOME/.kube/config`
- Prefers `/usr/bin/az` and `/usr/local/bin/kubelogin` when present, so Windows copies on a polluted WSL `PATH` are avoided
- `az login` stays manual — in Cursor/WSL use:

  ```bash
  /usr/bin/az login --use-device-code
  ```

- Pressing `a` runs interactive `az login` with `AZURE_CORE_LOGIN_EXPERIENCE_V2=off` on that process only (skips the subscription picker; your `az config` is unchanged). Azure CLI **≥ 2.87.0** also gets `az login -s {subscription}`; older versions use plain `az login`. After login, kube-commander always runs `az account set --subscription {subscription}` before ensuring credentials.

### Quick start

1. Copy the example:

   ```bash
   mkdir -p ~/.config/kube-commander
   cp configs/example.yaml ~/.config/kube-commander/config.yaml
   ```

2. Edit the copy: replace placeholder contexts, namespaces, and Azure fields with your own.

3. Run `az login --use-device-code` (or normal `az login` outside Cursor).

4. Start kube-commander, select an environment, press `l` to ensure credentials, then browse.

## Keys

The bottom help line is **context-sensitive** (focus pane + current right view). Full reference:

| Key | Action |
|-----|--------|
| `Tab` | Switch focus between panes |
| `↑` / `↓` or `k` / `j` | Move cursor |
| `Enter` / `→` | Expand pool / open deployment / pod / logs |
| `g` | Tail last 200 lines of logs for the selected container (pod detail) |
| `f` / `/` | In the log pane: open on-the-fly filter (shown on the bottom help line); type to narrow lines; Esc clears filter (stays on logs). Global hotkeys are disabled while filtering so letters like `c`/`r` type into the query. |
| `s` | Interactive shell into the selected container (`bash` if present, else `sh`) via `kubectl exec` |
| `a` | Left pane: interactive `az login` (picker suppressed via env; `-s` when Azure CLI ≥ 2.87), then `az account set` + ensure credentials |
| `l` | Left pane: ensure Azure credentials into `$HOME/.kube/config` (`az` + `kubelogin`) |
| `c` | Toggle bottom commands panel (equivalent `kubectl` for the current view + recent shell commands) |
| `Backspace` / `←` / `h` / `Esc` | Go up one level (logs → detail → pods → deployments → left pools) |
| `r` | Refresh the current right-pane view |
| `q` | Quit |

When a context is **unavailable**, focus the left pane and press `a` to log in to that environment’s subscription, then credentials are refreshed automatically (same as `l`).

### Drill-down

Namespaces → Deployments → Pods → Pod detail (containers + CPU/memory requests/limits) → Logs (`g` or Enter on a container).

Assigned resources are requests/limits from the pod spec (not live metrics).

## Layout

```
┌─ Pools ─────────────┬─ Pod · my-api-abc ────────────────────┐
│ ▾ pool-x            │ phase=Running  ready=1/1  node=…      │
│     testing         │ containers (enter/g = logs)           │
│     production      │ app  yes  rst 0  Running  cpu 100m/…  │
│ ▾ pool-y            │   image registry/app:1.2.3            │
└─────────────────────┴───────────────────────────────────────┘
pool=pool-x · env=testing · deploy=my-api · pod=my-api-abc · container=app
```

## Next increments

- Live log follow / metrics-server usage
- Second panel locked to another environment (side-by-side compare)
- Mutating actions (scale, restart) behind confirmation
