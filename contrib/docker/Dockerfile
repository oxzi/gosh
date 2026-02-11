# Build stage
FROM golang:1.23-bookworm AS build

WORKDIR /src
COPY . .

RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /gosh .

# Runtime stage
FROM gcr.io/distroless/static-debian12:latest

# gosh needs root at startup to chroot() and setuid()/setgid()
# It drops privileges itself to the configured user/group

# Copy the binary
COPY --from=build /gosh /gosh

# Create passwd/group files for user.Lookup() to work
# Using UID/GID 65532 which is the "nonroot" user in distroless
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# We need to add passwd and group files for gosh's user.Lookup()
# distroless has no shell, so we create these in the build stage
FROM golang:1.23-bookworm AS passwd-builder
RUN echo 'root:x:0:0:root:/root:/sbin/nologin' > /tmp/passwd && \
    echo 'gosh:x:65532:65532:gosh:/nonexistent:/sbin/nologin' >> /tmp/passwd
RUN echo 'root:x:0:' > /tmp/group && \
    echo 'gosh:x:65532:' >> /tmp/group

# Final stage
FROM gcr.io/distroless/static-debian12:latest

COPY --from=build /gosh /gosh
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=passwd-builder /tmp/passwd /etc/passwd
COPY --from=passwd-builder /tmp/group /etc/group

# Create store directory (will be owned by root initially, gosh chowns it)
WORKDIR /data

EXPOSE 8080

ENTRYPOINT ["/gosh"]
CMD ["-config", "/config/gosh.yml"]
