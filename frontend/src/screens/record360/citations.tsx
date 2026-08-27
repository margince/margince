// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Grounded prose and the receipts under it.
//
// Every record page renders sentences a model or the deterministic fallback
// wrote, each citing the records it rests on. This is that rendering, once: a
// citation can never be clickable on the company page and flat on the deal
// page, because both pages call the same component.

import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { formatNumber } from "../../format/format";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";

/**
 * One sentence of grounded prose, with what it rests on.
 *
 * Typed against the contract's own shared sentence — which the contract
 * spells `OrganizationBriefSentence` and uses for the org brief, the deal
 * status card, Person360 and the growth-fit panel alike. The name is the
 * contract's; the shape has never been a company's.
 */
export type BriefSentence = components["schemas"]["OrganizationBriefSentence"];

/** One record a sentence was written from. */
export type Cited = BriefSentence["evidence"][number];

/** Which kind of record a citation points at. */
export type CitedKind = Cited["entity_type"];

/** How prose says which writer produced it. */
export type WrittenByWriter = components["schemas"]["WrittenBy"];

export type CitationChip =
  | {
      openable: true;
      entityType: CitedKind;
      entityId: string;
      count: number;
      // The record's own name, when the citation carried one — a deal's
      // name, an activity's subject. Absent on a grouped chip: a count
      // already speaks for several records, and one of their names would
      // read as though it spoke for the rest.
      name?: string;
    }
  | {
      openable: false;
      entityType: CitedKind;
      count: number;
      // A chip a reader cannot open needs its name MORE than an openable one,
      // not less: there is no click to find out which record it means. An
      // activity citation without it renders the bare word "activity", which
      // is the run of identical labels the counted chip exists to avoid.
      // Absent once the chip counts several, for the same reason as above.
      name?: string;
    };

/**
 * citationChips turns a sentence's raw evidence into what a reader should see.
 *
 * Three reductions, all of which the raw list gets wrong on its own. The same
 * record cited twice is one source, not two. Several records of a kind the app
 * cannot open are one statement about that kind — rendered one by one they
 * became a run of identical unopenable labels ("activity activity activity"),
 * which says nothing the count does not say better. And several RECEIPT
 * citations of the same kind (`groupable`) are one counted chip too, opening
 * the first and stepping through the rest — rendered one per record they
 * became the same run under a different reason: a receipt has no name of its
 * own, so ten profile fields all read "profile field", ten times, with nothing
 * to tell them apart. `deal`/`person` stay one chip per record: each opens its
 * OWN screen rather than a shared stepper, so collapsing them would silently
 * drop every record after the first.
 *
 * Order is first-seen, so the chips follow the sentence's own reasoning.
 */
export function citationChips(
  evidence: readonly Cited[],
  openable: (entityType: CitedKind) => boolean,
  groupable: (entityType: CitedKind) => boolean = () => false,
): CitationChip[] {
  const chips: CitationChip[] = [];
  const seenAt = new Map<string, number>();
  const groupAt = new Map<CitedKind, number>();
  for (const cited of evidence) {
    const identity = `${cited.entity_type}:${cited.entity_id}`;
    const already = seenAt.get(identity);
    if (already !== undefined) {
      // The same record cited twice is one source, so the chip stays — but the
      // server names a record on the citation rather than once per record, and
      // nothing promises the FIRST mention is the one that carried the name.
      // Dropping the repeat outright therefore threw away the only name there
      // was, and the chip fell back to its bare kind. Still one record, so it
      // may still speak with its own name; `count === 1` is what says it has
      // not since become a group speaking for several.
      const held = chips[already];
      if (held.name === undefined && held.count === 1 && cited.name) {
        held.name = cited.name;
      }
      continue;
    }
    const isOpenable = openable(cited.entity_type);
    if (isOpenable && !groupable(cited.entity_type)) {
      seenAt.set(identity, chips.length);
      chips.push({
        openable: true,
        entityType: cited.entity_type,
        entityId: cited.entity_id,
        count: 1,
        name: cited.name,
      });
      continue;
    }
    const at = groupAt.get(cited.entity_type);
    if (at === undefined) {
      // The record's chip is the one just pushed. Recorded per BRANCH rather
      // than once above, because a record joining an existing group lands on
      // that group's index and not at the end.
      seenAt.set(identity, chips.length);
      groupAt.set(cited.entity_type, chips.length);
      chips.push(
        isOpenable
          ? {
              openable: true,
              entityType: cited.entity_type,
              entityId: cited.entity_id,
              count: 1,
            }
          : {
              openable: false,
              entityType: cited.entity_type,
              count: 1,
              name: cited.name,
            },
      );
      continue;
    }
    seenAt.set(identity, at);
    const grouped = chips[at];
    grouped.count += 1;
    // It now speaks for several records, so it may no longer carry one of
    // their names: a chip reading "Slots for the pilot review" over three
    // activities claims the other two are that thread as well.
    grouped.name = undefined;
  }
  return chips;
}

