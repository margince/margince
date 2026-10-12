<!-- prose:plain -->
# record360: the shared kit behind Deal360, Company360 and Contact360

Three record pages answer the same question about other records: *where do we stand with this, and
what do I do about it?* They render the same parts. These are grounded text with sources, a section
shell that tells "nothing here" from "you may not read this", and wire values turned into words. This
folder holds those parts.

## The reading, in parts

Every record page reads in the same order, and the parts are here:

- `reading.tsx`: the group (`RecordReading`, `RecordReadingPair`) and the call (`CallCard`). The head
  of the call has an indigo mark that says a machine read the record. Then comes the standing, with
  the sentence it rests on, and then what the call was read from.
- `today.tsx`: the "Needs attention" panel (`TodayPanel`) and its two row shapes. One is the move
  the agent is asking for (`FoundMove`); the other is a to-do the record already has (`TodoRow`).
- `spine.tsx` and `timelinespine.ts`: the thread. `timelinespine.ts` reads its source from a bare
  timeline page, for the records that have no composite read.

A record page hands in its own answers (which standing, which rows), and owes the reader the same
shape as the record next to it.

`moment.ts` is the moment as a page reads it. The server picks one from a fixed ladder. This file
holds the calls every page must answer the same way:

- the word for the rule (`MOMENT_RULE_LABEL`)
- whether it belongs in the day's work at all (`momentIsARow`)
- whether what it rests on says something its headline does not (`basisAddsARecord`)

`FoundMove` draws the moment, on the contact page and in the account brief alike, and
`MomentEvidence` in `momentevidence.tsx` draws the chips under it.

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
the ones `verdict.tsx` uses, are in `record360.css`. `RailPanel` is in `src/design-system/`, where
`frontend/AGENTS.md` says a building block that a second screen imports belongs.
