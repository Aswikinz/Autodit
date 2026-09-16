# SonarCloud findings review

Reviewed 2026-09-16 for project `Aswikinz_Autodit`, organization `aswikinz`.
The public issues API returned 17 unresolved findings and the security-hotspot
API returned zero. The latest analysis covered revision
`dce71f8a1b795ff4df82cbaca2dd1c2ec0f5ff3d`.

## Changes by finding

| Rule | Count | Resolution in source |
| --- | ---: | --- |
| GitHub Actions / Docker S6505 | 2 | Both npm CI installations disable dependency lifecycle scripts; the production build and frontend tests work with scripts disabled. |
| Docker S8545 | 1 | The Caddy build consumes committed `deploy/proxy/go.mod` and `go.sum`, verifies checksums, and builds with `-mod=readonly`. Dependency resolution is no longer performed during the image build. |
| Python S8707 | 2 | Coverage accepts only the two named report files inside the repository. The deployment smoke test accepts archives confined to `dist/`. Resolved paths must remain within the allowed directory. |
| Python S2083 | 1 | Generator output paths are fixed and confined to the repository. JSON is written through an explicit file handle; manifest content is never interpreted as a destination path. |
| Python S5443 | 2 | The restore drill streams its binary database dump through a private, uniquely created temporary directory. It no longer opens a predictable `/tmp` filename in the database container. |
| Go S2083 | 1 | Static serving opens files through `os.Root`, rejects invalid paths, exposes only built assets and the SPA entry point, and blocks symlink escapes. |
| Go S2092 | 1 | Cookies default to `Secure: true`. Only explicit HTTP demo mode on `localhost` or `127.0.0.1` can opt out. OIDC, HTTPS and non-loopback origins retain Secure. Tests cover each boundary. |
| TypeScript S2871 / S9379 / S9011 | 3 | Weekdays use numeric sorting, the login input no longer steals focus, and the login button explicitly submits its form. |
| PL/SQL CharVarchar | 4 | Corrected analysis scope for PostgreSQL migrations; Oracle's VARCHAR2 recommendation does not apply to this database. Migration SQL remains verified against real PostgreSQL. |

The generator and CLI findings described local files/arguments as HTTP inputs;
these tools are not HTTP handlers. Explicit path confinement still makes their
operational boundary easier to verify and prevents accidental access elsewhere.

## PostgreSQL analysis scope

SonarCloud [treats `.sql` as Oracle PL/SQL by default](https://docs.sonarsource.com/sonarqube-cloud/advanced-setup/languages/t-sql).
PostgreSQL is not listed among its [supported SQL dialects](https://docs.sonarsource.com/sonarqube-cloud/advanced-setup/languages).
The root `.sonarcloud.properties` excludes only the PostgreSQL migration SQL
from that incompatible analyzer. It does not disable Go SQL-injection checks.
Existing immutable migrations are not rewritten to satisfy an Oracle rule.

The [automatic-analysis configuration](https://docs.sonarsource.com/sonarqube-cloud/advanced-setup/automatic-analysis)
uses `.sonarcloud.properties`; `sonar-project.properties` is ignored by that mode.
The connected IDE may continue displaying previously downloaded server findings
until the next successful cloud analysis and synchronization.

## Verification and cloud status

Regression checks cover encoded traversal, Windows path separators, alternate
stream syntax, symlink escapes, private-file exposure and cookie transport
policy. Python boundary tests run on Linux so symlink coverage is exercised.
The real database and race-enabled test suite, TypeScript build and frontend
contract tests pass. The revised restore drill verified 32 observations and
eight immutable snapshots without replacing the live database.

These are local source fixes and review decisions. No findings were remotely
marked resolved, no credentials were extracted from VS Code, and no commits were
pushed. SonarCloud must analyze the updated revision before its issue count or
quality gate can be reported as cleared. Revisit the local-demo cookie exception
if the next analysis requests explicit review; HTTPS/OIDC must retain Secure.
