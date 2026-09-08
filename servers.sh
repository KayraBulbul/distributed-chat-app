#!/bin/bash

start_or_run () {
    docker inspect caddy-balancer > /dev/null 2>&1

    if [ $? -eq 0 ]; then
        echo "Starting websocket servers..."
        docker start redis
        docker start caddy-balancer
        docker start ws1
        docker start ws2
        docker start ws3
    fi
}

case "$1" in
  start)
    start_or_run
    ;;
  stop)
    echo "Stopping websocket servers..."
    docker stop ws1
    docker stop ws2
    docker stop ws3
    docker stop caddy-balancer
    docker stop redis
    ;;
  *)
    echo "Usage: $0 {start|stop}"
    exit 1
esac
