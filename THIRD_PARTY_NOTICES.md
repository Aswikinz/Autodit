# Third-party components

Autodit is Apache-2.0. Dependencies retain their own licences.
Exact application versions are pinned in `go.mod`, `go.sum`, `web/package.json`
and `web/package-lock.json`. Runtime image versions are in the deployment manifest.

| Component | Licence | Upstream |
|---|---|---|
| Go | BSD-3-Clause | https://go.dev |
| ZEN / zen-go 2.0.1 | MIT | https://github.com/gorules/zen-go |
| JDM Editor 1.52.0 | MIT | https://github.com/gorules/jdm-editor |
| Monaco Editor 0.52.2 and Monaco React 4.7.0 | MIT | Package manifests / upstream repositories |
| PostgreSQL | PostgreSQL | https://www.postgresql.org/about/licence/ |
| pgx | MIT | https://github.com/jackc/pgx |
| shopspring/decimal | MIT | https://github.com/shopspring/decimal |
| parquet-go | BSD-3-Clause | https://github.com/parquet-go/parquet-go |
| google/uuid | BSD-3-Clause | https://github.com/google/uuid |
| go-oidc | Apache-2.0 | https://github.com/coreos/go-oidc |
| Go OAuth2 packages | BSD-3-Clause | https://go.googlesource.com/oauth2 |
| React, TanStack, openapi-fetch, Vite, TypeScript | MIT | Package manifests / upstream repositories |
| Lucide | ISC | https://github.com/lucide-icons/lucide |
| Caddy 2.11.4 | Apache-2.0 | https://github.com/caddyserver/caddy |
| Podman, podman-compose | Apache-2.0 | https://github.com/containers |
| Debian runtime distribution | Multiple FOSS licences | https://www.debian.org/legal/licenses/ |

Transitive notices remain in their package/source distributions. The JDM editor
depends on ExcelJS; its UUID dependency is overridden to 11.1.1 to address the
published buffer-bounds advisory. No commercial GoRules BRMS component is used.
No AGPL service is bundled in this evaluation stack.

The web build adapts three change callbacks in JDM Editor 1.52.0 so closing
the editor cannot discard a pending Function or Decision table edit. The
version-checked adapter is in `web/src/lib/jdmTransform.ts`; dependency upgrades
must review it. Monaco, its workers and the expression WebAssembly module are
bundled locally for offline use.

The Caddy build applies dependency fixes for `golang.org/x/text` (0.41.0),
gRPC (1.83.2), compression (1.18.7) and OpenTelemetry (1.44.0). Its source version remains 2.11.4;
see `Containerfile.proxy` and the complete `deploy/proxy/go.mod` / `go.sum`
lock files for reproducible build inputs. The application
also pins patched compression and Go system packages in `go.mod`.
