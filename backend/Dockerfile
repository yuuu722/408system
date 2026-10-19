FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /exam408 ./cmd/api
FROM debian:bookworm-slim
COPY --from=golang:1.25-alpine /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
RUN sed -i 's|http://deb.debian.org|https://deb.debian.org|g' /etc/apt/sources.list.d/debian.sources && apt-get update && apt-get install -y --no-install-recommends ca-certificates poppler-utils tesseract-ocr tesseract-ocr-chi-sim && rm -rf /var/lib/apt/lists/*
RUN useradd --create-home app && mkdir -p /data/uploads && chown -R app /data
USER app
COPY --from=build /exam408 /exam408
EXPOSE 8080
ENTRYPOINT ["/exam408"]

