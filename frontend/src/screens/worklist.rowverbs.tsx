// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The verbs one row of the day offers, and the ONE line they stand on.
//
// Split from the row for the reason the row was split from the screen: they
// answer different questions. The row decides how a piece of work READS — its
// rank, its kind, its name, the facts under it. This decides what can be DONE
// about it and in what order, which is the half a reader of the other question
// does not need.
//
// THE ANSWER COMES LAST, and the line stands on the row's trailing edge. That
// ordering is the whole layout: a reader runs a row left to right — the rank,
// the kind, the work — and the verb the lane is asking for is the end of that
// sentence, where the eye and the thumb finish. Right-aligned, it also sits at
// one x down the whole queue however many quiet verbs the row ahead of it
// carries, so a rep answering row after row presses in the same place.
//
// It is ONE flow rather than two groups, at both of the widths the row is drawn
// at. The verbs do not DIVIDE here — every one of them is about this row — and
// two groups held apart say which is which only while both edges are on screen.

import { Pin, PinOff } from "lucide-react";
import type { ReactNode } from "react";
import { Button } from "../design-system/atoms";
import { IconAction } from "../design-system/iconaction";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { moveHref, moveLabel } from "./worklist.copy";
import { DispositionVerbs } from "./worklist.dispositions";
import { ReassignControl } from "./worklist.manager";
import { usePinRow, type WorklistItem } from "./worklist.queries";

/**
 * Every verb a row carries, on one right-aligned line, the lane's answer LAST.
 *
 * The order is the ranking read backwards from the answer: the ways to reach
 * the record, then the reader's own two marks — the pin and the hand-off — then
 * the lane's verbs of equal weight, the ways to put the row down, and finally
 * what the lane is asking for. It is one flow and not two groups — see the note
 * at the top of this file.
 *
 * THE TWO GLYPHS STAND AMONG THE WORDS, which is what keeps them out of the
 * middle of a wrap: a lone 32px glyph opening or closing a line of its own
 * reads as a stray mark rather than as a verb, and the pin and the hand-off are
 * the only controls here with no word on them. What falls to a second line is a
 * labelled button, and the tail of the line is the answer itself.
 */
export function RowActs({
  item,
  href,
  density,
  owner,
  primary,
  equals,
  onReview,
}: Readonly<{
  item: WorklistItem;
  href: string | undefined;
  /**
   * `compact` withholds the verb that only REACHES the record, because at that
   * density the row's title carries the link itself — two controls on one line
   * opening the same page ask the reader to choose between the same thing
   * twice. Everything that ACTS is drawn at both densities.
   */
  density?: "compact";
  /** Whose queue this row is on — `ReassignControl` resolves an empty one. */
  owner: string;
  /** The lane's one call to action, drawn last and nearest the reader's thumb. */
  primary?: ReactNode;
  /**
   * The lane's verbs of EQUAL weight, drawn among the quiet ones. None of them
   * is the row's answer, so none of them takes the end of the line: promoting
   * one would tell a reader that "Held" is the expected outcome of a meeting
   * that may equally have been cancelled.
   */
  equals?: ReactNode;
  /** Where a grouped row is reviewed, on the surface that has a filter. */
  onReview?: () => void;
}>) {
  return (
    <div className="worklist-row-acts">
      {item.batch && onReview ? (
        <BatchVerb onReview={onReview} />
      ) : (
        <RowVerbs
          item={item}
          href={href}
          density={density}
          move={moveHref(item)}
        />
      )}
      {/* The reader's own override, on every row that can carry one. It is not
          a disposition — those put a row DOWN and this lifts one up — so it
          stands before them rather than among them. */}
      <PinVerb item={item} />
      {/* Only a task carries an assignee, so only a task can be handed on. A
          group row stands for a pile and names no single activity to move.

          Offered on the reader's OWN queue too: handing work on is not a
          manager's verb, and gating it on a selected rep left somebody looking
          at their own list with no way to pass a task along. Who is excluded
          from the destinations follows the queue rather than this condition —
          ReassignControl falls back to the reader when no rep is selected, so
          the current holder is never offered as the new one.

          Beside the pin, because it is the row's other GLYPH — see the doc
          above: the two stand together among the labelled verbs. */}
      {item.source === "task" && !item.batch && (
        <ReassignControl item={item} owner={owner} />
      )}
      {equals}
      {/* The ways this row can be PUT DOWN, as the server declares them. Drawn
          from `dispositions` rather than inferred from `source`: which rows a
          rep may judge is a server rule, and a client keeping its own copy
          draws a verb that 404s or hides one the rep is entitled to. */}
      <DispositionVerbs item={item} />
      {/* WHAT THE LANE IS ASKING FOR, at the end of the line and on the row's
          trailing edge. A reader who has read the work looks here for the move,
          and finds it at the same x on every row of the queue. */}
      {primary}
    </div>
  );
}

