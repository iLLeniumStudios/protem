# Builds two images from one context:
#   docker build -t <manager image> .                      (the operator, UI and API; default target)
#   docker build --target enforcer -t <agent image> .      (the agent for target clusters)
#
# The manager embeds the built UI from web/dist. Run `cd web && bun run build` first when
# web/src changed, so the image does not ship a stale UI.
#
# The builder runs on the build machine's platform and cross-compiles, so a multi-arch build
# does not compile under emulation.
FROM --platform=$BUILDPLATFORM golang:1.26 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/
COPY web/web.go web/web.go
COPY web/dist/ web/dist/

# GOARCH has no default so the binary matches the platform the build was asked for.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -o manager ./cmd/ && \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -o enforcer ./cmd/enforcer/

FROM gcr.io/distroless/static:nonroot AS enforcer
WORKDIR /
COPY --from=builder /workspace/enforcer .
USER 65532:65532
ENTRYPOINT ["/enforcer"]

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532
ENTRYPOINT ["/manager"]
