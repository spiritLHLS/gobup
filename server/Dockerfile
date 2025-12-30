FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o gobup main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata sqlite
WORKDIR /root/

COPY --from=builder /app/gobup .
COPY --from=builder /app/web ./web

RUN mkdir -p /rec /root/data

ENV TZ=Asia/Shanghai
EXPOSE 12380

CMD ["./gobup", "-port", "12380", "-work-path", "/rec"]