// The way into a group.
//
// It narrows the queue to decisions rather than opening a screen of its own:
// that screen is its own piece of work, and a row whose only verb led nowhere
// would be worse than the pile it replaced.
//
// A button, not a link. The dials live in this screen's state today, so an
// address carrying `?filter=decisions` would be read by nobody and the control
// would do nothing — which is the defect it exists to avoid. Moving them into
// the URL is the right shape and is its own change.
function BatchVerb({ onReview }: Readonly<{ onReview: () => void }>) {
  const t = useT();
  return (
    <Button small onClick={onReview}>
      {t("worklist.verb.review_batch")}
    </Button>
  );
}

// What this row offers, as the item itself declares it.
//
// Every verb is a LINK to the surface that owns it rather than a mutation from
// here: this queue adds no authority of its own, so deciding an approval goes
// to the decision surface and merging a pair to the dedupe queue, exactly as
// they do from any other door. Rendering a button that acted here would be a
// second place for those rules to live.
//
// A verb whose destination this page cannot name draws nothing. A control that
// looks pressable and goes nowhere is worse than no control.
function RowVerbs({
  item,
  href,
  density,
  move,
}: Readonly<{
  item: WorklistItem;
  href: string | undefined;
  density?: "compact";
  move: string | undefined;
}>) {
  const t = useT();
  const drawn = new Set<string>();
  type Verb = {
    action: WorklistItem["actions"][number];
    destination: string;
  };
  const verbs = item.actions.flatMap<Verb>((action) => {
    const destination = verbDestination(item, action, href);
    if (!destination) {
      // A verb this build cannot route draws nothing. A control that looks
      // pressable and goes nowhere is worse than no control.
      return [];
    }
    // ONE CONTROL PER DESTINATION, and the ADDRESS is the whole key. `open`,
    // `complete` and `snooze` all reach the record this row is about, and so
    // does an introduction ask's `decide` — two links to one page ask the reader
    // to choose between the same thing twice.
    //
    // The WORD is no part of that key, because one address arrives under
    // different words: an ask sends `decide` and `open`, which is "Decide" and
    // "Open" over one route. Which word survives is the first the SERVER sent,
    // in the order it ranked them.
    if (drawn.has(destination)) {
      return [];
    }
    // THE TITLE IS THE LINK at list density, so the verb that merely opens the
    // record is the same press twice on one line. Keyed on the DESTINATION and
    // not on the word: the dedupe above keeps whichever verb the server ranked
    // first, so this row's way to its own record arrives as "Open" on one lane
    // and as "Complete" or "Snooze" on another — a check against the word
    // would withhold one and leave the others.
    //
    // `move` is untouched: it opens the composer, which is a different
    // destination and the most-pressed control on a waiting row.
    if (density === "compact" && destination === href) {
      return [];
    }
    drawn.add(destination);
    return [{ action, destination }];
  });
  if (verbs.length === 0 && !move) {
    return null;
  }
  return (
    <>
      {/* The step the product already worked out, offered where the reader is
          standing rather than on a screen they have to go and find. */}
      {move && (
        <a className={NAVIGATING_VERB} href={move}>
          {/* THE LABEL MOVES WITH THE ROUTE AND WITH THE VERB. Where the
              address opens the composer the label is the act; where it only
              reaches the record it says so. And it names the verb the SERVER
              chose, so an opening outreach is not offered as a reply to a
              conversation nobody has had. */}
          {moveLabel(item, t)}
        </a>
      )}
      {verbs.map(({ action, destination }) => (
        <a key={action} className={NAVIGATING_VERB} href={destination}>
          {VERB_LABEL[action](t)}
        </a>
      ))}
    </>
  );
}

