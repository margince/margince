<!-- prose:plain -->
<!-- describes: backend/internal/platform/httpserver/chassis.go backend/internal/platform/httpserver/observe.go backend/internal/platform/httpserver/requesttrace.go frontend/src/api/requestreference.ts -->
# Trace a request from the browser to the server log

For an operator or engineer who has a HAR file or a reference ID from a user and needs the server side
of the same request.

## What the browser sees

Every response under `/v1` carries `X-Request-Id`. The value is the `correlation_id` the server made for
that request. It is made on the server, so a header a client sends is ignored.
The same value is also sent as `X-Correlation-ID`, the older name. That header is deprecated: read
`X-Request-Id` in new code.

A signed-in response also carries `Server-Timing: total;dur=<ms>`. It is the time the server took to
start the answer. Responses from public routes and the sign-in routes do not carry it, so those routes
give no timing a caller could measure. A 401 carries no `Server-Timing` either, because no session
answered it.

When a request fails, the app ends the error message with `Reference: <id>`. That id is the
`X-Request-Id` of the failed answer.

## Find the server side

1. Take the id from the `X-Request-Id` response header of a HAR row, or from the `Reference:` text.
2. Search the server log for `correlation_id=<id>`, or `"correlation_id":"<id>"` in JSON logs.
3. The `http request` line has `method`, `path`, `route`, `status` and `duration_ms`. It also has
   `db_ms`, `pool_wait_ms` and `statements`: the database time, the wait for a pool connection and the
   number of statements the request ran.
4. Other log lines of the same request carry the same `correlation_id`.
5. A write leaves an `event_outbox` row whose `trace.correlation_id` is the same id, and
   `trace.audit_log_id` names the `audit_log` row.

A request that panicked has an `http request` line with status 500, and the `handler panic` line carries the
same id.

## Read the timings

`route` is the matched route pattern, so lines group by endpoint. It is missing for the mounts that
chi does not serve: `/mcp`, `/oauth`, the webhooks and the extension inbound routes.

The HAR time of a request minus its `Server-Timing` total is the time spent outside the API: the browser
queue, the network and any proxy. For a streamed download the total is the time to the first byte.
