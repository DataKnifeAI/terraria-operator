# Architecture

Kubebuilder-style Go operator that reconciles a `TerrariaServer` custom resource into a
Terraria/TShock workload: StatefulSet, PVC, Secret, ClusterIP services, and Envoy Gateway
(TCPRoute only — simplest Gateway integration of the operator family).

Patterns align with [palworld-operator](https://github.com/DataKnifeAI/palworld-operator)
and [bedrock-operator](https://github.com/DataKnifeAI/bedrock-operator).

## Licensing (read first)

- **Terraria dedicated server:** free zip from Re-Logic / terraria.org
- **TShock:** GPL-3.0 ([Pryaxis/TShock](https://github.com/Pryaxis/TShock))
- This operator never redistributes game or TShock binaries — consumes
  `ghcr.io/pryaxis/tshock` (default) or `ryshe/terraria` (vanilla)

## Data path

```
Clients → spec.gateway.address (Kube-VIP / MetalLB)
              ↓
      {base}-gateway  (GatewayClass: envoy)
              ↓
   TCPRoute (7777/TCP only)
              ↓
      {name}-envoy  (ClusterIP)  →  {name} (ClusterIP: 7777 + 7878)
              ↓
      StatefulSet  (ghcr.io/pryaxis/tshock)  replicas=1
              ↓
      PVC (Worlds + tshock.sqlite)
```

REST **7878** stays on the game Service (ClusterIP). UI (Phase 3) proxies it; do not
public-route.

## Owned resources

| Kind | Purpose |
|------|---------|
| StatefulSet | Single-instance Terraria pod (`replicas` hard-capped at 1) |
| PersistentVolumeClaim | Worlds `.wld` + `tshock.sqlite`; optional plugins + backups PVCs |
| Secret | REST bootstrap password + token; optional game password ref |
| Service (ClusterIP) | Game TCP 7777 + REST 7878 |
| Service (Envoy backend) | `{name}-envoy` ClusterIP (game only) |
| Gateway + EnvoyProxy | External VIP binding |
| TCPRoute | Game `7777/TCP` |
| CronJob | Optional `{name}-backup` (REST save + tar) |

## Design rules

| Rule | Why |
|------|-----|
| StatefulSet, replicas=1 | Single-instance + port bind |
| TCP-only Gateway | Terraria is TCP; no UDPRoute edge cases |
| World on PVC | Recreate-safe; no world-identity seeding |
| Update watcher + opt-in apply | Status by default; `autoUpdateImage` pins TShock tags |
| REST token bootstrap | Novel vs palworld; idempotent via status probe |
| Annotation commands | One-shot REST admin via `terraria.dataknife.ai/command` |
| Consume published images | Licensing; no operator-built game images |

## Images

| Choice | Detail |
|--------|--------|
| **Default game image** | `ghcr.io/pryaxis/tshock:stable` |
| Vanilla alternative | `ryshe/terraria` when `serverType: VANILLA` |
| Operator image (CI) | `harbor.dataknife.net/library/terraria-operator` — see [GITLAB_MIRROR.md](GITLAB_MIRROR.md) |

## Terraria vs Palworld / Bedrock

| | terraria-operator | palworld-operator | bedrock-operator |
|--|-------------------|-------------------|------------------|
| Image | Pryaxis TShock GHCR | Official Pocketpair | Community itzg |
| CRD | `TerrariaServer` | `PalworldServer` | `BedrockServer` |
| Primary port | `7777/TCP` | `8211/UDP` | `19132/UDP` (+ TCP) |
| Config | CLI args + TShock config | INI + env | Env → properties |
| Admin API | REST + token (7878) | REST + basic (8212) | Console only |
| Updates | TShock releases + Terraria compat | Image tags | Mojang feed |

## Prerequisites

- Kubernetes 1.28+
- [Envoy Gateway](https://gateway.envoyproxy.io/) with GatewayClass `envoy` (or LB Service fallback)
- StorageClass suitable for ReadWriteOnce
- One dedicated external IP per server when Gateway is enabled
