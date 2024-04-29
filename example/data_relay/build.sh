#!/bin/bash

# 设置编译目标为Linux amd64
export CGO_ENABLED=0
export GOOS=linux
export GOARCH=amd64

go mod tidy
go build -tags netgo -v -o go-example-server main.go
scp -r go-example-server Dockerfile service.yml etc root@v4.airiot.tech:/opt/app/build/example