#!/bin/bash
mkdir -p bin/resources
cd go
cp -R ../resources resources
cp ../.env .
echo "unit tests"
go test -v -cover -coverprofile=coverage.out
echo
echo "building"
CGO_ENABLED=0 GOOS=linux go build -o ../bin/sa-exporter .
echo
rm -rf resources
rm .env
cd ..

