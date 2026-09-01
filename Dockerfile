# ---------- build stage ----------
# This stage carries the whole Go toolchain. Nothing here ships: once the binary
# is built, the runtime stage below copies it out and Docker discards the rest.
FROM golang:1.26.0 AS build

WORKDIR /src

COPY . .

# CGO_ENABLED=0 produces a statically linked binary with no libc dependency, so
# the runtime image needs to contain nothing but the binary itself.
RUN CGO_ENABLED=0 go build -o /out/groupie-tracker .


# ---------- runtime stage ----------
# This is the image that actually ships.
FROM alpine:3.20

LABEL org.opencontainers.image.title="Groupie Tracker" \
      org.opencontainers.image.description="Runs a dockerized version of the Groupie Tracker project"

# FetchAll and GeocodeAllLocations call the Groupie Trackers API and Nominatim
# over HTTPS at start up, so the image needs root certificates to verify those
# connections. Without them both calls fail and the site comes up empty.
RUN apk add --no-cache ca-certificates

WORKDIR /app

# Only what the running server actually reads: the binary from the build stage,
# plus the three directories the handlers open by relative path from here.
COPY --from=build /out/groupie-tracker ./groupie-tracker
COPY templates/ ./templates/
COPY static/ ./static/
COPY data/ ./data/

# Run as an unprivileged user rather than root.
RUN adduser -u 1001 -D groupie-tracker

# /app stays owned by root, so the two paths the server writes to have to be
# handed over explicitly:
#   logs/ - initLog creates it and appends every log line to a file inside it.
#           Without this the MkdirAll in initLog fails and trips its log.Fatalf.
#   data/ - coordinates.json is rewritten whenever a location that is not
#           already cached has to be geocoded.
RUN mkdir -p /app/logs && chown -R groupie-tracker /app/logs /app/data

USER groupie-tracker

# The server listens on 8080 unless the PORT environment variable overrides it.
EXPOSE 8080

CMD ["./groupie-tracker"]
