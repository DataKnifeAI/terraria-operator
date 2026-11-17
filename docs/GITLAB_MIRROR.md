# GitLab mirror and Harbor publish

## CI split

| System | Responsibility |
|--------|----------------|
| **GitHub Actions** | Lint, test, compile/build, then mirror `main` to GitLab |
| **GitLab CI** | Build the operator container image and push to Harbor |

Quality gates run on GitHub before mirror. GitLab does **not** re-run lint/unit tests; it only builds and publishes:

`harbor.dataknife.net/library/terraria-operator`

Mirror project (create when publishing): `gitlab.com/dk-raas/dkai/game-servers/terraria-operator`.

## GitHub workflows

| Workflow | Role |
|----------|------|
| `lint.yml` | `golangci-lint` |
| `test.yml` | `go vet` + `go test ./...` |
| `build.yml` | `go build` of `cmd/main.go` |
| `mirror-gitlab.yml` | Re-runs lint + test + build, then pushes to GitLab on `main` |

## Credentials

Same org/group setup as sibling game operators (`palworld-operator`, `bedrock-operator`):

- GitHub org secret `GITLAB_TOKEN`
- GitLab group variables on `dk-raas/dkai` for Harbor (`HARBOR_*`, `DOCKER_AUTH_CONFIG`)

## License reminder

Published operator images contain **only** the manager binary — never Terraria or TShock
binaries. Game pods pull `ghcr.io/pryaxis/tshock` (or `ryshe/terraria`).
