FROM golang:1.24 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /kindtks .

FROM alpine:3.20

COPY --from=builder /kindtks /usr/local/bin/kindtks
COPY profiles/ /profiles/

ENTRYPOINT ["kindtks"]