// Where one verb goes, or nowhere.
//
// ONE entry point, so every verb passes the dedupe above and `decide` is not an
// exception to it: that verb's route depends on the SOURCE rather than only on
// the verb, and a route answered beside the dedupe rather than through it puts a
// second link on an address the row already offers.
function verbDestination(
  item: WorklistItem,
  action: WorklistItem["actions"][number],
  href: string | undefined,
): string | undefined {
  return action === "decide"
    ? decideDestination(item, href)
    : VERB_DESTINATION[action]?.(href);
}

// A verb that NAVIGATES, wearing the same face as the verbs that act.
//
// It stays an anchor, because that is what it is: middle-click, copy-link and
// the browser's own status bar are the whole difference between a link and a
// button, and a reader who wants the record in a second tab is a reader this
// row is for. What changes is the chrome. Drawn as link text among small
// buttons, "Draft the reply" — the most-pressed control on a waiting row —
// read as a caption beside the verbs, and the row had two visual grammars for
// one question.
//
// The atom's own classes rather than a face of this screen's: `Button` renders
// a `<button>` and takes no `href`, so there is no component to reach for, and
// the alternative is a second spelling of the small ghost button in
// worklist.css. `screens/client.tsx` reaches the same conclusion the same way.
const NAVIGATING_VERB = "btn btn-ghost btn-sm";

// Where each verb lives. A total map over the ones this page can route, so a
// verb the contract adds either gets a destination here or is not drawn —
// never a button that does nothing.
const VERB_DESTINATION: Partial<
  Record<
    WorklistItem["actions"][number],
    (href: string | undefined) => string | undefined
  >
> = {
  // `decide` and `merge` are deliberately absent: the surface that answers
  // them IS this page, so a link would send the reader where they already are.
  // They come back when the decision card is drawn inline, which is its own
  // piece of work.
  //
  // `acknowledge` is absent too — see NoticeAcknowledge, which draws it
  // inline instead of through this table.
  //
  // Everything routable is the record the row is about.
  open: (href) => href,
  complete: (href) => href,
  snooze: (href) => href,
};

// The one verb whose routing depends on the SOURCE rather than only the verb.
//
// `decide` is answered inline for an approval — the card is right there, so a
// link would send the reader where they already are. An introduction ask has no
// inline card: its four answers are the colleague's own, given on the contact's
// Network tab. Without this the ask row names somebody waiting and offers
// nothing at all, which is the worst of both.
function decideDestination(
  item: WorklistItem,
  href: string | undefined,
): string | undefined {
  return item.source === "introduction_request" ? href : undefined;
}

// What each routable verb is called. Spelled as a map of functions rather than
// a composed key, so a verb the contract adds without copy here does not
// compile — which is the only way this cannot reach a reader as a raw word.
const VERB_LABEL: Record<
  WorklistItem["actions"][number],
  (t: ReturnType<typeof useT>) => string
