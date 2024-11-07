FROM golang:1.23-alpine AS build
RUN apk add --no-cache gcc musl-dev
RUN mkdir /app
COPY . /app
WORKDIR /app
RUN go mod tidy
ENV CGO_ENABLED=1
RUN go build -o server .

FROM alpine:latest
RUN apk add --no-cache curl
RUN mkdir /app
COPY ./static /app/static
COPY ./migrations /app/migrations
COPY --from=build /app/server /app/
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
 CMD curl --fail http://localhost:8080/health || exit 1

VOLUME [ "/app/dbdata", "/app/files" ]
WORKDIR /app
ENV WUZAPI_ADMIN_TOKEN=SetToRandomAndSecureTokenForAdminTasks
CMD [ "/app/server", "-logtype", "json" ]
