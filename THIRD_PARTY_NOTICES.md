# Third-party components

Autodit is Apache-2.0. Unmodified dependencies retain their own licences.
Exact application versions are pinned in `go.mod`, `go.sum`, `web/package.json`
and `web/package-lock.json`. Runtime image versions are in the deployment manifest.

| Component | Licence | Upstream |
|---|---|---|
| Go | BSD-3-Clause | https://go.dev |
| ZEN / zen-go 2.0.1 | MIT | https://github.com/gorules/zen-go |
| JDM Editor 1.52.0 | MIT | https://github.com/gorules/jdm-editor |
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
