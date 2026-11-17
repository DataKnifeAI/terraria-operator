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

## Licensing

- Re-Logic distributes the dedicated server zip free from terraria.org (no purchase).
- TShock is GPL-3.0 (Pryaxis).
- This operator does not redistribute either; it consumes published images.
