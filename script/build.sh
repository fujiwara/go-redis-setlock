#!/bin/sh -x

set -e
set -u

for GOOS in darwin
do
    for GOARCH in 386 amd64
    do
        mkdir -p "bin/$GOOS-$GOARCH"
        GOOS="$GOOS" GOARCH="$GOARCH" go build -o "bin/$GOOS-$GOARCH/go-redis-setlock"
    done
done

for GOOS in linux
do
    for GOARCH in 386 amd64 arm
    do
        mkdir -p "bin/$GOOS-$GOARCH"
        GOOS="$GOOS" GOARCH="$GOARCH" go build -o "bin/$GOOS-$GOARCH/go-redis-setlock"
    done
done