// The citation kinds that open a RECEIPT rather than a record page. Only these
// can be stepped through, because only these render in the drawer.
const RECEIPT_CITATIONS = new Set(["fact", "profile_field"]);

// The citation kinds a reader can open something for. `deal` and `person` route
// to their own screens; `fact` and `profile_field` open their receipt instead —
// where the value came from, when it was read, and what could not be recorded.
//
// An activity has no detail route of its own (it lives in a timeline) and no
// receipt either, and the organization citation is usually the page the reader
// is already on. Both stay flat: a clickable element that does nothing teaches
// the reader that citations do not work, which costs more than the click it
// saves.
const ROUTABLE_CITATIONS = new Set(["deal", "person", "fact", "profile_field"]);

/** One steppable citation, in the receipt's own shape. */
export type CitedSibling = {
  entityType: "fact" | "profile_field";
  entityId: string;
};

// The sentence's receipt-bearing citations, once each, in the order it cites
// them. Mapped here at the one place that knows both shapes: the wire is
// snake_case and the drawer's CitedRecord is not.
function dedupeCited(evidence: readonly Cited[]): CitedSibling[] {
  const seen = new Set<string>();
  const out: CitedSibling[] = [];
  for (const each of evidence) {
    const key = `${each.entity_type}:${each.entity_id}`;
    if (!RECEIPT_CITATIONS.has(each.entity_type) || seen.has(key)) {
      continue;
    }
    seen.add(key);
    out.push({
      entityType: each.entity_type as "fact" | "profile_field",
      entityId: each.entity_id,
    });
  }
  return out;
}

/**
 * What one chip says — the same answer whether or not it can be opened, which
 * is why it is decided once here.
 *
 * A chip that stands for ONE record names that record: "deal" told a reader
 * nothing they could not already see; the deal's own name tells them which one.
 * A chip a reader cannot open needs that most of all, because there is no click
 * to find out which activity it means — an emailed citation rendering the bare
 * word "activity" is the run of identical labels this design exists to avoid.
 *
 * A chip that stands for SEVERAL names the count and the kind instead. It
 * carries no name by then (citationChips drops it), for the reason it must: one
 * member's name would read as though it spoke for the rest. A grouped receipt
 * chip opens the FIRST and the drawer's stepper reaches the others, which is the
 * receipt kind's whole reason for having one.
 *
 * The kind label is also what a record with no name falls back to: the server
 * sends `name` when it has one, and nothing here invents one when it does not.
 */
function chipLabel(chip: CitationChip, t: Translator, locale: Locale): string {
  return (
    chip.name ??
    (chip.count === 1
      ? t(`co.brief.cite.${chip.entityType}`)
      : t(`co.brief.cite.${chip.entityType}.many`, {
          count: formatNumber(chip.count, locale),
        }))
  );
}

