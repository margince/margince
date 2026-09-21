import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Sparkles } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import { Badge, Button, Skeleton, StatCard } from "../design-system/atoms";
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
  formatMoneyCompact,
  formatMoneyOrAbsent,
  formatNumber,
  formatTimeOfDay,
} from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  problemCodeOf,
  problemMessageOf,
  throwProblem,
  useFinanceSummary,
} from "./common";
import type { CompanyTab } from "./companytab";
import { dealsFilteredBy } from "./dealsaddress";
import "./company360.css";
import { FactList } from "../design-system/factlist";
import {
  HEALTH_DIMENSION_LABEL,
  HEALTH_RATING_LABEL,
  LIFECYCLE_LABELS,
} from "./companylookups";
import { EntityRef } from "./entityref";
import {
  EvidenceSources,
  FoundMove,
  fromCitations,
  SentenceList,
  WrittenBy,
} from "./record360";
import { TaskCompleteCheck, type useTaskUpdate } from "./taskactions";

// The company view's data layer and its right-rail cards.
//
// One read (GET /companies/{id}/360) serves the whole page, and its
// `sections_omitted` is the thing that makes the page honest: a section the
// caller's role cannot read is ABSENT from the payload and named there, so
// every card below can say "hidden from you" instead of drawing an empty
// list that reads as "there is none".

type Company360 = components["schemas"]["Company360"];
type Deal360 = components["schemas"]["Company360Deal"];
type NextStep = components["schemas"]["Company360NextStep"];
/**
 * useCompany360 reads the whole company page in one round trip.
 *
 * `enabled` exists for callers that are not the page: chrome mounted on every
 * screen has to hold the hook unconditionally and ask for nothing when there is
 * no record under it — an empty id is a 422, not an empty answer.
 */
