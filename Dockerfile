FROM scratch
COPY image-prefetcher /
ENTRYPOINT ["/image-prefetcher"]
