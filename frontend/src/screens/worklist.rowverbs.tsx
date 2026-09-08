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
// THE ANSWER COMES FIRST. Every verb sits in one wrapping line with the lane's
// call to action at its head, and that ordering is the whole layout: the verbs
// used to divide, quiet ones on the leading edge and the answer held on the
// trailing one, which reads well on a row wide enough to hold both and breaks
// on every row that is not. Once the line wrapped, the trailing group dropped
// alone to a second line — so a queue of ten rows drew ten one-button lines,
// and the thing a reader came to press was the one control not in the row of
// controls. Leading the line with it cannot do that at any width: what wraps is
// the quiet tail, which is what a reader skips anyway.

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
 * Every verb a row carries, on one line, the lane's answer at its head.
 *
 * The order is the ranking: what this lane asks the reader to DO, then the ways
 * to reach the record, then the reader's own two marks — the pin and the
 * hand-off — then the lane's verbs of equal weight and the ways to put the row
 * down. It is one flow and not two groups — see the note at the top of this
 * file.
 *
 * NO GLYPH IS LAST, which is what puts the pin and the hand-off in the middle
 * of the ranking rather than at the end of it. The end is where the line wraps:
 * on a seven-verb waiting row the pin went over alone, and a lone 32px glyph on
 * a line of its own reads as a stray mark rather than as a verb. What wraps now
 * is a button with a word on it, which reads as the continuation it is.
 */
export function RowActs({
  item,
  href,
  owner,
  primary,
  equals,
  onReview,
}: Readonly<{
  item: WorklistItem;
  href: string | undefined;
  /** Whose queue this row is on — `ReassignControl` resolves an empty one. */
  owner: string;
  /** The lane's one call to action, drawn first. */
  primary?: ReactNode;
  /**
   * The lane's verbs of EQUAL weight, drawn among the quiet ones. None of them
   * is the row's answer, so none of them leads the line: promoting one would
   * tell a reader that "Held" is the expected outcome of a meeting that may
   * equally have been cancelled.
   */
  equals?: ReactNode;
  /** Where a grouped row is reviewed, on the surface that has a filter. */
  onReview?: () => void;
}>) {
  return (
    <div className="worklist-row-acts">
      {primary}
      {item.batch && onReview ? (
        <BatchVerb onReview={onReview} />
      ) : (
        <RowVerbs item={item} href={href} move={moveHref(item)} />
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
          above: what wraps off the end of this line must be a word. */}
      {item.source === "task" && !item.batch && (
        <ReassignControl item={item} owner={owner} />
      )}
      {equals}
      {/* The ways this row can be PUT DOWN, as the server declares them. Drawn
          from `dispositions` rather than inferred from `source`: which rows a
          rep may judge is a server rule, and a client keeping its own copy
          draws a verb that 404s or hides one the rep is entitled to. Last, so
          the line's tail is a labelled button. */}
      <DispositionVerbs item={item} />
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
  move,
}: Readonly<{
  item: WorklistItem;
  href: string | undefined;
  move: string | undefined;
}>) {
  const t = useT();
  const drawn = new Set<string>();
  type Verb = {
    action: WorklistItem["actions"][number];
    destination: string;
  };
  const verbs = item.actions.flatMap<Verb>((action) => {
    if (action === "decide") {
      const to = decideDestination(item, href);
      return to ? [{ action, destination: to }] : [];
    }
    const route = VERB_DESTINATION[action];
    if (!route) {
      // A verb this build cannot route draws nothing. A control that looks
      // pressable and goes nowhere is worse than no control.
      return [];
    }
    const destination = route(href);
    if (!destination) {
      return [];
    }
    // One control per DESTINATION. `complete` and `snooze` both open the
    // record this row is about, and two identical "Open" links side by side
    // ask the reader to choose between the same thing twice.
    const key = `${VERB_LABEL[action](t)}|${destination}`;
    if (drawn.has(key)) {
      return [];
    }
    drawn.add(key);
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
