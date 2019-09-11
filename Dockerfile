# ==================
# Build stage.
# ==================
ARG target
FROM golang:1.13 as builder

ARG goarch
ENV GOOS linux
ENV GOARCH $goarch
ENV CGO_ENABLED 0

ENV image heptio/velero
WORKDIR /go/src/github.com/${image}
RUN \
  git clone --depth=1 https://github.com/${image} . && \
  go build -o /velero cmd/velero/main.go

# ==================
# Final stage.
# ==================
FROM $target/alpine

LABEL maintainer="Jesse Stuart <hi@jessestuart.com>"

COPY qemu-* /usr/bin/
COPY --from=builder /velero /velero

ARG goarch
ADD https://github.com/restic/restic/releases/download/v0.9.5/restic_0.9.5_linux_${goarch}.bz2 /restic.bz2
RUN apk add --no-cache --update ca-certificates && \
  bunzip2 restic.bz2 && \
  chmod +x /restic && \
  mv /restic /usr/bin/restic

USER nobody:nobody

ENTRYPOINT ["/velero"]
