FROM golang:1.24-alpine AS build
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN go build -o /volcanopredict ./cmd/server

FROM alpine:3.22
WORKDIR /app
COPY --from=build /volcanopredict /app/volcanopredict
EXPOSE 8080
CMD ["/app/volcanopredict"]
