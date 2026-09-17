// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId } from "react";
import { useT } from "../i18n";
import { Popover } from "./popover";
import "./atoms.css";
import "./evidencemark.css";

/**
 * StatCard is one reading at the top of a record: a label, the reading itself,
 * and one line of detail saying what it is drawn from.
 *
 * Every reading has a label and a value, and the value is never EMPTY: an
 * empty reading spells its emptiness — "0", "None yet" — because a blank where
 * a figure belongs reads as a page that failed. So there is no fallback here.
 *
 * The detail line is not decoration. A reading with no basis stated is a number
 * a reader has to trust, and this surface exists because a number nobody could
 * scale — "Relationship 2/100" — was doing exactly that.
 *
 * `tone` colours the value, never the whole tile: a strip of coloured boxes
 * reads as a dashboard, and the reader is meant to see three facts, not a
 * traffic light.
 *
 * `alert` is the one exception, for the one slot whose reading is bad news
 * simply by being present (an overdue balance, a lapsed renewal) rather than by
 * its number — those tint the whole tile, because there is no value to colour
 * that says the same thing on its own. Wire it only there; a reading that could
 * be read either way stays plain and lets its own words carry the judgement.
 *
 * `basis` is the reading's receipt: the rows it was computed from, folded away
 * until a reader asks for them. It is a `Popover` that opens to a settled
 * pointer AND to a click or Enter — a row of readings is compared by moving
 * across it, and asking for a click at every stop makes the comparison cost
 * five presses, while a receipt only a pointer could open would be a reading
 * half the readers do not have. It sits OVER the card rather than expanding
 * it, so opening one does not move the readings beside it.
 *
 * The receipt is marked on the reading's NAME line rather than in the card's
 * foot. What it says is "this figure has a source", which qualifies the
 * reading the way its label does; in the foot it sat beside the door to the
 * tab, and two controls a card's width apart both reading as quiet grey text
 * are two things a reader has to tell apart before either is useful.
 *
 * Its word is the component's, not the caller's. Every one of the six callers
 * passed the same string through a `basisLabel` prop, which made a shared
 * spelling look like a per-reading decision and gave a seventh caller a place
 * to invent a different word for the same control.
 *
 * `detail` still carries the one-line basis either way: a reader who never
 * opens this must not meet a number with nothing behind it.
 */

// The bar under a reading: segments a reader would count, or one track they
// would not.
//
// Six is the line, and it is about counting rather than width: "one of three
// signals" is a set a reader checks off, and "two of ten contacts" is a share
// they read as a length. Drawn the other way round, three segments of a hundred
// are invisible and a tenth of one track says nothing about which signal is out.
const COUNTABLE = 6;

// The segments' own names. A position in a bar has no identity of its own —
// nothing distinguishes the second segment from the third except where it
// sits — so they are named here rather than keyed on the loop index, which is
// the same claim in the spelling a linter cannot tell apart from a keyed list
// of records.
const SEGMENTS = ["first", "second", "third", "fourth", "fifth", "sixth"];

function Meter({ filled, total }: Readonly<{ filled: number; total: number }>) {
  const held = Math.min(Math.max(filled, 0), total);
  if (total <= COUNTABLE) {
    return (
      <span
        className="stat-card-meter stat-card-meter-segments"
        aria-hidden="true"
      >
        {SEGMENTS.slice(0, total).map((segment, index) => (
          <span
            key={segment}
            className={
              index < held
                ? "stat-card-meter-fill"
                : "stat-card-meter-fill stat-card-meter-empty"
            }
          />
        ))}
      </span>
    );
  }
  return (
    <span className="stat-card-meter" aria-hidden="true">
      <span
        className="stat-card-meter-fill"
        style={{ inlineSize: `${Math.round((held / total) * 100)}%` }}
      />
    </span>
  );
}

