# Tasks

## Phase 1 — MVP (current)

- [x] CRD `TerrariaServer` + markers
- [x] StatefulSet / PVC / Service / Envoy Gateway (TCPRoute only)
- [x] World-args assembly, REST token bootstrap, compat watcher (status-only)
- [x] envtest suite + sample CR + docs

## Phase 2 — Operational polish

- [ ] Opt-in `autoUpdateImage` apply flow
- [ ] Backups CronJob + plugin PVC/URL fetch
- [ ] Graceful-stop hooks; REST-backed annotation commands

## Phase 3 — Parity

- [ ] Server Manager UI (player list/kick/ban, save & restart, updates)
- [ ] Helm chart + docs site section
