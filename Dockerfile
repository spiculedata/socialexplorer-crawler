# Multi-stage build: compile in the first stage, run from a lean runtime image.

# tdlib base image (republishable from Dockerfile.tdlib in this repo).
# NOTE: keep this reference pinned once the image is next published:
#   FROM acrnetcus.azurecr.io/tdlib@sha256:<digest>
# Pinning by digest protects the build against a mutable :latest tag.
FROM acrnetcus.azurecr.io/tdlib:latest AS builder

# The tdlib base image defaults to a non-root user; switch back for the build.
USER root

# Go toolchain: go.mod requires go >= 1.26.0, which alpine's packaged go
# (3.21 ships 1.23.x) does not satisfy — install the official tarball,
# checksum-verified.
ARG GO_VERSION=1.26.11
ARG GO_AMD64_SHA256=9849f4e04ee35303714e34fbc3ac8ab44ea3e448a95aa182f91e3b9b79c710ec
ARG GO_ARM64_SHA256=6c2f29b458b5add0835f18b776f69c8881a6d88428f1484a6da55755e620757e

RUN apk add --no-cache curl ca-certificates

# Install the Go toolchain from the official tarball, checksum-verified.
RUN set -eux; \
    arch="$(apk --print-arch)"; \
    case "$arch" in \
      x86_64)  goarch=amd64; gosha="${GO_AMD64_SHA256}" ;; \
      aarch64) goarch=arm64; gosha="${GO_ARM64_SHA256}" ;; \
      *) echo "unsupported arch: $arch"; exit 1 ;; \
    esac; \
    curl -fsSLO "https://go.dev/dl/go${GO_VERSION}.linux-${goarch}.tar.gz"; \
    echo "${gosha}  go${GO_VERSION}.linux-${goarch}.tar.gz" | sha256sum -c -; \
    tar -C /usr/local -xzf "go${GO_VERSION}.linux-${goarch}.tar.gz"; \
    rm "go${GO_VERSION}.linux-${goarch}.tar.gz"
ENV PATH=/usr/local/go/bin:$PATH

RUN apk add --no-cache g++ make cmake git linux-headers binutils

RUN ln -s /usr/include/asm-generic /usr/include/asm

# Set the working directory
WORKDIR /app

# Copy Go modules manifests
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy application source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o main .

# Stage 2: minimal runtime image — no compiler, no source tree
FROM acrnetcus.azurecr.io/tdlib:latest

# Set the working directory
WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/main .

# Default storage root, writable by the runtime user
RUN mkdir -p /tmp/crawl && chown tdlib:tdlib /tmp/crawl

# Run as the non-root user created by the tdlib base image
USER tdlib

# Set the entrypoint.
ENTRYPOINT ["./main"]
CMD ["--dapr", "--dapr-mode", "job"]
