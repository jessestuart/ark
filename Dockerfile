# ==================
# Build stage.
# ==================
ARG target
FROM golang:1.11 as builder

ARG goarch
ENV GOARCH $goarch
ENV GOROOT /usr/local/go
ENV GOPATH /go
ENV CGO_ENABLED 0
ENV PATH "$GOROOT/bin:$GOPATH/bin:$GOPATH/linux_$GOARCH/bin:$PATH"

ARG image
WORKDIR /go/src/github.com/${image}
RUN git clone https://github.com/${image} .
RUN go build cmd/ark/main.go && mv ./main /ark

# ==================
# Final stage.
# ==================
FROM $target/alpine

LABEL maintainer="Jesse Stuart <hi@jessestuart.com>"

COPY qemu-* /usr/bin/
COPY --from=builder /ark /ark

ARG goarch
ADD https://github.com/restic/restic/releases/download/v0.9.3/restic_0.9.3_linux_${goarch}.bz2 /restic.bz2
RUN apk add --no-cache --update ca-certificates && \
    bunzip2 restic.bz2 && \
    chmod +x /restic && \
    mv /restic /usr/bin/restic

USER nobody

ENTRYPOINT ["/ark"]
