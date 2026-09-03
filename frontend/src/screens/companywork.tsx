// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The account's work in flight: one line per open deal, written from that
// record's own facts.
//
// This replaced a written account brief. On an account carrying several
// engagements the brief blended them — correspondence about one deal became a
// sentence about another, and a figure read out of the blend had nowhere to be
// checked. So every line is one record's own story, and the header above them
// counts and nothing else.
//
// Deals only. The account's projects have exactly one home on the record —
// the ProjectLinks section, which also holds attach and detach — and a second
// list of them here was a second answer to "which bodies of work is this part
// of". hasWorkInFlight still reads the projects half: whether the fit panel
// stands in for this card is a question about the account, not about what
// this card draws.
//
// Every line is a template over typed fields the server picked. Nothing on
// this card is model-written, which is what makes each line checkable against
// the record it links to.

import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import { Badge } from "../design-system/atoms";
import { Eyebrow } from "../design-system/eyebrow";
import { Panel, PanelBody } from "../design-system/panel";
import {
  omitted,
  SurfaceState,
  sectionState,
} from "../design-system/surfacestate";
import {
  formatDate,
  formatMoneyOrAbsent,
  formatNumber,
} from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
// The row shapes this card draws — `co-rowlink`, `co-row-meta` — are the
// record page's, defined in company360.css. Imported here rather than left to
// whichever screen happens to mount the card: without it the card renders
// unstyled anywhere company360.css is not already on the page, which is what
// Storybook showed — the meta parts ran together with no separator.
import "./company360.css";
import "./companywork.css";

type Organization360 = components["schemas"]["Organization360"];
type WorkDeal = components["schemas"]["Organization360Deal"];
type WorkProject = components["schemas"]["Organization360Project"];
type Attention = components["schemas"]["Organization360WorkAttention"];

/**
 * CompanyWorkCard is the overview's lead: what is moving on this account, and
 * for each piece of it, the one reason it wants a person today.
 *
 * A withheld deals section says so where its rows would have been — never a
 * count over rows this reader may not see, and never the fit panel in this
 * card's place, which would read as "there is no pipeline here, go read the
 * fit".
 */
export function CompanyWorkCard({
  view,
  loading = false,
  onOpenRecord,
  bare = false,
  verbs,
}: Readonly<{
  view?: Organization360;
  // The composite read's own pending flag — see sectionState's own doc.
  loading?: boolean;
  // Where a cited conversation opens. The page owns it, because the same
  // receipt is cited from several cards and two owners would mean two
  // receipts open over each other.
  onOpenRecord?: (entityType: string, entityId: string) => void;
  // Render the sections without this card's own Panel, for a caller that
  // holds the chrome. The Company 360 card does: this is one reading of the
  // account among four, and a card inside a card is two borders around one
  // list.
  bare?: boolean;
  // What opens a new deal. Handed in rather than built here: whether this
  // reader may write, and what a create costs to mount, are the page's
  // questions — this card knows only where the verb goes.
  verbs?: { deal?: ReactNode };
}>) {
  const t = useT();
  const deals = view?.deals;
  const dealState = sectionState(
    view,
    "deals",
    Boolean(deals),
    deals?.data.length ?? 0,
    loading,
  );
  const body = (
    <>
      <WorkGroup
        label={t("co.work.deals")}
        level={bare ? "h4" : "h3"}
        state={dealState}
        emptyLabel={t("co.work.noDeals")}
        emptyDetail={t("co.work.noDealsDetail")}
        action={verbs?.deal}
      >
        {(deals?.data ?? []).map((deal) => (
          <DealLine
            key={deal.deal_id}
            deal={deal}
            onOpenRecord={onOpenRecord}
          />
        ))}
      </WorkGroup>
      {view?.attention_withheld && (
        <PanelBody>
          <p className="t-caption co-work-incomplete">
            {t("co.work.statusesWithheld")}
          </p>
        </PanelBody>
      )}
    </>
  );
  // Bare, the groups stand under the pane's own head: a second head naming
  // the work in flight, over a group already named Deals, was a pane that
  // introduced itself twice.
  if (bare) {
    return body;
  }
  return (
    <Panel
      title={t("co.work.title")}
      titleAction={<WorkCount view={view} />}
      footer={sinceLastVisitFooter(view)}
    >
      {body}
    </Panel>
  );
}

