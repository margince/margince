import { ExternalLink, FileText } from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import { Avatar, Badge, Button, Checkbox } from "../design-system/atoms";
import { EmailReference } from "../design-system/emailreference";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import {
  formatDate,
  formatDayMonth,
  formatMoneyCompact,
  formatNumber,
} from "../format/format";
import { daysPast } from "../format/lateness";
import { type Locale, useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useViewerId } from "./common";
import { interactionIcon, useInteractionLabel } from "./interactionchrome";
import { SentenceList, WrittenBy } from "./record360";

// The overview's four cards (concept §5.6–5.9). Each one is a read of what the
// 360 already assembled — none of them fetches, so a card can never show a
// record the page beside it is withholding.

type Person360 = components["schemas"]["Person360"];
type PersonBrief = components["schemas"]["PersonBrief"];
type Activity = components["schemas"]["Activity"];
type BriefEvidence = components["schemas"]["OrganizationBriefEvidence"];

// --- Relationship brief (§5.6) ---------------------------------------------

export function PersonBriefCard({
  brief,
  loading,
  view,
  onOpenEmail,
}: Readonly<{
  brief: PersonBrief | undefined;
  loading: boolean;
  view: Person360;
  /**
   * Opens a cited message in the record's email drawer. The page owns the
   * drawer, so it owns the opener; a card that mounted its own would put a
   * second one behind the first.
   */
  onOpenEmail?: (activityId: string) => void;
}>) {
  const t = useT();
  const viewerId = useViewerId();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const firstName = view.person.full_name.split(" ")[0];
  // The timeline the page already read, by id. A citation is resolved from it
  // rather than fetched, on the same terms as every other card here: a chip
  // can never name a record the page beside it is withholding.
  const citedActivities = new Map(
    (view.activities?.data ?? []).map((row) => [row.id, row]),
  );
  const written = brief && brief.sentences.length > 0;
  return (
    <Panel
      title={t("person.brief.title")}
      // Who wrote it, on the band that claims it. A reader weighing a sentence
      // needs to know whether a model or the deterministic fallback wrote it,
      // and the two are not interchangeable.
      titleAction={written ? <WrittenBy by={brief.generated_by} /> : undefined}
      footer={
        written ? (
          <span className="t-caption">
            {t("co.brief.generatedAt", {
              when: formatDate(brief.generated_at, locale, recordZone),
            })}
          </span>
        ) : undefined
      }
    >
      <PanelBody>
        {loading && <p className="pe-prose">{t("person.brief.reading")}</p>}
        {!loading && !written && (
          // Honest rather than blank: a brief with nothing to say has nothing to
          // say, and inventing prose to fill the card is the one thing the
          // grounding rule forbids.
          <p className="pe-prose">{t("person.brief.empty")}</p>
        )}
        {written && (
          <>
            {/* The kit's prose, judgement first: what the brief ADDS to the
                cards above it is what the agent makes of them, and every
                sentence says what kind of claim it is. The sources are drawn
                below by transport rather than as citation chips, because a
                chip cannot know whether a cited conversation was mail or a
                chat message, and this card's reader has been told wrong
                before. */}
            <SentenceList
              sentences={brief.sentences}
              leadWithJudgement
              citations="none"
            />
            <div className="pe-chiprow">
              {/* One chip per distinct source, not per citation: several
                  sentences routinely cite the same thread, and rendering the
                  chip once per mention would repeat it and collide on its key. */}
              {[
                ...new Map(
                  brief.sentences.flatMap((sentence) =>
                    sentence.evidence.map(
                      (cited) =>
                        [
                          `${cited.entity_type}-${cited.entity_id}`,
                          cited,
                        ] as const,
                    ),
                  ),
                ).entries(),
              ].map(([key, cited]) => (
                <SourceChip
                  key={key}
                  cited={cited}
                  activity={citedActivities.get(cited.entity_id)}
                  onOpenEmail={onOpenEmail}
                />
              ))}
            </div>
          </>
        )}
      </PanelBody>
      <PanelBody className="pe-brief-state">
        {/* The band under the prose: the three detail panels below only render
            when they have something beyond their own empty sentence, so this
            is the one place a reader always finds all three states named,
            whether or not the panel that expands on them is present. */}
        <div className="pe-brief-block">
          <h3 className="pe-brief-label t-caption">
            {t("person.commercial.title")}
          </h3>
          <p className="pe-brief-line">{commercialLine(view, t, locale)}</p>
        </div>
        <div className="pe-brief-block">
          <h3 className="pe-brief-label t-caption">
            {t("person.loops.title")}
          </h3>
          <p className="pe-brief-line">{commitmentsLine(view, viewerId, t)}</p>
        </div>
        <div className="pe-brief-block">
          <h3 className="pe-brief-label t-caption">
            {t("person.matters.title", { name: firstName })}
          </h3>
          <p className="pe-brief-line">{mattersLine(view, t)}</p>
        </div>
      </PanelBody>
    </Panel>
  );
}

