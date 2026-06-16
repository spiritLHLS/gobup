FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o gobup main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata sqlite ffmpeg
WORKDIR /root/

COPY --from=builder /app/gobup .

RUN mkdir -p /rec /root/data

ENV TZ=Asia/Shanghai
EXPOSE 12380

CMD ["./gobup", "-port", "12380", "-work-path", "/rec", "-data-path", "/root/data"]
