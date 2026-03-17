FROM golang:1.25-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o cyclaw .

FROM alpine:3.23

RUN apk add --no-cache ca-certificates bash curl

WORKDIR /app

COPY --from=builder /build/cyclaw .

VOLUME ["/app/data"]

ENTRYPOINT ["./cyclaw"]
