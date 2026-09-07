#!/bin/bash

start_or_run () {
    if [ $? -eq 0 ]; then
        echo "Removing websocket servers.."
        docker rm ws1 ws2 ws3
    fi
}

start_or_run
