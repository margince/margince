// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The head of a 360 page: the call, and the signals it rests on.
//
// A record page is read in two ways and only one of them is reading. A rep
// working one deal reads the prose; a rep working thirty before a forecast
// call scans for the one that needs them. The prose serves the first and
// defeats the second — five headed paragraphs have no shape to scan, so
// finding the deal in trouble means reading all thirty.
//
// So the call goes on top, alone, in the one loud element on the card, and the
// signals under it each state the NUMBER that tripped them. "Going cold" is a
// label a reader has to trust; "no contact in 84 days" is the finding itself,
// and a reader can disagree with it. A chip that cannot say its number says
// its rule's own words instead — never nothing, which would read as a signal
// somebody cleared.
//
// Entity-agnostic on purpose. Company360 and Person360 answer the same
// question about a different record, and the day they grow a verdict they take
// this one rather than a second shaped like it.

import { ChevronDown, ChevronRight } from "lucide-react";
import { type ReactNode, useId, useState } from "react";
import { Badge } from "../../design-system/atoms";
import { PanelBody } from "../../design-system/panel";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import "./record360.css";

/**
 * How loud a standing is. The four words are the deal card's today; a record
 * kind with its own vocabulary passes its own map.
 *
 * `calm` is the healthy state and is deliberately quiet: a card where every
 * state is coloured has no colour left for the one that needs it.
 *
 * `unknown` is NOT calm, and keeping them apart is the point. A newer server
 * can send a standing this build has never heard of, and folding that into the
 * healthy tone painted the loudest element on the card in the all-clear colour
 * for a word nobody could interpret. It renders neutral instead — the card
 * shows the call it was given and does not pretend to grade it.
 */
export type StandingTone = "danger" | "warn" | "accent" | "calm" | "unknown";

/**
 * One reading the call was made from: what it said, and which reading said it.
 *
 * A quote with no source is a claim, and a source with no quote is a citation
 * nobody can check. The pair is the unit, which is why this is one type rather
 * than two parallel lists.
 */
export type Grounding = {
  // Stable within the list: two readings can legitimately reach the same
  // words about an account.
  key: string;
  // The reading in its own words, as the rule that produced it phrased it.
  quote: string;
  // Which reading it came from — the dimension, the document, the thread.
  from: string;
};

/**
 * VerdictHead is the call, the one line under it, and what it rests on.
 *
 * `standing` is an open wire string, like every other enum this product reads
 * back: a word this build does not know renders its own text rather than
 * vanishing. A missing verdict renders nothing at all — a head that said
 * "unknown" would be the card inventing a call it was not given.
 *
 * The grounding is SHUT by default and its count sits on the trigger. A reader
 * scanning thirty records wants the call, not the working; a reader who
 * doubts one wants the working immediately. A closed block with no count would
 * be neither — it would be a box nobody can judge whether to open.
 */
export function VerdictHead({
  label,
  tone,
  because,
  restsOn,
}: Readonly<{
  label: string;
  tone: StandingTone;
  // One line saying what the call rests on. The prose underneath says the
  // rest; this is the half a scanner reads.
  because?: ReactNode;
  // The readings behind the call. Absent when the call was not read from
  // anything a reader could be shown — which is a real state, and different
  // from a call resting on nothing.
  restsOn?: readonly Grounding[];
}>) {
  return (
    <PanelBody>
      <div className="r360-verdict">
        <span className={`r360-standing r360-standing-${tone}`}>{label}</span>
        {/* The sentence and its working are ONE column beside the word, not two
            more items in a row with it. Laid out as three siblings, the working
            was pushed to the far end of the head and the sentence wrapped
            underneath the word rather than beside it — the call read as a
            headline with a caption below instead of a word a reader takes the
            record in by and the line that says why. */}
        <div className="r360-verdict-body">
          {because ? <span className="r360-because">{because}</span> : null}
          {restsOn && restsOn.length > 0 ? <RestsOn items={restsOn} /> : null}
        </div>
      </div>
    </PanelBody>
  );
}

