FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/vkr ./cmd/app

FROM alpine:3.21
WORKDIR /app
COPY --from=build /out/vkr /app/vkr
COPY migrations /app/migrations
EXPOSE 8080
CMD ["/app/vkr"]
