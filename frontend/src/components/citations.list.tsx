import { Badge, Button } from "../design-system/atoms";
import { EmailReference } from "../design-system/emailreference";
import { Popover } from "../design-system/popover";
import { formatActivityWhen } from "../format/activitywhen";
import { formatDate } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useRecordZone } from "../lib/recordzone";
import type {
  BriefSentence,
  Cited,
  CitedKind,
  WrittenByWriter,
} from "./citations";
import {
  type CitationChip,
  chipLabel,
  citationChips,
  dedupeCited,
  dedupeMessages,
  doorOf,
  emailOf,
  type OpenReceipt,
  RECEIPT_CITATIONS,
} from "./citations.chips";

/**
 * Citations renders the chips for one sentence.
 *
 * A citation the app cannot open is rendered as a label, not as a button. A
 * clickable element that does nothing teaches the reader that citations do not
 * work, which costs more than the click it saves.
 */
export function Citations({
  evidence,
  onOpenRecord,
  onOpenReceipt,
  onOpenEmail,
  nameOf,
}: Readonly<{
  evidence: readonly Cited[];
  // The record's own name, from a page that already holds it.
  //
  // The server names a citation when the writer had the name at hand, and
  // this file invents nothing. A page showing an account's own 360 holds the
  // names of that account's contacts and deals. Printing "contact" while the
  // roster below says "Frédéric de Gombert" is the page failing to read
  // itself. Answers undefined for a record it does not know, which falls back
  // to the kind.
  nameOf?: (entityType: CitedKind, entityId: string) => string | undefined;
  onOpenRecord?: (entityType: string, entityId: string) => void;
  // The receipt and email drawers are the host's: one that mounts neither
  // passes nothing, and those chips render as prose rather than dead controls.
  onOpenReceipt?: OpenReceipt;
  onOpenEmail?: (activityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const named = evidence.map((cited) =>
    cited.name || !nameOf
      ? cited
      : { ...cited, name: nameOf(cited.entity_type, cited.entity_id) },
  );
  // A cited message is drawn as a message, never folded into the chip run.
  // The chips speak in record kinds ("deal", "3 activities"). A message has a
  // subject and a date the reader checks the claim against.
  // Deduplicated like the chips: the same record cited twice is one source.
  // Otherwise four sentences on one thread draw the message four times, under
  // one React key.
  const messages = dedupeMessages(named);
  // This sentence's receipts, once each, in its order: the drawer's prev/next
  // walks the sentence the reader is looking at, and finds its place by id.
  const siblings = dedupeCited(evidence);
  const doors = { onOpenRecord, onOpenReceipt, siblings };
  const chips = citationChips(
    named.filter((cited) => !emailOf(cited)),
    (cited) => Boolean(doorOf(cited.entity_type, cited.entity_id, doors)),
    (entityType) => RECEIPT_CITATIONS.has(entityType),
  );
  if (chips.length === 0 && messages.length === 0) {
    return null;
  }
  return (
    <span className="co-brief-cites">
      {messages.map((cited) => {
        const summary = emailOf(cited);
        if (!summary) {
          return null;
        }
        // EmailReference drops the opener from a withheld message itself, and a
        // second copy of that privacy rule here would be free to disagree.
        const withheld = summary.display_status === "withheld";
        return (
          <EmailReference
            key={`activity:${cited.entity_id}`}
            subject={summary.subject}
            occurredAt={formatActivityWhen(
              summary.occurred_at,
              summary.occurred_precision,
              locale,
              recordZone,
            )}
            withheld={withheld}
            onOpen={
              onOpenEmail ? () => onOpenEmail(summary.activity_id) : undefined
            }
          />
        );
      })}
      {chips.map((chip) => {
        const open = chip.openable
          ? doorOf(chip.entityType, chip.entityId, doors)
          : undefined;
        if (chip.quote || chip.at || chip.origin) {
          return (
            <CitationWithReceipt
              key={chipKey(chip)}
              chip={chip}
              label={chipLabel(chip, t, locale)}
              onOpen={open}
            />
          );
        }
        return chip.openable ? (
          <button
            key={chipKey(chip)}
            type="button"
            className="co-brief-cite"
            onClick={open}
          >
            {chipLabel(chip, t, locale)}
          </button>
        ) : (
          <span key={chipKey(chip)} className="co-brief-cite-flat">
            {chipLabel(chip, t, locale)}
          </span>
        );
      })}
    </span>
  );
}

// Keyed by the record where the chip stands for one, and by the kind where it
// counts several. Two receipted activities on one sentence are two chips, and
// keying both on their kind would draw one.
function chipKey(chip: CitationChip): string {
  return chip.count === 1 && chip.entityId
    ? `${chip.entityType}:${chip.entityId}`
    : chip.entityType;
}

/**
 * A chip with the evidence behind it. Resting on it opens the record's own
 * words in the agent's rule, and under them where and when they were said.
 * A reader checks the claim there. The record or its receipt is one more
 * step, where the host has a door for its kind. It is never the chip's own
 * click, since one that navigated away would be a receipt the reader cannot
 * rest on.
 */
function CitationWithReceipt({
  chip,
  label,
  onOpen,
}: Readonly<{
  chip: CitationChip;
  label: string;
  onOpen?: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const dated = chip.at ? formatDate(chip.at, locale, recordZone) : undefined;
  const provenance = [chip.origin, dated].filter(Boolean).join(" · ");
  return (
    <Popover onHover className="co-brief-cite co-cite-receipted" label={label}>
      {chip.quote && (
        <blockquote className="co-cite-quote">{chip.quote}</blockquote>
      )}
      {provenance && <p className="co-cite-origin t-caption">{provenance}</p>}
      {onOpen && (
        <p className="co-cite-open">
          <Button variant="ghost" onClick={onOpen}>
            {t("co.cite.open")}
          </Button>
        </p>
      )}
    </Popover>
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
 * SentenceList renders grounded prose. The standing brief, the deal's status
 * card and the answers to prepared questions read identically. They are the
 * same thing, written from the same records with the same citations.
 * One component, so a chip opens wherever its host mounts the door for its
 * kind and is prose elsewhere, never a control that does nothing.
 */
export function SentenceList({
  sentences,
  onOpenRecord,
  onOpenReceipt,
  onOpenEmail,
  nameOf,
  citations = "per-sentence",
  leadWithJudgement = false,
}: Readonly<{
  sentences: BriefSentence[];
  onOpenRecord?: (entityType: string, entityId: string) => void;
  onOpenReceipt?: OpenReceipt;
  /** Opens one cited message; see `Citations`. */
  onOpenEmail?: (activityId: string) => void;
  // The record's own name for a citation the writer could not name (see
  // `Citations`). Passed through rather than resolved here: the names belong to
  // the page holding the record, not to the prose.
  nameOf?: (entityType: CitedKind, entityId: string) => string | undefined;
  // Where the receipts go, which is a reading decision rather than a styling
  // one.
  //
  // "per-sentence" is the brief's: each line is a separate claim a reader
  // checks on its own, so its chips belong beside it.
  //
  // "collected" is the dossier's: one continuous description of a record,
  // where a chip after every clause makes a wall of "fact fact fact". The
  // sources are gathered once underneath, so every claim stays checkable.
  //
  // "none" is for a caller that draws the sources itself, under the prose.
  // The contact's brief names the transport each cited conversation was
  // carried on, which a citation chip cannot know. That caller owes the
  // chips; this mode keeps the list from drawing a second, blinder set.
  citations?: "per-sentence" | "collected" | "none";
  // Whether the block's own judgement is pulled out and set as its opening
  // claim. The facts under it are already on the cards above, so what the
  // block adds is what the agent makes of them. Four sentences at one volume
  // have no shape to scan.
  leadWithJudgement?: boolean;
}>) {
  const [lead, ...rest] = leadWithJudgement
    ? judgementFirst(sentences)
    : [undefined, ...sentences];
  const citationsFor = (evidence: readonly Cited[]) => (
    <Citations
      evidence={evidence}
      nameOf={nameOf}
      onOpenRecord={onOpenRecord}
      onOpenReceipt={onOpenReceipt}
      onOpenEmail={onOpenEmail}
    />
  );
  const list = (
    <ul className="co-brief-lines">
      {(lead ? rest : sentences).map((sentence, index) => (
        // Indexed: two sentences may read the same, and a text key merges them.
        // biome-ignore lint/suspicious/noArrayIndexKey: the list is replaced wholesale on every read, never reordered in place
        <li key={index}>
          <NatureBadge sentence={sentence} />
          {sentence.text}
          {citations === "per-sentence" && citationsFor(sentence.evidence)}
        </li>
      ))}
      {citations === "collected" && (
        <li className="co-brief-sources">
          {citationsFor(sentences.flatMap((sentence) => sentence.evidence))}
        </li>
      )}
    </ul>
  );
  if (!lead) return list;
  return (
    <div className="co-brief-read">
      <p className="co-brief-lead">
        {/* No nature word: being set apart is the lead's mark; the lines
            under it need theirs to tell a suggestion from a fact. */}
        {lead.text}
        {citations === "per-sentence" && citationsFor(lead.evidence)}
      </p>
      {list}
    </div>
  );
}

/**
 * What kind of claim a sentence is, marked where it is made. The prose may
 * judge, and a judgement that looked like a stored fact is the one thing a
 * reader could not check.
 *
 * The leading sentence carries none: being set apart at the top is its mark.
 */
function NatureBadge({ sentence }: Readonly<{ sentence: BriefSentence }>) {
  const t = useT();
  if (!sentence.nature || sentence.nature === "fact") {
    return null;
  }
  // A word in the sentence's own line, not a pill in front of it. "Suggested:
  // use the call to…" reads as prose with its kind named. A pill per sentence
  // makes a four-line brief carry four boxes.
  const cls =
    sentence.nature === "recommendation"
      ? "co-brief-nature co-brief-nature-suggested"
      : "co-brief-nature";
  return <span className={cls}>{t(NATURE_LABELS[sentence.nature])}: </span>;
}

/**
 * The block reordered so its judgement opens it, and the rest follow in the
 * order they were written.
 *
 * Which sentence leads is not arbitrary. A brief that judges leads with the
 * judgement. A brief with none (the deterministic fallback writes no
 * assessments) leads with its first line. An empty lead slot would read as a
 * sentence that failed to load.
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