/**
 * SinceLastVisit is what moved on this account while the reader was away, and
 * whether any of the account was withheld from them.
 *
 * It is about the ACCOUNT rather than about any one section of it, so it sits
 * in the footer of whichever card is the account's reading — this one when it
 * stands alone, and the Company 360 card's own footer when this is a section
 * of it.
 *
 * PRIVATE on purpose: both mounts reach it through sinceLastVisitFooter, and
 * a caller that could put this element in a footer slot directly is the
 * blank-band defect waiting to happen again. It already happened twice.
 *
 * Withheld sections are named ONCE, about the whole page, rather than as a
 * refusal beside each line a reader did not get.
 */
function SinceLastVisit({ view }: Readonly<{ view?: Organization360 }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const since = newActivities(view);
  const first = firstVisit(view);
  const withheld = (view?.sections_omitted?.length ?? 0) > 0;
  if (!speaksSinceLastVisit(view)) {
    return null;
  }
  return (
    <p className="co-work-foot">
      {/* Never both: on a first open the server counts every activity as new,
          and "14 new items" beside "you are opening this account for the
          first time" is the page contradicting itself. */}
      {first && <span className="t-caption">{t("co.since.first")}</span>}
      {!first && since > 0 && (
        <span className="t-caption">
          {plural("co.read.newActivity", since, {
            count: formatNumber(since, locale),
          })}
        </span>
      )}
      {withheld && <span className="t-caption">{t("co.prep.withheld")}</span>}
    </p>
  );
}

// The since-last-visit line for a footer slot, or nothing at all.
//
// UNDEFINED rather than an element that renders null, and that is the whole
// point of this function existing: Panel draws its footer band on the slot's
// truthiness, and an element is truthy whatever it renders — so handing it
// `<SinceLastVisit/>` on an account with nothing to report costs the record a
// blank row. Both mounts call this; passing the component straight into a
// footer is the defect it exists to make unavailable.
export function sinceLastVisitFooter(
  view?: Organization360,
): ReactNode | undefined {
  return speaksSinceLastVisit(view) ? (
    <SinceLastVisit view={view} />
  ) : undefined;
}

// Whether there is a sentence to say at all — read by the component's own
// guard and by the footer helper above, so the band and the sentence cannot
// disagree about whether there is one.
function speaksSinceLastVisit(view?: Organization360): boolean {
  return (
    firstVisit(view) ||
    newActivities(view) > 0 ||
    // Optional on BOTH hops: a composite that answered without the omission
    // list names nothing withheld, and reading `.length` off the absent list
    // throws where the whole record page then renders as "this view no longer
    // works".
    (view?.sections_omitted?.length ?? 0) > 0
  );
}

// newActivities is how many landed since the reader's baseline.
//
// Zero and "not counted" are different answers and neither earns a line: a
// withheld section means nobody counted, and a counted zero means nothing
// happened — reporting either as news would be a claim the page cannot make.
function newActivities(view?: Organization360): number {
  if (!view || omitted(view, "since_last_visit")) {
    return 0;
  }
  return view.since_last_visit?.new_activities ?? 0;
}

// firstVisit is true only when the account HAS a baseline section and it is
// empty. Read off an absent section it would turn data a reader's grants
// withheld into a claim about their own history.
function firstVisit(view?: Organization360): boolean {
  if (!view || omitted(view, "since_last_visit")) {
    return false;
  }
  return Boolean(view.since_last_visit) && !view.since_last_visit?.baseline_at;
}

/**
 * hasWorkInFlight is whether this card has anything to say, which is what
 * decides between it and the growth-fit panel in the same slot.
 *
 * A WITHHELD half counts as work: a reader who may not see the projects has
 * not been told this account has none, and swapping in the fit panel would
 * tell them exactly that.
 */
