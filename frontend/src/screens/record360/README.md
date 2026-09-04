# record360 — the shared kit behind Deal360, Company360 and Person360

Three record pages answer the same question about different records: *where do
we stand with this, and what do I do about it?* They render the same parts —
grounded prose with citations, a section shell that knows the difference
between "nothing here" and "you may not read this", wire values turned into
words. This directory is those parts, owned in one place.

It exists because the parts were already shared and had no home. They lived in
`company360.tsx`, which grew to 3140 lines — six times the repo's 500-line file
cap — and eighteen other modules imported from it. `person360.tsx` and
`network.tsx` reached into a *company* screen for `dealRoleLabel`;
`dealstatus.tsx` reached in for `SentenceList`. Every one of those imports was
a shared component wearing one entity's name.

## The reading, in parts

Every record page now reads in the same order, and the parts are here:
`reading.tsx` holds the group (`RecordReading`, `RecordReadingPair`) and THE
CALL (`CallCard`: the head whose indigo mark says a machine read the record,
the standing with the sentence it rests on, and whatever the call was read
from under it);
`today.tsx` holds WHAT NEEDS A PERSON TODAY (`TodayPanel`) and its two row
shapes — the move the agent is asking for (`FoundMove`) and a to-do the record
already carries (`TodoRow`); `spine.tsx` is the thread, and `timelinespine.ts` reads
its source off a bare timeline page for the records that have no composite
read. A record page hands in its own answers — which standing, which rows —
and owes the reader the same shape as the record beside it.

## What belongs here

A part belongs in the kit when it holds no opinion about WHICH record it is
rendering. `SentenceList` renders sentences and their citations; it never asks
whether they came from a deal or a company. That is the test.

A part does NOT belong here because two pages happen to both use it today. A
company's commercial panel and a deal's offer table are different components
that look alike, and merging them produces one component with two modes and a
flag — which is the thing this kit exists to avoid, not an instance of it.

## The contract already agrees

`OrganizationBriefSentence` is the sentence type for the org brief, the deal
status card, Person360 and the growth-fit panel alike. Only its NAME says
"Organization". The kit types against it directly and calls it what it is, so a
reader of `record360` is not told that a deal's sentence is a company's.

## The CSS keeps its `co-` prefix, for now

The stylesheets still spell these classes `co-brief-lines`, `co-card` and so
on, and this kit leaves them alone. Renaming them reaches four stylesheets and
every page that overrides one, which would bury the extraction it was mixed
into. The names are wrong and the styles are shared; that is a rename to do on
its own.

No module here imports a screen stylesheet. `shells.tsx` used to, because
`SectionCard` rendered `.co-card` and that rule lives in `company360.css` —
that coupling is gone with the component, whose call sites moved to `RailPanel`
and which nothing drew afterwards. `RailPanel` and `OverlayFallback` are in
`src/design-system/` now, where `frontend/AGENTS.md` says a primitive another
screen imports belongs. `verdict.tsx` carries no such import either; its classes
are the kit's own and live in `record360.css`.
