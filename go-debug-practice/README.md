# Go debugging practice

Eight realistic backend packages, each shipped with a bug ticket and a failing
test. Your job is the same every time: read the ticket, reproduce, find the
cause, fix it, make the test pass.

Nothing in the source code hints at the bug. Comments, names and structure are
written the way the code would actually look in a repo.

## Exercises

Roughly easiest to hardest.

| # | Package | Scenario | Run |
|---|---|---|---|
| ex01 | `ex01_order_service` | Order service — draft orders and warehouse stock reservations | `go test ./ex01_order_service/` |
| ex02 | `ex02_shipping_planner` | Shipping planner — splitting a picklist into manifests | `go test ./ex02_shipping_planner/` |
| ex03 | `ex03_payment_client` | Payment gateway client — charging a card with retries | `go test ./ex03_payment_client/` |
| ex04 | `ex04_rate_poller` | FX rate poller — snapshotting many currency pairs | `go test ./ex04_rate_poller/` |
| ex05 | `ex05_shard_aggregator` | Shard metrics aggregator — fan-out counts across DB shards | `cd ex05_shard_aggregator && go test ./...` |
| ex06 | `ex06_batch_processor` | Batch processor — bounded-concurrency job publishing | `go test ./ex06_batch_processor/` |
| ex07 | `ex07_ttl_cache` | Session cache — in-memory TTL cache with hit/miss stats | `go test -race ./ex07_ttl_cache/` |
| ex08 | `ex08_url_fetcher` | "Review this PR" — concurrent URL fetcher for record enrichment | `go test ./ex08_url_fetcher/` |

Two things to note:

- **ex05 is a separate Go module** pinned to `go 1.21`. Run it from inside its
  own directory; the root `go test ./...` will skip it.
- **ex07 needs `-race`.** Without it the test may well pass. That is part of the
  exercise.

Every other exercise fails with a plain `go test`. All eight have been checked
to fail on the buggy code and pass (with and without `-race`) once fixed.

## How to practise

For each exercise:

1. Read only `README.md` first. Write down what you think the cause might be
   before opening the source — the ticket is written to support a hypothesis.
2. Run the test and read the failure. Say out loud what the failure tells you.
3. Then read the code. Narrate what you are checking and why — in a real
   interview, the reasoning is most of the score.
4. Fix it, re-run, and re-run with `-race` even where the README does not ask.

Timebox yourself to ~15 minutes per exercise. If you are stuck past that, read
the solution — getting stuck silently is the thing to avoid in the interview.

## Running everything

```bash
./check.sh
```

Runs every exercise (expect failures — that is the point), then every solution
(expect passes).

Solutions only:

```bash
go test ./solutions/... && go test -race ./solutions/...
cd solutions/ex05 && go test ./... && go test -race ./...
```

## Solutions

`solutions/exNN/` holds the fixed source plus `SOLUTION.md`, written in Thai:
what the bug is, why it happens, the minimal fix, and what to say out loud while
debugging it in an interview.

Do not read ahead. The whole value is in the fifteen minutes before you look.
