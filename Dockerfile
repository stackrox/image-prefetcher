FROM golang:1.21 AS builder
LABEL authors="porridge@redhat.com"
COPY . /image-prefetcher
RUN cd /image-prefetcher && CGO_ENABLED=0 go build -a -ldflags '-extldflags "-static"' . && find . -ls

FROM scratch
COPY --from=builder /image-prefetcher/image-prefetcher /
CMD ["image-prefetcher"]