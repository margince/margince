import type { ReactNode } from "react";
import { useId, useRef, useState } from "react";
import { Button } from "../../design-system/atoms";
import { formatNumber } from "../../format/format";
import { type Locale, useLocale, useT } from "../../i18n";
import type { CompanyFieldName } from "../onboarding";
import type { ReviewRow } from "./company-review-state";
import "./review-deck.css";

/**
 * The confirm stop as a deck: one thing at a time, and a count of what is left.
 *
 * IT REPLACED A WALL. The review used to put everything on one screen at once:
 * a section rail, every field of four groups, a hundred facts with their source
 * chips, and a bar at the bottom listing what still blocked. All of it true, and
 * all of it at once, which asked a reader to hold the whole profile in their
 * head to answer six questions. The read already knows which ones it cannot
 * settle by itself, so those are the ones it asks, and the rest is stated as a
 * number that went in on evidence.
 *
 * NOT `DecisionDeck`. That component has the same shape and the right chrome,
 * and its item type is `DecisionApproval` — the agent-approval domain, with a
 * verdict ladder, a bundle form and a staging tray this surface has no meaning
 * for. Fitting a company field into it would make one type answer two
 * questions, and the first change to either would break the other. What is
 * shared is the LOOK, and that is the design system's job, not the type's.
 *
 * EVERY CARD ENDS IN A WRITTEN STRING. A pick card shows what each answer would
 * put on the record before it is chosen; a supply card shows what the reader is
 * typing. Same claim in both: nothing lands on the profile that was not shown
 * as the exact words first.
 */

/** One thing the read could not settle, and how a reader settles it. */
export type DeckCard = Readonly<{
  field: CompanyFieldName;
  /** What is being asked, in the reader's words. */
  question: string;
  /** What the read saw, where it saw anything. */
  evidence?: string;
  /** The page it was read from. */
  source?: string;
  /** Whether the flow refuses to continue while this is unanswered. */
  required: boolean;
  /** Longer than a line, so the control is a box rather than a field. */
  multiline: boolean;
  /** What is in the draft for this field right now. */
  value: string;
}>;

/**
 * The work the read is handing back, in the order it should be met.
 *
 * REQUIRED FIRST, because those are the ones that refuse a confirm, and a
 * reader who answers four optional cards and is then told they cannot continue
 * has been walked the wrong way round. Derived from the same rows the confirm
 * gate reads, so the count on the tray and the button's own verdict cannot
 * disagree: two spellings of "what is outstanding" is how a screen ends up
 * saying "nothing left" beside a Continue that refuses.
 */
export function deckCards(
  blocking: readonly ReviewRow[],
  advisory: readonly ReviewRow[],
): DeckCard[] {
  return [...blocking, ...advisory].map((row) => ({
    field: row.field,
    question: row.label,
    evidence: row.evidence?.snippet,
    source: row.evidence?.source,
    required: row.state === "required",
    multiline: row.multiline,
    value: row.value,
  }));
}

