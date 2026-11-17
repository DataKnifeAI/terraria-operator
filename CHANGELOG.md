# Changelog

## 0.1.0-dev — Phase 1 MVP

- `TerrariaServer` CRD (`terraria.dataknife.ai/v1alpha1`)
- Reconcile StatefulSet (replicas=1), PVC, ClusterIP Service, Envoy Gateway TCPRoute
- World command-line args from `spec.world`
- TShock REST token bootstrap Secret + status conditions
- Compat watcher polls Pryaxis/TShock GitHub releases (status-only)
- envtest coverage for reconcile determinism, PVC adoption, validation
