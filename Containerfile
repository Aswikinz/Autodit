FROM docker.io/library/node:24-bookworm-slim AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --ignore-scripts --no-audit --no-fund
COPY web/ ./
COPY test/fixtures/population.json /src/test/fixtures/population.json
RUN npm run build

FROM docker.io/library/golang:1.27-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/audit ./cmd/audit

FROM docker.io/library/debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 audit && useradd --uid 10001 --gid audit --no-create-home audit \
    && mkdir -p /data/snapshots /app && chown -R audit:audit /data /app
COPY --from=build /out/audit /usr/local/bin/audit
COPY --from=web /src/web/dist /app/web
COPY rulepack/ /app/rulepack/
WORKDIR /app
ENV AUTODIT_WEB_DIR=/app/web AUTODIT_RULEPACK=/app/rulepack AUTODIT_SNAPSHOT_DIR=/data/snapshots
USER 10001:10001
EXPOSE 8080
STOPSIGNAL SIGTERM
ENTRYPOINT ["audit"]
CMD ["api"]