export function StatCard({
  label,
  value,
  detail,
  basis,
  tone,
  source,
  alert,
  onOpen,
  meter,
  narrow,
}: Readonly<{
  label: string;
  value: string;
  // The line under the figure: what it rests on, in the reader's words. A node
  // rather than a string, because a reading whose detail is two facts — how much
  // is failing, and why — says them on two lines, not in one parsed sentence.
  detail?: ReactNode;
  // The way OUT of the reading: the tab that holds what it was read from. The
  // whole CARD is this button's target (atoms.css stretches it over the tile).
  // ONE control and not two — the basis trigger layers above it and keeps its
  // own press, so asking what a figure rests on never also leaves the page.
  onOpen?: () => void;
  // How far along this reading is, as the two numbers it is made of. Drawn as
  // separate segments when there are few enough to count (a verdict made of
  // three signals) and as one filled track when there are not (two of ten
  // contacts replying) — the difference is whether a reader would count them.
  //
  // Only for a reading that HAS a denominator. A figure with nothing to be out
  // of gets no bar rather than a bar with an invented one.
  meter?: { filled: number; total: number };
  // What the reading rests on, and the words that name it. The copy belongs to
  // the caller, because no copy lives in a primitive.
  basis?: ReactNode;
  // The reading's verdict, in the product's one state vocabulary. `success` is
  // not "no tone": a slot whose reading is a VERDICT says so in both
  // directions, and a verdict that is fine reads as fine rather than as one
  // nobody has judged yet. `info` is the reading a job has not finished
  // writing, which is a different thing from one nobody has judged.
  tone?: "info" | "success" | "warning" | "danger";
  // Where the figure came from, named on the card that shows it. A money
  // reading a reader cannot trace is one they must verify elsewhere.
  source?: ReactNode;
  // Tints the whole tile. See the docblock above — this is not `tone` at
  // stronger volume, it is a different judgement (the slot itself is bad
  // news, not just its figure).
  alert?: boolean;
  // What the reading becomes where its strip can no longer hold two tiles
  // abreast: `row` folds it into one full-width line, label and basis leading,
  // figure trailing (statstrip.css). Nothing about the tile's air or type at
  // any wider width — this reading is the reading beside it on a record page,
  // and a tighter tile here once made the same figure read as a different
  // card. A closed word, not a boolean: `narrow={true}` is a place for a
  // second SIZE to arrive unnoticed, and a size here is the removed `hero`.
  narrow?: "row";
}>) {
  const t = useT();
  const labelId = useId();
  // No `t-h3`: the card owns the figure's face and size (atoms.css), because
  // a reading is compared across a row and the row is the thing that has to
  // agree. Sharing the page's heading class made the figure change size with a
  // scale that answers a different question.
  const valueClass = tone
    ? `stat-card-value stat-card-${tone}`
    : "stat-card-value";
  const cardClass = [
    "stat-card",
    alert && "stat-card-alert",
    narrow && `stat-card-narrow-${narrow}`,
  ]
    .filter(Boolean)
    .join(" ");
  return (
    <section className={cardClass}>
      <span className="stat-card-label">
        {/* Its own box, so a clamp on the name spares the badge and trigger. */}
        <span className="stat-card-label-text" id={labelId}>
          {label}
        </span>
        {source && <span className="stat-card-source">{source}</span>}
        {basis && (
          // The panel is READ, never operated: it holds the working and
          // nothing to press. That is what lets it open on hover — a panel
          // carrying a control could not, because `onHover` closes it as the
          // pointer leaves and keyboard focus never enters it, which is how
          // the door repeated inside here became a control no keyboard reader
          // could reach. The door lives once, in the card's foot, where a
          // reader who has just read the working finds it directly below.
          <Popover
            className="stat-card-basis evmark-trigger"
            onHover
            label={t("stat.evidence")}
          >
            {/* The receipt names the QUESTION it answers, not itself: the
                trigger is gone from under the reader's eye the moment the
                panel is over it, and an unnamed list of facts beside a figure
                is a reader guessing what they are looking at. */}
            <span className="stat-card-basis-head t-eyebrow">
              {t("stat.evidence.rests")}
            </span>
            {basis}
          </Popover>
        )}
      </span>
      <span className={valueClass}>{value}</span>
      {detail && <span className="stat-card-detail t-caption">{detail}</span>}
      {/* The proportion under the words that state it. A bar rather than a
          second figure: the reader has the number above it, and what a bar
          adds is the SHARE at a glance, which is what a row of readings is
          compared on. Hidden from a screen reader — the figure and its detail
          line already say it in words, and a bar announced as well is the
          same fact twice. */}
      {meter && meter.total > 0 && <Meter {...meter} />}
      {/* THE CARD'S FOOT: the way out, at the end of the card a reader
          finishes on rather than up beside the reading's name, where it
          competed with the label for the first glance. */}
      {onOpen && (
        <span className="stat-card-foot">
          {/* THE DOOR SAYS "Open", ALWAYS. One word for one control: a caller
              naming its own destination gave the product several spellings of
              the same door, and the reading a door belongs to is already said
              beside it. Which reading it opens reaches a screen reader as this
              button's DESCRIPTION — folded into the NAME it read "Open Open
              pipeline", so it is `aria-describedby` and never the word. */}
          <button
            type="button"
            className="stat-card-open"
            onClick={onOpen}
            aria-describedby={labelId}
          >
            {t("stat.open")}
            <span className="stat-card-arrow" aria-hidden="true">
              {"\u2192"}
            </span>
          </button>
        </span>
      )}
    </section>
  );
}