/**
 * Citations renders the chips for one sentence.
 *
 * A citation the app cannot open is rendered as a label, not as a button: a
 * clickable element that does nothing teaches the reader that citations do not
 * work, which costs more than the click it saves.
 */
export function Citations({
  evidence,
  onOpenRecord,
  nameOf,
}: Readonly<{
  evidence: readonly Cited[];
  // The record's own name, from a page that already holds it.
  //
  // The server names a citation when the writer had the name at hand and
  // leaves it out otherwise, and this file invents nothing — but a page
  // showing an account's own 360 is HOLDING the names of that account's people
  // and deals, and printing "contact" beside a reason while the roster three
  // sections down says "Frédéric de Gombert" is the page failing to read
  // itself. Answers undefined for a record it does not know, which falls back
  // to the kind exactly as before.
  nameOf?: (entityType: CitedKind, entityId: string) => string | undefined;
  onOpenRecord?: (
    entityType: string,
    entityId: string,
    siblings?: readonly CitedSibling[],
  ) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const chips = citationChips(
    evidence.map((cited) =>
      cited.name || !nameOf
        ? cited
        : { ...cited, name: nameOf(cited.entity_type, cited.entity_id) },
    ),
    (entityType) => Boolean(onOpenRecord) && ROUTABLE_CITATIONS.has(entityType),
    (entityType) => RECEIPT_CITATIONS.has(entityType),
  );
  // THIS sentence's citations, in the order it cites them, so the receipt's
  // prev/next walks the sentence the reader is actually looking at. The order
  // belongs to the sentence, which is why it is passed from here rather than
  // rebuilt in the drawer.
  // Deduplicated, because the stepper finds its position by id: a sentence
  // citing the same fact twice would leave `findIndex` returning the first
  // occurrence forever, and Next would never move past it.
  const siblings = dedupeCited(evidence);
  if (chips.length === 0) {
    return null;
  }
  return (
    <span className="co-brief-cites">
      {chips.map((chip) =>
        chip.openable ? (
          <button
            key={`${chip.entityType}:${chip.entityId}`}
            type="button"
            className="co-brief-cite"
            onClick={() =>
              onOpenRecord?.(chip.entityType, chip.entityId, siblings)
            }
          >
            {chipLabel(chip, t, locale)}
          </button>
        ) : (
          <span key={chip.entityType} className="co-brief-cite-flat">
            {chipLabel(chip, t, locale)}
          </span>
        ),
      )}
    </span>
  );
}

const NATURE_LABELS: Record<
  NonNullable<BriefSentence["nature"]>,
  MessageKey
> = {
  fact: "co.brief.nature.fact",
  assessment: "co.brief.nature.assessment",
  recommendation: "co.brief.nature.recommendation",
};

/**
 * SentenceList renders grounded prose — the standing brief, the deal's status
 * card and the answers to prepared questions read identically, because they
 * are the same thing written from the same records with the same citations.
 * One component, so a citation can never be clickable in one place and flat in
 * the other.
 */
export function SentenceList({
  sentences,
  onOpenRecord,
  nameOf,
  citations = "per-sentence",
  leadWithJudgement = false,
}: Readonly<{
  sentences: BriefSentence[];
  onOpenRecord?: (entityType: string, entityId: string) => void;
  // The record's own name for a citation the writer could not name — see
  // `Citations`. Passed through rather than resolved here: the names belong to
  // the page holding the record, not to the prose.
  nameOf?: (entityType: CitedKind, entityId: string) => string | undefined;
  // WHERE the receipts go, which is a reading decision rather than a styling
  // one.
  //
  // "per-sentence" is the brief's: each line is a separate claim a reader
  // checks on its own, so its chips belong beside it.
  //
  // "collected" is the dossier's: it is one continuous description of a
  // record, and a chip after every clause turned three sentences into a wall
  // of "fact fact fact". The sources are the same, gathered once underneath —
  // every claim stays checkable, and the prose stays readable.
  citations?: "per-sentence" | "collected";
  // Whether the block's own judgement is pulled out and set as its opening
  // claim. The facts under it are already on the cards above, so what the
  // block ADDS is what the agent makes of them — and four sentences at one
  // volume have no shape to scan.
  leadWithJudgement?: boolean;
}>) {
  const [lead, ...rest] = leadWithJudgement
    ? judgementFirst(sentences)
    : [undefined, ...sentences];
  return (
    <>
      {lead ? (
        <p className="co-brief-lead">
          <NatureBadge sentence={lead} />
          {lead.text}
          {citations === "per-sentence" && (
            <Citations
              evidence={lead.evidence}
              nameOf={nameOf}
              onOpenRecord={onOpenRecord}
            />
          )}
        </p>
      ) : null}
      <ul className="co-brief-lines">
        {(lead ? rest : sentences).map((sentence, index) => (
          // Indexed because two sentences may legitimately read the same;
          // keying on the text collapses them into one row.
          // biome-ignore lint/suspicious/noArrayIndexKey: the list is replaced wholesale on every read, never reordered in place
          <li key={index}>
            <NatureBadge sentence={sentence} />
            {sentence.text}
            {citations === "per-sentence" && (
              <Citations
                evidence={sentence.evidence}
                nameOf={nameOf}
                onOpenRecord={onOpenRecord}
              />
            )}
          </li>
        ))}
        {citations === "collected" && (
          <li className="co-brief-sources">
            <Citations
              evidence={sentences.flatMap((sentence) => sentence.evidence)}
              nameOf={nameOf}
              onOpenRecord={onOpenRecord}
            />
          </li>
        )}
      </ul>
    </>
  );
}

/**
 * What KIND of claim a sentence is, marked where it is made. A judgement that
 * looked like a stored fact would be the one thing a reader could not check —
 * and the prose is allowed to judge now.
 *
 * It rides the leading sentence too, so a promoted judgement keeps its mark
 * rather than losing it to the promotion. A fact leading a block carries no
 * badge and is meant to — that is a block with nothing to judge, and the
 * absence of the mark is what says so.
 */
function NatureBadge({ sentence }: Readonly<{ sentence: BriefSentence }>) {
  const t = useT();
  if (!sentence.nature || sentence.nature === "fact") {
    return null;
  }
  return (
    <>
      <Badge tone={sentence.nature === "recommendation" ? "accent" : undefined}>
        {t(NATURE_LABELS[sentence.nature])}
      </Badge>{" "}
    </>
  );
}

/**
 * The block reordered so its judgement opens it, and the rest follow in the
 * order they were written.
 *
 * Which sentence leads is not arbitrary. A brief that judges leads with the
 * judgement; a brief with none — the deterministic fallback writes no
 * assessments — leads with its first line, because it has nothing to judge and
 * an empty lead slot would read as a sentence that failed to load.
 */
function judgementFirst(
  sentences: BriefSentence[],
): [BriefSentence | undefined, ...BriefSentence[]] {
  if (sentences.length === 0) {
    return [undefined];
  }
  const at = sentences.findIndex(
    (sentence) => sentence.nature === "assessment",
  );
  const index = at === -1 ? 0 : at;
  return [sentences[index], ...sentences.filter((_, each) => each !== index)];
}

/**
 * WrittenBy names which writer produced a piece of prose. Always shown: a
 * reader weighing a sentence needs to know whether a model or the
 * deterministic fallback wrote it, and the two are not interchangeable.
 */
export function WrittenBy({ by }: Readonly<{ by: WrittenByWriter }>) {
  const t = useT();
  return (
    <Badge tone={by === "model" ? "ai" : undefined}>
      {t(`co.brief.by.${by}`)}
    </Badge>
  );
}
