# The UI is built first and embedded into the Go binary, so the deployed image
# is a single static binary with no runtime dependencies.

FROM node:22-alpine AS ui
WORKDIR /app/ui
COPY ui/package.json ui/package-lock.json ./
RUN npm ci
COPY ui/ ./
# vite writes to ../web/dist, i.e. /app/web/dist
RUN npm run build

FROM golang:1.24-alpine AS build
WORKDIR /src
# go.sum is optional: this module has no external dependencies.
COPY go.mod go.su[m] ./
RUN go mod download
COPY . .
COPY --from=ui /app/web/dist ./web/dist
RUN go vet ./... && go test ./... && \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/erpx ./cmd/server

FROM alpine:3.20
RUN adduser -D -u 10001 erpx && mkdir -p /data && chown erpx /data
COPY --from=build /out/erpx /usr/local/bin/erpx
USER erpx
ENV ERPX_DATA=/data/erpx.json PORT=8080
EXPOSE 8080
CMD ["erpx"]
