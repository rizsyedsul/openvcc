# Quickstart: local

Boots two echo backends, the engine, Prometheus, and Grafana via Docker Compose. ~30 seconds.

## Prerequisites

- Docker Engine 24+ and Docker Compose v2
- `make`

## Steps

```sh
make compose-up
```

The first run builds the engine and echo container images locally; subsequent runs reuse the cache.

When everything is up:

| URL | What |
|---|---|
| http://localhost:8080/ | Proxy. Each response carries `X-Backend` and `X-Cloud`. |
| http://localhost:9090/metrics | Prometheus metrics directly off the engine. |
| http://localhost:9091/ | Bundled Prometheus UI. |
| http://localhost:3000/ | Grafana (anonymous viewer; admin login `admin`/`admin`). |

## See it work

Distribution check:

```sh
for i in $(seq 1 20); do
  curl -sD - http://localhost:8080/ -o /dev/null \
    | awk -F': ' '/^X-Cloud/ {gsub(/\r/,"",$2); print $2}'
done | sort | uniq -c
```

You should see ~10 `aws` and ~10 `azure`.

Failover:

```sh
docker compose -f deploy/compose.yaml stop app-aws
sleep 8
# now every X-Cloud is azure
```

Restart:

```sh
docker compose -f deploy/compose.yaml start app-aws
```

Reload via admin (bearer token comes from the compose file's env):

```sh
curl -X POST -H "Authorization: Bearer dev-admin-token-change-me" \
     http://localhost:8081/admin/reload
```

## Tear down

```sh
make compose-down
```
