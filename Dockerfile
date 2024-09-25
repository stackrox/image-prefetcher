FROM scratch
ARG ARCH=amd64
COPY ./image-prefetcher-${ARCH} /image-prefetcher
ENTRYPOINT ["/image-prefetcher"]
