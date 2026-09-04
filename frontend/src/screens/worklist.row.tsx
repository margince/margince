// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One row of the day, and the verbs it offers.
//
// Split from the screen because they answer different questions. The screen
// decides WHAT the page shows — whose day, which cut, which headings. A row
// decides how one piece of work reads and where each of its verbs goes, and
// that is the half a reader of either question does not need the other for.

import { Badge, Button } from "../design-system/atoms";
import { PanelRow } from "../design-system/panel";
import { useToast } from "../design-system/toast";
import { formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { ApprovalRow } from "./approvalrow";
import { useNoticeRead, useTaskUpdate } from "./taskactions";
import {
  comparisonText,
  consequenceText,
  dealFactsText,
  isUnprepared,
  itemTitle,
  moveHref,
  moveLabel,
  phrasedReasons,
  reasonText,
  rowHref,
  whenText,
} from "./worklist.copy";
import { DispositionVerbs } from "./worklist.dispositions";
import { WaitingEmailLine } from "./worklist.emailtitle";
import { ReassignControl } from "./worklist.manager";
import { PairDecision } from "./worklist.pair";
import {
  useApproval,
  type WorklistItem,
  worklistKey,
} from "./worklist.queries";
import { syncHealthDetail } from "./worklist.synchealth";

/**
 * A grouped row's named members, each ONCE.
 *
 * The contract asks for "a few members, named, so the group can be checked
 * before it is answered", and a group of eight failures of one automation sends
 * that automation's name eight times: the Worklist's top row read
 * "Post-meeting recap draft · Post-meeting recap draft · Post-meeting recap
 * draft", which tells a reader nothing about the group except that the list
 * repeats.
 *
 * Order is kept — first appearance wins — because the server sends them in the
 * order it thinks matters.
 */
function namedMembers(item: WorklistItem): string[] {
  return [...new Set(item.batch?.sample ?? [])];
}

export function WorklistRow({
  item,
  position,
  owner,
  selected,
  onSelect,
  onReview,
  onOpenEmail,
}: Readonly<{
  item: WorklistItem;
  position: number;
  // Whose queue this row is on, empty for the reader's own. A row can only be
  // handed to somebody else from a page that is already about somebody else.
  owner: string;
  // Whether the pane beside the queue is about this row.
  //
  // BOTH CALLBACKS ARE OPTIONAL, because one surface has no pane. The Brief
  // shows the head of this same queue on the page a rep opens first, and it has
  // no second column to open a row INTO — so passing a no-op would draw a rank
  // button that answers nothing, which is exactly the dead control the comment
  // on that button warns about. Absent means the affordance is not drawn and
  // the rank reads as the number it is.
  selected?: boolean;
  onSelect?: () => void;
  // Where a grouped row is reviewed. Also a filter change the Brief cannot
  // make: it shows a fixed cut and has no filter to move.
  onReview?: () => void;
  // Opens a waiting email. Absent on a surface with no drawer — the Brief
  // draws the message and does not offer to open it, rather than drawing a
  // control that answers nothing.
  onOpenEmail?: (activityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const href = rowHref(item);
  const title = itemTitle(item, t, locale);
  const facts = dealFactsText(item, t, locale, zone);
  const sample = namedMembers(item);
  // The clock this row is racing. A meeting said "starting shortly" whether it
  // began in four minutes or in fifty, and a task said "Overdue" without saying
  // by how long — on the two rows whose whole claim is a moment.
  const when = whenText(item, t, locale, zone, new Date());
  // The supporting line. Every source but one sends a sentence already;
  // sync_health sends its condition's facts in its own vocabulary, so its line
  // is written from `kind` and `detail` together.
  const detail =
    item.source === "sync_health"
      ? syncHealthDetail(item.kind, item.detail, t)
      : item.detail;
  // The badged reasons are drawn as badges above and left out here, so one
  // meeting does not report the same finding twice in two registers.
  const because = phrasedReasons(item)
    .map((reason) => reasonText(reason, t, locale, zone))
    .filter((phrase): phrase is string => phrase !== null)
    .join(" · ");
  const above = comparisonText(item.above_next, t, locale, zone);
  const consequence = consequenceText(item, t);
  const emailRow = item.email_summary != null;
  return (
    <PanelRow
      className={
        selected ? "worklist-row worklist-row-selected" : "worklist-row"
      }
    >
      <Rank
        position={position}
        title={title}
        selected={selected}
        onSelect={onSelect}
      />
      <div className="worklist-row-text">
        {/* A waiting EMAIL names itself with the canonical row — the same one
            the timeline draws — so the queue shows the message rather than a
            sentence about it. Everything else keeps the title line it had, and
            the badges below stay on both: they say where the row sits in the
            day, which the email row does not answer. */}
        <WaitingEmailLine item={item} onOpen={onOpenEmail} />
        <p className="t-body worklist-row-title">
          {emailRow ? null : href ? (
            <a className="entity-link" href={href}>
              {title}
            </a>
          ) : (
            title
          )}
          <Badge>{t(`worklist.category.${item.category}` as const)}</Badge>
          {item.overdue && <Badge tone="danger">{t("worklist.overdue")}</Badge>}
          {/* A state of the meeting, not a reason among reasons: a rep
              scanning for the one to open before it starts has to see it
              without reading the line under the title. Warn rather than
              danger — an unprepared meeting is work to do, not a deadline
              already missed. */}
          {isUnprepared(item) && (
            <Badge tone="warn">{t("worklist.needsPrep")}</Badge>
          )}
        </p>
        {/* The supporting line, from every source that sends PROSE.

            It was drawn for `notice` alone, because three sources used this
            field as a typed channel — two wrote a bare day count, one wrote the
            marker words the queue groups by — and drawing it would have printed
            "90" under one title and "machine_sender" under another. Those three
            send their values typed now, so the twelve sources that were already
            writing sentences get to say them: which mailbox stopped, why a
            message bounced, why a send was held, what an AI task was about,
            which rule failed and how. That is the decisive line on most of these
            rows, and a reader was reading around it.

            `sync_health` sends its facts in the producer's own vocabulary —
            `shed`, `rate_limited`, `deals, contacts` — so its line is WRITTEN
            from that pair rather than drawn, by worklist.synchealth.ts. A value
            that build does not recognise draws nothing, which is what this row
            did for every sync value before. */}
        {detail && <p className="t-caption worklist-row-detail">{detail}</p>}
        {sample.length > 0 && (
          // A group nobody can see into is a group nobody trusts, and an
          // untrusted group is worse than the pile it replaced.
          <p className="t-caption worklist-row-sample">{sample.join(" · ")}</p>
        )}
        <RowCaptions
          when={when}
          facts={facts}
          because={because}
          consequence={consequence}
          above={above}
        />
      </div>
      {item.batch && onReview ? (
        <BatchVerb onReview={onReview} />
      ) : (
        <RowVerbs item={item} href={href} move={moveHref(item)} />
      )}
      {/* The ways this row can be PUT DOWN, as the server declares them. Drawn
          from `dispositions` rather than inferred from `source`: which rows a
          rep may judge is a server rule, and a client keeping its own copy
          draws a verb that 404s or hides one the rep is entitled to. */}
      <DispositionVerbs item={item} />
      {/* Only a task carries an assignee, so only a task can be handed on. A
          group row stands for a pile and names no single activity to move. */}
      {owner !== "" && item.source === "task" && !item.batch && (
        <ReassignControl item={item} owner={owner} />
      )}
      <RowAnswer item={item} />
    </PanelRow>
  );
}

/**
 * The answer a row can carry INSIDE it.
 *
 * Three kinds, and what they share is the reason they are here rather than
 * behind a link: the server already sent everything the decision needs, so
 * routing the reader to another screen to make it would be the hand-off this
 * queue exists to remove. An approval carries its staged payload, a duplicate
 * carries both records, a notice needs only to be seen.
 *
 * Together rather than inline in the row, because the row's own job — rank,
 * title, reasons, verbs — is already at the complexity the linter allows, and
 * a fourth kind of answer should extend this list rather than that function.
 */
function RowAnswer({ item }: Readonly<{ item: WorklistItem }>) {
  if (decidable(item)) {
    return <RowDecision item={item} />;
  }
  if (item.source === "dedupe_candidate" && item.pair) {
    return <PairDecision item={item} />;
  }
  if (item.source === "notice" && item.actions.includes("acknowledge")) {
    return <NoticeAcknowledge id={item.id} />;
  }
  // A task the server says can be finished, finished HERE. Not a batch: a group
  // row stands for a pile and names no single activity to complete.
  if (
    item.source === "task" &&
    !item.batch &&
    item.actions.includes("complete")
  ) {
    return <TaskComplete id={item.id} />;
  }
  return null;
}

/**
 * Everything the row says about itself under the title, in the order a reader
 * needs it.
 *
 * When it happens, what it is worth, why it is ranked where it is, what doing
 * nothing costs, and why it beat the row below. Each is absent when the server
 * sent nothing for it — a caption drawn empty is a line of furniture the reader
 * has to look past on every row.
 *
 * Together in one component because they are one idea — the row's own account
 * of itself — and because the row's function had reached the complexity the
 * linter allows, which is a fair reading of how much a person can hold at once.
 */
function RowCaptions({
  when,
  facts,
  because,
  consequence,
  above,
}: Readonly<{
  when: string | null;
  facts: string | null;
  because: string;
  consequence: string | null;
  above: string | null;
}>) {
  return (
    <>
      {/* When it starts, or when it is due. Above the reasons because it is the
          fact those reasons are ABOUT: "starting shortly" explains a rank, and
          this says what time. */}
      {when && <p className="t-caption worklist-row-when">{when}</p>}
      {facts && <p className="t-caption worklist-row-facts">{facts}</p>}
      {because && <p className="t-caption worklist-row-because">{because}</p>}
      {/* What it costs to do nothing. The question a queue exists to answer,
          and the one the lane feed had no field for. */}
      {consequence && (
        <p className="t-caption worklist-row-consequence">{consequence}</p>
      )}
      {/* Why this row beat the one below it. Absent on the last row, which has
          nothing below it to beat. */}
      {above && <p className="t-caption worklist-row-above">{above}</p>}
    </>
  );
}

/**
 * The rank, and the way INTO the pane where there is one.
 *
 * A button on the rank rather than the whole row being pressable: the row
 * already holds links and verbs, and a control wrapping controls is a press
 * whose target the reader has to guess.
 *
 * Without `onSelect` it is a plain number. The Brief draws these rows on a page
 * with no second column, so a button there would open nothing — and a control
 * that answers nothing is worse than no control, because a reader presses it
 * once and learns the page lies about what is pressable.
 */
function Rank({
  position,
  title,
  selected,
  onSelect,
}: Readonly<{
  position: number;
  title: string;
  selected?: boolean;
  onSelect?: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  // The rank is the page's central claim, so it is readable rather than
  // decorative: the list element carries the order for a screen reader and the
  // number states it for everybody else.
  const digit = (
    <span className="t-caption worklist-rank">
      {formatNumber(position, locale)}
    </span>
  );
  if (!onSelect) {
    return digit;
  }
  return (
    <button
      type="button"
      className="worklist-rank-select"
      aria-pressed={selected ?? false}
      // Names the ROW, not the verb. Every rank button carrying the same label
      // made them indistinguishable to anybody navigating by name, and overrode
      // the visible digit — which is the one thing that told them apart on
      // screen.
      aria-label={t("worklist.pane.openRow", {
        position: formatNumber(position, locale),
        title,
      })}
      onClick={onSelect}
    >
      {digit}
    </button>
  );
}

// Whether this row is a decision a person answers HERE.
//
// The queue holds no authority of its own — the card below is the same one the
// record page draws, posting to the same endpoint. What the queue adds is that
// the decision is answerable where it was ranked, instead of sending a reader
// to a second screen to do what the row already described.
function decidable(item: WorklistItem): boolean {
  return item.actions.includes("decide") && item.source === "approval";
}

// The decision itself, fetched whole because a row cannot carry it.
function RowDecision({ item }: Readonly<{ item: WorklistItem }>) {
  const approval = useApproval(item.id, true);
  // A body with no `kind` is not a proposal this card can draw: the kind
  // chooses the label, the tool chip and the autonomy dot. Treated as a failed
  // read rather than rendered, because the alternative is a throw that takes
  // the whole day's page down over one malformed answer.
  const usable = approval.data?.kind ? approval.data : undefined;
  if (!usable) {
    return null;
  }
  return (
    <div className="worklist-row-decision">
      <ApprovalRow approval={usable} extraInvalidateKeys={[worklistKey]} />
    </div>
  );
}

// A notice's one verb: the reader has seen it, and it leaves the lane.
// Settled here rather than routed as a link — the mutation this calls is
// the notice's own read endpoint, and there is no separate surface to send
// the reader to for "I've seen this".
function NoticeAcknowledge({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const toast = useToast();
  const acknowledge = useNoticeRead([worklistKey]);
  return (
    <div className="worklist-row-verbs">
      <Button
        small
        pending={acknowledge.isPending}
        onClick={() =>
          acknowledge.mutate(id, {
            // A rejected read leaves the button idle with nothing else on
            // screen to say so — the same rendering a click that did nothing
            // would leave. Without this the notice stays in the lane and the
            // reader has no reason to try again.
            onError: () =>
              toast.show(t("worklist.verb.acknowledgeFailed"), {
                mark: false,
              }),
          })
        }
      >
        {t("worklist.verb.acknowledge")}
      </Button>
    </div>
  );
}

// Finishing a task where the reader is standing.
//
// The verb ACTS, it does not navigate. `VERB_DESTINATION` routes `complete` to
// the task's own record, which is a reasonable address and the wrong promise: a
// control labelled "Done" that opens a page leaves the task open, and the reader
// believes otherwise. The mutation exists and every other surface already uses
// it, so the row completes the task rather than renaming the promise down.
function TaskComplete({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const toast = useToast();
  const update = useTaskUpdate([worklistKey]);
  return (
    <div className="worklist-row-verbs">
      <Button
        small
        variant="primary"
        pending={update.isPending}
        onClick={() =>
          update.mutate(
            { id, body: { is_done: true } },
            {
              // Undoable from the confirmation, the way every disposition
              // beside it is. Done REMOVES the row, so a misclick otherwise
              // costs the reader the only address they had for the task —
              // they must remember what it was to find it again.
              onSuccess: () =>
                toast.show(t("worklist.verb.completed"), {
                  action: {
                    label: t("worklist.verb.completeUndo"),
                    // The toast dismisses itself the moment the action is
                    // pressed, so a failed undo leaves the task done with the
                    // only way back already off the screen.
                    onAct: () =>
                      update.mutate(
                        { id, body: { is_done: false } },
                        {
                          onError: () =>
                            toast.show(t("worklist.verb.completeUndoFailed"), {
                              mark: false,
                            }),
                        },
                      ),
                  },
                }),
              // A rejected PATCH otherwise leaves the button idle with nothing
              // on screen to say so — the same rendering a click that did
              // nothing would leave, and the reader has no reason to try again.
              onError: () =>
                toast.show(t("worklist.verb.completeFailed"), { mark: false }),
            },
          )
        }
      >
        {t("tasks.complete")}
      </Button>
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
    <div className="worklist-row-verbs">
      <Button small onClick={onReview}>
        {t("worklist.verb.review_batch")}
      </Button>
    </div>
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
    <div className="worklist-row-verbs">
      {/* The step the product already worked out, offered where the reader is
          standing rather than on a screen they have to go and find. */}
      {move && (
        <a className="link-button" href={move}>
          {/* THE LABEL MOVES WITH THE ROUTE AND WITH THE VERB. Where the
              address opens the composer the label is the act; where it only
              reaches the record it says so. And it names the verb the SERVER
              chose, so an opening outreach is not offered as a reply to a
              conversation nobody has had. */}
          {moveLabel(item, t)}
        </a>
      )}
      {verbs.map(({ action, destination }) => (
        <a key={action} className="link-button" href={destination}>
          {VERB_LABEL[action](t)}
        </a>
      ))}
    </div>
  );
}

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
};

// The day's figures, and the dials that narrow them.