> = {
  decide: (t) => t("worklist.verb.decide"),
  merge: (t) => t("worklist.verb.merge"),
  open: (t) => t("worklist.verb.open"),
  complete: (t) => t("worklist.verb.complete"),
  snooze: (t) => t("worklist.verb.snooze"),
  acknowledge: (t) => t("worklist.verb.acknowledge"),
  // The briefing queue's three verbs. Named here because the map is total over
  // the contract's actions — they route nowhere from this page yet, so
  // VERB_DESTINATION does not carry them and no control is drawn.
  act: (t) => t("worklist.verb.open"),
  dismiss: (t) => t("worklist.verb.open"),
  set_aside: (t) => t("worklist.verb.open"),
  // Named for the same reason: the map is total. `retry` is drawn by
  // AutomationRetry, which acts in place, so VERB_DESTINATION routes it
  // nowhere and this label is never the one a reader sees.
  retry: (t) => t("worklist.verb.retry"),
  // The composer's own word, not a second one: ChannelReplyAction draws the
  // button this labels, and two spellings of one act would read as two acts.
  reply: (t) => t("compose.reply"),
  // The record history's own word, for the same reason. `undo` is drawn by
  // ReceiptUndo on the handled panel, which acts in place, so VERB_DESTINATION
  // routes it nowhere and this label is never the one a reader sees — it is
  // here because the map is total over the contract's actions.
  undo: (t) => t("history.undo.action"),
};

// The reader's own override: this row leads their day, whatever the ranking
// chose.
//
// The ranking has carried a pin level since it was written and, until the store
// shipped, nothing could set it. Every other control on this page changes what
// the SERVER thinks — a disposition, a filter, a scope. This is the only one
// that says "I know, and I want this first anyway", which is the difference
// between a queue a rep works and a queue a rep argues with.
//
// WHAT IT READS to know which way to toggle: the row's own `pinned` reason. The
// server states it on a pinned row, so the client asks the response rather than
// keeping a second record of what it pressed — a local flag would disagree with
// the page the moment the reader pinned from another tab, and the button would
// offer to pin a row that already leads their day.
//
// A BATCH row is skipped. Its id is synthetic and minted by the fold, so a pin
// on one names a group that will not exist under that key on the next read.
function PinVerb({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const toast = useToast();
  const pin = usePinRow();
  if (item.batch) {
    return null;
  }
  const pinned = (item.because ?? []).some((why) => why.kind === "pinned");
  return (
    <IconAction
      small
      // A GLYPH, because the pin already IS the verb: it is the one control on
      // the row that a reader recognises without reading, and a row that has
      // grown a move, an Open, three judgements, a hand-off and an answer can
      // no longer spend a word on it. The name is not lost — `IconAction`
      // speaks it and shows it on hover from the one `label`.
      icon={pinned ? <PinOff aria-hidden="true" /> : <Pin aria-hidden="true" />}
      label={t(pinned ? "worklist.verb.unpin" : "worklist.verb.pin")}
      // WHAT THE GLYPH CANNOT SAY. "Pin" answers what the control IS and
      // nothing about what it does: a reader could not tell whether it marks
      // the row urgent, whose order it changes, or how long it lasts.
      //
      // Per STATE, like the label beside it. One sentence for both states had
      // the Unpin button describing itself as keeping the row on top, which is
      // the control contradicting what pressing it now does.
      //
      // A description rather than part of the name: a control list repeating a
      // sentence once per row would be worse than the bare word.
      hint={t(pinned ? "worklist.verb.unpinHint" : "worklist.verb.pinHint")}
      // It SETS rather than does, and the two states of one switch look
      // identical without it: a glyph has no label on screen to carry the
      // difference, so the pressed state is what tells a reader this row is
      // already the one they put first.
      pressed={pinned}
      pending={pin.isPending}
      onClick={() =>
        pin.mutate(
          { source: item.source, rowId: item.id, pinned },
          {
            // A refused write otherwise leaves the button exactly as an
            // unpressed one looks, and the row keeps the place it had — so
            // the reader is told nothing and sees nothing change.
            onError: () =>
              toast.show(
                t(
                  pinned
                    ? "worklist.verb.unpinFailed"
                    : "worklist.verb.pinFailed",
                ),
                { mark: false },
              ),
          },
        )
      }
    />
  );
}
