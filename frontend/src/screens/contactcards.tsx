import { ExternalLink, FileText } from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import { Avatar, Badge, Button, Checkbox } from "../design-system/atoms";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import {
  formatDayMonth,
  formatMoneyCompact,
  formatNumber,
} from "../format/format";
import { daysPast } from "../format/lateness";
import { type Locale, useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useViewerId } from "./common";

// The overview's four cards (concept §5.6–5.9). Each one is a read of what the
// 360 already assembled — none of them fetches, so a card can never show a
// record the page beside it is withholding.

type Contact360 = components["schemas"]["Contact360"];

// The band's commercial block: the same "does this card have anything to
// show" test ContactCommercialCard makes, so the band and the panel below it
// never disagree about whether there is a deal to speak of.
export function hasCommercial(view: Contact360): boolean {
  const commercial = view.commercial;
  if (!commercial) {
    return false;
  }
  return (
    commercial.deal != null ||
    commercial.role != null ||
    commercial.committee.length > 0
  );
}

// --- What matters (§5.7) ---------------------------------------------------

// The three what-matters kinds, in the order a reader asks them. The
// communication-preference row the concept once proposed is deliberately
// absent: observed-style inference was dropped from the product (ADR-0097 D1).
const MATTERS: ReadonlyArray<{ kind: string; labelKey: MessageKey }> = [
  { kind: "priority", labelKey: "contact.matters.priorities" },
  { kind: "objection", labelKey: "contact.matters.objections" },
  { kind: "success_criterion", labelKey: "contact.matters.successCriteria" },
];

export function ContactMattersCard({
  view,
  firstName,
}: Readonly<{ view: Contact360; firstName: string }>) {
  const t = useT();
  const claims = view.claims ?? [];
  return (
    <Panel title={t("contact.matters.title", { name: firstName })}>
      {MATTERS.map((row) => {
        const match = claims.find(
          (claim) => claim.kind === row.kind && claim.status !== "dismissed",
        );
        return (
          <PanelRow className="pe-row" key={row.kind}>
            <span>{t(row.labelKey)}</span>
            <span className="pe-row-value">
              {match ? match.body : <Absent />}
            </span>
            {match && <FileText size={15} aria-hidden="true" />}
          </PanelRow>
        );
      })}
    </Panel>
  );
}

// The band's what-matters block: true the moment any row ContactMattersCard
// lists has a live claim behind it, so a band that says "captured" is never
// followed by a panel showing nothing but "Nothing captured yet" three times.
export function hasMatters(view: Contact360): boolean {
  const claims = view.claims ?? [];
  return MATTERS.some((row) =>
    claims.some(
      (claim) => claim.kind === row.kind && claim.status !== "dismissed",
    ),
  );
}

// Absence has meaning (concept §4.7): a row nobody has said anything about
// says so, rather than disappearing and leaving the card looking complete.
function Absent(): ReactNode {
  const t = useT();
  return <span>{t("contact.matters.absent")}</span>;
}

// --- Open deal and buying role (§5.8) --------------------------------------

export function ContactCommercialCard({
  view,
}: Readonly<{ view: Contact360 }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const commercial = view.commercial;
  if (!commercial) {
    // The section was withheld. "You may not see deals" and "there is no deal"
    // are different facts, and only the first belongs here.
    return (
      <Panel title={t("contact.commercial.title")}>
        <PanelBody>
          <p className="pe-prose t-body">{t("contact.commercial.withheld")}</p>
        </PanelBody>
      </Panel>
    );
  }
  const deal = commercial.deal;
  return (
    <Panel title={t("contact.commercial.title")}>
      <PanelBody>
        {/* "No open deal" is a fact about the deal, not about the role or the
            committee — a contact can carry a buying role and sit on a
            committee with nothing currently for sale, and both facts belong
            on the card whether or not there is a deal to hang them off. */}
        {!deal && (
          <p className="pe-prose t-body">{t("contact.commercial.noDeal")}</p>
        )}
        {!deal && commercial.role && (
          <Badge tone="success">{readableRole(commercial.role)}</Badge>
        )}
        {deal && (
          <>
            <div className="pe-deal-head">
              <span className="pe-deal-title">{deal.title}</span>
              {commercial.role && (
                <Badge tone="success">{readableRole(commercial.role)}</Badge>
              )}
            </div>
            <div className="pe-deal-figures">
              {[
                deal.amount_minor != null && deal.currency
                  ? formatMoneyCompact(deal.amount_minor, deal.currency, locale)
                  : null,
                deal.stage,
                deal.close_date
                  ? t("contact.commercial.closes", {
                      // The record's own zone: a close date is a date-only
                      // wire value with no instant to localize, and a reader
                      // west of UTC rendering it in their own would quote the
                      // day before to a colleague quoting the right one.
                      date: formatDayMonth(deal.close_date, locale, recordZone),
                    })
                  : null,
              ]
                .filter(Boolean)
                .join(" · ")}
            </div>
          </>
        )}

        {commercial.committee.length > 0 && (
          <>
            <div className="pe-committee-label t-sub">
              {t("contact.commercial.committee")}
            </div>
            {commercial.committee.map((member) => (
              <div className="pe-committee-row" key={member.contact_id}>
                <span className="pe-committee-contact">
                  <Avatar name={member.full_name} src={member.photo_url} />
                  <span>{member.full_name}</span>
                </span>
                <span className="t-sub">{readableRole(member.role)}</span>
              </div>
            ))}
          </>
        )}

        {deal && (
          <Button
            className="pe-rail-more"
            onClick={() => navigate({ screen: "deals", id: deal.deal_id })}
          >
            {t("contact.commercial.openDeal")}{" "}
            <ExternalLink aria-hidden="true" />
          </Button>
        )}
      </PanelBody>
    </Panel>
  );
}

