# 05 — CI/CD

## 1. Principles

- **The Makefile is the interface.** CI calls `make <target>`; it does not inline commands. A developer running `make check` runs exactly what CI runs.
- **Fast feedback first.** Lint and unit tests before integration; fail early.
- **Releases are reproducible.** A tag produces byte-identical artefacts given the same inputs.
- **Nothing reaches a client unsigned.** Every image signed, every release accompanied by an SBOM.
- **No secrets in the repo, ever.** OIDC to the registry; short-lived credentials only.

---

## 2. Workflows

```
.github/workflows/
├── ci.yml              # every push and PR
├── integration.yml     # PRs and main, heavier suite
├── security.yml        # daily + on dependency changes
├── release.yml         # on tag v*
├── perf.yml            # nightly on main, and on label
└── docs.yml            # docs site build and deploy
```

---

## 3. `ci.yml` — the main gate

```yaml
name: CI

on:
  pull_request:
  push:
    branches: [main]

concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true

permissions:
  contents: read

env:
  GO_VERSION: "1.23"
  NODE_VERSION: "22"

jobs:
  commitlint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: wagoid/commitlint-github-action@v6

  lint-go:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "${{ env.GO_VERSION }}", cache: true }
      - uses: golangci/golangci-lint-action@v6
        with: { version: latest, args: --timeout=5m }
      - name: gofumpt
        run: make fmt-check
      - name: no financial data in logs
        run: make lint-logs

  lint-web:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with: { node-version: "${{ env.NODE_VERSION }}", cache: npm, cache-dependency-path: web/package-lock.json }
      - run: npm ci --prefix web
      - run: npm run lint --prefix web
      - run: npm run typecheck --prefix web

  lint-sql:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-python@v5
        with: { python-version: "3.12" }
      - run: pip install sqlfluff sqlfluff-templater-dbt
      - run: make lint-sql

  generated-artifacts:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "${{ env.GO_VERSION }}", cache: true }
      - name: regenerate
        run: make api-gen deploy-gen sqlc-gen
      - name: fail if generated output differs
        run: |
          if [[ -n "$(git status --porcelain)" ]]; then
            echo "::error::Generated artefacts are stale. Run 'make api-gen deploy-gen sqlc-gen' and commit."
            git --no-pager diff
            exit 1
          fi

  test-go:
    runs-on: ubuntu-latest
    needs: [lint-go]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "${{ env.GO_VERSION }}", cache: true }
      - name: unit tests with race detector
        run: make test
      - name: coverage
        run: make cover
      - uses: codecov/codecov-action@v4
        with:
          files: ./coverage.out
          flags: go
          fail_ci_if_error: true
        env:
          CODECOV_TOKEN: ${{ secrets.CODECOV_TOKEN }}

  test-web:
    runs-on: ubuntu-latest
    needs: [lint-web]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with: { node-version: "${{ env.NODE_VERSION }}", cache: npm, cache-dependency-path: web/package-lock.json }
      - run: npm ci --prefix web
      - run: npm run test:coverage --prefix web
      - uses: codecov/codecov-action@v4
        with: { files: ./web/coverage/lcov.info, flags: web, fail_ci_if_error: true }
        env:
          CODECOV_TOKEN: ${{ secrets.CODECOV_TOKEN }}

  golden:
    runs-on: ubuntu-latest
    needs: [test-go]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "${{ env.GO_VERSION }}", cache: true }
      - name: golden dataset suite
        run: make test-golden
      - name: annotate expected.json changes
        if: github.event_name == 'pull_request'
        run: |
          if git diff --name-only origin/${{ github.base_ref }}... | grep -q 'expected.json'; then
            echo "::warning::A golden expectation changed. This alters findings at every client. Domain review required."
          fi

  build:
    runs-on: ubuntu-latest
    needs: [test-go, test-web]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "${{ env.GO_VERSION }}", cache: true }
      - uses: actions/setup-node@v4
        with: { node-version: "${{ env.NODE_VERSION }}", cache: npm, cache-dependency-path: web/package-lock.json }
      - run: make build
```

### 3.1 Why `generated-artifacts` exists

`deploy/compose`, `deploy/quadlet`, `deploy/chart` and `web/src/api` are generated from single sources of truth. Hand-maintained copies drift within two releases, and the divergence surfaces as a client running a configuration nobody has tested. This job makes drift impossible rather than discouraged.

---

## 4. `security.yml`

```yaml
name: Security

on:
  schedule: [{ cron: "0 3 * * *" }]
  pull_request:
    paths: ["go.mod", "go.sum", "web/package-lock.json", "**/Dockerfile"]
  workflow_dispatch:

permissions:
  contents: read
  security-events: write

jobs:
  vulns:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "1.23", cache: true }
      - name: govulncheck
        run: |
          go install golang.org/x/vuln/cmd/govulncheck@latest
          govulncheck ./...
      - name: npm audit
        run: npm audit --audit-level=high --prefix web

  licences:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "1.23", cache: true }
      - name: go licence check
        run: |
          go install github.com/google/go-licenses@latest
          go-licenses check ./... \
            --allowed_licenses=MIT,Apache-2.0,BSD-2-Clause,BSD-3-Clause,ISC,MPL-2.0,PostgreSQL
      - name: npm licence check
        run: |
          npx license-checker --production \
            --onlyAllow "MIT;Apache-2.0;BSD-2-Clause;BSD-3-Clause;ISC;0BSD;CC0-1.0" \
            --start web

  codeql:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: github/codeql-action/init@v3
        with: { languages: go,javascript }
      - uses: github/codeql-action/autobuild@v3
      - uses: github/codeql-action/analyze@v3

  secrets:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: gitleaks/gitleaks-action@v2
        env: { GITHUB_TOKEN: "${{ secrets.GITHUB_TOKEN }}" }

  images:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: build for scan
        run: make images-local
      - uses: aquasecurity/trivy-action@master
        with:
          image-ref: localhost/audit:ci
          format: sarif
          output: trivy.sarif
          severity: HIGH,CRITICAL
          exit-code: "1"
      - uses: github/codeql-action/upload-sarif@v3
        if: always()
        with: { sarif_file: trivy.sarif }
```

