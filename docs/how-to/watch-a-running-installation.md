# Watch a running installation

Margince measures itself over one Prometheus exposition per program. This page
is how to scrape it and what its shape means for an alert. The settings that
decide who may scrape, and the address the worker answers on, are in
[configuration.md](../reference/configuration.md).

## Scrape the API and the worker separately

Each program answers for itself. The API serves `/metrics` on the port it
serves the app on, and the worker serves its own on the address
`observe_addr` names. A scrape of one says nothing about the other, so both are
targets.

`/metrics` is closed by default: it wants a bearer credential until an operator
opens it. Give the scraper that credential rather than opening the endpoint,
because the exposition names queue depths and connection counts.

## An exposition is sections, and a missing one is a signal

The handler writes one section per subsystem and concatenates them: the process
answering the scrape, the outbox backlog, what the relay has published, the
database pool, and whatever the program adds.

A section that cannot measure writes **nothing** and the rest still serves.
That keeps one broken probe from costing the whole exposition, and it has a
consequence for alerting:

> A series that is absent says the target could not measure that part of
> itself.

So alert on absence as well as on value. `absent(margince_outbox_unpublished)`
is a different fault from that series being high, and a rule that only
thresholds the value stays quiet through the first one.

## The scrape has two seconds

The handler bounds the whole response at two seconds. A section that would take
longer is cut, which shows up as that section's series going absent rather than
as a failed scrape. Keep the scrape interval well above two seconds so a slow
section cannot overlap the next request.

## The format is pinned, not negotiated

The response is always the Prometheus text format. A scraper that asks for
OpenMetrics first still receives text. The sections are rendered by different
writers, so negotiating would let one exposition carry two formats and parse as
neither.

## What to watch first

- `margince_outbox_unpublished`: domain writes that have not reached the bus.
  Every mutation writes one, so a number that climbs and stays means the relay
  is behind, and events are the trail that audit and search read.
- `margince_relay_published_total`: the other half of that pair. A flat
  counter beside a climbing backlog says the relay stopped rather than slowed.
- `margince_pgxpool_acquire_empty_total`: acquisitions that found no free
  connection. It rises before latency does.
- `margince_process_goroutines`: a count that only grows is a leak, and it
  grows for a while before anything else notices.

The full set is whatever the writers register, so read those rather than a list
here: `backend/internal/platform/httpserver/metrics.go` names the sections in
the order they are written, and each section's writer names its own series.

## Logs carry the request's identity

Every request makes one `correlation_id`, and the write path links it to the
audit row it produced. A log line, the audit entry and the event of one request
can be followed as one trace, which is what makes a report of "this did not
happen" answerable.
