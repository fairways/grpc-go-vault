FROM golang:1.18-alpine as builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build server.go

FROM alpine as release
COPY --from=builder /src/server server
EXPOSE 3000
ENTRYPOINT ["/server"]