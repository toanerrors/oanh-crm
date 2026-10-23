FROM node:26.5.1-alpine@sha256:233761595746769ebfdb6090f44fc7cdf818ae0ce62d2b37e0367723b9823e36 AS web-build
WORKDIR /src/web
RUN npm install --global --ignore-scripts pnpm@10.34.7
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm test && VITE_DEMO_MODE=false pnpm build

FROM golang:1.26.5-alpine@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 AS server-build
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 GOMAXPROCS=2 go build -trimpath -ldflags="-s -w" -o /out/oanh-server ./cmd/server \
    && CGO_ENABLED=0 GOMAXPROCS=2 go build -trimpath -ldflags="-s -w" -o /out/healthcheck ./cmd/healthcheck \
    && mkdir /out/data && chmod 0700 /out/data

FROM scratch
ARG VCS_REF=unknown
LABEL org.opencontainers.image.title="Oanh Accounting" \
    org.opencontainers.image.version="0.11.0" \
    org.opencontainers.image.revision=$VCS_REF
COPY --from=server-build /out/oanh-server /oanh-server
COPY --from=server-build /out/healthcheck /healthcheck
COPY --from=server-build --chown=65532:65532 /out/data /data
COPY --from=web-build /src/web/dist /web
USER 65532:65532
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 CMD ["/healthcheck"]
ENTRYPOINT ["/oanh-server"]
CMD ["-addr=0.0.0.0:8080", "-data=/data", "-web=/web", "-secure-cookie=true"]
