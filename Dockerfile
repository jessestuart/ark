# ==================
# Build stage.
# ==================
ARG target
FROM golang:1.12 as builder

ARG goarch
ENV GOARCH $goarch
ENV GOROOT /usr/local/go
ENV GOPATH /go
ENV CGO_ENABLED 0
ENV PATH "$GOROOT/bin:$GOPATH/bin:$GOPATH/linux_$GOARCH/bin:$PATH"

ARG image
WORKDIR /go/src/github.com/${image}
RUN git clone https://github.com/${image} .
RUN go build cmd/velero/main.go && mv ./main /velero

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

USER nobody

ENTRYPOINT ["/velero"]
