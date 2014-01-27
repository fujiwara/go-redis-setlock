# go-redis-setlock

Like the setlock command using Redis.

## Build & Install

Install to $GOPATH.

    $ go get github.com/fujiwara/go-redis-setlock

## Getting binary

* [bin/darwin-386/go-redis-setlock](http://fujiwara.github.io/go-redis-setlock/bin/darwin-386/go-redis-setlock)
* [bin/darwin-amd64/go-redis-setlock](http://fujiwara.github.io/go-redis-setlock/bin/darwin-amd64/go-redis-setlock)
* [bin/linux-386/go-redis-setlock](http://fujiwara.github.io/go-redis-setlock/bin/linux-386/go-redis-setlock)
* [bin/linux-amd64/go-redis-setlock](http://fujiwara.github.io/go-redis-setlock/bin/linux-amd64/go-redis-setlock)
* [bin/linux-arm/go-redis-setlock](http://fujiwara.github.io/go-redis-setlock/bin/linux-arm/go-redis-setlock)
* [bin/windows-386/go-redis-setlock](http://fujiwara.github.io/go-redis-setlock/bin/windows-386/go-redis-setlock)
* [bin/windows-amd64/go-redis-setlock](http://fujiwara.github.io/go-redis-setlock/bin/windows-amd64/go-redis-setlock)

## Usage

    $ go-redis-setlock [-nNxX] KEY program [ arg ... ]

    --redis (Default: 127.0.0.1:6379): redis-host:redis-port
    --expires (Default: 86400): The lock will be auto-released after the expire time is reached.
    --keep: Keep the lock after invoked command exited.
    -n: No delay. If KEY is locked by another process, redis-setlock gives up.
    -N: (Default.) Delay. If KEY is locked by another process, redis-setlock waits until it can obtain a new lock.
    -x: If KEY is locked, redis-setlock exits zero.
    -X: (Default.) If KEY is locked, redis-setlock prints an error message and exits nonzero.

Redis Server >= 2.6.12 is required.
