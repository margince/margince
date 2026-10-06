# record360: the shared kit behind Deal360, Company360 and Contact360

Three record pages answer the same question about different records: *where do
we stand with this, and what do I do about it?* They render the same parts.
These are grounded prose with citations, a section shell that tells "nothing
here" from "you may not read this", and wire values turned into words. This directory holds those parts.

## The reading, in parts

Every record page reads in the same order, and the parts are here:

- `reading.tsx`: the group (`RecordReading`, `RecordReadingPair`) and the call
  (`CallCard`). The call's head carries an indigo mark saying a machine read the
  record, then the standing with the sentence it rests on, then what the call
  was read from.
- `today.tsx`: the "Needs attention" panel (`TodayPanel`) and its two row shapes,
  the move the agent is asking for (`FoundMove`) and a to-do the record already
  carries (`TodoRow`).
- `spine.tsx` and `timelinespine.ts`: the thread. `timelinespine.ts` reads its
  source off a bare timeline page for the records that have no composite read.

A record page hands in its own answers (which standing, which rows) and owes
the reader the same shape as the record beside it.

`moment.ts` is the moment as a page reads it. The server picks one from a fixed
ladder. This file holds the judgements every page must answer the same way:

- the word for the rule (`MOMENT_RULE_LABEL`)
- whether it belongs in the day's work at all (`momentIsARow`)
- whether what it rests on says anything its headline has not
  (`basisAddsARecord`)

`FoundMove` draws the moment, on the contact page and in the account brief
alike, and `momentevidence.tsx`'s `MomentEvidence` draws the chips under it.

## What belongs here

A part lives here when two or more record pages render it and it does not
depend on which record it renders. `SentenceList` renders sentences and their
citations; it never asks whether they came from a deal or a company.

Two pages using a part today is not enough. A company's commercial panel and a
deal's offer table are different components that look alike, and merging them
produces one component with two modes and a flag.

## The contract already agrees

`CompanyBriefSentence` is the sentence type for the company brief, the deal
status card, Contact360 and the growth-fit panel alike. Only its name says
"Company". The kit types against it directly and calls it what it is, so a
reader of `record360` is not told that a deal's sentence is a company's.

## Styles

The shared classes keep their `co-` prefix (`co-brief-lines`, `co-card` and so
on) and live in `company360.css`. No module here imports a screen stylesheet.
The kit's own classes, including those `verdict.tsx` uses, live in
`record360.css`. `RailPanel` is in `src/design-system/`, where
`frontend/AGENTS.md` says a primitive another screen imports belongs.