/**
 * The grounding disclosure: shut, counted, and opened by a click.
 *
 * Opened on click rather than on hover on purpose. A pointer crossing the head
 * on its way to the tab strip is not a request to read the working, and a
 * block that unfolds under a passing cursor pushes the card's own content
 * down while the reader is looking at it.
 */
function RestsOn({ items }: Readonly<{ items: readonly Grounding[] }>) {
  const t = useT();
  return <Proof label={t("record.restsOn")} items={items} count />;
}

/**
 * The working behind one claim: what was read, and where each piece came from.
 *
 * The same disclosure wherever a claim is made — the call at the head of the
 * card, the move the agent found, the finding it volunteered, the task it put
 * on somebody's list. One component because they are one promise: nothing this
 * product asserts about a record is unreachable from the assertion itself.
 *
 * Shut by default. A reader scanning thirty records wants the claim; a reader
 * who doubts one wants the working immediately, and the disclosure is how both
 * get what they came for out of the same card.
 */
export function Proof({
  label,
  items,
  count = false,
}: Readonly<{
  // What the working IS, in the claim's own terms — "what this rests on",
  // "why Margince put this here". Named by the caller, because the honest
  // label differs with the claim and a shared one would flatten them.
  label: string;
  items: readonly Grounding[];
  // Whether the trigger states how many readings are behind it. Worth saying
  // where a reader is judging whether to open at all, noise on a row whose
  // proof is one quote.
  count?: boolean;
}>) {
  const plural = usePlural();
  const { locale } = useLocale();
  const [open, setOpen] = useState(false);
  const panelId = useId();
  const Caret = open ? ChevronDown : ChevronRight;
  if (items.length === 0) {
    return null;
  }
  return (
    <>
      <button
        type="button"
        className="r360-rests-toggle"
        onClick={() => setOpen((shown) => !shown)}
        aria-expanded={open}
        aria-controls={panelId}
      >
        <Caret aria-hidden="true" />
        {label}
        {count ? (
          <span className="r360-rests-count">
            <span className="t-mono">{formatNumber(items.length, locale)}</span>{" "}
            {/* The unit as a word, not a bare figure. "What this rests on 2"
                asks the reader to guess what was counted; the count is only
                worth putting on a shut block if it says what it counts. */}
            {plural("record.restsOn.source", items.length)}
          </span>
        ) : null}
      </button>
      {open ? (
        <div className="r360-rests" id={panelId}>
          {items.map((item) => (
            <div className="r360-rests-item" key={item.key}>
              <p className="r360-rests-quote">{item.quote}</p>
              <p className="r360-rests-from">{item.from}</p>
            </div>
          ))}
        </div>
      ) : null}
    </>
  );
}

/**
 * How loud one SIGNAL chip is. Badge's own vocabulary, deliberately not
 * StandingTone: a chip is a badge among badges, while a standing is the one
 * loud element on the card and answers a different question — including
 * "I do not recognise this word", which a badge never has to say.
 */
export type SignalTone = "danger" | "warn" | "accent" | undefined;

/** One scannable finding, stating the number that tripped it. */
export type Signal = {
  // Stable across renders and unique within the strip: the chips are a set of
  // findings, and two rules can legitimately produce the same words.
  key: string;
  // What tripped, in the rule's own words.
  label: string;
  // The number behind it — "84 days", "1 of 4". Absent when the rule has no
  // figure, which is a real state and not a missing one.
  figure?: string;
  tone: SignalTone;
};

/**
 * SignalStrip renders the findings a reader scans before deciding to read.
 *
 * Renders nothing when there are none. An empty strip would be a row of
 * whitespace where findings go, which reads as a card still loading — and the
 * page has a real "nothing is wrong" sentence for that, which says it.
 */
export function SignalStrip({ signals }: Readonly<{ signals: Signal[] }>) {
  if (signals.length === 0) {
    return null;
  }
  return (
    <PanelBody>
      <ul className="r360-signals">
        {signals.map((signal) => (
          <li key={signal.key}>
            <Badge tone={signal.tone}>{signal.label}</Badge>
            {signal.figure ? (
              <span className="t-mono r360-figure">{signal.figure}</span>
            ) : null}
          </li>
        ))}
      </ul>
    </PanelBody>
  );
}
