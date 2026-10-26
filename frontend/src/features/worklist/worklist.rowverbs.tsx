// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The verbs one row of the day offers, on one line: Done where the row offers
// it, then the lane's answer, then the row's menu. So the answer and the menu
// stand at one x down the queue; in a card the answer leads.
// The menu holds the rest, most used first: the snooze, a way to the record,
// the pin, the hand-off, then the hand-backs.

import { Sparkles } from "lucide-react";
import { type ReactNode, useState } from "react";
import { Button, OverflowMenu } from "../../design-system/atoms";
import { useToast } from "../../design-system/toast";
import { useTooltip } from "../../design-system/tooltip";
import { useT } from "../../i18n";
import { useFoldedViewport } from "../../lib/viewport";
import {
  moveHref,
  moveLabel,
  moveOpensComposer,
  subjectHref,
} from "./worklist.copy";
import {
  DoneVerb,
  HandBackLines,
  PutDownLines,
  useRowPutDown,
} from "./worklist.dispositions";
import { ReassignForm } from "./worklist.manager";
import { usePinRow, type WorklistItem } from "./worklist.queries";
import { replyIsTheMove } from "./worklist.reply";

/** Every verb a row carries: Done, the lane's answer, then the row's menu. */
export function RowActs({
  item,
  title,
  href,
  owner,
  allowPin = true,
  primary,
  equals,
  onReview,
  onOpenEmail,
  context,
  framed,
  titled,
}: Readonly<{
  item: WorklistItem;
  /** The row's name, which names its menu among every other row's. */
  title: string;
  href: string | undefined;
  /** Whose queue this row is on, which the hand-off moves work away from. */
  owner: string;
  allowPin?: boolean;
  /** The lane's one call to action, drawn last before the menu. */
  primary?: ReactNode;
  /**
   * The lane's verbs of equal weight, drawn before Done. None of them is the
   * row's answer. Promoting one would tell a reader that "Held" is the expected
   * outcome of a meeting that may have been cancelled.
   */
  equals?: ReactNode;
  /** Where a grouped row is reviewed, on the surface that has a filter. */
  onReview?: () => void;
  onOpenEmail?: (id: string) => void;
  /** The way into what the row is about, where the surface has one; first. */
  context?: ReactNode;
  /** A card around this row already names and links its record. */
  framed?: boolean;
  /** The row's title already links the record the row is about. */
  titled?: boolean;
}>) {
  const t = useT();
  const folded = useFoldedViewport();
  const [handingOff, setHandingOff] = useState(false);
  const reviews = item.batch !== undefined && onReview !== undefined;
  const routed = reviews ? [] : routedVerbs(item, href);
  // A row with no answer of its own is answered on the page a verb reaches, so
  // that verb stands on the line. An Open only reaches the record.
  const lead =
    primary === undefined && routed[0]?.action !== "open"
      ? routed[0]
      : undefined;
  // The other ways to the record are withheld wherever the row already links
  // it: two controls opening one page ask for the same choice twice.
  const linked = framed || titled;
  const ways = routed.filter(
    (way) => way !== lead && !(linked && way.destination === href),
  );
  const pins = allowPin && !item.batch;
  // Only a task carries an assignee, and a group row names no single activity.
  const handsOff = item.source === "task" && !item.batch;
  // One write for Done and the menu's lines, so the two cannot write twice.
  const write = useRowPutDown(item);
  const putsDown = write.offered.some(
    (disposition) => folded || disposition !== "handled",
  );
  const answer = (
    <RowAnswer
      item={item}
      lead={lead}
      primary={primary}
      onOpenEmail={onOpenEmail}
    />
  );
  const others = (
    <>
      {context}
      {reviews && onReview && <BatchVerb onReview={onReview} />}
      {equals}
      <DoneVerb write={write} />
    </>
  );
  // A card is read top to bottom and finished on its answer, so the answer
  // leads there. Down the queue it stands last, at one x on every row.
  return (
    <div className="worklist-row-acts">
      {framed ? (
        <>
          {answer}
          {others}
        </>
      ) : (
        <>
          {others}
          {answer}
        </>
      )}
      {(ways.length > 0 || pins || handsOff || putsDown) && (
        <OverflowMenu label={t("worklist.rowMenu", { title })}>
          <PutDownLines write={write} />
          {ways.map(({ action, destination }) => (
            <a key={action} className={NAVIGATING_VERB} href={destination}>
              {VERB_LABEL[action](t)}
            </a>
          ))}
          {pins && <PinLine item={item} />}
          {handsOff && (
            <Button variant="ghost" onClick={() => setHandingOff(true)}>
              {t("worklist.manager.reassign")}
            </Button>
          )}
          <HandBackLines write={write} />
        </OverflowMenu>
      )}
      {handingOff && (
        <ReassignForm
          item={item}
          owner={owner}
          onClose={() => setHandingOff(false)}
        />
      )}
    </div>
  );
}

