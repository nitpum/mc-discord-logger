FROM golang:1.20-alpine AS builder

WORKDIR /build
COPY go.mod ./
RUN go mod download
COPY *.go .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o app

FROM alpine:3.18 AS compressor
WORKDIR /compress
RUN apk add --no-cache upx binutils
COPY --from=builder /build/app .
RUN strip app -o app-striped
# RUN upx --best --lzma app-striped -o app-compressed

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=compressor /compress/app-striped /app
ENTRYPOINT ["/app"]
