FROM golang:alpine AS builder

ENV GOOS="linux" CGO_ENABLED=0

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . ./

RUN go build -ldflags '-extldflags "-static"' -o bin/server $TARGET

FROM alpine
WORKDIR /app
COPY --from=builder /src/bin/ ./
CMD ./server
