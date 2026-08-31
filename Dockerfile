FROM golang:1.23.6-alpine3.21 AS builder

WORKDIR /src/gospace

COPY gospace/go.mod gospace/go.sum ./
RUN go mod download

COPY gospace ./
RUN CGO_ENABLED=0 go build -trimpath -o /out/gospace ./cmd/api

FROM alpine:3.21.2

RUN apk add --no-cache ca-certificates \
    && addgroup -S goapp \
    && adduser -S goapp -G goapp

COPY --from=builder /out/gospace /usr/local/bin/gospace

USER goapp
EXPOSE 6060
ENTRYPOINT ["/usr/local/bin/gospace"]
CMD ["-port", "6060"]
