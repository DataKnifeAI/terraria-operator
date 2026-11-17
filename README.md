# terraria-operator

<p align="center">
  <img src="docs/assets/hero-terraria.jpg" alt="Terraria Operator — night forest world" width="640">
</p>

Kubernetes operator for [Terraria](https://terraria.org/) dedicated servers via
[TShock](https://github.com/Pryaxis/TShock). Sibling to
[palworld-operator](https://github.com/DataKnifeAI/palworld-operator) /
[bedrock-operator](https://github.com/DataKnifeAI/bedrock-operator): Go/kubebuilder,
CRD-first, Envoy Gateway (TCP-only).

**Site:** [dataknifeai.github.io/terraria-operator](https://dataknifeai.github.io/terraria-operator/)  
**Repos:** [GitHub](https://github.com/DataKnifeAI/terraria-operator)

**Licensing:** We neither bundle nor redistribute Terraria or TShock binaries. The
default image [`ghcr.io/pryaxis/tshock`](https://github.com/Pryaxis/TShock) is
published by the TShock project. Terraria dedicated server is free from Re-Logic;
TShock is GPL-3.0. Alternative: `ryshe/terraria` when `spec.serverType: VANILLA`.

## Status

Phase 1 MVP: `TerrariaServer` CRD, StatefulSet/PVC/TCPRoute/Service reconcile,
world-args assembly, REST token bootstrap, TShock compat/update watcher (status-only),
envtest suite, docs.

## Docs

- [TERRARIA_SERVER.md](docs/TERRARIA_SERVER.md) — user guide
- [ARCHITECTURE.md](docs/ARCHITECTURE.md) — resource graph & design rules
- [GITLAB_MIRROR.md](docs/GITLAB_MIRROR.md) — CI / Harbor publish

## Develop

```bash
make generate manifests
make test
make build
```

## Install (dev)

```bash
make install          # CRDs
make deploy IMG=...   # controller
kubectl apply -f config/samples/terraria_v1alpha1_terrariaserver.yaml
```

## Example

```yaml
apiVersion: terraria.dataknife.ai/v1alpha1
kind: TerrariaServer
metadata:
  name: main
spec:
  serverType: TSHOCK
  version:
    mode: STABLE
  world:
    name: main
    size: MEDIUM
    difficulty: CLASSIC
    maxPlayers: 16
  gateway:
    enabled: true
    address: "10.0.0.60"
  persistence:
    size: 4Gi
```

## License

Apache-2.0 (operator). Consumed images: Re-Logic Terraria dedicated server (free
download) + TShock (GPL-3.0).
