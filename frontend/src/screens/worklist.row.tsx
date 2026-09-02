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
import { useNoticeRead } from "./taskactions";
import {
  comparisonText,
  consequenceText,
  dealFactsText,
  itemTitle,
  moveHref,
  reasonText,
  rowHref,
} from "./worklist.copy";
import { DispositionVerbs } from "./worklist.dispositions";
import { ReassignControl } from "./worklist.manager";
import {
  useApproval,
  type WorklistItem,
  worklistKey,
} from "./worklist.queries";

export function WorklistRow({
  item,
  position,
  owner,
  asOf,
  selected,
  onSelect,
  onReview,
}: Readonly<{
  item: WorklistItem;
  position: number;
  // Whose queue this row is on, empty for the reader's own. A row can only be
  // handed to somebody else from a page that is already about somebody else.
  owner: string;
  // When the server took this snapshot. The waiting_days tie-break's elapsed
  // days are computed against THIS, not the render's own wall clock — a cached
  // read rendered later, or a client clock that has drifted from the
  // server's, must not silently change what the row says about an order the
  // server already decided as of a fixed instant.
  asOf: string;
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
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const href = rowHref(item);
  const title = itemTitle(item, t, locale);
  const facts = dealFactsText(item, t, locale, zone);
  const because = item.because
    .map((reason) => reasonText(reason, t, locale, zone))
    .filter((phrase): phrase is string => phrase !== null)
    .join(" · ");
  const above = comparisonText(
    item.above_next,
    t,
    locale,
    zone,
    new Date(asOf),
  );
  const consequence = consequenceText(item, t);
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
        <p className="t-body worklist-row-title">
          {href ? (
            <a className="entity-link" href={href}>
              {title}
            </a>
          ) : (
            title
          )}
          <Badge>{t(`worklist.category.${item.category}` as const)}</Badge>
          {item.overdue && <Badge tone="danger">{t("worklist.overdue")}</Badge>}
        </p>
        {/* `detail` is not prose on every source: a relationship-decay row
            carries a bare day COUNT there (attention/render.go's lapsedItem,
            "the client writes 'quiet N days'"), which this row already says
            properly through `because`. Only the `notice` source's detail is
            a full sentence — the server sets it from the notice's own body,
            the deal's name included — so only that source renders it here
            rather than every source that happens to send one. */}
        {item.source === "notice" && item.detail && (
          <p className="t-caption worklist-row-detail">{item.detail}</p>
        )}
        {item.batch?.sample && item.batch.sample.length > 0 && (
          // A group nobody can see into is a group nobody trusts, and an
          // untrusted group is worse than the pile it replaced.
          <p className="t-caption worklist-row-sample">
            {item.batch.sample.join(" · ")}
          </p>
        )}
        {facts && <p className="t-caption worklist-row-facts">{facts}</p>}
        {because && <p className="t-caption worklist-row-because">{because}</p>}
        {/* What it costs to do nothing. The question a queue exists to answer,
            and the one the lane feed had no field for. */}
        {consequence && (
          <p className="t-caption worklist-row-consequence">{consequence}</p>
        )}
        {/* Why this row beat the one below it. Absent on the last row, which
            has nothing below it to beat. */}
        {above && <p className="t-caption worklist-row-above">{above}</p>}
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
      {decidable(item) && <RowDecision item={item} />}
      {/* Settled inline rather than drawn by RowVerbs — see NoticeAcknowledge. */}
      {item.source === "notice" && item.actions.includes("acknowledge") && (
        <NoticeAcknowledge id={item.id} />
      )}
    </PanelRow>
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
          {t("worklist.verb.draft_reply")}
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
