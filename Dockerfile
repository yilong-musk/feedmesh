# The build stage runs on the native builder and cross-compiles, so the arm64
# image is produced without QEMU emulation.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev

WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/feedmesh ./cmd/feedmesh
RUN mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/feedmesh /usr/local/bin/feedmesh
# Docker initialises an empty named volume with the ownership of the image
# directory, so the non-root user can write its cache.
COPY --from=build --chown=65532:65532 /out/data /data

ENV FEEDMESH_LISTEN=0.0.0.0:3000 \
    FEEDMESH_CACHE=/data

VOLUME ["/data"]
EXPOSE 3000

ENTRYPOINT ["/usr/local/bin/feedmesh"]
CMD ["serve"]
