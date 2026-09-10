# Chat traffic generator

Run from the repository root with the app, database, Redis, and Caddy running:

```sh
go run ./cmd/loadtest -users 100
```

This creates 100 users through `/users`, connects them through Caddy's `/echo`
endpoint, and sends a plain text message per user roughly every five seconds.
It reads broadcasts continuously and reconnects disconnected users with the same
user ID, using exponential backoff with jitter. Press Ctrl+C to close all clients.

For a timed run:

```sh
go run ./cmd/loadtest -users 100 -interval 2s -ramp 10s -duration 5m
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `-users` | `10` | Number of simulated users |
| `-url` | `http://localhost:8880` | Caddy's HTTP base URL, without a path |
| `-origin` | `http://localhost:5173` | Origin accepted by the WebSocket server |
| `-interval` | `5s` | Average delay between messages per user, varied by +/-25% |
| `-ramp` | `5s` | Spread user starts over this duration; `0s` starts all at once |
| `-duration` | `0s` | Total duration including ramp-up; `0s` runs until interrupted |

No new dependencies are needed. This uses the project's existing Go module.

## Watching a failure

While the tester runs, stop one server, for example `docker stop ws1`.
The tester reconnects through Caddy; watch the connection distribution in Grafana.
Restore the server with `docker start ws1`. Existing connections won't automatically
rebalance when it returns. This exercises simulated clients, not the frontend's
JavaScript reconnect implementation.

## Output and limits

Progress is printed every five seconds after ramp-up, plus a final summary:

- `created`: users successfully created. Creation failures are logged and not retried.
- `connected`: currently open tester sockets; becomes zero on shutdown.
- `sent`: successful socket writes, not confirmed database saves or deliveries.
- `received`: chat broadcasts read across all simulated users; excludes server info.
- `reconnects`: successful connections after a user's first connection.
- `errors`: user creation, connection, read, or write failures, excluding shutdown.

Each run creates persistent users with a `loadtest-` username prefix and real
messages in the app database. It does not delete them afterward. Broadcast copies
are counted individually, so received can greatly exceed sent. This is a traffic
generator, not a delivery correctness or latency benchmark. With N users each
sending every T seconds, expect about N/T sends and up to N*N/T broadcast
deliveries per second across the clients.

## Tests

```sh
go test -race ./cmd/loadtest
```

Tests use a local mock HTTP/WebSocket server and don't write to the app database.
