FROM golang:1.25.1 AS build-stage

# Update and clean up to reduce vulnerabilities
RUN apt-get update && apt-get dist-upgrade -y && apt-get autoremove -y && apt-get clean && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY go/ ./
ENV BUILD_VERSION=0.0.2

RUN go get -u ./...
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-X 'main.BuildVersion=$BUILD_VERSION'" -o sa-exporter .

FROM quay.io/prometheus/busybox-linux-amd64:glibc AS bin
LABEL maintainer="The Prometheus Authors <prometheus-developers@googlegroups.com>"

COPY --from=build-stage /app/sa-exporter /
COPY resources/ /resources/
RUN mkdir /data/
RUN chown "nobody:nobody" /data/
RUN chmod +x /sa-exporter

USER nobody
EXPOSE 9800

ENTRYPOINT [ "/sa-exporter" ]