**AGPL is deliberately absent from the allowed list.** MinIO and OpenObserve are deployed as unmodified network services, not linked into our code, so they never appear in a dependency manifest. If AGPL shows up in `go-licenses` output, something has been linked that should not be — investigate rather than adding it to the allowlist.

---

## 5. `release.yml`

Triggered by a `v*` tag on `main`. Produces the client-shippable bundle.

```yaml
name: Release

on:
  push:
    tags: ["v*"]

permissions:
  contents: write
  packages: write
  id-token: write        # cosign keyless signing

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }

      - uses: actions/setup-go@v5
        with: { go-version: "1.23", cache: true }
      - uses: actions/setup-node@v4
        with: { node-version: "22", cache: npm, cache-dependency-path: web/package-lock.json }

      - name: version consistency
        run: make verify-version VERSION=${{ github.ref_name }}

      - uses: docker/setup-qemu-action@v3
      - uses: docker/setup-buildx-action@v3

      - name: build multi-arch images
        run: make images VERSION=${{ github.ref_name }} PLATFORMS=linux/amd64,linux/arm64

      - uses: sigstore/cosign-installer@v3
      - name: sign images
        run: make sign VERSION=${{ github.ref_name }}

      - uses: anchore/sbom-action@v0
        with: { format: spdx-json, output-file: sbom.spdx.json }

      - name: build offline bundle
        run: make bundle VERSION=${{ github.ref_name }}

      - name: bundle smoke test on a clean host
        run: make test-install-clean BUNDLE=dist/audit-platform-${{ github.ref_name }}.tar.gz

      - name: changelog
        run: make changelog VERSION=${{ github.ref_name }}

      - uses: softprops/action-gh-release@v2
        with:
          files: |
            dist/audit-platform-${{ github.ref_name }}.tar.gz
            dist/audit-platform-${{ github.ref_name }}.tar.gz.sha256
            sbom.spdx.json
          body_path: dist/CHANGELOG-${{ github.ref_name }}.md
```

### 5.1 `verify-version`

Fails the release unless the tag, the Go version constant, the Helm chart version, the compose bundle version and `web/package.json` all agree. A client must be able to state exactly what they are running; a mismatch here makes that impossible.

### 5.2 `test-install-clean`

Runs `install.sh` from the bundle on a fresh container with no network egress, then asserts the health endpoint responds and a login page renders. This catches the release that builds perfectly and cannot be installed — which is the failure that costs a deployment day.

### 5.3 Changelog

Generated from conventional commits, then **hand-edited** to add the audit-relevant section: *which shipped tests changed behaviour in this release*. That section is what clients read, and it cannot be generated.

---

## 6. Branch protection on `main`

- Require PR, at least one approval (two for `CODEOWNERS` paths per `02-REPOSITORY.md` §5.3)
- Required checks: `commitlint`, `lint-go`, `lint-web`, `lint-sql`, `generated-artifacts`, `test-go`, `test-web`, `golden`, `build`, `codecov/project`, `codecov/patch`
- Linear history, no force push, no deletion
- Dismiss stale approvals on new commits
- Include administrators

---

## 7. Dependencies

Renovate, grouped and scheduled to reduce noise:

```json
{
  "extends": ["config:recommended"],
  "schedule": ["before 6am on monday"],
  "packageRules": [
    { "matchUpdateTypes": ["minor", "patch"], "groupName": "non-major", "automerge": true,
      "matchCurrentVersion": "!/^0/" },
    { "matchPackagePatterns": ["zen", "duckdb", "^dbt"], "automerge": false,
      "labels": ["domain-review-required"],
      "description": "Engine and analytics versions can change findings. Never automerge." },
    { "matchDepTypes": ["action"], "pinDigests": true }
  ],
  "vulnerabilityAlerts": { "labels": ["security"], "automerge": false }
}
```

The ZEN, DuckDB and dbt rule matters: a minor version bump in an engine can change evaluation or floating-point behaviour and therefore change findings. Those upgrades run the golden suite and get domain review.

---

## 8. Runner and cost notes

- `ubuntu-latest` throughout. No self-hosted runners — they accumulate state and undermine reproducibility.
- `concurrency` cancels superseded runs on the same ref.
- Integration tests use testcontainers on the standard runner; no service containers, so the same setup works locally.
- Multi-arch builds only run on release, not on every PR — QEMU emulation of arm64 is slow and the coverage is not worth it per-PR.
