FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
ARG VERSION=0.1.0
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/codex-auth-sync ./cmd/codex-auth-sync

FROM alpine:3.20
RUN adduser -D -H -u 10001 sync && mkdir -p /data && chown sync:sync /data
COPY --from=build /out/codex-auth-sync /usr/local/bin/codex-auth-sync
USER sync
EXPOSE 8787
ENTRYPOINT ["codex-auth-sync"]
CMD ["serve", "--listen", "0.0.0.0:8787", "--data", "/data/events.json"]
