# ── Stage 1: frontend ────────────────────────────────────────────────────────
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ── Stage 2: backend (embeds the built SPA) ──────────────────────────────────
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -tags embedui \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/console ./cmd/console

# ── Stage 3: runtime ─────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/console /console
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/console"]
