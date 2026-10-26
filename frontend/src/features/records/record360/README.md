<!-- prose:plain -->
# record360: the shared kit behind Deal360, Company360 and Contact360

Three record pages answer the same question about other records: *where do we stand with this, and
what do I do about it?* They render the same parts. These are grounded text with sources, a section
shell that tells "nothing here" from "you may not read this", and wire values turned into words. This
folder holds those parts, except two that code outside the kit reads as well. The grounded text with
its sources is in `src/components/citations.tsx`, and the wire values turned into words are in
`src/lib/recordlabels.ts`. `index.ts` still exports both, so a record page takes the whole kit from
one place.

## The reading, in parts

Every record page reads in the same order, and the parts are here:

- `reading.tsx`: the two reference sections a reader looks up and does not read through, side by side
  (`RecordReadingPair`).
- `today.tsx`: the "Needs attention" panel (`TodayPanel`) and its two row shapes. One is the move
  the agent is asking for (`FoundMove`); the other is a to-do the record already has (`TodoRow`).
- `setaside.ts`: the "Not now" a move carries and the Undo after it (`useSetAside`). Each page hands
  in its own two calls, so the deal, contact and company moves are put off the same way.

A record page hands in its own answers (which standing, which rows), and owes the reader the same
shape as the record next to it.

`moment.ts` is the moment as a page reads it. The server picks one from a fixed ladder. This file
holds the call every page must answer the same way: whether what the moment rests on says something
its headline does not (`basisAddsARecord`).

`FoundMove` draws a move, and `MomentEvidence` in `momentevidence.tsx` draws the chips under a
moment.

## What belongs here

A part belongs here when two or more record pages render it, and it does not depend on which record
it renders. `SentenceList` renders sentences and their sources; it never asks whether they came from
a deal or a company.

Two pages that use a part today is not enough. The commercial panel of a company and the offer table
of a deal are separate components that look alike. Merging them makes one component with two modes
and a flag.

## The contract already agrees

`CompanyBriefSentence` is the sentence type for the company brief, the deal status card, Contact360
and the growth-fit panel alike. Only its name says "Company". The kit types against it directly and
calls it what it is. So a reader of `record360` is not told that a deal's sentence is a company's.

## Styles

The shared classes keep their `co-` prefix (`co-brief-lines`, `co-card` and so on) and are in
`company360.css`. No module here imports the stylesheet of a screen. The kit's own classes, including
the signal strip's in `verdict.tsx`, are in `record360.css`. `RailPanel` is in `src/design-system/`, where
`frontend/AGENTS.md` says a building block that a second screen imports belongs.