export function useCompany360(id: string, enabled = true) {
  return useQuery<Company360>({
    queryKey: ["company360", id],
    enabled: enabled && id !== "",
    queryFn: async () => {
      const { data, error } = await api.GET("/companies/{id}/360", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

// VIEW_ACK_DWELL_MS is how long the account must stay open before the visit
// counts. Opening a record and bouncing straight back out is not reading it,
// and an ack from that would mark unread activity as seen.
const VIEW_ACK_DWELL_MS = 5_000;

/**
 * useAcknowledgeCompanyView advances THIS reader's "last seen" baseline
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
export function useAcknowledgeCompanyView(id: string, visited: boolean) {
  const ack = useMutation({
    mutationFn: async (companyId: string) => {
      const { error } = await api.POST("/companies/{id}/view-ack", {
        params: { path: { id: companyId } },
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

/** DealsCard lists the open deals plus the two lifetime figures. */
export function DealsCard({
  view,
  actions,
  extra,
  loading = false,
}: Readonly<{
  view?: Company360;
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
          <p className="co-row-meta t-caption">
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
              href={dealsFilteredBy("company_id", view.company.id, {
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
          <SurfaceState
            state={state}
            emptyLabel={t("co.deals.empty")}
            loadingLabel={t("co.deals.title")}
          >
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
      <span className="co-row-meta t-caption">
        <span>{deal.stage_name ?? t("co.deals.noStage")}</span>
        {deal.amount?.amount_minor != null && (
          <span className="t-num">
            {formatMoneyOrAbsent(
              deal.amount.amount_minor,
              deal.amount.currency,
              locale,
            )}
          </span>
        )}
        {deal.stalled && <Badge tone="warning">{t("deal.stalledBadge")}</Badge>}
      </span>
    </PanelRow>
  );
}

/**
 * CommercialPanel is the overview's own reading of the deals: the two
 * lifetime figures the deals section actually carries, then the open deals
 * themselves. It is deliberately not DealsCard reused wholesale — the Deals
 * tab keeps that card in full, and this is the shorter reading a rep gets
 * without leaving Overview.
 *
 * No open-deal total is drawn: nothing in Company360 sums the open
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
  view?: Company360;
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
  // reason it needs a contact, and repeating them underneath would show each
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
  // the cap this reads as every open deal unless it says otherwise.
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
            href={dealsFilteredBy("company_id", view.company.id, {
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
            <SurfaceState
              state={state}
              emptyLabel={t("co.deals.empty")}
              loadingLabel={t("co.deals.title")}
            >
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
              <Button variant="ghost" onClick={onAllDeals}>
                {t("co.commercial.allDeals")}
              </Button>
            )}
          </>
        ) : undefined
      }
    >
      {/* Before the open deals, and before the panel's own deals footer: what
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
                <span>{deal.name}</span>
                {deal.expected_close_date && (
                  <span className="t-sub">
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
              <span className="co-row-meta t-caption">
                {deal.stage_name && <Badge>{deal.stage_name}</Badge>}
                {deal.amount?.amount_minor != null && (
                  <span className="t-num">
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
          <SurfaceState
            state={state}
            emptyLabel={t("co.deals.empty")}
            loadingLabel={t("co.deals.title")}
          >
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
  proposed,
  renderAction,
  onOpenTask,
  update,
}: Readonly<{
  view: Company360;
  // The steps nobody has accepted yet, drawn above the ones on the list. They
  // are the caller's rows because only a caller that can WRITE one should offer
  // it — a read-only account draws none — and they lead rather than follow: a
  // reader whose account has no open task at all would otherwise meet the
  // empty-list sentence and stop reading before the recommendation.
  proposed?: ReactNode;
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
  const recordZone = useRecordZone();
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
      {proposed}
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
                version={step.version}
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
              <span className="co-row-meta t-caption">
                {step.overdue && (
                  <Badge tone="danger">{t("co.next.overdue")}</Badge>
                )}
                {!step.overdue && step.due_at && (
                  <span>
                    {t("co.next.due", {
                      // The record's own clock, like the timeline below it. A
                      // deadline is a promise colleagues read back, so
                      // `dueInstant` mints the picked day's end in this same
                      // zone and it is rendered in it. Reading that last second
                      // on the browser's clock instead is what showed an
                      // approved 9 September as a next step due the 10th.
                      when: formatDate(step.due_at, locale, recordZone),
                    })}
                  </span>
                )}
                {!step.due_at && <span>{t("co.next.undated")}</span>}
                {step.linked_deal_id && (
                  <EntityRef kind="deal" id={step.linked_deal_id} />
                )}
                {step.linked_contact_id && (
                  <EntityRef kind="contact" id={step.linked_contact_id} />
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

type Question = components["schemas"]["CompanyQuestion"];
type Suggestion = components["schemas"]["Company360Suggestion"];
// The body an `add_task` suggestion carries, and the body POST /tasks takes —
// one type, so a step the server prepared cannot be posted as something else.
type CreateTaskRequest = components["schemas"]["CreateTaskRequest"];
type Answer = components["schemas"]["CompanyAnswer"];
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
 * The questions are BUTTONS, not a text box — indigo, because the agent
 * answers them, and quiet because no one of them is the move. Each names the
 * records its answer is written from, so every sentence carries a citation the
 * reader can open; a text box answering from a subset would look the same.
 */
export function AskSection({
  companyId,
  onOpenRecord,
  onOpenEmail,
  projects,
}: Readonly<{
  companyId: string;
  onOpenRecord?: (entityType: string, entityId: string) => void;
  // Opens a cited message in the page's email drawer; see `Citations`.
  onOpenEmail?: (activityId: string) => void;
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
      const { data, error } = await api.POST("/companies/{id}/ask", {
        params: { path: { id: companyId } },
        body: { question, ...(project ? { project_id: project } : {}) },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

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
            variant="aiQuiet"
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
        /* The answer as its own plate rather than as three loose paragraphs
            under the buttons: a reply belongs in a shape that says where it
            starts and where it ends, and on the panel's tinted ground the
            white card is what makes the prose the thing being read. */
        <div className="co-ask-answer">
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
              onOpenEmail={onOpenEmail}
              // Gathered under the prose, not trailing each clause: an answer
              // is one reply to one question, and a chip after every sentence
              // breaks the reply into a list of filed facts.
              citations="collected"
            />
          )}
          <p className="co-ask-foot co-row-meta t-caption">
            <WrittenBy by={readable.generated_by} />
            <span>
              {t("co.brief.generatedAt", {
                when: formatDate(readable.generated_at, locale, recordZone),
              })}
            </span>
          </p>
        </div>
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
type Health = NonNullable<Company360["health"]>;
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
export type StateStrip = NonNullable<Company360["state_strip"]>;

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
  Record<NonNullable<StateStrip["engagement"]>["state"], "warning">
> = {
  waiting_on_them: "warning",
  dormant: "warning",
};

// A reading the caller's grants withheld, in the word every stat card in the
// product uses. `record.notShown` stays on the contact record's readings row
// and rail: retargeting it would restyle two surfaces nobody looked at here.
const WITHHELD_READING: MessageKey = "reading.restricted";

// A reading nobody has judged. It is NOT the withheld word — "you may not see
// this" and "there is no verdict yet" are opposite facts about who is missing
// what, and confusing them sends the reader to ask for a grant that would show
// them nothing. Its own key rather than the lifecycle label it matches today.
const UNASSESSED_READING: MessageKey = "co.strip.notAssessed";

/**
 * StateStrip is the readings row under the tab strip: FIVE doors, always
 * five — open deals, invoiced, the relationship, the last contact, and what
 * is next — each a reading of the tab it opens. The account's standing is not
 * here: the verdict word and the three dimensions it is read from are the
 * 360's, directly under this row, so a reading is said once.
 *
 * EVERY SLOT ALWAYS DRAWS, and says honestly that it has no reading when it
 * has none. A slot that vanishes leaves the reader unable to tell WHICH
 * reading is missing, and only an empty state may claim there is none
 * (SurfaceState's rule). So the three absences are three different words:
 * withheld is a fact about the READER, unassessed about how much has been
 * judged, "no date" about the ACCOUNT. Inventing "never contacted" out of a
 * withheld engagement states a conclusion a rep acts on, from a permission.
 *
 * What it must never render is the harder half: no €0 when the figure is
 * unavailable, no cross-currency sum without its conversion source, nothing
 * called "revenue" that is only a count of open deals.
 *
 * Each slot binds its own label and the NARROW shape once, because both hold
 * for every branch that slot can take: the label must not move (a slot read
 * twice), and below the two-up width a `row` slot folds to a full-width line
 * (statstrip.css). The fold is the PLATE's — `.stat-strip:has(...)` — so every
 * card on the row carries it or the row draws a bordered box among a column of
 * borderless ones.
 */
export function StateStrip({
  companyId,
  view,
  onOpenTab,
}: Readonly<{
  companyId: string;
  view?: Company360;
  // The tab each reading is a reading OF. Optional, because a surface that
  // draws these outside the record page (the storybook, a mirror) has no tab
  // strip to send anybody to, and a door with nowhere behind it is not drawn.
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
  // The stage the money slot reads to know whether there is a window to read
  // at all, and to name itself under the word when there is not.
  const lifecycle = strip.account.lifecycle;
  // The contract pairs an absent optional section with its name in
  // `sections_omitted` (Company360), so the reason `health` did not arrive
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
        companyId={companyId}
        locale={locale}
        lifecycle={lifecycle}
        dimension={view?.health?.payment}
        onOpen={door("finance")}
        t={t}
      />
      {/* The relationship and the last contact both read off the account's
          exchanges: one says how balanced the talk has been, the other how
          long ago the last word fell. The outbound date rides along because
          silence with nothing sent and silence after we wrote are opposite
          problems, and the reading itself carries only the inbound side. */}
      <HealthStat
        health={view?.health}
        lastOutboundAt={view?.last_outbound_at ?? undefined}
        asOf={view?.as_of}
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
        onOpen={door("timeline")}
        t={t}
      />
    </StatStrip>
  );
}

// How long ago an instant fell, against the read's own `as_of` and never the
// reader's clock. `undefined` is TODAY, decided here and nowhere else: zero
// days is today, and a NEGATIVE span is skew between a timestamp and the read
// instant, which no slot may print.
function daysAgo(at: string, asOf: string): number | undefined {
  const days = calendarDaysBetween(new Date(at), new Date(asOf));
  return days > 0 ? days : undefined;
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
  view?: Company360;
  locale: Locale;
  recordZone: string;
  onOpen?: () => void;
  t: ReturnType<typeof useT>;
}>) {
  const slot = { label: t("co.strip.lastTouch"), narrow: "row" } as const;
  if (!view || omitted(view, "last_touch")) {
    return <StatCard onOpen={onOpen} {...slot} value={t(WITHHELD_READING)} />;
  }
  const inbound = view.last_inbound_at ?? undefined;
  const outbound = view.last_outbound_at ?? undefined;
  const theirs = Boolean(inbound && (!outbound || inbound > outbound));
  const last = theirs ? inbound : outbound;
  if (!last) {
    return (
      <StatCard
        onOpen={onOpen}
        {...slot}
        value={t("co.strip.lastTouch.never")}
      />
    );
  }
  const days = daysAgo(last, view.as_of);
  return (
    <StatCard
      onOpen={onOpen}
      {...slot}
      value={
        days === undefined
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
  view?: Company360;
  locale: Locale;
  recordZone: string;
  onOpen?: () => void;
  t: ReturnType<typeof useT>;
}>) {
  // This card reads next_meeting and nothing else, so it says so and its door
  // goes where a company's meetings are. It used to say "Next" over a meeting
  // date and open the TASK list — three nouns in one card, which let a company
  // with a due task and no meeting booked read as a contradiction with itself.
  //
  // History rather than a meetings tab because a company has none
  // (companytab.ts); the contact record, which does, sends the same card to
  // meetings (contactreadings.tsx), so one question is answered one way.
  //
  // That leaves tasks with no door here, and it is the honest trade: a card
  // that reads meetings cannot also be the route to the task list, and the tab
  // strip reaches tasks directly.
  const slot = { label: t("co.strip.nextMeeting"), narrow: "row" } as const;
  if (!view || omitted(view, "next_meeting")) {
    return <StatCard onOpen={onOpen} {...slot} value={t(WITHHELD_READING)} />;
  }
  const meeting = view.next_meeting;
  if (!meeting) {
    return (
      <StatCard onOpen={onOpen} {...slot} value={t("co.strip.next.none")} />
    );
  }
  return (
    <StatCard
      onOpen={onOpen}
      {...slot}
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

// The caveat on a figure that IS shown but is not current, and undefined when
// it needs none. Stale, error and syncing say DIFFERENT things about whether
// anything is broken, so they never fold into one.
function staleDetailKey(
  state?: components["schemas"]["FinanceSummaryState"],
): MessageKey | undefined {
  switch (state) {
    case "stale":
      // Beside a figure the caveat qualifies the FIGURE — the last good one,
      // which may have moved since; the empty slot says it of the connection.
      return "co.strip.fin.notCurrent";
    case "error":
      return "co.strip.fin.errorFigure";
    case "syncing":
      // The first pass has not finished, so what is shown may be partial.
      return "co.strip.fin.syncing";
    default:
      return undefined;
  }
}

// The slot when there is no figure: the word it stands on and the reason under
// it, decided TOGETHER because each reason puts itself in a different one of
// the two. A refusal and a read in flight are facts about the REQUEST and are
// the reading; a broken source explains itself underneath.
function financeAbsence({
  pending,
  withheld,
  failed,
  state,
  lifecycle,
}: Readonly<{
  pending: boolean;
  withheld: boolean;
  failed: boolean;
  state?: components["schemas"]["FinanceSummaryState"];
  lifecycle: StripLifecycle;
}>): Readonly<{ value: MessageKey; detail?: MessageKey }> {
  if (pending) {
    return { value: "co.strip.fin.loading" };
  }
  // Before any state is read: with no answer there is no state, and guessing
  // one from its absence is how a denial became setup advice.
  if (withheld) {
    return { value: WITHHELD_READING };
  }
  if (failed) {
    return { value: "co.strip.fin.error", detail: "co.strip.fin.errorWhy" };
  }
  if (state === "connected") {
    // A live, mapped source that produced no figure: nothing is broken and
    // nothing to set up — we have simply never billed them. The account's own
    // STAGE under the word keeps it apart from the one that never bought, and
    // is READ rather than assumed: a former customer comes down here too.
    return {
      value: "co.strip.fin.neverInvoiced",
      detail: LIFECYCLE_LABELS[lifecycle],
    };
  }
  return { value: "co.strip.fin.noFigure", detail: noFigureReason(state) };
}

// Why the source produced no figure. Naming the wrong state sends the reader
// to set up a connection they already have.
function noFigureReason(
  state?: components["schemas"]["FinanceSummaryState"],
): MessageKey {
  switch (state) {
    case "unmapped":
      return "co.strip.fin.unmapped";
    case "syncing":
      return "co.strip.fin.syncing";
    case "stale":
      return "co.strip.fin.staleFigure";
    case "error":
      return "co.strip.fin.errorFigure";
    default:
      // no_connection, and the read that never answered: both mean there is no
      // source, the one case the setup advice fits.
      return "co.strip.fin.noConnection";
  }
}

type StripLifecycle = NonNullable<
  Company360["state_strip"]
>["account"]["lifecycle"];
type Translate = ReturnType<typeof useT>;

// The stages whose page leads with a money figure. A former customer's does:
// the trailing year is a fact about invoices, not about where the account
// stands today, and three years of billing reading "Not invoiced" was the
// lifecycle answering a question about money. NARROWER than `hasFinance`
// (companyfinance.tsx) and not one invariant with it: that asks whether an
// account could EVER have been billed and says yes for an unknown or
// disqualified stage, so the Finance tab can hide no money.
const INVOICEABLE: ReadonlySet<StripLifecycle> = new Set<StripLifecycle>([
  "customer",
  "former_customer",
]);

/**
 * The customer row's ONE money slot: what this account has been invoiced over
 * the trailing year.
 *
 * One slot, not three. The strip is a GLANCE and the Finance tab
 * (companyfinance.tsx) holds the detail, which is why open balance and the
 * payment-habit median are not here: three of five slots on windows of one
 * figure buried the account's own standing behind the money.
 *
 * ONE label on every branch. A slot whose label moves is read twice — and "Not
 * invoiced" under a twelve-month label is still true, because nothing was
 * invoiced in any window.
 */
function MoneyStat({
  companyId,
  locale,
  lifecycle,
  dimension,
  onOpen,
  t,
}: Readonly<{
  companyId: string;
  locale: Locale;
  // Whether there is a window to read at all, and the word for it when not.
  lifecycle: StripLifecycle;
  // The payment health reading, shown as this card's basis so the verdict on
  // the health card can be checked against the money it was read from.
  dimension?: HealthDimension;
  // Handed to every shape this reading takes: a door on only one would make
  // the way out look like a property of the figure.
  onOpen?: () => void;
  t: ReturnType<typeof useT>;
}>) {
  // The SAME query the finance card and the payment health dimension run, so
  // every money reading on one page agrees and all but the first are free.
  const { data, isPending, isError, error } = useFinanceSummary(companyId);
  const slot = { label: t("co.strip.netInvoiced"), narrow: "row" } as const;
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
  // Never invoiced is a fact about the ACCOUNT, and it outranks every state
  // the finance connection could be in: a prospect on an installation with no
  // accounting must not be told to connect one, and a prospect on one that HAS
  // it must not read as though we had billed them and got nothing.
  if (!INVOICEABLE.has(lifecycle)) {
    return (
      <StatCard
        onOpen={onOpen}
        {...slot}
        value={t("co.strip.fin.neverInvoiced")}
        // Which stage, rather than "not a customer yet": every stage that
        // reaches here answers why there is nothing to invoice, and the record
        // page reads them all out of the one catalog.
        detail={t(LIFECYCLE_LABELS[lifecycle])}
      />
    );
  }
  // A refusal is not a failure and neither is a setup gap: a reader whose role
  // cannot see finance, told to "connect your accounting", is sent to fix a
  // permission on the one page that cannot fix it.
  const withheld = isError && problemCodeOf(error) === "permission_denied";
  const amount = data?.net_invoiced;
  const caveat = staleDetailKey(data?.state);
  // No figure is not €0, and the reasons there is none are not one reason.
  if (!amount || amount.amount_minor == null || !amount.currency) {
    const absence = financeAbsence({
      pending: isPending,
      withheld,
      failed: isError && !withheld,
      state: data?.state,
      lifecycle,
    });
    return (
      <StatCard
        onOpen={onOpen}
        {...slot}
        value={t(absence.value)}
        detail={absence.detail ? t(absence.detail) : undefined}
        basis={basis}
      />
    );
  }
  return (
    <StatCard
      onOpen={onOpen}
      {...slot}
      value={formatMoneyCompact(amount.amount_minor, amount.currency, locale)}
      // One line, in the order a reader needs it. The caveat first: a figure
      // that is not current is shown WITH it rather than withheld, and which
      // accounting system it came from matters less than whether it is current.
      // Then lifetime, the one comparison this page carries that a second money
      // slot would cost a row. The provider last, IN the line: beside the label
      // a badge stands this slot taller. Overdue and the open balance stay OUT
      // — the Finance tab renders both one tab away, so a copy here is a second
      // answer to one question, drifting the moment either changes.
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

// One money figure as a phrase for the detail line, or nothing. Both halves
// are required and neither absence has a substitute: a figure with no currency
// cannot be rendered, and a zero the server did not send would say the account
// owes us nothing when the truth is that nobody has told us.
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
  NonNullable<Company360["state_strip"]>["commercial"]
>;

// The open deals, labelled as exactly what they are: their summed value, never
// "potential" and never "revenue" (§4.2). Unpriced when nothing on the account
// carries a convertible figure — a €0 would claim deals worth nothing, where
// the truth is the page cannot price them.
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
  // on the health card can be checked against the deals it was read from.
  dimension?: HealthDimension;
  locale: Locale;
  recordZone: string;
  // Handed to every shape this reading takes: a door on only one would make
  // the way out look like a property of the figure.
  onOpen?: () => void;
  t: ReturnType<typeof useT>;
}>) {
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
  const slot = {
    basis,
    label: t("co.strip.pipeline"),
    narrow: "row",
  } as const;
  if (!commercial) {
    // A null `commercial` is the contract's way of saying the caller has no
    // deal grant, so this is the READER's boundary and not an account with
    // nothing running. "No open deals" here would be the business conclusion a
    // rep acts on, invented out of a permission.
    return <StatCard onOpen={onOpen} value={t(WITHHELD_READING)} {...slot} />;
  }
  // No open deals is not an unpriced one. Saying "no convertible amount" about
  // an account that has nothing open reports a data problem where the truth is
  // simply that nothing is running.
  if (commercial.open_count === 0) {
    return (
      <StatCard onOpen={onOpen} value={t("co.strip.noOpenDeals")} {...slot} />
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
    // count is a fact, the money is not, so the COUNT takes its place. The
    // unpriced note is never dropped for the stalled one — a reader told only
    // "1 stalled" cannot know nothing was priced.
    return (
      <StatCard
        onOpen={onOpen}
        value={formatNumber(commercial.open_count, locale)}
        detail={join(t("co.strip.unpriced"), stalled)}
        tone={stalled ? "warning" : undefined}
        {...slot}
      />
    );
  }
  // Everything qualifying this figure travels WITH it. §4.2 forbids a
  // cross-currency sum without an explicit conversion source and as-of date,
  // and forbids a total that silently covers only part of the open deals — so
  // a partial total names its share, and a converted one names the oldest rate
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
      onOpen={onOpen}
      value={formatMoneyCompact(value, currency, locale)}
      tone={stalled ? "warning" : undefined}
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
      {...slot}
    />
  );
}

// One detail line from the parts that apply. A card has room for one, and
// dropping a part because another is present is how a qualification goes
// missing exactly when it matters.
function join(...parts: (string | undefined)[]): string {
  return parts.filter(Boolean).join(" · ");
}

// How long nothing has come back, in the ONE spelling this row has: the
// unanswered slot and the quiet one make the same claim.
function noReply(days: number, locale: Locale, t: Translate): string {
  return t("co.strip.unansweredDetail", { days: formatNumber(days, locale) });
}

// They have never written, which is two different accounts and only one is bad
// news. With nothing sent either, nobody has approached them — a fact about how
// far the account has been worked, and a row that lit up for every untouched
// account would say the same of the ones being ignored. With something sent we
// are talking into silence.
//
// A letter posted TODAY is not yet unanswered news: the word stands, but there
// is no span to state and nothing to warn about until a day has passed.
function silenceReading(
  lastOutboundAt: string | undefined,
  asOf: string | undefined,
  locale: Locale,
  t: Translate,
): Readonly<{ value: string; detail?: string; tone?: "warning" }> {
  if (!lastOutboundAt || !asOf) {
    return { value: t("co.strip.noInboundEver") };
  }
  const days = daysAgo(lastOutboundAt, asOf);
  return days === undefined
    ? { value: t("co.strip.unanswered") }
    : {
        value: t("co.strip.unanswered"),
        tone: "warning",
        detail: noReply(days, locale, t),
      };
}

// Health as a STATUS with its reason, never a 0-100 verdict (§4.2). The card
// below the fold decomposes it; this says which way it points and why.
//
// A LIVE relationship is reported by the balance of the exchange rather than by
// its recency: one where they write and we do not answer, and one where we
// write into silence, are equally recent and opposite problems. A silent one
// has no balance worth stating — that nothing came back, and for how long.
function HealthStat({
  health,
  lastOutboundAt,
  asOf,
  locale,
  withheld,
  onOpen,
  t,
}: Readonly<{
  health?: Health;
  // The last word WE sent, which the reading itself does not carry.
  lastOutboundAt?: string;
  // The instant the 360 was read at, which every age on this row measures from.
  asOf?: string;
  locale: Locale;
  withheld: boolean;
  // Handed to every shape this reading takes: a door on only one would make
  // the way out look like a property of the figure.
  onOpen?: () => void;
  t: ReturnType<typeof useT>;
}>) {
  const dimension = health?.relationship;
  const slot = {
    label: t("co.strip.health"),
    narrow: "row",
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
  } as const;
  if (!health) {
    // No health section at all. Withheld says so; anything else has simply not
    // been assessed. Neither is "they have never written" — that is a claim
    // about the account this read has no basis for.
    return (
      <StatCard
        onOpen={onOpen}
        {...slot}
        value={t(withheld ? WITHHELD_READING : UNASSESSED_READING)}
      />
    );
  }
  const days = health.days_since_last_inbound;
  const share = health.reply_balance;
  if (days == null) {
    const silence = silenceReading(lastOutboundAt, asOf, locale, t);
    return (
      <StatCard
        onOpen={onOpen}
        value={silence.value}
        tone={silence.tone}
        detail={silence.detail}
        {...slot}
      />
    );
  }
  if (days > HEALTH_QUIET_DAYS) {
    // A share of the exchange here would describe a conversation that has
    // stopped; what a reader acts on is that nothing has come back.
    return (
      <StatCard
        onOpen={onOpen}
        value={t("co.strip.healthQuiet")}
        tone="warning"
        detail={noReply(days, locale, t)}
        {...slot}
      />
    );
  }
  // A live relationship: say who is carrying it. Below a third of the
  // exchange coming from them is us talking to ourselves, whatever the dates
  // say; above two thirds they are asking more than we are answering.
  if (share == null) {
    return (
      <StatCard onOpen={onOpen} value={t("co.strip.healthActive")} {...slot} />
    );
  }
  const oneSided = share < 0.34 || share > 0.66;
  return (
    <StatCard
      onOpen={onOpen}
      value={
        oneSided ? t("co.strip.healthOneSided") : t("co.strip.healthBalanced")
      }
      tone={oneSided ? "warning" : undefined}
      detail={t("co.strip.replyShare", {
        percent: formatNumber(Math.round(share * 100), locale),
      })}
      {...slot}
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
 * contacts and its deals, and printing "contact" beside a reason while the
 * roster three sections down says the contact's name is the page failing to
 * read itself. Only records this view actually carries — anything else answers
 * undefined and falls back to the kind.
 */
export function recordNamesIn(view?: Company360) {
  const names = new Map<string, string>();
  for (const contact of view?.contacts?.data ?? []) {
    names.set(`contact:${contact.contact_id}`, contact.full_name);
  }
  for (const deal of view?.deals?.data ?? []) {
    names.set(`deal:${deal.deal_id}`, deal.name);
  }
  const company = view?.company;
  if (company) {
    names.set(`company:${company.id}`, company.display_name);
  }
  return (entityType: string, entityId: string) =>
    names.get(`${entityType}:${entityId}`);
}

function SuggestionActionButton({
  action,
  pending,
  onPerform,
}: Readonly<{
  action: SuggestionAction;
  // Whether this row's own write is in flight. Only this row's: one task being
  // written must not freeze the reader's other choices.
  pending?: boolean;
  onPerform: (action: SuggestionAction) => void;
}>) {
  const t = useT();
  // The draft and the prepared step are the agent's own work — a rule wrote the
  // sentence, and pressing the button accepts it. Opening a deal is not: it is
  // navigation, and painting it indigo would spend the one mark that means "a
  // machine wrote this" on a click where nothing did.
  const byMargince = action.kind !== "open_deal";
  return (
    <Button
      variant={byMargince ? "ai" : "primary"}
      pending={pending}
      onClick={() => onPerform(action)}
    >
      {byMargince && <Sparkles aria-hidden="true" />}
      {t(SUGGESTION_ACTION_LABELS[action.kind])}
    </Button>
  );
}

/**
 * Whether a row can offer this action as a button.
 *
 * `add_task` is the section's OWN verb: the server prepared the body, so the
 * row writes it and needs no caller to open anything. It still refuses to draw
 * without that body — the contract promises one on every `add_task`, and a
 * button that posted nothing would be the control-that-does-nothing this whole
 * surface is careful not to draw.
 *
 * The other two open a surface that lives above this section — the composer,
 * the deal page — so without a caller to open it there is no button.
 */
function performable(
  action: SuggestionAction,
  onPerform?: (action: SuggestionAction) => void,
): boolean {
  return action.kind === "add_task" ? Boolean(action.task) : Boolean(onPerform);
}

// nextCommitmentLine is the daily brief's own footer reading: what is owed
// and how soon. It is not a suggestion — nobody proposed it, the open tasks
// section simply has one — so it sits in the footer rather than as a row.
// Exported so the brief (companytoday.tsx) reads the same truncation-honesty
// logic rather than a second copy of it.
export function nextCommitmentLine(
  view: Company360 | undefined,
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
  companyId,
  view,
  onOpenRecord,
  onOpenEmail,
  onPerform,
  advice,
  keep,
}: Readonly<{
  companyId: string;
  view?: Company360;
  onOpenRecord?: (entityType: string, entityId: string) => void;
  // Opens a cited message in the page's email drawer. A rule that fired on an
  // unanswered mail names that mail as its grounds, and the reader's next act
  // is to read it.
  onOpenEmail?: (activityId: string) => void;
  // Opening a surface is the page's job, not this section's: the composer and
  // the deal page both live above it. Writing the prepared step is this
  // section's own verb and needs no caller — see `performable`.
  onPerform?: (action: SuggestionAction) => void;
  // The merged advice — the rules' rows and the scan's findings as one list
  // — when the page holds a scan. It replaces the 360's own rows rather than
  // joining them: the server merged, deduplicated and capped once, and a
  // second list here would be a second answer to "what needs a contact".
  advice?: { findings: Suggestion[]; dropped: number };
  // Which advice this caller draws. Absent, all of it — the advice card. The
  // Tasks tab passes a predicate because it shows the steps and not the moves,
  // and a tab called Tasks listing a stalled deal would be the advice card
  // again under a heading that says otherwise.
  keep?: (suggestion: Suggestion) => boolean;
}>): {
  // Whether the section has rows worth showing. A withheld, empty or
  // unavailable suggestion block carries none — advice is additive, and
  // "no advice" or "we cannot advise you" are not things a rep acts on.
  ready: boolean;
  rows: ReactNode;
  // How many rows `rows` draws: one node carries several, so a caller weighing
  // what the list holds cannot count them for itself.
  count: number;
  // Whether any row this section DRAWS offers to answer a specific message.
  //
  // Computed from the post-filter list rather than from the raw advice: a
  // caller that skips its own generic "write to them" row must skip it exactly
  // when the reader can see the specific one, and a `keep` predicate that
  // filtered the reply out would otherwise leave the account offering neither.
  hasDraftReply: boolean;
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
      const { error } = await api.POST("/companies/{id}/suggestions/dismiss", {
        params: { path: { id: companyId } },
        body: { fingerprint },
      });
      if (error) {
        throwProblem(error);
      }
    },
    // The 360 is the only thing that knows which suggestions survive, so the
    // row goes when the re-read says it does. Hiding it locally on click would
    // hide it even when the dismissal never reached the server.
    onSuccess: () =>
      Promise.all([
        client.invalidateQueries({ queryKey: ["company360", companyId] }),
        // The scan serves the merged list, so it re-reads too — else a
        // dismissed model finding would stand until the next open.
        client.invalidateQueries({ queryKey: ["account-scan", companyId] }),
      ]),
  });
  const write = useMutation({
    // The body is the SERVER's, passed as a variable: the click posts the step
    // the row was drawn from, never one recomposed here from the row's words.
    mutationFn: async (body: CreateTaskRequest) => {
      const { error } = await api.POST("/tasks", { body });
      if (error) {
        throwProblem(error, t);
      }
    },
    // Three reads change. The 360 decides whether the advice still stands — it
    // fired on there being no open task, and there is one now — while the task
    // lists elsewhere gain the row this just wrote.
    onSuccess: () => {
      client.invalidateQueries({ queryKey: ["company360", companyId] });
      client.invalidateQueries({ queryKey: ["activities"] });
      client.invalidateQueries({ queryKey: ["tasks"] });
    },
  });
  // Performing the advice. `add_task` is this section's own verb and is written
  // here; the rest opens a surface only the caller can reach.
  const perform = (action: SuggestionAction) => {
    if (action.kind !== "add_task") {
      onPerform?.(action);
      return;
    }
    if (action.task) {
      write.mutate(action.task);
    }
  };

  const all: Suggestion[] = advice?.findings ?? view?.suggestions ?? [];
  const nameOf = recordNamesIn(view);
  const dropped = advice ? advice.dropped : view?.suggestions_dropped;
  const state = sectionState(
    view,
    "suggestions",
    Boolean(advice ?? view?.suggestions),
    all.length,
  );
  // The state is read off the WHOLE section, because that is what the grant and
  // the omission are about. `keep` then narrows what this caller draws, and a
  // caller left with nothing has nothing to show — the same answer an empty
  // section gives.
  const suggestions = keep ? all.filter(keep) : all;
  if (state !== "ready" || suggestions.length === 0) {
    return { ready: false, rows: null, count: 0, hasDraftReply: false };
  }
  // How many the cap dropped that THIS caller should report. The count
  // describes the whole list, so a narrowed caller reports none: "2 more" under
  // a filtered card counts advice that card was never going to draw.
  const notShown = keep ? 0 : (dropped ?? 0);
  const footer =
    notShown > 0 || dismiss.isError || write.isError ? (
      <>
        {/* A truncated list with no count reads as "that is everything".
            Absent means the section was never computed, which this card
            does not render at all. */}
        {notShown > 0 && (
          <p className="co-row-meta t-caption">
            {t("co.suggest.more", { count: formatNumber(notShown, locale) })}
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
        {/* Same rule for the write, and it matters more: a rep who thinks the
            step was written stops looking for it. */}
        {write.isError && (
          <p className="surfacestate-withheld">
            {t("co.suggest.addTaskFailed")}
            {` ${problemMessageOf(write.error, t)}`}
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
      // The WHY's grounds, as rows rather than chips: a suggestion says what to
      // do and its basis is the message the reader acts on, so the message gets
      // its subject, its sender and its preview instead of a kind word.
      basis={
        <EvidenceSources
          sources={fromCitations(suggestion.evidence)}
          nameOf={nameOf}
          onOpenRecord={onOpenRecord}
          onOpenEmail={onOpenEmail}
        />
      }
      // What performing the advice means, named by the server. A rule that
      // could not name one carries null and this renders nothing rather than
      // a control that does nothing.
      action={
        suggestion.action && performable(suggestion.action, onPerform) ? (
          <SuggestionActionButton
            action={suggestion.action}
            pending={
              write.isPending && write.variables === suggestion.action.task
            }
            onPerform={perform}
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
  return {
    ready: true,
    rows,
    count: suggestions.length,
    hasDraftReply: suggestions.some(
      (suggestion) => suggestion.action?.kind === "draft_reply",
    ),
    footer,
  };
}

// A suggestion that would BECOME a task. The action is what decides it, not the
// kind: a rule that names a prepared step is offering one whatever it fired on,
// and reading the kind here would leave the next such rule silently out.
function proposesAStep(suggestion: Suggestion): boolean {
  return suggestion.action?.kind === "add_task";
}

/**
 * ProposedNextSteps is the advice that would become a task, drawn where the
 * tasks are.
 *
 * A recommended step nobody has accepted yet is not on the list, so it used to
 * live only in the day's brief — and a reader who opened Tasks to find out what
 * happens next met an account with nothing scheduled and no way to learn that
 * something had been worked out for it two cards away.
 *
 * Only the advice that names a step. A stalled deal and an unanswered mail are
 * moves too, and neither is a task; drawing them here would make this a second
 * copy of the advice card under a heading that says otherwise.
 */
export function ProposedNextSteps({
  companyId,
  view,
  onOpenRecord,
  onOpenEmail,
}: Readonly<{
  companyId: string;
  view?: Company360;
  onOpenRecord?: (entityType: string, entityId: string) => void;
  onOpenEmail?: (activityId: string) => void;
}>) {
  const body = useSuggestionsBody({
    companyId,
    view,
    onOpenRecord,
    onOpenEmail,
    keep: proposesAStep,
  });
  if (!body.ready) {
    return null;
  }
  return (
    <>
      {body.rows}
      {body.footer && <PanelBody>{body.footer}</PanelBody>}
    </>
  );
}

/**
 * SuggestionsSection is the advice rows on their own, in their own Panel —
 * indigo, because a rule wrote every row under that head. Used standalone
 * where nothing else carries this chrome (the stories file, and the suites
 * that exercise the rows without the daily brief); the live record page mounts
 * the merged brief instead (`TodayOnThisAccount`, companytoday.tsx), which
 * composes the same body via `useSuggestionsBody` beside its context band.
 */
export function SuggestionsSection({
  companyId,
  view,
  onOpenRecord,
  onOpenEmail,
  onPerform,
  onOpenTasks,
}: Readonly<{
  companyId: string;
  view?: Company360;
  onOpenRecord?: (entityType: string, entityId: string) => void;
  onOpenEmail?: (activityId: string) => void;
  onPerform?: (action: SuggestionAction) => void;
  // Where the footer's commitment reading leads. Absent for a caller with no
  // Tasks tab of its own (the stories file).
  onOpenTasks?: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const body = useSuggestionsBody({
    companyId,
    view,
    onOpenRecord,
    onOpenEmail,
    onPerform,
  });
  if (!body.ready) {
    return null;
  }
  const commitment = nextCommitmentLine(view, locale, t);
  const footer =
    commitment || onOpenTasks || body.footer ? (
      <>
        {commitment && (
          <Badge tone={commitment.overdue ? "warning" : undefined}>
            {commitment.headline}
          </Badge>
        )}
        {onOpenTasks && (
          <Button variant="ghost" onClick={onOpenTasks}>
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
      tone="ai"
      titleAction={<Badge tone="ai">{t("co.assistant.aiTag")}</Badge>}
      className="co-lead"
    >
      {body.rows}
    </Panel>
  );
}