/** The row's answer: the prepared move, the verb a page reaches, the lane's. */
function RowAnswer({
  item,
  lead,
  primary,
  onOpenEmail,
}: Readonly<{
  item: WorklistItem;
  lead: RoutedVerb | undefined;
  primary?: ReactNode;
  onOpenEmail?: (id: string) => void;
}>) {
  const t = useT();
  return (
    <>
      {!item.batch && <PreparedMove item={item} onOpenEmail={onOpenEmail} />}
      {lead && (
        <a className={NAVIGATING_VERB} href={lead.destination}>
          {VERB_LABEL[lead.action](t)}
        </a>
      )}
      {item.source === "brief_item" &&
        !primary &&
        !moveHref(item) &&
        subjectHref(item) && (
          <a className={NAVIGATING_VERB} href={subjectHref(item)}>
            {t("deal.openDeal")}
          </a>
        )}
      {primary}
    </>
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
  return <Button onClick={onReview}>{t("worklist.verb.review_batch")}</Button>;
}

type RoutedVerb = {
  action: WorklistItem["actions"][number];
  destination: string;
};

// What this row offers to reach, as the item itself declares it. Each verb is a
// link to the surface that owns it, since this queue adds no authority of its
// own. A verb whose destination this page cannot name draws nothing.
//
// One control per destination, keyed on the address: `open`, `complete` and
// `snooze` all reach the row's record, and the first the server ranked stays.
function routedVerbs(
  item: WorklistItem,
  href: string | undefined,
): RoutedVerb[] {
  const drawn = new Set<string>();
  return item.actions.flatMap<RoutedVerb>((action) => {
    const destination = verbDestination(item, action, href);
    if (!destination || drawn.has(destination)) {
      return [];
    }
    drawn.add(destination);
    return [{ action, destination }];
  });
}

// The step the product already worked out, offered where the reader stands.
//
// Indigo only where the press makes the model write. That is the composer,
// which drafts on arrival, and the meeting brief, assembled on open. Where the
// row's answer is this reply, that control already carries the move.
function PreparedMove({
  item,
  onOpenEmail,
}: Readonly<{ item: WorklistItem; onOpenEmail?: (id: string) => void }>) {
  const t = useT();
  if (replyIsTheMove(item)) {
    return null;
  }
  const replyActivity =
    item.move?.action === "draft_reply" ? item.move.activity_id : undefined;
  if (replyActivity && onOpenEmail && item.subject?.type !== "contact") {
    return (
      <Button variant="ghost" onClick={() => onOpenEmail(replyActivity)}>
        {t("worklist.verb.draft_reply")}
      </Button>
    );
  }
  const move = moveHref(item);
  if (!move) {
    return null;
  }
  const modelWrites =
    moveOpensComposer(item) || item.move?.action === "open_meeting_brief";
  return (
    <a className={modelWrites ? PREPARED_VERB : NAVIGATING_VERB} href={move}>
      {modelWrites && <Sparkles aria-hidden="true" />}
      {/* The label follows the route and the verb. Where the address opens
          the composer it names the act; where it only reaches the record it
          says so. It names the verb the server chose. */}
      {moveLabel(item, t)}
    </a>
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
// row is for. What changes is the chrome. Drawn as link text among the verbs,
// "Draft the reply" — the most-pressed control on a waiting row — read as a
// caption beside them, and the row had two visual grammars for one question.
//
// The atom's own classes rather than a face of this screen's. `Button` renders
// a `<button>` and takes no `href`, so there is no component to reach for.
// The alternative is a second spelling of the ghost button in worklist.css.
// `features/settings/client.tsx` reaches the same conclusion the same way.
const NAVIGATING_VERB = "btn btn-ghost";
// The move the product worked out, as an anchor in the agent's own chrome:
// the same `ai` face `Button` draws, for the reason given where it is drawn.
const PREPARED_VERB = "btn btn-ai";

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
  // Domain review owns the inline controls; these labels keep the contract map total.
  reopen: (t) => t("worklist.domainReview.retry"),
  keep: (t) => t("worklist.verb.keep"),
  keep_as_vendor: (t) => t("worklist.verb.keepAsVendor"),
  discard: (t) => t("worklist.verb.discard"),
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
// chose. It reads the row's own `pinned` reason to know which way to go. A
// local flag would disagree with the page once the reader pinned elsewhere.
// RowActs offers no pin on a batch row. Its id is minted by the fold and will
// not exist under that key on the next read.
function PinLine({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const toast = useToast();
  const pin = usePinRow();
  const pinned = (item.because ?? []).some((why) => why.kind === "pinned");
  // What the word cannot say: whose order it changes and for how long. Per
  // state, so Unpin never describes itself as keeping the row on top.
  const { ref, trigger, tip } = useTooltip<HTMLButtonElement>(
    t(pinned ? "worklist.verb.unpinHint" : "worklist.verb.pinHint"),
  );
  return (
    // A verb rather than a switch, so the menu closes on it. The row says
    // "pinned by you" in its own captions.
    <Button
      variant="ghost"
      ref={ref}
      {...trigger}
      pending={pin.isPending}
      onClick={() =>
        pin.mutate(
          { source: item.source, rowId: item.id, pinned },
          {
            // A refused write otherwise leaves the row where it was with
            // nothing said, so the reader is told nothing and sees nothing.
            onError: () =>
              toast.show(
                t(
                  pinned
                    ? "worklist.verb.unpinFailed"
                    : "worklist.verb.pinFailed",
                ),
                { tone: "danger" },
              ),
          },
        )
      }
    >
      {t(pinned ? "worklist.verb.unpin" : "worklist.verb.pin")}
      {tip}
    </Button>
  );
}
