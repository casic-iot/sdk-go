FROM alpine:latest

ADD go-example-server /app/
ADD etc/config.docker.yaml /app/etc/config.yaml
WORKDIR /app
ENTRYPOINT ["./go-example-server"]