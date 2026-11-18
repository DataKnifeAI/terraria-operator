# TerrariaServer

User guide for the `TerrariaServer` custom resource.

## Quick start

```bash
kubectl apply -f config/crd/bases/
kubectl apply -f examples/terrariaserver-sample.yaml
kubectl get terrariaserver
```

Connect clients to `.status.connectionAddress`:`status.connectionPort` (default TCP 7777).

## Image & versions

| Field | Meaning |
|-------|---------|
| `spec.serverType` | `TSHOCK` (default) or `VANILLA` |
| Default TShock image | `ghcr.io/pryaxis/tshock:stable` |
| `spec.version.mode=PINNED` | Use `pinnedTag` (e.g. `v6.1.0`) |
| Vanilla | Forces `ryshe/terraria`-style image unless `serverImage.image` set |

Terraria clients and server **must** match. TShock releases target a specific Terraria
build (e.g. TShock 6.1.0 ↔ Terraria 1.4.5.6). The operator polls Pryaxis/TShock GitHub
releases and sets `status.updateAvailable` when the installed Terraria version is behind
the latest TShock-supported version. Phase 1 is **status-only**; set
`spec.update.autoUpdateImage: true` for Phase 2 apply.

## World

| Field | Server arg |
|-------|------------|
| `world.name` | `-worldname` + `.wld` path |
| `world.size` | `-autocreate` 1/2/3 (SMALL/MEDIUM/LARGE) |
| `world.difficulty` | `-difficulty` 0–3 (CLASSIC/EXPERT/MASTER/JOURNEY) |
| `world.seed` | `-seed` (optional) |
| `world.maxPlayers` | `-maxplayers` |

World state is a single `.wld` plus `tshock.sqlite` on the PVC. No worldguid seeding is
required; StatefulSet Recreate is safe when the PVC survives.

## Persistence

Default PVC size `4Gi`. Set `persistence.existingClaim` to adopt a restored volume.

Mount path (validate against your image): `/root/.local/share/Terraria/Worlds`
(ryshe-documented; confirm for `ghcr.io/pryaxis/tshock`).

## Admin API (TShock REST)

REST listens on **7878** ClusterIP-only (never Gateway-exposed by default).

**Token bootstrap**

1. If `adminAPI.tokenSecretRef` is set, the Secret must contain key `token`.
2. Otherwise the operator creates `{name}-rest-token` with a `rest-password`, then calls
   `GET /v2/token/create/{password}` once the Service is reachable, and stores `token`.
3. Subsequent reconciles reuse the token when `/status` succeeds.

## Gateway

TCP-only: Gateway + TCPRoute on port 7777. Set `gateway.address` to a dedicated VIP when
enabled. Disable Gateway and use `serviceType: LoadBalancer` if preferred.

## Opt-in auto-update (`spec.update`)

```yaml
spec:
  update:
    checkInterval: 6h
    autoUpdateImage: true   # patches version.mode=PINNED + pinnedTag to latest TShock release
```

When `autoUpdateImage` is false (default), the compat watcher only flips
`status.updateAvailable`. When true, the operator pins `ghcr.io/pryaxis/tshock:vX.Y.Z`
and the StatefulSet rolls (Recreate semantics).

## Annotation commands (REST)

One-shot admin actions via annotation (stripped after run):

```bash
kubectl annotate terrariaserver main terraria.dataknife.ai/command='broadcast Hello' --overwrite
kubectl annotate terrariaserver main terraria.dataknife.ai/command=save --overwrite
kubectl annotate terrariaserver main terraria.dataknife.ai/command='kick Alice' --overwrite
kubectl annotate terrariaserver main terraria.dataknife.ai/command='ban Bob grief' --overwrite
kubectl annotate terrariaserver main terraria.dataknife.ai/command=off --overwrite
```

Results land in `status.lastCommand` / `status.lastCommandResult`.

## Plugins

```yaml
spec:
  plugins:
    enabled: true
    urls:
      - https://example.com/MyPlugin.dll
    # mountRef: { name: my-plugins-pvc }  # bring-your-own instead of owned PVC
```

Init container fetches URLs into a plugins PVC mounted at `/tshock/ServerPlugins`
(override with `plugins.path`).

## Backups

```yaml
spec:
  backup:
    enabled: true
    schedule: "0 4 * * *"
    retain: 7
    restSaveFirst: true   # default — hit REST /save before tar
```

Creates `{name}-backups` PVC + `{name}-backup` CronJob. Archives are
`/backups/<timestamp>.tar.gz` of the worlds volume.

## Graceful stop

Pods include a `preStop` hook that calls REST `/save` (when a token is present) and
waits briefly so Terraria can flush `.wld` / `tshock.sqlite` before SIGTERM.

## Licensing

- Re-Logic distributes the dedicated server zip free from terraria.org (no purchase).
- TShock is GPL-3.0 (Pryaxis).
- This operator does not redistribute either; it consumes published images.