// The stored role key rendered as words. An unrecognized key is shown as it
// was stored — inventing a label for a role nobody defined would be a claim.
export function readableRole(role: string): string {
  const words = role.replace(/_/g, " ");
  return words.charAt(0).toUpperCase() + words.slice(1);
}

// --- Commitments and open loops (§5.9) -------------------------------------

// ours / theirs / questions, in that order: what WE owe leads, because it is
// the only one entirely within the reader's control.
// A loop whose prefix key is null is prefixed with the contact's own name
// instead, which no catalog can carry.
const LOOPS: ReadonlyArray<{ kind: string; prefixKey: MessageKey | null }> = [
  { kind: "commitment_ours", prefixKey: "contact.loops.ours" },
  { kind: "commitment_theirs", prefixKey: null },
  { kind: "open_question", prefixKey: "contact.loops.question" },
];

// Tasks have their own attention list; this decides whether a claim still
// needs attention. The detail card may also retain completed claims as history.
export function hasOpenCommitments(view: Contact360): boolean {
  return openLoops(view, undefined, false).some((loop) => !loop.done);
}

// Everything this record owes, from BOTH places a promise is written down: a
// claim an extractor read out of a conversation, and a task somebody filed.
// The card once read claims alone, so a record whose only open promise was a
// task — which is what an accepted transcript proposal becomes — said "nothing
// has been promised" directly under a headline naming the promise.
//
// Tasks lead: one was typed or confirmed by a contact, a claim was inferred.
// The task list arrives ordered by urgency (the next-steps read), and that
// order is kept.
//
// This list is what the record has open in BOTH directions: it carries
// `commitment_theirs` and an `open_question` too, and keeps a done loop so the
// card can strike it through. A count of what we owe would be a narrower
// question (ours, and only while unfinished); this list answers the wider one.
function openLoops(
  view: Contact360,
  viewerId: string | undefined,
  includeTasks = true,
): readonly OpenLoop[] {
  const claims = view.claims ?? [];
  const tasks = (includeTasks ? (view.next_steps?.data ?? []) : []).map(
    (task): OpenLoop => ({
      key: task.id,
      // A task can arrive without a subject — one filed without one, and one
      // whose content this reader may not see, which the server nulls. Both
      // are still owed, and the card names them the way the backend's own
      // card does rather than printing a bare "You:".
      body: task.subject ?? "an open task",
      // "You" only when the task is not somebody else's. The activity writer
      // assigns every human-written task to its author, so "has an assignee"
      // is true of nearly all of them and would drop the prefix from the
      // reader's own work; the comparison that matters is against the reader.
      // While /me is in flight the id is unknown, and an unattributed row is
      // the honest reading — better than telling someone they owe a
      // colleague's promise.
      prefixKey: heldByReader(task.assignee_id, viewerId)
        ? "contact.loops.ours"
        : null,
      dueAt: task.due_at ?? null,
      done: task.is_done === true,
      theirs: false,
    }),
  );
  const fromClaims = LOOPS.flatMap((loop) =>
    claims
      .filter(
        (claim) => claim.kind === loop.kind && claim.status !== "dismissed",
      )
      .map(
        (claim): OpenLoop => ({
          key: claim.id,
          body: claim.body,
          prefixKey: loop.prefixKey,
          dueAt: claim.due_at ?? null,
          done: claim.status === "done",
          theirs: loop.kind === "commitment_theirs",
        }),
      ),
  );
  return [...tasks, ...fromClaims];
}