export function hasWorkInFlight(view?: Organization360): boolean {
  if (!view) {
    return false;
  }
  const withheld = omitted(view, "deals") || omitted(view, "projects");
  return (
    withheld ||
    (view.deals?.data.length ?? 0) > 0 ||
    liveWork(view.projects).length > 0
  );
}

// The projects this card is about: the ones still in motion. A closed project
// is history, and the same filter the project picker applies — one answer to
// "live", so the card and the picker cannot disagree.
function liveWork(projects?: readonly WorkProject[]): readonly WorkProject[] {
  return (projects ?? []).filter((project) => project.phase !== "closed");
}

// One group, in whichever state its own half of the payload is in. A withheld
// half says so where its rows would have been, so the other half still reads.
function WorkGroup({
  label,
  level,
  state,
  emptyLabel,
  emptyDetail,
  action,
  children,
}: Readonly<{
  label: string;
  // Deals and Projects sit one level under whatever names this card. Standing
  // alone that is the card title, an h2, so they are h3; as a section of the
  // Company 360 card they are h4 — the outline nests rather than flattening
  // into a row of equal siblings a screen reader cannot walk.
  level: "h3" | "h4";
  state: ReturnType<typeof sectionState>;
  emptyLabel: string;
  // What this kind of work IS, said only where there is none of it. A group
  // with rows in it needs no definition; a reader looking at an empty one is
  // deciding whether to start something, and that is the moment the sentence
  // is worth its line.
  emptyDetail: string;
  // The verb that opens one. Absent on a record nobody may write to, and on a
  // group whose read did not come back — a create offered over a failed read
  // is a write into a section the page cannot show.
  action?: ReactNode;
  children: ReactNode;
}>) {
  // The head is drawn in EVERY state, so the group's own verb keeps one place
  // on the card. Moved into the empty plate it changed position with the
  // content — a reader who has just read one group looks for the next verb
  // where the last one was.
  const head = (
    <PanelBody className="co-work-head">
      <Eyebrow as={level}>{label}</Eyebrow>
      {action}
    </PanelBody>
  );
  if (state === "ready") {
    return (
      <>
        {head}
        {/* A grid of items rather than a stack of rows. Each piece of work is
            a thing a reader picks up and decides about, and a row that has to
            carry a name, a figure, a stage, a date and a sentence has nowhere
            to put five facts except side by side in one line. */}
        <div className="co-work-items">{children}</div>
      </>
    );
  }
  return (
    <>
      {head}
      <PanelBody>
        {/* An empty group gets a PLATE rather than a line of grey text. There
            being no deal on an account is a state a reader has to decide
            about, and drawn as one quiet sentence it reads as a section that
            failed to load. The dashes say the space is waiting to be filled;
            the verb that fills it is in the head above, where every other
            group keeps its own.

            No label on the state itself either way: the head carries it, and
            a section that names itself twice reads as two sections. */}
        {state === "empty" ? (
          <div className="co-work-empty">
            <p className="co-work-empty-title">{emptyLabel}</p>
            <p className="co-work-empty-note">{emptyDetail}</p>
          </div>
        ) : (
          <SurfaceState state={state} emptyLabel={emptyLabel}>
            {null}
          </SurfaceState>
        )}
      </PanelBody>
    </>
  );
}

/**
 * WorkCount is the header's whole content: how much is in flight, as a count.
 *
 * Counts and facts only. The reasons live on the rows, where each one sits
 * beside the record it was read from — a summary sentence up here would be
 * the blended narrative this card replaced.
 *
 * It counts what the card draws — the open deals — and says nothing at all
 * when that half is withheld: "0 in flight" to a reader who may not see the
 * deals is a false statement about the account, not a partial one.
 */
function WorkCount({ view }: Readonly<{ view?: Organization360 }>) {
  const t = useT();
  const { locale } = useLocale();
  if (!view?.deals) {
    return null;
  }
  const count = view.deals.data.length;
  const capped = view.deals.page.has_more;
  return (
    <span className="t-small">
      {capped
        ? t("co.work.countAtLeast", { count: formatNumber(count, locale) })
        : t("co.work.count", { count: formatNumber(count, locale) })}
    </span>
  );
}

