<!-- prose:plain -->
<!-- describes: backend/internal/modules/contacts backend/internal/modules/deals backend/api/crm.yaml -->
# Where a record keeps its id in an outside system

A record that came from another system keeps that system's id on the record itself, next to the name of the system. A caller who holds the id can then find the record again. The rules a caller follows are in [rest-api.md](../reference/rest-api.md#an-id-from-your-system).

## Why not a separate table

A table that links a record to its outside ids would be a third answer to "where did this come from". The first answer is the system name on each record. The second is the map the import keeps. Each one can change on its own, so the three can disagree about where a record came from.

A separate table also allows several ids for one record in one system. The pair kept on the record allows one.

## Why the fields on a deal are not enough

`source` and `acquisition_source` name a channel. They say how a deal came in, not which record it is in another system. Many deals share one channel, so a lookup by either returns many deals.

## What it costs

Erasing a record erases its outside id with it, because the id is a field of the record. There is no single table to erase from.

## Why 128 characters

The system name and the id each stop at 128 characters. The create request and the list filters refuse a longer value, and the database also refuses a longer id. A cap keeps the index small and stops a pasted block of text from being stored as an id. The number matches the cap `capture_trace.source_system` already has.