// One line of the card, whatever it was read from: a claim carries its kind
// in a prefix, a task is always ours.
type OpenLoop = {
  key: string;
  body: string;
  // The word before the promise. Null means the contact's own name, which no
  // catalog can carry.
  prefixKey: MessageKey | null;
  dueAt: string | null;
  done: boolean;
  // Whether the OTHER side owes it, which decides the badge when no date is set.
  theirs: boolean;
};

// Whether this task is the reader's to deliver. Unassigned work is the
// workspace's, and the reader is the workspace.
function heldByReader(
  assigneeId: string | null | undefined,
  viewerId: string | undefined,
): boolean {
  if (!assigneeId) {
    return true;
  }
  return viewerId !== undefined && assigneeId === viewerId;
}

export function ContactCommitmentsCard({
  view,
  firstName,
  includeTasks = true,
}: Readonly<{ view: Contact360; firstName: string; includeTasks?: boolean }>) {
  const t = useT();
  const rows = openLoops(view, useViewerId(), includeTasks);
  return (
    <Panel title={t("contact.loops.title")}>
      {rows.length === 0 && (
        // An empty commitments card on a record whose mail contains no
        // promises is CORRECT behaviour, not a gap (ADR-0097 consequences).
        <PanelBody>
          <p className="pe-prose t-body">{t("contact.loops.empty")}</p>
        </PanelBody>
      )}
      {rows.map((loop) => (
        <PanelRow className="pe-loop" key={loop.key}>
          {/* A read of the claim's done state, never a write: disabled so a
              click can't nudge the tick, and the accessible name lives here
              (sr-only) because the visible body sits in its own cell so the
              row's three-column rhythm holds. */}
          <Checkbox
            label={
              <span className="sr-only">
                {loopPrefix(loop, firstName, t)}
                {loop.body}
              </span>
            }
            checked={loop.done}
            disabled
          />
          <span className="pe-loop-body">
            {loopPrefix(loop, firstName, t)}
            {loop.body}
          </span>
          <LoopStatus loop={loop} />
        </PanelRow>
      ))}
    </Panel>
  );
}

function loopPrefix(
  loop: OpenLoop,
  firstName: string,
  t: ReturnType<typeof useT>,
): string {
  if (loop.theirs) {
    return `${firstName}: `;
  }
  // No prefix at all for a task somebody else holds: the card does not know
  // their name, and naming the wrong desk is worse than naming none.
  return loop.prefixKey ? `${t(loop.prefixKey)}: ` : "";
}

function LoopStatus({ loop }: Readonly<{ loop: OpenLoop }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  // An unreadable due instant names no deadline, so the row reads as one with
  // no date rather than as a promise due at some NaN o'clock.
  const dueMs = loop.dueAt ? Date.parse(loop.dueAt) : Number.NaN;
  if (!Number.isNaN(dueMs)) {
    // The verdict comes from the instant and the count only picks the wording:
    // a promise 23 hours past due is late by no whole days and still late, and
    // reading `days > 0` as the verdict is what let this card call it "due
    // yesterday" while the task list called the same promise overdue.
    const nowMs = Date.now();
    const { days, late } = daysPast(dueMs, nowMs);
    if (late) {
      return (
        // ds:ignore an overdue marker in the danger ink, not a message
        <span className="pe-loop-due pe-loop-overdue">
          {days > 0
            ? plural("contact.loops.overdue", days, {
                count: formatNumber(days, locale),
              })
            : t("contact.loops.overdueUnderDay")}
        </span>
      );
    }
    return (
      <span className="pe-loop-due">
        {t("contact.loops.due", { when: dueWord(nowMs, dueMs, t, locale) })}
      </span>
    );
  }
  if (loop.theirs) {
    return <Badge tone="accent">{t("contact.loops.waiting")}</Badge>;
  }
  return <Badge>{t("contact.loops.openBadge")}</Badge>;
}

// When a promise that is not yet late falls due. The arguments are swapped on
// purpose: how many whole days `now` is past the DUE moment is how many whole
// days that moment is still ahead, so the days ahead and the days late are one
// spelling of the count rather than two. Counting the elapsed days instead read
// -1 for anything less than a day out, which filed a promise due this evening
// under tomorrow.
function dueWord(
  nowMs: number,
  dueMs: number,
  t: ReturnType<typeof useT>,
  locale: Locale,
): string {
  const { days } = daysPast(nowMs, dueMs);
  if (days === 0) {
    return t("contact.loops.dueToday");
  }
  if (days === 1) {
    return t("contact.loops.dueTomorrow");
  }
  return t("contact.loops.dueInDays", { count: formatNumber(days, locale) });
}
