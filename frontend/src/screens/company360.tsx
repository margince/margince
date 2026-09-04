import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Sparkles } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import { Badge, Button, Skeleton, StatCard } from "../design-system/atoms";
import { type TimelineEntry, TimelineRow } from "../design-system/composed";
import { Eyebrow } from "../design-system/eyebrow";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import {
  liveProjects,
  type PickableProject,
  ProjectPicker,
  useClearVanishedChoice,
  useSoleProjectDefault,
} from "../design-system/projectpicker";
import { StatStrip } from "../design-system/statstrip";
import {
  omitted,
  SurfaceState,
  sectionState,
} from "../design-system/surfacestate";
import {
  calendarDaysBetween,
  formatDate,
  formatDateAbbrev,
  formatMoney,
  formatMoneyCompact,
  formatMoneyOrAbsent,
  formatNumber,
  formatTimeOfDay,
} from "../format/format";
import { viewerZone } from "../format/timezone";
import { type Locale, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  problemCodeOf,
  problemMessageOf,
  throwProblem,
  useFinanceSummary,
  useViewerId,
} from "./common";
import type { CompanyTab } from "./companytab";
import { dealsFilteredBy } from "./dealsaddress";
import "./company360.css";
import { activityTimeline } from "../design-system/activitytimeline";
import { FactList } from "../design-system/factlist";
import { HEALTH_DIMENSION_LABEL, HEALTH_RATING_LABEL } from "./companylookups";
import { EntityRef } from "./entityref";
import { Citations, FoundMove, SentenceList, WrittenBy } from "./record360";
import { TaskCompleteCheck, type useTaskUpdate } from "./taskactions";

// The company view's data layer and its right-rail cards.
//
// One read (GET /organizations/{id}/360) serves the whole page, and its
// `sections_omitted` is the thing that makes the page honest: a section the
// caller's role cannot read is ABSENT from the payload and named there, so
// every card below can say "hidden from you" instead of drawing an empty
// list that reads as "there is none".

type Organization360 = components["schemas"]["Organization360"];
type Deal360 = components["schemas"]["Organization360Deal"];
type NextStep = components["schemas"]["Organization360NextStep"];
const OVERLAY_REFUSAL = "unsupported_in_overlay_mode";

export type Org360Result =
  | { state: "ready"; view: Organization360 }
  | { state: "overlay" };

/**
 * useOrganization360 reads the whole company page in one round trip.
 *
 * `enabled` exists for callers that are not the page: chrome mounted on every
 * screen has to hold the hook unconditionally and ask for nothing when there is
 * no record under it — an empty id is a 422, not an empty answer.
 */
export function useOrganization360(id: string, enabled = true) {
  return useQuery<Org360Result>({
    queryKey: ["organization360", id],
    enabled: enabled && id !== "",
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/organizations/{id}/360",
        { params: { path: { id } } },
      );
      if (error) {
        if (response.status === 422 && isOverlayRefusal(error)) {
          return { state: "overlay" };
        }
        throwProblem(error);
      }
      return { state: "ready", view: data };
    },
  });
}

// VIEW_ACK_DWELL_MS is how long the account must stay open before the visit
// counts. Opening a record and bouncing straight back out is not reading it,
// and an ack from that would mark unread activity as seen.
const VIEW_ACK_DWELL_MS = 5_000;

/**
 * useAcknowledgeOrganizationView advances THIS reader's "last seen" baseline
 * for the account — the thing that makes "N new since your last visit" mean
 * anything on the next visit. Without it the server keeps answering with no
 * baseline at all, so every visit reads as the first one.
 *
 * The 360 deliberately does not advance the baseline itself (a prefetch must
 * not be indistinguishable from a visit), so this is the only caller. Leaving
 * before the dwell elapses cancels the timer: the baseline moves only for a
 * visit that actually happened, and when in doubt it stays where it is —
 * showing an item twice is a smaller wrong than hiding one.
 *
 * Success does NOT invalidate the 360. The "new since your last visit" line
 * describes the visit in progress; refetching it out from under the reader
 * would erase the very thing they opened the page to see.
 */
export function useAcknowledgeOrganizationView(id: string, visited: boolean) {
  const ack = useMutation({
    mutationFn: async (organizationId: string) => {
      const { error } = await api.POST("/organizations/{id}/view-ack", {
        params: { path: { id: organizationId } },
      });
      if (error) {
        throwProblem(error);
      }
    },
  });
  // The mutation's own error state holds a failure; nothing renders it. A
  // baseline that did not move costs the reader one repeated line next time,
  // which is not worth an error banner over the account they came to read.
  const fire = ack.mutate;
  useEffect(() => {
    if (!visited) {
      return;
    }
    const timer = window.setTimeout(() => fire(id), VIEW_ACK_DWELL_MS);
    return () => window.clearTimeout(timer);
  }, [id, visited, fire]);
}

// isOverlayRefusal distinguishes "this workspace reads elsewhere" from every
// other 422 (a malformed id, say), which stays an error the caller sees.
//
// It narrows by checking rather than asserting: a problem body that is not
// the shape we expect — null, a string, an older server's payload — is not
// an overlay refusal, and must read as one failure rather than throwing a
// second one on the way to saying so.
function isOverlayRefusal(problem: unknown): boolean {
  const errors = asRecord(asRecord(problem)?.details)?.errors;
  if (!Array.isArray(errors)) {
    return false;
  }
  return errors.some((entry) => asRecord(entry)?.code === OVERLAY_REFUSAL);
}

// asRecord narrows an unknown to a readable object, or gives up. Truthiness
// first, because typeof null is "object" — the one case that would otherwise
// pass the guard and throw on the next property read.
function asRecord(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === "object"
    ? (value as Record<string, unknown>)
    : undefined;
}

/** DealsCard lists the open pipeline plus the two lifetime figures. */
export function DealsCard({
  view,
  actions,
  extra,
  loading = false,
}: Readonly<{
  view?: Organization360;
  // The verbs that change this section, rendered under it. Absent on an
  // archived record, which takes no new deals.
  actions?: ReactNode;
  // Whatever else belongs beside this account's deals — the Deals tab hands
  // in the last offer read here rather than drawing it as a second card, so
  // the two readings that both start from "this account's open deals" stop
  // reading as two different sections.
  extra?: ReactNode;
  // The composite read's own pending flag — see sectionState's own doc. The
  // Deals tab already gates its own skeleton on `!view && !failed` before
  // this ever renders, so `view` is always defined by the time this call
  // runs; passed anyway so the card is correct on its own terms rather than
  // depending on a caller's guard it cannot see.
  loading?: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const deals = view?.deals;
  const won = deals?.won_lifetime;
  const state = sectionState(
    view,
    "deals",
    Boolean(deals),
    deals?.data.length ?? 0,
    loading,
  );
  const present = state === "ready" || state === "empty";
  return (
    <Panel
      title={t("co.deals.title")}
      titleAction={present ? actions : undefined}
      footer={
        deals && (
          <p className="co-row-meta">
            <span>
              {t("co.deals.wonLifetime")}{" "}
              {formatMoneyOrAbsent(won?.amount_minor, won?.currency, locale)}
            </span>
            {/* The lost count names a set of this company's deals, so it is
                the way into them — and it opens exactly that set. Narrowed by
                `status` and not by a lost STAGE, because no single closed-lost
                stage id exists across pipelines while `status` is a dial the
                deals endpoint reads. */}
            <a
              className="link-button"
              href={dealsFilteredBy("organization_id", view.organization.id, {
                status: "lost",
              })}
            >
              {t("co.deals.lostCount", {
                count: formatNumber(deals.lost_count, locale),
              })}
            </a>
          </p>
        )
      }
    >
      {present ? (
        <>
          {(deals?.data ?? []).map((deal) => (
            <DealRow key={deal.deal_id} deal={deal} />
          ))}
          {state === "empty" && (
            <PanelBody>
              <p className="surfacestate-empty">{t("co.deals.empty")}</p>
            </PanelBody>
          )}
          {extra}
        </>
      ) : (
        <PanelBody>
          <SurfaceState state={state} emptyLabel={t("co.deals.empty")}>
            {null}
          </SurfaceState>
        </PanelBody>
      )}
    </Panel>
  );
}

function DealRow({ deal }: Readonly<{ deal: Deal360 }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <PanelRow className="co-row">
      <button
        type="button"
        className="co-rowlink"
        onClick={() => navigate({ screen: "deals", id: deal.deal_id })}
      >
        {deal.name}
      </button>
      <span className="co-row-meta">
        <span>{deal.stage_name ?? t("co.deals.noStage")}</span>
        {deal.amount?.amount_minor != null && (
          <span className="t-mono">
            {formatMoneyOrAbsent(
              deal.amount.amount_minor,
              deal.amount.currency,
              locale,
            )}
          </span>
        )}
        {deal.stalled && <Badge tone="warn">{t("deal.stalled")}</Badge>}
      </span>
    </PanelRow>
  );
}

