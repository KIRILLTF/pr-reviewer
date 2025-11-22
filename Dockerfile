FROM golang:1.21-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go env -w GOPROXY=https://proxy.golang.org
RUN apk add --no-cache git
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /pr-service ./cmd/service

FROM alpine:3.18
COPY --from=builder /pr-service /pr-service
EXPOSE 8080
CMD ["/pr-service"]
