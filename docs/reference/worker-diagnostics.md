<!-- prose:plain -->
# Profile, test and stop the worker

How to profile a running worker, read one site with no database, and stop the worker cleanly. The
flags of the worker are in [programs.md](programs.md).

## Profiling a running worker: `--observe-pprof`

`MARGINCE_OBSERVE_PPROF=true` serves the Go `net/http/pprof` handlers under `/debug/pprof/` on the
`--observe-addr` listener. Use it when the metrics of
[the operator surface of the worker](operator-endpoints.md#the-operator-surface-of-the-worker) show a problem they cannot explain. That
may be a worker whose heap grows fast, or whose CPU stays at its limit. You have no outside view of
which code path did it. `go_memstats_*` says there is more heap; a heap profile says what holds it.

**Off by default; turn it on briefly** for the rollout that has to find a problem, then
turn it off. It adds no listener and no port. The profiles are served on the same address as
`/healthz` and `/metrics`, with no sign-in as there, and kept inside the same cluster. The
surface it adds shows more than the probes. A goroutine dump names the stack of every goroutine.

And `/debug/pprof/cmdline` answers the command line of the process, with any flag passed there and
not through the environment (a `--dsn` carries its password). The worker logs a `WARN` line that
names the address at every boot with it on.

To set it `true` with no `--observe-addr` is a **boot error**, because there would be no listener to
serve it on. A value `strconv.ParseBool` refuses is a boot error too, and is not read as off.

From the own network of a pod (the listener binds a private interface):

```sh
# the heap as it is right now — what a memory burst is diagnosed from
curl -s http://<pod>:9101/debug/pprof/heap > heap.pb.gz
# allocations over the next 10s, as a delta — what is being allocated DURING a burst
curl -s 'http://<pod>:9101/debug/pprof/allocs?seconds=10' > allocs.pb.gz
go tool pprof -top heap.pb.gz
```

`/debug/pprof/` lists every named profile (`heap`, `allocs`, `goroutine`, `block`, `mutex`,
`threadcreate`); `profile` and `trace` are CPU profiling and the run trace. The listener keeps its
`10s` write timeout for everything else. A request with `?seconds=N` moves its own time limit out by
`N`, so `profile?seconds=30` works as it does in any other place. A heap snapshot comes at once.

## `worker siteread`: the deep-read debug loop (no DB)

`worker siteread <url…> [--urls-file f]` runs the whole `crawl→extract→merge` pipeline in memory
(no Postgres, no Redis, no staging). It prints each step on the way:

- pages, with the reasons it skipped them;
- every field or fact it pulled out, with its evidence;
- every finding the gate dropped, with why;
- merge decisions;
- token and time numbers per model call.

It needs one model: `--model provider:model` (such as `anthropic:claude-opus-4-8`, which needs the
BYOK env key of the provider) or `--ai-fake` (a crawl dry run). This lane opens no database, so it
never reads the stored binding of the installation. `--max-pages/--max-bytes/--wall` override the
limits per run. `--json <path|->` writes a report a machine can read and diff. `--dump-pages <dir>`
writes the cut-down text of each page.

The step that reads facts runs two routed lanes at the same time as the crawl (page calls start as
pages commit):

- `site_fact_extract`: one small call per page that holds facts, cheap tier first. The reply names
  numbered parts of the page, and does not quote. A fast model can do that.
- `site_extract`: the one profile call, premium tier first, over the parts of the page that say
  the most about who the company is.

Go checks the evidence against the named part (reference evidence: the stored text is the own text
of the page). Judge any binding you think of using against the pinned quality floor:
`make -C backend e2e-siteread` with `MARGINCE_E2E_MODEL=provider:model`. That costs money and uses
the network, as an E2E test against `gradion.com`. Another model must score the same or higher to
pass. A normal read takes 10 to 25 seconds from start to end, based on how hard the origin slows the
crawl.

The runner and the embedding lane start only on a stored model binding, or on `--ai-fake` for the
offline fake model. A binding is stored through Settings → AI, or on a fresh install from
`seeds.ai_routing` in `margince.yaml`. The relay, retention, the workflow dispatch that events start (`cg:workflows`), and the clock
time scan always run. Shutdown is clean: subscriber handlers already running end their ack before
the process stops.

## Stopping the worker

`SIGTERM` (or `SIGINT`) stops the worker in this order. The times add up to the budget a supervisor
has to leave room for:

1. **The job runner stops taking jobs at once.** No job is claimed after that. Anything still in
   the queue stays for another replica, or for the next start of this one.
2. **Running jobs go on for `--job-drain-window`** (default `20s`, and it must be above zero), with
   their context whole. A
   job that ends inside the window completes as normal; it is not retried and does not run twice.
3. **Then their context is cancelled.** The worker waits up to **`5s`** more for each to return.
   River records the stopped try and retries it; the failure is marked `interrupted`, and
   gets a reason.
4. The event lanes, the bus and the database pool are closed, and the process stops.

So the worker needs `--job-drain-window` + `5s`, plus some seconds of shutdown steps, between
`SIGTERM` and `SIGKILL`. On Kubernetes that is the `terminationGracePeriodSeconds` of the pod:
set it **at or above the drain window plus `10s`**. The default window fits the common default of
`30s`. Raise the grace time each time you raise the window. If not, `SIGKILL` reaches the pod before
the cancel step, and cuts off the goroutines of the stopped jobs in the middle of a write:

```yaml
spec:
  terminationGracePeriodSeconds: 30   # >= --job-drain-window (20s) + 10s
```

Some jobs often run longer than the window, such as a long crawl or an import of many records.
They are stopped at every rollout, and run again on another replica. That is safe (River retries them) but wastes work.
So size the window, and the grace time with it, to the jobs this installation runs.