/**
 * CommercialPanel is the overview's own reading of the pipeline: the two
 * lifetime figures the deals section actually carries, then the open deals
 * themselves. It is deliberately not DealsCard reused wholesale — the Deals
 * tab keeps that card in full, and this is the shorter reading a rep gets
 * without leaving Overview.
 *
 * No open-pipeline total is drawn: nothing in Organization360 sums the open
 * deals' amounts, and inventing one here would be exactly the fabricated
 * figure the deals section's own honesty rule forbids.
 */
export function CommercialPanel({
  view,
  titleAction,
  extra,
  onAllDeals,
  loading = false,
  figuresOnly = false,
}: Readonly<{
  view?: Organization360;
  // The "new deal" verb, gated by the caller on the record being writable.
  titleAction?: ReactNode;
  // What else belongs to this account's commercial standing but is not read
  // off its deals — the overview hands in what it is already under contract
  // for, rather than a second card repeating "the commercial picture" under
  // its own heading.
  //
  // Rendered OUTSIDE the deals branch below, unlike DealsCard's slot of the
  // same name: the two readings answer to different grants, and a reader who
  // may see contracts and not deals would otherwise lose theirs to somebody
  // else's permission.
  extra?: ReactNode;
  onAllDeals?: () => void;
  // The composite read's own pending flag — see sectionState's own doc.
  loading?: boolean;
  // Draw the FIGURES and the contract block without this card's own header
  // band or its list of deals, for a caller that already lists them. The
  // Company 360 card does: its work section names every open deal with the
  // reason it needs a person, and repeating them underneath would show each
  // deal twice on one screen.
  //
  // The figures are what does not appear there — what the account has won
  // over its life and how much it has lost — so this is the half of the
  // reading the work list cannot carry, not a second copy of it.
  figuresOnly?: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const deals = view?.deals;
  const state = sectionState(
    view,
    "deals",
    Boolean(deals),
    deals?.data.length ?? 0,
    loading,
  );
  const present = state === "ready" || state === "empty";
  // The section is a page of `deals.data` with `has_more` beside it — past
  // the cap this reads as the whole open pipeline unless it says otherwise.
  const truncated = deals?.page.has_more === true;
  const figures = state === "ready" && deals && (
    <PanelBody className="co-figures">
      <CommercialFigure
        label={t("co.deals.wonLifetime")}
        value={formatMoneyOrAbsent(
          deals.won_lifetime?.amount_minor,
          deals.won_lifetime?.currency,
          locale,
        )}
      />
      {/* The same figure as the deals card's, and the same door: one company,
          status lost. Two spellings of one address is how the two figures come
          to open different lists. */}
      <CommercialFigure
        label={t("co.commercial.lostFigure")}
        value={
          <a
            className="link-button"
            href={dealsFilteredBy("organization_id", view.organization.id, {
              status: "lost",
            })}
          >
            {formatNumber(deals.lost_count, locale)}
          </a>
        }
      />
    </PanelBody>
  );
  if (figuresOnly) {
    // The contract block first, for the same reason it leads the whole card:
    // what the account is already signed for frames the deals still moving.
    return (
      <>
        {extra}
        {figures}
        {/* `figures` covers `ready` alone, and the work group under these
            figures says "no deals" with its own plate, so `empty` says nothing
            here — twice is a pane that names one absence as two. Every other
            state still owes the reader a sentence: a withheld section is a
            fact about the reader, and one that fell silently blank would be
            read as an empty account. */}
        {state !== "ready" && state !== "empty" && (
          <PanelBody>
            <SurfaceState state={state} emptyLabel={t("co.deals.empty")}>
              {null}
            </SurfaceState>
          </PanelBody>
        )}
      </>
    );
  }
  return (
    <Panel
      title={t("co.commercial.title")}
      titleAction={present ? titleAction : undefined}
      footer={
        present && (onAllDeals || truncated) ? (
          <>
            {truncated && (
              <p className="co-row-meta">{t("co.commercial.truncated")}</p>
            )}
            {onAllDeals && (
              <Button small variant="ghost" onClick={onAllDeals}>
                {t("co.commercial.allDeals")}
              </Button>
            )}
          </>
        ) : undefined
      }
    >
      {/* Before the pipeline, and before the panel's own deals footer: what
          the account is already signed for frames the deals that are still
          moving, and the Deals tab reads in that order too. */}
      {extra}
      {state === "ready" && deals ? (
        <>
          {figures}
          {deals.data.map((deal) => (
            <PanelRow key={deal.deal_id} className="co-commercial-row">
              <button
                type="button"
                className="co-rowlink co-commercial-name"
                onClick={() => navigate({ screen: "deals", id: deal.deal_id })}
              >
                <span className="co-commercial-title">{deal.name}</span>
                {deal.expected_close_date && (
                  <span className="co-commercial-sub">
                    {t("commercial.closes", {
                      when: formatDate(
                        deal.expected_close_date,
                        locale,
                        recordZone,
                      ),
                    })}
                  </span>
                )}
              </button>
              <span className="co-row-meta">
                {deal.stage_name && <Badge>{deal.stage_name}</Badge>}
                {deal.amount?.amount_minor != null && (
                  <span className="t-mono">
                    {formatMoneyOrAbsent(
                      deal.amount.amount_minor,
                      deal.amount.currency,
                      locale,
                    )}
                  </span>
                )}
              </span>
            </PanelRow>
          ))}
        </>
      ) : (
        <PanelBody>
          <SurfaceState state={state} emptyLabel={t("co.deals.empty")}>
            {null}
          </SurfaceState>
        </PanelBody>
      )}
    </Panel>
  );
}

// One eyebrow-labelled figure. Shared shape with the finance panel, so the
// two read as the same kind of reading rather than two different cards that
// happen to sit near each other.
function CommercialFigure({
  label,
  value,
}: Readonly<{ label: string; value: ReactNode }>) {
  return (
    <div className="co-figure">
      <Eyebrow>{label}</Eyebrow>
      {/* A figure the page does not have still occupies its slot, as the
          absence its formatter returned: the reader sees WHICH reading is
          missing rather than a shorter row that reads as complete. */}
      <span className="co-figure-value">{value}</span>
    </div>
  );
}

// How many entries the overview's chronology carries. The full history is the
// History tab; this is "what happened lately" without leaving Overview.
const RECENT_ACTIVITY_LIMIT = 5;

// One run of the timeline under the day it happened. Consecutive rather than
// grouped-by-key, because `entries` arrives newest-first from the server and a
// day is never revisited later in the same page.
type ActivityDay = { key: string; entries: TimelineEntry[] };

function groupByDay(
  entries: readonly TimelineEntry[],
  locale: Locale,
  recordZone: string,
) {
  const days: ActivityDay[] = [];
  for (const entry of entries) {
    const key = formatDate(entry.atIso, locale, recordZone);
    const last = days.at(-1);
    if (last?.key === key) {
      last.entries.push(entry);
    } else {
      days.push({ key, entries: [entry] });
    }
  }
  return days;
}

/**
 * RecentActivityPanel is the overview's chronology: the same activities
 * section the rail used to carry, grouped under the day they happened rather
 * than as a flat list — a day is one thing that happened, several messages
 * are how it happened.
 *
 * Reads the SAME activities section the account's Suggestions and health
 * cards read, so the story here cannot disagree with what they cite.
 */
export function RecentActivityPanel({
  view,
  onOpenHistory,
  loading = false,
  bare = false,
}: Readonly<{
  view?: Organization360;
  // Where the header's link leads. Absent for a caller with no History tab
  // of its own (the stories file).
  onOpenHistory?: () => void;
  // The composite read's own pending flag — see sectionState's own doc.
  loading?: boolean;
  // Render the BODY without this card's own header band, for a caller that
  // holds the Panel itself and labels the section. One implementation, two
  // mounts: the Deals tab still draws the whole card, and the Company 360
  // card draws this section inside its own chrome — a second copy of the
  // timeline is how two surfaces come to disagree about what happened.
  bare?: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const viewerId = useViewerId();
  const recordZone = useRecordZone();
  // Every logged activity, not only the ones with a subject: a call or a note
  // often has none, and filtering them out here would under-report the
  // chronology and — because the count feeds sectionState — draw "nothing
  // logged with them yet" on an account that has been called five times.
  const logged = view?.activities?.data ?? [];
  const state = sectionState(
    view,
    "activities",
    Boolean(view?.activities),
    logged.length,
    loading,
  );
  const days = groupByDay(
    activityTimeline(logged.slice(0, RECENT_ACTIVITY_LIMIT), viewerId),
    locale,
    recordZone,
  );
  const body =
    state === "ready" ? (
      days.map((day) => (
        <div key={day.key} className="co-timeline-day">
          {/* One level under whatever names this timeline: the card's own
              title when it stands alone, the section subhead when it is a
              section of the Company 360 card. */}
          <Eyebrow as={bare ? "h4" : "h3"} className="co-timeline-day-heading">
            {day.key}
          </Eyebrow>
          <ul className="timeline">
            {day.entries.map((entry) => (
              <TimelineRow key={entry.id} entry={entry} zone={recordZone} />
            ))}
          </ul>
        </div>
      ))
    ) : (
      <PanelBody>
        <SurfaceState state={state} emptyLabel={t("co.recent.empty")}>
          {null}
        </SurfaceState>
      </PanelBody>
    );
  if (bare) {
    return <>{body}</>;
  }
  return (
    <Panel
      title={t("co.recent.title")}
      titleAction={
        onOpenHistory && (
          <Button small variant="ghost" onClick={onOpenHistory}>
            {t("co.recent.viewHistory")}
          </Button>
        )
      }
    >
      {body}
    </Panel>
  );
}

/**
 * NextSteps is the middle column's first block: the open tasks on this
 * account, overdue first, each showing what it is linked to.
 *
 * The tick is `update`'s own verb (`TaskCompleteCheck`), not `renderAction`'s
 * — a row names its primary move as the row, not as one more item in a menu.
 * `renderAction` is left for whatever ELSE a caller wants beside the tick
 * (snooze), and stays hidden until the row is hovered or focused, since a
 * list of open tasks reads by title and due date first and only reveals its
 * verbs on approach.
 */
export function NextSteps({
  view,
  renderAction,
  onOpenTask,
  update,
}: Readonly<{
  view: Organization360;
  renderAction?: (step: NextStep) => ReactNode;
  // Given, the subject opens the task where it is listed. Absent, it stays
  // plain text rather than a button that goes nowhere.
  onOpenTask?: (step: NextStep) => void;
  // Wires the tick to the real completion write. Absent (the stories file,
  // a read-only account) draws the row with no checkbox at all — a box that
  // cannot be ticked is worse than no box.
  update?: ReturnType<typeof useTaskUpdate>;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const steps = view.next_steps?.data ?? [];
  const state = sectionState(
    view,
    "next_steps",
    Boolean(view.next_steps),
    steps.length,
  );
  // A withheld block is dropped entirely — the middle column is the story,
  // and a refusal in the middle of it says nothing a rep can act on. Every
  // other state is shown, because "no open task" and "we could not tell"
  // lead to different next moves.
  if (state === "withheld") {
    return null;
  }
  return (
    <Panel title={t("co.next.title")}>
      {state === "unavailable" && (
        <PanelBody>
          <p className="surfacestate-withheld">{t("co.section.unavailable")}</p>
        </PanelBody>
      )}
      {state === "empty" && (
        <PanelBody>
          <p className="surfacestate-empty">{t("co.next.empty")}</p>
        </PanelBody>
      )}
      {state === "ready" &&
        steps.map((step) => (
          <PanelRow key={step.activity_id} className="co-task-row">
            {update && (
              <TaskCompleteCheck
                activityId={step.activity_id}
                update={update}
              />
            )}
            <span className="co-task-body">
              {onOpenTask ? (
                <button
                  type="button"
                  className="co-rowlink"
                  onClick={() => onOpenTask(step)}
                >
                  {step.subject}
                </button>
              ) : (
                <span>{step.subject}</span>
              )}
              <span className="co-row-meta">
                {step.overdue && (
                  <Badge tone="danger">{t("co.next.overdue")}</Badge>
                )}
                {!step.overdue && step.due_at && (
                  <span>
                    {t("co.next.due", {
                      // The one viewer-clock reading on this record page, and
                      // it is not a preference: `dueInstant` mints a due date
                      // as the end of the picked day in the BROWSER's zone, so
                      // the stored instant already carries the picker's clock.
                      // Read in the organization's zone it names a different
                      // calendar day than the one the picker chose, for every
                      // reader outside that zone — there is no organization
                      // reading of it to prefer. The timeline below still reads
                      // in the record zone, because an activity's occurrence IS a
                      // fact about the record.
                      when: formatDate(step.due_at, locale, viewerZone()),
                    })}
                  </span>
                )}
                {!step.due_at && <span>{t("co.next.undated")}</span>}
                {step.linked_deal_id && (
                  <EntityRef kind="deal" id={step.linked_deal_id} />
                )}
                {step.linked_person_id && (
                  <EntityRef kind="person" id={step.linked_person_id} />
                )}
                {step.assignee_id && (
                  <EntityRef kind="user" id={step.assignee_id} />
                )}
              </span>
            </span>
            {renderAction && (
              <span className="co-task-verbs">{renderAction(step)}</span>
            )}
          </PanelRow>
        ))}
    </Panel>
  );
}

type Question = components["schemas"]["OrganizationQuestion"];
type Suggestion = components["schemas"]["Organization360Suggestion"];
type Answer = components["schemas"]["OrganizationAnswer"];
// The prepared questions, in the order the card offers them: what is open now,
// then what to walk in with, then what has moved.
//
// Keyed by question rather than listed, so the type is EXHAUSTIVE: a question
// declared upstream and not given a position here fails to compile, instead of
// shipping a server that answers it and a card that never asks.
const QUESTIONS: readonly Question[] = Object.keys({
  whats_open: 0,
  meeting_prep: 0,
  whats_changed: 0,
} satisfies Record<Question, 0>) as Question[];

/**
 * AskCard is "Ask Margince": three prepared questions, answered from this
 * account's own records.
 *
 * The questions are BUTTONS, not a text box. Each one names the records its
 * answer is written from, which is what lets every sentence carry a citation
 * the reader can open — and a text box that quietly answered from a subset
 * would look exactly like one that had searched everything.
 */
export function AskSection({
  orgId,
  enabled,
  onOpenRecord,
  projects,
}: Readonly<{
  orgId: string;
  enabled: boolean;
  onOpenRecord?: (entityType: string, entityId: string) => void;
  // The account's projects, as the page read them. Offered as a picker
  // when any is live, so a question can be asked about one engagement
  // rather than the whole account.
  projects?: readonly PickableProject[];
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [projectId, setProjectId] = useState("");
  const recordZone = useRecordZone();
  const live = liveProjects(projects);
  useSoleProjectDefault(live, projectId, setProjectId);
  useClearVanishedChoice(live, projectId, setProjectId);
  const ask = useMutation({
    // The project travels as the mutation variable beside the question, so
    // a stale closure cannot ask about a project the picker no longer shows.
    mutationFn: async ({
      question,
      project,
    }: {
      question: Question;
      project: string;
    }) => {
      const { data, error } = await api.POST("/organizations/{id}/ask", {
        params: { path: { id: orgId } },
        body: { question, ...(project ? { project_id: project } : {}) },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

  if (!enabled) {
    return null;
  }
  const answer: Answer | undefined = ask.data;
  // A payload without sentences is an answer this build cannot read, not an
  // account with nothing to say — the same distinction every card here keeps.
  const readable = Array.isArray(answer?.sentences) ? answer : undefined;
  return (
    <section className="co-part" aria-label={t("co.ask.title")}>
      <Eyebrow as="h3">{t("co.ask.title")}</Eyebrow>
      <ProjectPicker
        projects={live}
        projectId={projectId}
        onChange={(next) => {
          setProjectId(next);
          // The answer on screen was written about the previous project, and
          // its scope line would otherwise stand over the next project's key.
          ask.reset();
        }}
        scope={readable?.scope}
      />
      <p className="co-ask-questions">
        {QUESTIONS.map((question) => (
          <Button
            key={question}
            small
            onClick={() => ask.mutate({ question, project: projectId })}
            disabled={ask.isPending}
          >
            {t(`co.ask.q.${question}`)}
          </Button>
        ))}
      </p>
      {ask.isPending && <Skeleton width="100%" height={40} />}
      {ask.isError && (
        <p className="surfacestate-withheld">
          {t("co.ask.failed")}
          {/* The server's own detail says WHICH failure — budget exhausted reads
              differently from a malformed request, and a rep can act on one. */}
          {` ${problemMessageOf(ask.error, t)}`}
        </p>
      )}
      {/* The previous answer is hidden while the next question is in flight.
          Leaving it under the spinner puts a finished answer next to a loading
          one, and the reader has no way to tell which question they are
          looking at the answer to. */}
      {readable && !ask.isPending && (
        <>
          {/* The question is repeated above its answer: three buttons and one
              answer block leaves the reader guessing which they pressed once
              they have scrolled, and the wrong pairing is worse than none. */}
          <p className="co-ask-asked">{t(`co.ask.q.${readable.question}`)}</p>
          {readable.sentences.length === 0 ? (
            // An empty answer is a real outcome, not a failure: the question's
            // records are not ones this reader can see, so there is nothing to
            // say. Saying that is honest; a sentence written around the gap
            // would not be.
            <p className="surfacestate-empty">{t("co.ask.nothing")}</p>
          ) : (
            <SentenceList
              sentences={readable.sentences}
              onOpenRecord={onOpenRecord}
            />
          )}
          <p className="co-row-meta">
            <WrittenBy by={readable.generated_by} />
            <span>
              {t("co.brief.generatedAt", {
                when: formatDate(readable.generated_at, locale, recordZone),
              })}
            </span>
          </p>
        </>
      )}
    </section>
  );
}

/**
 * SuggestionsCard is what this account looks like it needs next.
 *
 * Each row leads with the REASON the rule fired, because a rep must be able to
 * disagree with the reason rather than with a verdict they cannot inspect. A
 * dismissal is theirs alone and is keyed on the evidence, so the same advice
 * stays gone while the situation holds and comes back when it changes.
 */
type Health = NonNullable<Organization360["health"]>;
// One rated dimension of the account's health: the rating, and the sentence it
// was read from. Named here because three readings carry it as their basis.
type HealthDimension = NonNullable<Health["relationship"]>;

/**
 * HealthCard is how the relationship stands, in the parts a reader can act on
 * (AC-company-3).
 *
 * It replaced a single 0–100 score. That number was the MAX over the account's
 * contacts of a decayed message count, so one talkative contact spoke for the
 * whole account and a long, low-volume relationship read as near-dead. Each
 * line here names a fact instead: "no inbound for 90 days" says what to do,
 * where "2/100" said only a mood.
 *
 * A part the server could not compute is ABSENT, never zero. Zero is a claim
 * about the account; absence is a fact about the reading.
 */
// The rating vocabulary, worst first. The ORDER is the worst-of rule: a
// verdict is the lowest-ranked rating among the dimensions that have one
// (PO-AC-N-11).
export type StateStrip = NonNullable<Organization360["state_strip"]>;

// Whose move it is, in words. Exported (and no longer rendered by this file
// as a strip tile) because the daily brief's context band reads the same
// `engagement` field now — companytoday.tsx composes the label from here
// rather than re-deriving it.
export const ENGAGEMENT_LABELS: Record<
  NonNullable<StateStrip["engagement"]>["state"],
  MessageKey
> = {
  never_contacted: "co.strip.engagement.never_contacted",
  active: "co.strip.engagement.active",
  waiting_on_them: "co.strip.engagement.waiting_on_them",
  waiting_on_us: "co.strip.engagement.waiting_on_us",
  dormant: "co.strip.engagement.dormant",
};

// The two states that name a problem rather than a condition. Colouring only
// these keeps the brief from reading as a dashboard where every tile is lit.
export const ENGAGEMENT_TONE: Partial<
  Record<NonNullable<StateStrip["engagement"]>["state"], "warn">
> = {
  waiting_on_them: "warn",
  dormant: "warn",
};

// A reading the caller's grants withheld. Shared with the person record's
// readings row and rail rather than spelled per surface: all three state the
// same fact about the same reader, and a second spelling is exactly the drift
// that had these rows drawn by two different components in the first place.
const WITHHELD_READING: MessageKey = "record.notShown";

// A reading nobody has judged. It is NOT the withheld word — "you may not see
// this" and "there is no verdict yet" are opposite facts about who is missing
// what, and a slot that confuses them sends the reader to ask for an access
// grant that would show them nothing. Its own key rather than the lifecycle
// label it happens to match today: a rename of one must not silently move the
// other.
const UNASSESSED_READING: MessageKey = "co.strip.notAssessed";

/**
 * StateStrip is the readings row under the tab strip: FIVE doors, always
 * five — open pipeline, invoiced, the conversation, the last touch, and what
 * is next — each a reading of the tab it opens. The account's standing is not
 * here: the verdict word and the three dimensions it is read from are the
 * 360's, directly under this row, so a reading is said once.
 *
 * EVERY SLOT ALWAYS DRAWS, and says honestly that it has no reading when it has
 * none. A slot that vanishes leaves the reader unable to tell WHICH reading is
 * missing — the row simply looks shorter — and only an empty state is allowed to
 * claim there is none (SurfaceState's rule). So the three absences are three
 * different words and never one: withheld is a fact about the READER, unassessed
 * is a fact about how much has been judged, and "no date" is a fact about the
 * ACCOUNT. Inventing "never contacted" out of a withheld engagement would state
 * the business conclusion a rep acts on, from a permission boundary.
 */
//
// What it must never render is the harder half of the rule, and every omission
// below is one of its bullets: no €0 when the figure is unavailable, no
// cross-currency sum without its conversion source, and nothing called
// "revenue" that is only a count of open deals.
export function StateStrip({
  orgId,
  view,
  onOpenTab,
}: Readonly<{
  orgId: string;
  view?: Organization360;
  // The tab each reading is a reading OF. Optional, because a surface that
  // draws these outside the record page (the storybook, a mirror) has no tab
  // strip to send anybody to — and a door with nowhere behind it is not drawn
  // rather than drawn dead.
  onOpenTab?: (tab: CompanyTab) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const strip = view?.state_strip;
  if (!strip) {
    // Absent for two different reasons, and only `sections_omitted` tells
    // them apart. The caller already withholds this component entirely while
    // the composite read is still in flight or has failed (`view` itself is
    // undefined then), so reaching this branch WITH a view means the read
    // succeeded and this one section did not — either it was withheld, or a
    // future account state has nothing here yet. Only the first is worth
    // saying: silently dropping the whole KPI row on a page that otherwise
    // rendered would read as "no readings for this account", the empty state
    // a permission boundary must never impersonate.
    if (view && omitted(view, "state_strip")) {
      return (
        <section className="co-strip-withheld" aria-label={t("co.strip.title")}>
          <p className="surfacestate-withheld">{t("co.section.restricted")}</p>
        </section>
      );
    }
    return null;
  }
  // A CURRENT customer only. A former one has invoices in its past, but "do
  // they pay us, and on time?" is not the question their page is opened with,
  // and leading with a money reading on an account that has stopped buying
  // reads as though the relationship were still running.
  const customer = strip.account.lifecycle === "customer";
  // The contract pairs an absent optional section with its name in
  // `sections_omitted` (Organization360), so the reason `health` did not arrive
  // is readable rather than guessable — and guessing is how a grant boundary
  // gets reported as an account nobody has assessed.
  const healthWithheld = view != null && omitted(view, "health");
  const door = (tab: CompanyTab) => onOpenTab && (() => onOpenTab(tab));
  return (
    // The shared strip: five readings read ACROSS as one row of doors, each
    // into the tab that holds its rows. The region's name is the SCREEN's to
    // set; the cards inside are the shared primitive's, unreached-into.
    <StatStrip label={t("co.strip.title")} testId="company-strip">
      <PipelineCard
        commercial={strip.commercial}
        dimension={view?.health?.commercial}
        locale={locale}
        recordZone={recordZone}
        onOpen={door("deals")}
        t={t}
      />
      {/* Money is a reading every account gets, not only a customer: on one we
          have never billed it says so, which is a fact about the account,
          where an absent card is a hole the reader has to interpret. */}
      <MoneyStat
        orgId={orgId}
        locale={locale}
        customer={customer}
        dimension={view?.health?.payment}
        onOpen={door("finance")}
        t={t}
      />
      {/* The conversation and the last touch both read off the account's
          exchanges: one says whose move it is and how balanced the talk has
          been, the other how long ago the last word fell. */}
      <HealthStat
        health={view?.health}
        locale={locale}
        withheld={healthWithheld}
        onOpen={door("timeline")}
        t={t}
      />
      <LastTouchStat
        view={view}
        locale={locale}
        recordZone={recordZone}
        onOpen={door("timeline")}
        t={t}
      />
      <NextStat
        view={view}
        locale={locale}
        recordZone={recordZone}
        onOpen={door("tasks")}
        t={t}
      />
    </StatStrip>
  );
}

// The last word exchanged, as days since it fell, and who said it. Read off
// the account's own timestamps rather than the health reading: those two
// dates are the fact, and the reading is a judgement made from them.
function LastTouchStat({
  view,
  locale,
  recordZone,
  onOpen,
  t,
}: Readonly<{
  view?: Organization360;
  locale: Locale;
  recordZone: string;
  onOpen?: () => void;
  t: ReturnType<typeof useT>;
}>) {
  const door = { openLabel: t("co.strip.open.history"), onOpen };
  if (!view || omitted(view, "last_touch")) {
    return (
      <StatCard
        {...door}
        label={t("co.strip.lastTouch")}
        value={t(WITHHELD_READING)}
      />
    );
  }
  const inbound = view.last_inbound_at ?? undefined;
  const outbound = view.last_outbound_at ?? undefined;
  const theirs = Boolean(inbound && (!outbound || inbound > outbound));
  const last = theirs ? inbound : outbound;
  if (!last) {
    return (
      <StatCard
        {...door}
        label={t("co.strip.lastTouch")}
        value={t("co.strip.lastTouch.never")}
      />
    );
  }
  const days = calendarDaysBetween(new Date(last), new Date(view.as_of));
  return (
    <StatCard
      {...door}
      label={t("co.strip.lastTouch")}
      value={
        days <= 0
          ? t("co.strip.lastTouch.today")
          : t("co.strip.lastTouch.ago", { count: formatNumber(days, locale) })
      }
      detail={join(
        t(theirs ? "co.strip.lastTouch.theirs" : "co.strip.lastTouch.ours"),
        formatDateAbbrev(last, locale, recordZone),
      )}
    />
  );
}

// What is next on the calendar with this account: the meeting's day, its
// subject and its hour. Nothing scheduled is a fact about the account and is
// said as one; a withheld calendar is said as withheld.
function NextStat({
  view,
  locale,
  recordZone,
  onOpen,
  t,
}: Readonly<{
  view?: Organization360;
  locale: Locale;
  recordZone: string;
  onOpen?: () => void;
  t: ReturnType<typeof useT>;
}>) {
  const door = { openLabel: t("co.strip.open.tasks"), onOpen };
  if (!view || omitted(view, "next_meeting")) {
    return (
      <StatCard
        {...door}
        label={t("co.strip.next")}
        value={t(WITHHELD_READING)}
      />
    );
  }
  const meeting = view.next_meeting;
  if (!meeting) {
    return (
      <StatCard
        {...door}
        label={t("co.strip.next")}
        value={t("co.strip.next.none")}
      />
    );
  }
  return (
    <StatCard
      {...door}
      label={t("co.strip.next")}
      value={formatDateAbbrev(meeting.starts_at, locale, recordZone)}
      detail={join(
        meeting.subject,
        formatTimeOfDay(meeting.starts_at, locale, recordZone),
      )}
    />
  );
}

/**
 * A median days-after-due as a sentence (FIN-FORM-3).
 *
 * Negative days mean they pay BEFORE the due date. "-4 days after due" is a
 * puzzle; "typically 4 days early" is the reading. Shared by the KPI slot and
 * the finance card so the two cannot come to describe earliness differently —
 * spelled twice, only one of the copies would be changed.
 */
export function medianDaysLabel(
  median: number,
  locale: Locale,
  t: ReturnType<typeof useT>,
): string {
  return median < 0
    ? t("finance.medianEarly", { days: formatNumber(Math.abs(median), locale) })
    : t("finance.medianAfterDue", { days: formatNumber(median, locale) });
}

// The caveat on a figure that IS shown but is not current. Undefined when the
// figure is current and needs none.
function staleDetailKey(
  state?: components["schemas"]["FinanceSummaryState"],
): MessageKey | undefined {
  switch (state) {
    case "stale":
      return "co.strip.fin.staleFigure";
    case "error":
      return "co.strip.fin.errorFigure";
    case "syncing":
      // The first pass has not finished, so what is shown may be partial.
      return "co.strip.fin.syncing";
    default:
      return undefined;
  }
}

// Why there is no figure, in the reader's terms. Each state has its own fix,
// and naming the wrong one costs the reader a trip to a settings page they did
// not need.
function financeDetailKey({
  pending,
  withheld,
  failed,
  state,
}: Readonly<{
  pending: boolean;
  withheld: boolean;
  failed: boolean;
  state?: components["schemas"]["FinanceSummaryState"];
}>): MessageKey {
  if (pending) {
    return "co.strip.fin.loading";
  }
  // Both before the state switch: with no answer there is no state to read,
  // and guessing one from its absence is how a denial became setup advice.
  if (withheld) {
    return "co.strip.fin.withheld";
  }
  if (failed) {
    return "co.strip.fin.error";
  }
  switch (state) {
    case "unmapped":
      return "co.strip.fin.unmapped";
    case "syncing":
      return "co.strip.fin.syncing";
    case "stale":
      return "co.strip.fin.staleFigure";
    case "error":
      return "co.strip.fin.error";
    case "connected":
      // A live, mapped source that produced no figure. Nothing is broken and
      // there is nothing to set up — we have simply never billed them, or no
      // invoice could be converted. Setup advice here sends the reader to fix
      // a connection that is already working.
      return "co.strip.fin.nothingBilled";
    default:
      // no_connection, and the read that never answered. Both mean there is
      // no source to read, which is the one case the setup advice fits.
      return "co.strip.fin.noConnection";
  }
}

/**
 * The customer row's ONE money slot: what this account has been invoiced over
 * the trailing year.
 *
 * One slot, not three. The strip is a GLANCE, and the Finance tab
 * (companyfinance.tsx) is where the detail lives — which is already why open
 * balance and the payment-habit median are not here. Spending three of five
 * slots on windows of the same figure buried the account's own standing behind
 * the money, and the standing is the frame the money is read in.
 *
 * The label follows the STATE, because the two states answer different
 * questions. With a figure it names the window the figure covers; with none it
 * says "Finance", because the reason there is none is usually a fact about the
 * connection rather than about that window — labelling a "connect your
 * accounting" slot "net invoiced, 12 months" claims we looked at those twelve
 * months and found nothing there.
 */
function MoneyStat({
  orgId,
  locale,
  customer,
  dimension,
  onOpen,
  t,
}: Readonly<{
  orgId: string;
  locale: Locale;
  // A CURRENT customer. Everyone else has never been invoiced, and the card
  // says exactly that rather than reporting a finance connection that has
  // nothing to do with them.
  customer: boolean;
  // The payment health reading, shown as this card's basis so the verdict a
  // reader meets on the health card can be checked against the money it was
  // read from, on the card that holds the money.
  dimension?: HealthDimension;
  onOpen?: () => void;
  t: ReturnType<typeof useT>;
}>) {
  // The door out of this reading, handed to every shape it takes: a
  // withheld reading and a priced one are the same reading, and only one
  // of them offering the tab would make the way out look like a property
  // of the figure.
  const door = { openLabel: t("co.strip.open.finance"), onOpen };
  // The SAME query the finance card and the payment health dimension run, so
  // every money reading on one page agrees and all but the first cost no
  // request.
  const { data, isPending, isError, error } = useFinanceSummary(orgId);
  const basis = dimension ? (
    <FactList
      facts={[
        {
          key: "payment",
          term: t(HEALTH_DIMENSION_LABEL.payment),
          value: t(HEALTH_RATING_LABEL[dimension.rating]),
          note: dimension.reason,
        },
      ]}
    />
  ) : undefined;
  // Never invoiced is a fact about the ACCOUNT, and it outranks every state the
  // finance connection could be in: a prospect on an installation with no
  // accounting connected must not be told to go and connect one, and a
  // prospect on an installation that HAS one must not read as though we had
  // billed them and got nothing.
  if (!customer) {
    return (
      <StatCard
        {...door}
        label={t("co.strip.finance")}
        value={t("co.strip.financeUnknown")}
        detail={t("co.strip.fin.notACustomer")}
      />
    );
  }
  // A refusal is not a failure and neither is a setup gap. A reader whose role
  // cannot see finance told to "connect your accounting" is sent to a settings
  // page to fix a permission — the one thing they cannot fix from there.
  const withheld = isError && problemCodeOf(error) === "permission_denied";
  const amount = data?.net_invoiced;
  const caveat = staleDetailKey(data?.state);
  // No figure is not €0, and the six reasons there is none are not one reason.
  // "Connect your accounting" is wrong advice for a connection that exists and
  // is syncing, stale, errored or unmatched — it sends the reader to set up
  // something they already have.
  if (!amount || amount.amount_minor == null || !amount.currency) {
    return (
      <StatCard
        {...door}
        label={t("co.strip.finance")}
        value={t("co.strip.financeUnknown")}
        detail={t(
          financeDetailKey({
            pending: isPending,
            withheld,
            failed: isError && !withheld,
            state: data?.state,
          }),
        )}
        basis={basis}
      />
    );
  }
  return (
    <StatCard
      {...door}
      label={t("co.strip.netInvoiced")}
      value={formatMoneyCompact(amount.amount_minor, amount.currency, locale)}
      // The provider name goes in the detail line rather than beside the label:
      // a strip slot is narrower than a free-standing stat card, and a badge
      // beside the label wraps onto its own row underneath it, standing the one
      // slot that names its source taller than every sibling in the row.
      //
      // A figure that is not current is shown WITH its caveat rather than
      // withheld: the last known number is usually the right one, and hiding it
      // tells the reader less than showing it qualified would. The caveat takes
      // the line ahead of the provider name, because which accounting system a
      // figure came from matters less than whether the figure is current — and
      // the Finance tab names the connection anyway.
      //
      // Stale and error say DIFFERENT things, which is why `staleDetailKey` does
      // not fold them: `stale` is a sync that SUCCEEDED, just long enough ago
      // that the date matters; `error` is the last good answer after an attempt
      // that failed. Calling either one the other is a wrong claim about whether
      // anything is broken.
      // Lifetime rides in the detail line rather than taking a slot of its own:
      // beside the trailing year it is the one comparison nothing else on this
      // page carries — what the account has ever been worth against what it has
      // been worth lately — and two money slots on a five-slot row would make
      // this row a finance report rather than a glance.
      //
      // Overdue and the open balance stay OUT, and not for want of room. The
      // Finance tab renders both as headline figures one tab away, so a copy
      // here is a second answer to the same question, read at a glance and
      // drifting from the first the moment either changes.
      detail={
        caveat
          ? t(caveat)
          : moneyPhrase(
              "co.strip.lifetimeOf",
              data?.net_invoiced_lifetime,
              locale,
              t,
            ) ||
            data?.provider ||
            undefined
      }
      basis={basis}
    />
  );
}

// One money figure as a phrase for the detail line, or nothing. Both halves are
// required and neither absence has a substitute: a figure with no currency
// cannot be rendered at all, and a zero the server did not send would say this
// account owes us nothing when the truth is that nobody has told us.
function moneyPhrase(
  key: "co.strip.lifetimeOf",
  amount:
    | { amount_minor?: number | null; currency?: string | null }
    | undefined,
  locale: Locale,
  t: ReturnType<typeof useT>,
): string | undefined {
  return amount?.amount_minor != null && amount.currency
    ? t(key, {
        amount: formatMoneyCompact(
          amount.amount_minor,
          amount.currency,
          locale,
        ),
      })
    : undefined;
}

type StripCommercial = NonNullable<
  NonNullable<Organization360["state_strip"]>["commercial"]
>;

// Open pipeline, labelled as exactly what it is: the sum of open deals, never
// "potential" and never "revenue" (§4.2). Unpriced when nothing on the account
// carries a convertible figure — a €0 there would claim a priced pipeline
// worth nothing, where the truth is the page cannot price it.
function PipelineCard({
  commercial,
  dimension,
  locale,
  recordZone,
  onOpen,
  t,
}: Readonly<{
  commercial?: StripCommercial | null;
  // The commercial health reading, shown as this card's basis so the verdict
  // on the health card can be checked against the deals it was read from, on
  // the card that holds those deals.
  dimension?: HealthDimension;
  locale: Locale;
  recordZone: string;
  onOpen?: () => void;
  t: ReturnType<typeof useT>;
}>) {
  // The door out of this reading, handed to every shape it takes: a
  // withheld reading and a priced one are the same reading, and only one
  // of them offering the tab would make the way out look like a property
  // of the figure.
  const door = { openLabel: t("co.strip.open.deals"), onOpen };
  const basis = dimension ? (
    <FactList
      facts={[
        {
          key: "commercial",
          term: t(HEALTH_DIMENSION_LABEL.commercial),
          value: t(HEALTH_RATING_LABEL[dimension.rating]),
          note: dimension.reason,
        },
      ]}
    />
  ) : undefined;
  const basisProps = {
    basis,
  };
  if (!commercial) {
    // A null `commercial` is the contract's way of saying the caller has no
    // deal grant, so this is the READER's boundary and not an account with
    // nothing running. "No open deals" here would be the business conclusion a
    // rep acts on, invented out of a permission.
    return (
      <StatCard
        {...door}
        label={t("co.strip.pipeline")}
        value={t(WITHHELD_READING)}
        {...basisProps}
      />
    );
  }
  // No open deals is not an unpriced pipeline. Saying "no convertible amount"
  // about an account that has nothing open reports a data problem where the
  // truth is simply that nothing is running.
  if (commercial.open_count === 0) {
    return (
      <StatCard
        {...door}
        label={t("co.strip.pipeline")}
        value={t("co.strip.noOpenDeals")}
        {...basisProps}
      />
    );
  }
  const { open_pipeline_minor_base: value, base_currency: currency } =
    commercial;
  const stalled =
    commercial.stalled_count > 0
      ? t("co.strip.stalled", {
          count: formatNumber(commercial.stalled_count, locale),
        })
      : undefined;
  if (value == null || !currency) {
    // Open deals with no priceable figure still say how many there are: the
    // count is a fact, the money is not. The unpriced note is never dropped
    // for the stalled one — a reader who is told only "1 stalled" has no way
    // to know the pipeline was never priced at all.
    return (
      <StatCard
        {...door}
        label={t("co.strip.pipeline")}
        value={t("co.strip.openDeals", {
          count: formatNumber(commercial.open_count, locale),
        })}
        detail={join(t("co.strip.unpriced"), stalled)}
        tone={stalled ? "warn" : undefined}
        {...basisProps}
      />
    );
  }
  // Everything qualifying this figure travels WITH it. §4.2 forbids a
  // cross-currency sum without an explicit conversion source and as-of date,
  // and forbids a total that silently covers only part of the pipeline — so a
  // partial total names its share, and a converted one names the oldest rate
  // date standing behind it.
  const partial = commercial.priced_count < commercial.open_count;
  const converted =
    commercial.converted_count > 0 && commercial.fx_as_of
      ? t("co.strip.convertedAsOf", {
          count: formatNumber(commercial.converted_count, locale),
          date: formatDate(commercial.fx_as_of, locale, recordZone),
        })
      : undefined;
  return (
    <StatCard
      {...door}
      label={t("co.strip.pipeline")}
      value={formatMoney(value, currency, locale)}
      tone={stalled ? "warn" : undefined}
      detail={join(
        partial
          ? t("co.strip.pricedPartly", {
              priced: formatNumber(commercial.priced_count, locale),
              total: formatNumber(commercial.open_count, locale),
            })
          : t("co.strip.openDeals", {
              count: formatNumber(commercial.open_count, locale),
            }),
        converted,
        stalled,
      )}
      {...basisProps}
    />
  );
}

// One detail line from the parts that apply. A card has room for one, and
// dropping a part because another is present is how a qualification goes
// missing exactly when it matters.
function join(...parts: (string | undefined)[]): string {
  return parts.filter(Boolean).join(" · ");
}

// Health as a STATUS with its reason, never a 0-100 verdict (§4.2). The card
// below the fold decomposes it; this says which way it points and why.
//
// It reports the BALANCE of the exchange rather than its recency, because the
// daily brief already answers "whose move is it" — two readings saying "in
// conversation" in different words is one reading's worth of information taking
// two slots of five. A relationship where they write and we do not answer, and
// one where we write into silence, are both "in conversation" by recency and are
// opposite problems.
function HealthStat({
  health,
  locale,
  withheld,
  onOpen,
  t,
}: Readonly<{
  health?: Health;
  locale: Locale;
  withheld: boolean;
  onOpen?: () => void;
  t: ReturnType<typeof useT>;
}>) {
  // The door out of this reading, handed to every shape it takes: a
  // withheld reading and a priced one are the same reading, and only one
  // of them offering the tab would make the way out look like a property
  // of the figure.
  const door = { openLabel: t("co.strip.open.history"), onOpen };
  const dimension = health?.relationship;
  const basisProps = {
    basis: dimension ? (
      <FactList
        facts={[
          {
            key: "relationship",
            term: t(HEALTH_DIMENSION_LABEL.relationship),
            value: t(HEALTH_RATING_LABEL[dimension.rating]),
            note: dimension.reason,
          },
        ]}
      />
    ) : undefined,
  };
  if (!health) {
    // No health section at all. Withheld says so; anything else has simply not
    // been assessed. Neither is "they have never written" — that is a claim
    // about the account this read has no basis for.
    return (
      <StatCard
        {...door}
        label={t("co.strip.health")}
        value={t(withheld ? WITHHELD_READING : UNASSESSED_READING)}
      />
    );
  }
  const days = health.days_since_last_inbound;
  if (days == null) {
    return (
      <StatCard
        {...door}
        label={t("co.strip.health")}
        value={t("co.strip.noInboundEver")}
        tone="warn"
        {...basisProps}
      />
    );
  }
  if (days > HEALTH_QUIET_DAYS) {
    return (
      <StatCard
        {...door}
        label={t("co.strip.health")}
        value={t("co.strip.healthQuiet")}
        tone="warn"
        detail={t("co.health.sinceInbound", {
          days: formatNumber(days, locale),
        })}
        {...basisProps}
      />
    );
  }
  // A live relationship: say who is carrying it. Below a third of the
  // exchange coming from them is us talking to ourselves, whatever the dates
  // say; above two thirds they are asking more than we are answering.
  const share = health.reply_balance;
  if (share == null) {
    return (
      <StatCard
        {...door}
        label={t("co.strip.health")}
        value={t("co.strip.healthActive")}
        {...basisProps}
      />
    );
  }
  const percent = Math.round(share * 100);
  const oneSided = share < 0.34 || share > 0.66;
  return (
    <StatCard
      {...door}
      label={t("co.strip.health")}
      value={
        oneSided ? t("co.strip.healthOneSided") : t("co.strip.healthBalanced")
      }
      tone={oneSided ? "warn" : undefined}
      detail={t("co.strip.replyShare", {
        percent: formatNumber(percent, locale),
      })}
      {...basisProps}
    />
  );
}

// The threshold that separates a live conversation from a quiet one. It names
// a number the strip states rather than one the reader must infer from a date,
// and it is deliberately the same span the dormant engagement state uses.
const HEALTH_QUIET_DAYS = 30;

export type SuggestionAction = NonNullable<Suggestion["action"]>;

const SUGGESTION_ACTION_LABELS: Record<SuggestionAction["kind"], MessageKey> = {
  draft_reply: "co.suggest.act.draftReply",
  open_deal: "co.suggest.act.openDeal",
  add_task: "co.suggest.act.addTask",
};

// SuggestionActionButton exists so the action is narrowed ONCE, at the call
// site, rather than re-narrowed inside a callback where TypeScript has already
// lost it.

/**
 * The names this page already holds, for citations the server could not name.
 *
 * The writer names a record when it had the name at hand and leaves it out
 * otherwise; nothing invents one. But an account's own 360 is HOLDING its
 * people and its deals, and printing "contact" beside a reason while the
 * roster three sections down says the person's name is the page failing to
 * read itself. Only records this view actually carries — anything else answers
 * undefined and falls back to the kind.
 */
export function recordNamesIn(view?: Organization360) {
  const names = new Map<string, string>();
  for (const person of view?.people?.data ?? []) {
    names.set(`person:${person.person_id}`, person.full_name);
  }
  for (const deal of view?.deals?.data ?? []) {
    names.set(`deal:${deal.deal_id}`, deal.name);
  }
  const org = view?.organization;
  if (org) {
    names.set(`organization:${org.id}`, org.display_name);
  }
  return (entityType: string, entityId: string) =>
    names.get(`${entityType}:${entityId}`);
}

function SuggestionActionButton({
  action,
  onPerform,
}: Readonly<{
  action: SuggestionAction;
  onPerform: (action: SuggestionAction) => void;
}>) {
  const t = useT();
  // Only the draft is the agent's own work. Opening a deal and adding a task
  // are things the reader does, and painting them indigo would spend the one
  // mark that means "a machine wrote this" on two clicks where nothing did.
  const byMargince = action.kind === "draft_reply";
  return (
    <Button
      small
      variant={byMargince ? "ai" : "primary"}
      onClick={() => onPerform(action)}
    >
      {byMargince && <Sparkles aria-hidden="true" />}
      {t(SUGGESTION_ACTION_LABELS[action.kind])}
    </Button>
  );
}

// nextCommitmentLine is the daily brief's own footer reading: what is owed
// and how soon. It is not a suggestion — nobody proposed it, the open tasks
// section simply has one — so it sits in the footer rather than as a row.
// Exported so the brief (companytoday.tsx) reads the same truncation-honesty
// logic rather than a second copy of it.
export function nextCommitmentLine(
  view: Organization360 | undefined,
  locale: Locale,
  t: ReturnType<typeof useT>,
): { headline: string; overdue: boolean } | undefined {
  const steps = view?.next_steps?.data ?? [];
  const step = steps[0];
  if (!step) {
    return undefined;
  }
  // The section is a page of 25 with `has_more` beside it, so past the cap
  // the count is a claim about the PAGE. "12 overdue" on an account with 40
  // is the kind of small wrong figure a rep plans against.
  const truncated = view?.next_steps?.page?.has_more === true;
  const overdueCount = steps.filter((each) => each.overdue).length;
  const count = overdueCount > 0 ? overdueCount : steps.length;
  const key = overdueCount > 0 ? "overdue" : "open";
  return {
    headline: truncated
      ? t(`co.suggest.commitment.${key}AtLeast`, {
          count: formatNumber(count, locale),
        })
      : t(`co.suggest.commitment.${key}Count`, {
          count: formatNumber(count, locale),
        }),
    overdue: overdueCount > 0,
  };
}

// useSuggestionsBody is the advice section's data and rows, split out of the
// Panel that used to own it: the daily brief now carries this chrome, so the
// dismiss mutation and the "move" rows live here where both that panel and
// the standalone `SuggestionsSection` (still used on its own in tests) can
// reach them without a second, drifting copy. Exported so companytoday.tsx
// composes the same rows rather than reimplementing them.
export function useSuggestionsBody({
  orgId,
  view,
  onOpenRecord,
  onPerform,
}: Readonly<{
  orgId: string;
  view?: Organization360;
  onOpenRecord?: (entityType: string, entityId: string) => void;
  // Performing the advice is the page's job, not this section's: the
  // composer, the deal and the task form all live above it.
  onPerform?: (action: SuggestionAction) => void;
}>): {
  // Whether the section has rows worth showing. A withheld, empty or
  // unavailable suggestion block carries none — advice is additive, and
  // "no advice" or "we cannot advise you" are not things a rep acts on.
  ready: boolean;
  rows: ReactNode;
  // How many rows `rows` draws. A caller that wants to count them beside its
  // own title cannot count a ReactNode, and a caller that recomputed the
  // number from the same view would be a second answer free to disagree with
  // the one on screen.
  count: number;
  // The truncation count and a failed dismissal, additive on top of whatever
  // else the caller's own footer carries.
  footer?: ReactNode;
} {
  const { locale } = useLocale();
  const t = useT();
  const recordZone = useRecordZone();
  const client = useQueryClient();
  const dismiss = useMutation({
    mutationFn: async (fingerprint: string) => {
      const { error } = await api.POST(
        "/organizations/{id}/suggestions/dismiss",
        { params: { path: { id: orgId } }, body: { fingerprint } },
      );
      if (error) {
        throwProblem(error);
      }
    },
    // The 360 is the only thing that knows which suggestions survive, so the
    // row goes when the re-read says it does. Hiding it locally on click would
    // hide it even when the dismissal never reached the server.
    onSuccess: () =>
      client.invalidateQueries({ queryKey: ["organization360", orgId] }),
  });

  const suggestions: Suggestion[] = view?.suggestions ?? [];
  const nameOf = recordNamesIn(view);
  const dropped = view?.suggestions_dropped;
  const state = sectionState(
    view,
    "suggestions",
    Boolean(view?.suggestions),
    suggestions.length,
  );
  if (state !== "ready") {
    return { ready: false, rows: null, count: 0 };
  }
  const footer =
    (dropped !== undefined && dropped > 0) || dismiss.isError ? (
      <>
        {/* A truncated list with no count reads as "that is everything".
            Absent means the section was never computed, which this card
            does not render at all. */}
        {dropped !== undefined && dropped > 0 && (
          <p className="co-row-meta">
            {t("co.suggest.more", { count: formatNumber(dropped, locale) })}
          </p>
        )}
        {/* The row staying put with no word reads as a click that missed,
            and the rep clicks again. */}
        {dismiss.isError && (
          <p className="surfacestate-withheld">
            {t("co.suggest.dismissFailed")}
            {` ${problemMessageOf(dismiss.error, t)}`}
          </p>
        )}
      </>
    ) : undefined;
  const rows = suggestions.map((suggestion) => (
    <FoundMove
      key={suggestion.fingerprint}
      // The day the reading behind the row is dated. Never a deadline the
      // system chose.
      when={
        suggestion.due_at
          ? formatDate(suggestion.due_at, locale, recordZone)
          : undefined
      }
      // The ASK: what the rule wants done. Falls back to the kind only when the
      // rule named no title of its own.
      title={suggestion.title ?? t(`co.suggest.kind.${suggestion.kind}`)}
      // The WHY, and behind it the records the rule fired on.
      why={suggestion.reason}
      basis={
        <Citations
          evidence={suggestion.evidence}
          nameOf={nameOf}
          onOpenRecord={onOpenRecord}
        />
      }
      // What performing the advice means, named by the server. A rule that
      // could not name one carries null and this renders nothing rather than
      // a control that does nothing.
      action={
        suggestion.action && onPerform ? (
          <SuggestionActionButton
            action={suggestion.action}
            onPerform={onPerform}
          />
        ) : undefined
      }
      // Only the row in flight is disabled: one dismissal must not freeze the
      // rep's other choices.
      defer={{
        onDefer: () => dismiss.mutate(suggestion.fingerprint),
        pending:
          dismiss.isPending && dismiss.variables === suggestion.fingerprint,
      }}
    />
  ));
  return { ready: true, rows, count: suggestions.length, footer };
}

/**
 * SuggestionsSection is the advice rows on their own, in their own Panel —
 * used standalone where nothing else on the page carries this chrome (the
 * stories file, and the suites that exercise the rows without the rest of
 * the daily brief). The live record page mounts the merged brief instead
 * (`TodayOnThisAccount`, companytoday.tsx), which composes the same rows
 * body via `useSuggestionsBody` alongside its own context band.
 */
export function SuggestionsSection({
  orgId,
  view,
  onOpenRecord,
  onPerform,
  onOpenTasks,
}: Readonly<{
  orgId: string;
  view?: Organization360;
  onOpenRecord?: (entityType: string, entityId: string) => void;
  onPerform?: (action: SuggestionAction) => void;
  // Where the footer's commitment reading leads. Absent for a caller with no
  // Tasks tab of its own (the stories file).
  onOpenTasks?: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const body = useSuggestionsBody({ orgId, view, onOpenRecord, onPerform });
  if (!body.ready) {
    return null;
  }
  const commitment = nextCommitmentLine(view, locale, t);
  const footer =
    commitment || onOpenTasks || body.footer ? (
      <>
        {commitment && (
          <Badge tone={commitment.overdue ? "warn" : undefined}>
            {commitment.headline}
          </Badge>
        )}
        {onOpenTasks && (
          <Button small variant="ghost" onClick={onOpenTasks}>
            {t("co.suggest.viewTasks")}
          </Button>
        )}
        {body.footer}
      </>
    ) : undefined;
  return (
    <Panel
      title={t("co.suggest.title")}
      footer={footer}
      tone="accent"
      className="co-lead"
    >
      {body.rows}
    </Panel>
  );
}
