FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -o tresbbs-server ./cmd/tresbbs-server

FROM alpine:3.19
RUN apk add --no-cache sqlite-libs ca-certificates
WORKDIR /bbs
COPY --from=builder /app/tresbbs-server /usr/local/bin/
COPY templates/ ./templates/

EXPOSE 2323

ENTRYPOINT ["tresbbs-server"]
CMD ["-addr", ":2323", "-db", "/bbs/data/tresbbs.db"]
