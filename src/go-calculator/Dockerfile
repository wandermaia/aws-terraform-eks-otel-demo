FROM golang:latest AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN GOOS=linux CGO_ENABLED=0 go build -ldflags="-w -s" -o api .

FROM scratch
COPY --from=builder /app/api .
CMD ["./api"]
EXPOSE 7000
