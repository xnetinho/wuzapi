FROM golang:1.21-alpine AS build
RUN apk add --no-cache gcc musl-dev
RUN mkdir -p /app/cmd/wuzapi
COPY . /app
WORKDIR /app/cmd/wuzapi
RUN ls -la /app/cmd/wuzapi # Adicione este comando para verificar se os arquivos Go estão sendo copiados
RUN go mod tidy
ENV CGO_ENABLED=1
RUN go build -o /app/server .

FROM alpine:latest
RUN mkdir /app
COPY ./static /app/static
COPY --from=build /app/server /app/
VOLUME [ "/app/dbdata", "/app/files" ]
WORKDIR /app
ENV WUZAPI_ADMIN_TOKEN=SetToRandomAndSecureTokenForAdminTasks
CMD [ "/app/server", "-logtype", "json" ]
