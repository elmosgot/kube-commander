# kube-commander

A Midnight Commander–style terminal UI for navigating Kubernetes **pools** and their DTAP environments (Develop, Testing, Acceptance, Production).

k9s is excellent inside one context. kube-commander keeps the map of every pool and environment visible while you browse namespaces and workloads.

## Status

Read-only two-pane navigator with optional Azure AKS credential refresh.

- Left: pools and environments from your config (friendly aliases)
- Right: namespaces → deployments → pods → pod detail (containers, CPU/mem requests/limits) → logs
- `a` / `l` for Azure login and credential ensure
- No mutating Kubernetes API calls

## Requirements

- Go 1.26+ (to build from source; see `go.mod`)
- `kubectl` / a valid kubeconfig with the contexts you reference
- Linux `az` CLI and `kubelogin` (only if you use `l` / azure blocks)
- Run `az login` yourself first (in WSL/Cursor prefer `az login --use-device-code`)

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
| `namespace` | Default app namespace opened on Enter |
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
- `namespace` and `azure` are optional
- If `azure` is set, `resource_group` and `cluster` are required; `subscription` is recommended
- Optional `kubeconfig` overrides the file used for that environment; default is always `$HOME/.kube/config`

Missing contexts show as **unavailable**. Press `l` (after `az login`) to fetch credentials for the selected environment.

### Azure / WSL kubeconfig

Ensure (`l`) always writes with `az aks get-credentials --file <resolved>` and then runs `kubelogin convert-kubeconfig -l azurecli --kubeconfig <resolved>` against the **same** file.

- Default resolved path: `$HOME/.kube/config`
- Prefers `/usr/bin/az` and `/usr/local/bin/kubelogin` when present, so Windows copies on a polluted WSL `PATH` are avoided
- `az login` stays manual — in Cursor/WSL use:

  ```bash
  /usr/bin/az login --use-device-code
  ```

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

| Key | Action |
|-----|--------|
| `Tab` | Switch focus between panes |
| `↑` / `↓` or `k` / `j` | Move cursor |
| `Enter` / `→` | Expand pool / open namespace / deployment / pod / logs |
| `g` | Tail last 200 lines of logs for the selected container (pod detail) |
| `a` | Interactive `az login -s {subscription}` then ensure credentials |
| `l` | Ensure Azure credentials into `$HOME/.kube/config` (`az` + `kubelogin`) |
| `Backspace` / `←` / `h` / `Esc` | Go up one level (logs → detail → pods → deployments → namespaces) |
| `r` | Refresh the current right-pane view |
| `q` | Quit |

When a context is **unavailable**, press `a` to log in to that environment’s subscription, then credentials are refreshed automatically (same as `l`).

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
