FROM golang:1.26-alpine AS build
ARG HTTPS_PROXY
ARG HTTP_PROXY
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/classifier ./cmd/classifier

FROM alpine:3.21
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/classifier /usr/local/bin/classifier
USER 10001:10001
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
    CMD ["classifier", "-healthcheck"]
ENTRYPOINT ["classifier"]
