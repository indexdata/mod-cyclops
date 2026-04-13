# syntax=docker/dockerfile:1

FROM golang:1.26.1 AS build

WORKDIR /app

# Copy module files first so dependency downloads can be cached.
COPY go.mod go.sum ./

# Download go deps, caches GOMODCACHE.
RUN --mount=type=cache,sharing=shared,target=/go/pkg/mod \
  go mod download

COPY . ./

# Build, caches GOCACHE
RUN --mount=type=cache,sharing=shared,target=/root/.cache/go-build \
  CGO_ENABLED=0 \
  GOOS=linux \
  go build -o /mod-cyclops .

# create runtime user
RUN adduser \
  --disabled-password \
  --gecos "" \
  --home "/nonexistent" \
  --shell "/sbin/nologin" \
  --no-create-home \
  --uid 65532 \
  cyclops-user

# create small runtime image
FROM scratch

# need to copy SSL certs and runtime use
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /etc/passwd /etc/passwd
COPY --from=build /etc/group /etc/group

# copy binaries
COPY --from=build /mod-cyclops .
# copy migrations if needed
#COPY --from=build /app/migrations /migrations

ENV HTTP_PORT=12370
EXPOSE ${HTTP_PORT}

# Run
USER cyclops-user:cyclops-user
CMD ["/mod-cyclops"]
