FROM golang:1.21-alpine AS build
RUN apk add --no-cache gcc musl-dev

# Defina o diretório de trabalho no contêiner
WORKDIR /app

# Copie os arquivos go.mod e go.sum e baixe as dependências
COPY go.mod go.sum ./
RUN go mod download

# Copie o restante dos arquivos do código
COPY . .

# Verifique se os arquivos Go foram copiados corretamente
RUN ls -la /app/cmd/wuzapi

# Compile a aplicação
RUN go build -o /app/server ./cmd/wuzapi

# Use uma imagem base mínima para a execução
FROM alpine:latest

# Crie o diretório /app e copie os arquivos estáticos e o binário compilado
RUN mkdir /app
COPY ./static /app/static
COPY --from=build /app/server /app/

# Defina os volumes para persistência de dados
VOLUME [ "/app/dbdata", "/app/files" ]

# Defina o diretório de trabalho
WORKDIR /app

# Defina a variável de ambiente para o token de administração
ENV WUZAPI_ADMIN_TOKEN=SetToRandomAndSecureTokenForAdminTasks

# Defina o ponto de entrada do contêiner
CMD [ "/app/server", "-logtype", "json" ]