FROM golang:1.24-alpine AS build
WORKDIR /app
# go.su[m] é um glob que casa com go.sum quando ele existe e não falha quando
# não existe. O módulo fica sem dependências externas até a primeira ser
# importada; a partir daí o lockfile entra na imagem e o build passa a ser
# reprodutível sem precisar editar este arquivo.
COPY go.mod go.su[m] ./
RUN go mod download
COPY . .
RUN go build -o /volcanopredict ./cmd/server

FROM alpine:3.22
WORKDIR /app
# Executa sem privilégio: nada aqui precisa de root.
RUN adduser -D -u 10001 volcano
COPY --from=build /volcanopredict /app/volcanopredict
USER volcano
EXPOSE 8080
CMD ["/app/volcanopredict"]
