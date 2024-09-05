FROM golang:1.22.0 AS build
WORKDIR /build
COPY ./ ./
RUN go mod verify
RUN make binary

FROM scratch
COPY --from=build /build/image-prefetcher /image-prefetcher
ENTRYPOINT ["/image-prefetcher"]