// The band's commercial block: the same "does this card have anything to
// show" test PersonCommercialCard makes, so the band and the panel below it
// never disagree about whether there is a deal to speak of.
export function hasCommercial(view: Person360): boolean {
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

function commercialLine(
  view: Person360,
  t: ReturnType<typeof useT>,
  locale: Locale,
): string {
  const commercial = view.commercial;
  if (!commercial) {
    return t("person.commercial.withheld");
  }
  const deal = commercial.deal;
  if (deal) {
    const amount =
      deal.amount_minor != null && deal.currency
        ? formatMoneyCompact(deal.amount_minor, deal.currency, locale)
        : null;
    return [deal.title, amount].filter(Boolean).join(" · ");
  }
  if (commercial.role) {
    return readableRole(commercial.role);
  }
  if (commercial.committee.length > 0) {
    return t("person.commercial.committee");
  }
  return t("person.commercial.noDeal");
}

// One cited source, named for what it is.
//
// A citation carries a RECORD TYPE and an id, never a transport — so an
// activity citation says nothing at all about how the conversation was
// carried, and calling every one of them a mail thread told the reader of a
// contact with no email address that they had been mailed. The activity the
// page ALREADY read is the resolver: it carries the kind, and for a message
// the provider the directory names. A citation whose activity is not on this
// page is named for the one thing it certainly is — a conversation — rather
// than for a transport nobody checked.
function SourceChip({
  cited,
  activity,
  onOpenEmail,
}: Readonly<{
  cited: BriefEvidence;
  activity: Activity | undefined;
  onOpenEmail?: (activityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const interactionLabel = useInteractionLabel();
  if (cited.entity_type === "activity") {
    // A cited EMAIL is named by its subject and openable, like every other
    // citation of a message in the product. A chip reading "Email" tells a
    // reader which transport carried the sentence and nothing about which
    // message — and the one they want is the one the sentence rests on.
    //
    // `email_summary` is the server's own answer to "is this an email", set
    // only for kind=email, so nothing here decides it from the kind string. A
    // withheld message carries the summary with its subject nulled, and the
    // reference draws the withheld wording and opens nothing.
    const summary = activity?.email_summary;
    if (summary) {
      return (
        <EmailReference
          subject={summary.subject}
          occurredAt={formatDate(summary.occurred_at, locale, recordZone)}
          withheld={summary.display_status === "withheld"}
          onOpen={
            onOpenEmail ? () => onOpenEmail(summary.activity_id) : undefined
          }
        />
      );
    }
    return (
      <span className="pe-memory-channel">
        {interactionIcon(activity?.kind)}
        {activity
          ? interactionLabel(activity.kind, activity.channel_provider)
          : t("person.brief.sourceActivity")}
      </span>
    );
  }
  return (
    <span className="pe-memory-channel">
      <FileText size={13} aria-hidden="true" />
      {cited.entity_type === "deal"
        ? t("person.brief.sourceDeal")
        : cited.entity_type}
    </span>
  );
}

// --- What matters (§5.7) ---------------------------------------------------

// The three what-matters kinds, in the order a reader asks them. The
// communication-preference row the concept once proposed is deliberately
// absent: observed-style inference was dropped from the product (ADR-0097 D1).
const MATTERS: ReadonlyArray<{ kind: string; labelKey: MessageKey }> = [
  { kind: "priority", labelKey: "person.matters.priorities" },
  { kind: "objection", labelKey: "person.matters.objections" },
  { kind: "success_criterion", labelKey: "person.matters.successCriteria" },
];

export function PersonMattersCard({
  view,
  firstName,
}: Readonly<{ view: Person360; firstName: string }>) {
  const t = useT();
  const claims = view.claims ?? [];
  return (
    <Panel title={t("person.matters.title", { name: firstName })}>
      {MATTERS.map((row) => {
        const match = claims.find(
          (claim) => claim.kind === row.kind && claim.status !== "dismissed",
        );
        return (
          <PanelRow className="pe-row" key={row.kind}>
            <span className="pe-row-label">{t(row.labelKey)}</span>
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

// The band's what-matters block: true the moment any row PersonMattersCard
// lists has a live claim behind it, so a band that says "captured" is never
// followed by a panel showing nothing but "Nothing captured yet" three times.
export function hasMatters(view: Person360): boolean {
  const claims = view.claims ?? [];
  return MATTERS.some((row) =>
    claims.some(
      (claim) => claim.kind === row.kind && claim.status !== "dismissed",
    ),
  );
}

function mattersLine(view: Person360, t: ReturnType<typeof useT>): string {
  const claims = view.claims ?? [];
  const present = MATTERS.filter((row) =>
    claims.some(
      (claim) => claim.kind === row.kind && claim.status !== "dismissed",
    ),
  );
  if (present.length === 0) {
    return t("person.matters.absent");
  }
  return present.map((row) => t(row.labelKey)).join(", ");
}

// Absence has meaning (concept §4.7): a row nobody has said anything about
// says so, rather than disappearing and leaving the card looking complete.
function Absent(): ReactNode {
  const t = useT();
  return (
    <span className="pe-rail-value-muted">{t("person.matters.absent")}</span>
  );
}

// --- Open deal and buying role (§5.8) --------------------------------------

export function PersonCommercialCard({ view }: Readonly<{ view: Person360 }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const commercial = view.commercial;
  if (!commercial) {
    // The section was withheld. "You may not see deals" and "there is no deal"
    // are different facts, and only the first belongs here.
    return (
      <Panel title={t("person.commercial.title")}>
        <PanelBody>
          <p className="pe-prose">{t("person.commercial.withheld")}</p>
        </PanelBody>
      </Panel>
    );
  }
  const deal = commercial.deal;
  return (
    <Panel title={t("person.commercial.title")}>
      <PanelBody>
        {/* "No open deal" is a fact about the deal, not about the role or the
            committee — a person can carry a buying role and sit on a
            committee with nothing currently for sale, and both facts belong
            on the card whether or not there is a deal to hang them off. */}
        {!deal && <p className="pe-prose">{t("person.commercial.noDeal")}</p>}
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
                  ? t("person.commercial.closes", {
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
            <div className="pe-committee-label">
              {t("person.commercial.committee")}
            </div>
            {commercial.committee.map((member) => (
              <div className="pe-committee-row" key={member.person_id}>
                <span className="pe-committee-person">
                  <Avatar name={member.full_name} src={member.photo_url} />
                  <span>{member.full_name}</span>
                </span>
                <span className="pe-committee-role">
                  {readableRole(member.role)}
                </span>
              </div>
            ))}
          </>
        )}

        {deal && (
          <Button
            small
            className="pe-rail-more"
            onClick={() => navigate({ screen: "deals", id: deal.deal_id })}
          >
            {t("person.commercial.openDeal")}{" "}
            <ExternalLink size={13} aria-hidden="true" />
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
  { kind: "commitment_ours", prefixKey: "person.loops.ours" },
  { kind: "commitment_theirs", prefixKey: null },
  { kind: "open_question", prefixKey: "person.loops.question" },
];

// The band's commitments block: the same non-dismissed, loop-kind test the
// card's row list runs, so an empty band block and an empty panel are always
// the same fact rather than two independent reads of the claims.
export function hasCommitments(view: Person360): boolean {
  // The band asks only WHETHER there is anything to show, which does not
  // depend on who is reading.
  return openLoops(view, undefined).length > 0;
}

// Everything this record owes, from BOTH places a promise is written down: a
// claim an extractor read out of a conversation, and a task somebody filed.
// The card once read claims alone, so a record whose only open promise was a
// task — which is what an accepted transcript proposal becomes — said "nothing
// has been promised" directly under a headline naming the promise.
//
// Tasks lead: one was typed or confirmed by a person, a claim was inferred.
// The task list arrives ordered by urgency (the next-steps read), and that
// order is kept.
function openLoops(
  view: Person360,
  viewerId: string | undefined,
): readonly OpenLoop[] {
  const claims = view.claims ?? [];
  const tasks = (view.next_steps?.data ?? []).map(
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
        ? "person.loops.ours"
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

function commitmentsLine(
  view: Person360,
  viewerId: string | undefined,
  t: ReturnType<typeof useT>,
): string {
  const openCount = openLoops(view, viewerId).length;
  if (openCount === 0) {
    return t("person.loops.empty");
  }
  // The sections this counts are summaries the server caps, so the number is a
  // floor whenever one of them ran out of room. Printing it flat said "25
  // open" on a record holding thirty-one, and the six it did not mention are
  // exactly the ones nobody is looking at.
  const count = `${openCount} ${t("person.loops.open")}`;
  return truncated(view) ? t("person.loops.atLeast", { count }) : count;
}

// Whether either section this card reads has more rows than it carried.
function truncated(view: Person360): boolean {
  return view.next_steps?.page.has_more === true;
}

export function PersonCommitmentsCard({
  view,
  firstName,
}: Readonly<{ view: Person360; firstName: string }>) {
  const t = useT();
  const rows = openLoops(view, useViewerId());
  return (
    <Panel title={t("person.loops.title")}>
      {rows.length === 0 && (
        // An empty commitments card on a record whose mail contains no
        // promises is CORRECT behaviour, not a gap (ADR-0097 consequences).
        <PanelBody>
          <p className="pe-prose">{t("person.loops.empty")}</p>
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
        <span className="pe-loop-due pe-loop-overdue">
          {days > 0
            ? plural("person.loops.overdue", days, {
                count: formatNumber(days, locale),
              })
            : t("person.loops.overdueUnderDay")}
        </span>
      );
    }
    return (
      <span className="pe-loop-due">
        {t("person.loops.due", { when: dueWord(nowMs, dueMs, t, locale) })}
      </span>
    );
  }
  if (loop.theirs) {
    return <Badge tone="accent">{t("person.loops.waiting")}</Badge>;
  }
  return <Badge>{t("person.loops.open")}</Badge>;
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
    return t("person.loops.dueToday");
  }
  if (days === 1) {
    return t("person.loops.dueTomorrow");
  }
  return t("person.loops.dueInDays", { count: formatNumber(days, locale) });
}
