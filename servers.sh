#!/bin/bash

start_or_run () {
    docker inspect caddy-balancer > /dev/null 2>&1

    if [ $? -eq 0 ]; then
        echo "Starting websocket servers..."
        docker start redis
        docker start caddy-balancer
        docker start chat-app-node-exporter-1
        docker start chat-app-grafana-1
        docker start chat-app-prometheus-1
        docker start chat-app-db-1
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
    docker stop chat-app-node-exporter-1
    docker stop chat-app-grafana-1
    docker stop chat-app-prometheus-1
    docker stop chat-app-db-1
    docker stop redis
    ;;
  build)
    echo "Building websocket servers..."
    docker run -d --name ws1 --hostname ws1 --env-file ./backend/config/.env --network ws-caddy ws-server:latest
    docker run -d --name ws2 --hostname ws2 --env-file ./backend/config/.env --network ws-caddy ws-server:latest
    docker run -d --name ws3 --hostname ws3 --env-file ./backend/config/.env --network ws-caddy ws-server:latest
    ;;
  remove)
    echo "Removing websocket servers.."
    docker rm ws1 ws2 ws3
    ;;
  *)
    echo "Usage: $0 {start|stop|build|remove}"
    exit 1
esac