export function ReviewDeck({
  cards,
  cardOf,
  settled,
  onField,
  onDone,
  onReadWhole,
  pending,
  disabled,
  digest,
}: Readonly<{
  cards: readonly DeckCard[];
  /**
   * One field's card as it stands RIGHT NOW, answered or not.
   *
   * `cards` carries only what is still outstanding, so a field leaves it on
   * its first keystroke. The card being typed into has to keep rendering from
   * somewhere, and this is that somewhere: the whole record, not the part of
   * it that still wants attention.
   */
  cardOf: (field: CompanyFieldName) => DeckCard | undefined;
  /** How many facts went onto the record without needing anybody. */
  settled: number;
  onField: (field: CompanyFieldName, value: string) => void;
  onDone: () => void;
  /** The way to the whole profile, for a reader who wants the wall after all. */
  onReadWhole: () => void;
  pending: boolean;
  disabled: boolean;
  /**
   * The record as it stands, shown beside the deck.
   *
   * The deck asks one thing at a time, which is the right way to ask and a bad
   * way to see: by the sixth card a reader has lost track of what the first
   * five did. The article is what they answered INTO, and it is handed in
   * rather than built here so the deck stays about asking.
   */
  digest: (active: CompanyFieldName | undefined) => ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [at, setAt] = useState(0);
  // The order the deck asks in, fixed the first time each field appears and
  // never shortened.
  //
  // `cards` is derived from what is still outstanding, so a field drops out of
  // it the moment its first character lands. A cursor that indexed THAT array
  // moved the deck on mid-word: the reader typed one letter, the row stopped
  // being outstanding, the array closed up, and the next question slid in
  // under the caret. The place in the queue is a field, and the field's
  // content is read live so what is typed still shows.
  const order = useRef<CompanyFieldName[]>([]);
  for (const entry of cards) {
    if (!order.current.includes(entry.field)) {
      order.current.push(entry.field);
    }
  }
  const asking = order.current[at];
  // Past the end is the honest state once every card is met, not a clamp back
  // onto the last one: the deck is finished and says so.
  const card = asking === undefined ? undefined : cardOf(asking);

  if (card === undefined) {
    return (
      <div className="rdeck">
        <div className="rdeck-split">
          <p className="rdeck-clear">
            {t("ob.deck.clear", { count: formatNumber(settled, locale) })}
          </p>
          {digest(undefined)}
        </div>
        <DeckFoot
          left={0}
          total={order.current.length}
          settled={settled}
          locale={locale}
          onDone={onDone}
          onReadWhole={onReadWhole}
          pending={pending}
          disabled={disabled}
        />
      </div>
    );
  }

  return (
    <div className="rdeck">
      <div className="rdeck-split">
        <div className="rdeck-stack">
          {/* The cards still to come, as edges behind the live one. They carry no
            content and are not read out: a reader is told how many are left in
            words by the tray, and the stack is what makes that number felt. */}
          {order.current.length - at > 1 && (
            <span className="rdeck-peek" aria-hidden="true" />
          )}
          {order.current.length - at > 2 && (
            <span className="rdeck-peek rdeck-peek-far" aria-hidden="true" />
          )}
          <DeckCardFace
            card={card}
            index={at}
            total={order.current.length}
            locale={locale}
            onField={onField}
            onNext={() => setAt((n) => n + 1)}
          />
        </div>
        {digest(card.field)}
      </div>
      <DeckFoot
        left={cards.length}
        total={cards.length}
        settled={settled}
        locale={locale}
        onDone={onDone}
        onReadWhole={onReadWhole}
        pending={pending}
        disabled={disabled}
      />
    </div>
  );
}

// One card's face: what is being asked, what the read saw, and the control that
// answers it. The answer is typed rather than picked because these are the
// fields the read came back WITHOUT — there is no candidate list to offer, and
// a picker with one option is a button pretending to be a choice.
function DeckCardFace({
  card,
  index,
  total,
  locale,
  onField,
  onNext,
}: Readonly<{
  card: DeckCard;
  index: number;
  total: number;
  locale: Locale;
  onField: (field: CompanyFieldName, value: string) => void;
  onNext: () => void;
}>) {
  const t = useT();
  const controlId = useId();
  return (
    <div className="rdeck-card" data-required={card.required}>
      <div className="rdeck-head">
        <span className="rdeck-tag t-eyebrow" data-required={card.required}>
          {t(card.required ? "ob.deck.needed" : "ob.deck.optional")}
        </span>
        <span className="rdeck-count">
          {t("ob.deck.counter", {
            n: formatNumber(index + 1, locale),
            m: formatNumber(total, locale),
          })}
        </span>
      </div>
      <label className="rdeck-question" htmlFor={controlId}>
        {card.question}
      </label>
      {card.evidence === undefined ? null : (
        <p className="rdeck-evidence">{card.evidence}</p>
      )}
      {card.multiline ? (
        <textarea
          id={controlId}
          value={card.value}
          onChange={(event) => onField(card.field, event.target.value)}
        />
      ) : (
        <input
          id={controlId}
          value={card.value}
          onChange={(event) => onField(card.field, event.target.value)}
        />
      )}
      {/* What this answer puts on the record, in the words it will be stored
          in. Blank while nothing is typed, because "nothing is written" is a
          statement about an empty field and this one may still be filled. */}
      {card.value.trim() === "" ? null : (
        <p className="rdeck-writes">
          <span className="rdeck-writes-lead">{t("ob.conv.scene.writes")}</span>
          <code>{card.value}</code>
        </p>
      )}
      {card.source === undefined || card.source === "" ? null : (
        <span className="rdeck-source">{card.source}</span>
      )}
      <div className="rdeck-acts">
        {/* Leaving something out is what an EMPTY optional card offers. Once
            an answer is typed the same control is simply the way onward, and
            a card that still said "leave it out" over a filled field would be
            offering to discard the sentence the reader just wrote. */}
        <Button variant="ghost" onClick={onNext}>
          {t(
            card.required || card.value.trim() !== ""
              ? "ob.deck.next"
              : "ob.deck.leaveOut",
          )}
        </Button>
      </div>
    </div>
  );
}

// The tray: what is left, what went in without anybody, and the way onward.
function DeckFoot({
  left,
  total,
  settled,
  locale,
  onDone,
  onReadWhole,
  pending,
  disabled,
}: Readonly<{
  left: number;
  total: number;
  settled: number;
  locale: Locale;
  onDone: () => void;
  onReadWhole: () => void;
  pending: boolean;
  disabled: boolean;
}>) {
  const t = useT();
  return (
    <div className="rdeck-tray">
      <p className="rdeck-left" role="status">
        {t("ob.deck.left", {
          n: formatNumber(left, locale),
          m: formatNumber(total, locale),
        })}
        <span className="rdeck-settled">
          {t("ob.deck.settled", { count: formatNumber(settled, locale) })}
        </span>
      </p>
      <button type="button" className="rdeck-whole" onClick={onReadWhole}>
        {t("ob.deck.readWhole")}
      </button>
      <Button variant="primary" onClick={onDone} disabled={disabled || pending}>
        {t(pending ? "ob.s1.saving" : "ob.deck.confirm")}
      </Button>
    </div>
  );
}
