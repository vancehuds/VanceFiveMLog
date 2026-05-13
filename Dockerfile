FROM golang:1.25.9-alpine3.22 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/vancefivemlog ./cmd/server

FROM alpine:3.22.4
LABEL org.opencontainers.image.title="VanceFiveMLog" \
      org.opencontainers.image.description="Go + PostgreSQL FiveM/Qbox log explorer" \
      org.opencontainers.image.source="https://github.com/vancehuds/VanceFiveMLog" \
      org.opencontainers.image.licenses="AGPL-3.0-or-later"
WORKDIR /app
COPY --from=build /out/vancefivemlog /app/vancefivemlog
COPY web /app/web
EXPOSE 8080
CMD ["/app/vancefivemlog"]