function DealLine({
  deal,
  onOpenRecord,
}: Readonly<{
  deal: WorkDeal;
  onOpenRecord?: (entityType: string, entityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const status = deal.attention ? (
    <AttentionLine attention={deal.attention} onOpenRecord={onOpenRecord} />
  ) : (
    deal.stalled && <StatusLine>{t("co.work.stalled")}</StatusLine>
  );
  return (
    <article className="co-work-item">
      <button
        type="button"
        className="co-rowlink co-work-item-name"
        onClick={() => navigate({ screen: "deals", id: deal.deal_id })}
      >
        {deal.name}
      </button>
      {deal.amount?.amount_minor != null && (
        <p className="co-work-figure">
          {formatMoneyOrAbsent(
            deal.amount.amount_minor,
            deal.amount.currency,
            locale,
          )}
        </p>
      )}
      <span className="co-row-meta">
        <span>{deal.stage_name ?? t("co.deals.noStage")}</span>
        {deal.expected_close_date && (
          <span>
            {t("co.work.closes", {
              date: formatDate(deal.expected_close_date, locale, zone),
            })}
          </span>
        )}
      </span>
      {/* One status clause per item, in the order that decides which one it
          is: an overdue task beats a stall, because a stall is the absence of
          a reason and an overdue task IS one. It is also the only colour the
          item carries — a card where every line is coloured has no colour
          left for the line that needs it. */}
      {status ? <div className="co-work-item-foot">{status}</div> : null}
    </article>
  );
}

/**
 * AttentionLine is the row's one reason, as a whole sentence.
 *
 * Whole-sentence templates, never a phrase concatenated from halves: German
 * puts the verb where English does not, and a sentence assembled from pieces
 * is a sentence no translator can fix. That is also why there is a separate
 * key for the unnamed case rather than a name substituted with a blank.
 *
 * A commitment QUOTES the body rather than asserting over it. The body is
 * free text an extractor read out of a conversation — "we'll revisit after
 * legal" is a proposition, not a noun phrase — so a template reading "they
 * owe us {body}" produces nonsense, and quoting also shows the reader the
 * words that were extracted instead of a claim written on top of them.
 */
function AttentionLine({
  attention,
  onOpenRecord,
}: Readonly<{
  attention: Attention;
  onOpenRecord?: (entityType: string, entityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const due = attention.due_at
    ? formatDate(attention.due_at, locale, zone)
    : "";
  if (attention.kind === "overdue_task") {
    return (
      <StatusLine tone="warn">
        {attention.who
          ? t("co.work.overdueTask", {
              who: attention.who,
              title: attention.title,
              date: due,
            })
          : t("co.work.overdueTaskUnnamed", {
              title: attention.title,
              date: due,
            })}
      </StatusLine>
    );
  }
  const sentence = attention.who
    ? t("co.work.owesUs", { who: attention.who, body: attention.title })
    : t("co.work.owesUsUnnamed", { body: attention.title });
  const source = attention.source_activity_id;
  return (
    <StatusLine tone={attention.due_at ? "warn" : undefined}>
      {source && onOpenRecord ? (
        <button
          type="button"
          className="co-rowlink"
          onClick={() => onOpenRecord("activity", source)}
        >
          {sentence}
        </button>
      ) : (
        sentence
      )}
      {due && ` ${t("co.work.wasDue", { date: due })}`}
    </StatusLine>
  );
}

// The row's status clause, on its own line under the facts. A sentence in the
// meta row would read as one more chip beside the stage and the money; this
// is the row saying why it is here, so it gets its own line.
//
// `quiet` on the badge for the same reason: one per row, in a column of rows,
// and a stack of filled pills is decoration a reader learns to skip.
function StatusLine({
  tone,
  children,
}: Readonly<{ tone?: "warn"; children: ReactNode }>) {
  if (tone) {
    return (
      <span className="co-work-status">
        <Badge tone={tone} quiet>
          {children}
        </Badge>
      </span>
    );
  }
  return <span className="co-work-status t-caption">{children}</span>;
}
