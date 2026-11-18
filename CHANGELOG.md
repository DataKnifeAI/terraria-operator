# Changelog

## 0.2.0-dev — Phase 2 operational polish

- Opt-in `spec.update.autoUpdateImage` pins latest TShock release tag
- `spec.backup` CronJob (+ optional REST save-before-tar)
- `spec.plugins` PVC / URL fetch init container → ServerPlugins mount
- Graceful-stop `preStop` REST save + annotation commands
  (`terraria.dataknife.ai/command`: save|broadcast|kick|ban|off)

## 0.1.0-dev — Phase 1 MVP

- `TerrariaServer` CRD (`terraria.dataknife.ai/v1alpha1`)
- Reconcile StatefulSet (replicas=1), PVC, ClusterIP Service, Envoy Gateway TCPRoute
- World command-line args from `spec.world`
- TShock REST token bootstrap Secret + status conditions
- Compat watcher polls Pryaxis/TShock GitHub releases (status-only)
- envtest coverage for reconcile determinism, PVC adoption, validation
- Docs site + hero graphic
