import { useMemo } from "react";
import type { components } from "../api/schema";
import { useUrlParams } from "../app/urlstate";
import type { DecisionDeckItem } from "../design-system/decisiondeck";
import { PageZones } from "../design-system/pagezones";
import type { SectionState } from "../design-system/surfacestate";
import { useNow } from "../format/now";
import { useT } from "../i18n";
import { useDecisionSink } from "./approvalrow";
import { usePendingApprovals } from "./approvals.queries";
import { ChangedSinceBrief } from "./brief.changed";
import { BriefDials } from "./brief.dials";
import { BriefFeed } from "./brief.feed";
import { PlanSection } from "./brief.plan";
import { TeamWeeklyPanel } from "./brief.teamweekly";
import { addressFrom, type BriefAddress, paramsFor } from "./brief.view";
import { BriefCoverage } from "./briefcoverage";
import { useMe } from "./common";
import { DecisionsSection } from "./home.decisions";
import { HomeGlance } from "./home.glance";
import { quietDeals, useHomeDeals, useWeeklyReview } from "./home.queries";
import { OvernightPanel, PositionPanel, WatchPanel } from "./home.rail";
import { HomeReadingsStrip } from "./home.readings";
import { PromisesPanel, SchedulePanel } from "./home.schedule";
import { HomeTeamBoard } from "./home.teamboard";
import { WeeklySection } from "./home.weekly";
import { useWorklist, type Worklist } from "./worklist.queries";
import "./home.css";

// Home — the morning handover.
//
// The night shift worked while the reader slept. This page is where it hands
// over: what it could not decide without them (and that expires), what it thinks
// the day is for (a ranking), and the context both are read against.
//
// Three things about the shape are deliberate and load-bearing:
//
//   1. The DECK STAGES, and the commit is a separate, explicit act. A committed
//      decision cannot be undone — `approvals/service.go` says so in as many
//      words, and rejecting is not an undo either — so the tray IS the undo a
//      swipe would otherwise not have.
//   2. The page's ORDER follows the day. While decisions are waiting they lead,
//      because they are the only thing here with a deadline. Once the deck is
//      clear the ranked queue leads, because the question stops being "what
//      needs me" and becomes "what do I do first".
//   3. Every read is gated on its OWN state. There is no combined "my day"
//      endpoint, and five independent reads mean a transient failure in one
//      never blanks the other four.

type Approval = components["schemas"]["Approval"];

/** The first word of a display name, which is what a greeting uses. */
function firstNameOf(displayName: string | undefined): string | null {
  const first = (displayName ?? "").trim().split(/\s+/)[0];
  return first === undefined || first === "" ? null : first;
}

/**
 * The pending queue as the deck's items: one entry per proposal, except where a
 * `bundle_id` groups several into one act — which the API decides as a unit, so
 * a reader answering it card by card would be answering a question that no
 * longer exists after the first one. A bundle of one is emitted as a single: a
 * group that hides exactly one card is a click for nothing.
 *
 * Wire order is kept. The deck reorders nothing, and a queue whose order the
 * page invents is a queue nobody can predict.
 *
 * Exported for the parts catalog (`home.parts.stories.tsx`), which documents the
 * decisions section on its own: a story that grouped the fixtures by hand would
 * be a second answer to what a bundle is.
 */
export function deckItems(approvals: readonly Approval[]): DecisionDeckItem[] {
  const seen = new Set<string>();
  const out: DecisionDeckItem[] = [];
  for (const approval of approvals) {
    const bundleId = approval.bundle_id;
    if (!bundleId) {
      out.push({ kind: "single", id: approval.id, approval });
      continue;
    }
    if (seen.has(bundleId)) {
      continue;
    }
    seen.add(bundleId);
    const members = approvals.filter((other) => other.bundle_id === bundleId);
    out.push(
      members.length === 1
        ? { kind: "single", id: members[0].id, approval: members[0] }
        : { kind: "bundle", id: bundleId, bundleId, members },
    );
  }
  return out;
}

/**
 * What one read says about itself, in the state vocabulary every section draws
 * from. Three of Home's five reads answer the same question, and answering it
 * three times inline is how a page ends up drawing a failure as an empty list on
 * two of them and honestly on the third.
 */
function readState(
  query: Readonly<{ isError: boolean; isPending: boolean }>,
): SectionState {
  if (query.isError) {
    return "failed";
  }
  return query.isPending ? "loading" : "ready";
}

/**
 * The work column, in the order the day sets: a deadline leads until there is no
 * deadline left, and then the plan does.
 */
function HomeWork({
  items,
  nowMs,
  deckState,
  teamOffered,
  day,
  dayState,
  address,
  onAlreadyDecided,
}: Readonly<{
  items: readonly DecisionDeckItem[];
  nowMs: number;
  deckState: SectionState;
  // Whether the reader's scope reaches a team, off the worklist read the page
  // already makes. The same gate the team BOARD uses, so one tier decides both.
  teamOffered: boolean;
  // The ONE ranked order, already read by the page for its coverage line. The
  // feed draws a prefix of it, from the same query key — so the Brief and the
  // Worklist cannot disagree about what is waiting.
  day: Worklist | undefined;
  dayState: SectionState;
  /** Which Brief the reader asked for, from the address. */
  address: BriefAddress;
  onAlreadyDecided: () => void;
}>) {
  const decisions = (
    <DecisionsSection
      key="decisions"
      items={items}
      nowMs={nowMs}
      state={deckState}
      onAlreadyDecided={onAlreadyDecided}
    />
  );
  // A KEYED array, not two fragments. Positional children of a fragment are
  // matched by slot, so swapping the order unmounted the deck and mounted a
  // fresh one — and everything it holds locally went with it: the staging tray,
  // the reader's Deck/List choice, and the tally behind the cleared plate. Which
  // meant the one state the flip exists to reveal, "deck clear, N sent", could
  // never be reached by clearing the deck. Keys make React reorder instead.
  // Last, always. The weekly is a retrospective — what a rep reads once, on
  // Monday, after the work that is waiting on them today. Ordering it above
  // either of those would put last week ahead of this morning.
  const lastWeek = <WeeklySection key="weekly" />;
  // The team's frozen week, on the same tier the team BOARD is offered on:
  // read off scope_options so the control and the refusal cannot disagree.
  const teamWeek = <TeamWeeklyPanel key="team-weekly" offered={teamOffered} />;
  // The plan goes UNDER the retrospective, never above it. A rep decides what
  // next week holds by reading what this one did, so the frozen past leads and
  // the live future follows.
  const nextWeek = <PlanSection key="plan" />;
  // THE MORNING, AS ONE FEED. It replaces the two panels that stood here —
  // "Do next" (the head of the ranked worklist) and "Focus" (the overnight
  // opportunity queue) — which drew two orders over one morning and made the
  // rep reconcile them. The server ranks everything once now, with the night's
  // composite as a tie-break inside a level, so the page draws that order and
  // adds nothing to it.
  const feed = <BriefFeed key="feed" day={day} state={dayState} />;
  const board = <HomeTeamBoard key="board" offered={teamOffered} />;

  // ONE VIEW AT A TIME, and every combination the dials offer has a surface
  // behind it — decision 5 of the plan, which is the sequencing defect it
  // exists to prevent: a dial that resolves to a legacy screen, an empty state
  // standing in for an unbuilt one, or nothing at all.
  //
  // Morning · mine — the decisions waiting, what to do next, the opportunity
  //   queue. Weekly · mine — the week that closed, then the week being planned.
  // Morning · team — who on the team is carrying what, right now.
  // Weekly · team — the team's frozen week.
  //
  // The team views are reachable only when the reader's scope admits them
  // (`teamOffered`), which is also what draws the scope dial: the control and
  // the surface read one answer, so neither can offer what the other refuses.
  if (address.scope === "team") {
    return address.view === "weekly" ? [teamWeek] : [board];
  }
  if (address.view === "weekly") {
    return [lastWeek, nextWeek];
  }
  // The feed leads unless decisions are waiting, and a decision is a judgement
  // somebody is blocked on rather than work to do — which is why it is the one
  // thing that goes above the day's own order rather than into it.
  return items.length > 0 ? [decisions, feed] : [feed, decisions];
}

export function HomeScreen() {
  const t = useT();
  // One clock for the whole page, ticking by the minute: the deck's countdowns
  // read in days and hours, so a per-second tick would re-render every card on
  // the page for a digit none of them draw.
  const nowMs = useNow(60_000);
  const me = useMe();

  // Approving can 409 already-decided, and that note must outlive the deck's
  // re-render on invalidation, so Home uses the same shared sink the Decisions
  // screen does.
  const { onAlreadyDecided, decidedNote } = useDecisionSink();
  const approvalsQuery = usePendingApprovals();
  const dealsQuery = useHomeDeals();
  // The ONE ranked order, read here for its coverage. Home has always been
  // deal-only; what it could not say is which sources it never saw, and that
  // answer lives on the worklist read rather than on any of the five deal
  // reads beside it. Same query key as the Worklist screen, so the two cannot
  // disagree about what was read.
  const worklistPages = useWorklist("mine", "all");
  // Home reads the day's FIGURES — coverage, readings, scope options — and
  // never its rows, so the first page is the whole of what it needs. Those
  // figures describe the assembled day rather than the page, so paging would
  // not change one of them.
  const worklistQuery = {
    ...worklistPages,
    data: worklistPages.data?.pages[0],
  };
  // Whether this reader's scope reaches a team, off the read the page already
  // makes. It decides BOTH which dials are drawn and which the address may
  // resolve to, so a control and a surface cannot disagree about it.
  const teamOffered =
    worklistQuery.data?.scope_options?.includes("team") ?? false;
  // The dials live in the ADDRESS, not in state — decision 2. The Brief is a
  // destination people return to and send each other, which is the case the
  // Worklist's "dials stay state" choice deliberately excluded.
  const [params, setParams] = useUrlParams();
  const address = addressFrom(params, teamOffered);
  // The closed week, for the weekly's opening sentence. The SAME query key
  // WeeklySection uses for the latest week, so the header band and the panel
  // below it read one answer rather than two that could differ — and the read
  // is served from cache, not made twice.
  const weeklyReview = useWeeklyReview();

  const approvals = approvalsQuery.data?.data ?? [];
  const items = useMemo(() => deckItems(approvals), [approvals]);
  const deals = dealsQuery.data?.rows ?? [];
  // Every count taken from this page is a floor: the read stops at one page,
  // and `more` is what keeps the glance and the watch panel from reporting a
  // bounded number as a total.
  const beyondPage = dealsQuery.data?.more ?? false;
  const quiet = quietDeals(deals);

  // `QueryLike` has no `isSuccess`: settled-and-answered is the absence of both
  // other states, and the difference matters here — a reading that is still in
  // flight must stay null rather than count an empty page as "nothing waiting".
  // The deck carries its own read state rather than the work column being gated
  // as a whole: gating the column would hide a healthy ranked queue behind a
  // failed decisions read, which is exactly the coupling five separate reads
  // exist to avoid.
  const deckState = readState(approvalsQuery);
  // ONE decision per deck item, not per proposal. An act's bundle is decided in
  // one call and the deck draws it as one card, so counting raw approvals here
  // made the same queue read as two different sizes on one page: the strip said
  // six while the deck drew four and its tray sent "4 decisions".

  return (
    <div className="wrap">
      <BriefDials
        address={address}
        offered={teamOffered}
        onChange={(next) => setParams(paramsFor(next))}
      />
      <HomeGlance
        view={address.view}
        day={worklistQuery.data}
        week={weeklyReview.data}
        firstName={firstNameOf(me.data?.user?.display_name)}
        now={new Date(nowMs)}
      />
      {/* Before the readings, because a strip of numbers a reader cannot trust
          is worse than one they can qualify — and in the MAIN column rather
          than the rail, though the rail is where the Brief plan drew it. This
          is a callout that appears only when a source was withheld, failed or
          stopped short, so it is a fact about the whole page rather than
          context beside it, and it qualifies the strip directly under it. In
          the rail it would be a warning about the queue, filed away from the
          queue. */}
      {worklistQuery.data && <BriefCoverage day={worklistQuery.data} />}
      {/* And what has happened since the night looked, under the coverage line
          and above the readings: both are facts about the page as a whole
          rather than about any one row, and a reader takes them before the
          figures they qualify. */}
      {worklistQuery.data && <ChangedSinceBrief day={worklistQuery.data} />}
      {/* The strip reads the SAME worklist answer the queue below it reads, so
          the five figures cannot disagree with the rows they summarise. */}
      {worklistQuery.data && <HomeReadingsStrip day={worklistQuery.data} />}
      {/* Screen-level so it survives the deck re-rendering under it. */}
      {decidedNote}
      <PageZones
        // The SHAPE moves with the aside, not just its contents. A grid track
        // does not collapse when its item is missing (pagezones.tsx says so in
        // as many words), so leaving this at "aside" would draw the weekly at
        // seventy per cent width with an empty third beside it — a column that
        // reads as a rail which failed to load.
        shape={address.view === "weekly" ? "single" : "aside"}
        mainClassName="home-main"
        main={
          <HomeWork
            items={items}
            nowMs={nowMs}
            deckState={deckState}
            // The optional chain reaches the FIELD, not just the payload: a
            // worklist answer that carried no scope_options crashed the whole
            // page, and a page that throws is a worse answer than one that
            // simply does not offer the team view.
            teamOffered={teamOffered}
            day={worklistQuery.data}
            dayState={readState(worklistQuery)}
            address={address}
            onAlreadyDecided={onAlreadyDecided}
          />
        }
        asideClassName="home-rail"
        asideLabel={t("home.rail")}
        // THE RAIL BELONGS TO THE VIEW IT IS BESIDE.
        //
        // Every panel below answers a question about TODAY: what the day is
        // booked with, what is owed now, what arrived overnight, which deals
        // have gone quiet. Beside the weekly they sat next to a week that had
        // closed, so a rep reading their retrospective was shown "Today's
        // schedule" against it and the page read as two screens overlaid.
        //
        // The work column has switched on the view since the dials shipped.
        // The rail did not, because it is drawn once outside that branch —
        // and nothing asserted it, so the mismatch was invisible.
        //
        // The weekly draws NO rail rather than a substitute one. Its three
        // planned panels each need something that is not there: the manager's
        // answer already renders on the commitment row it belongs to, and
        // "carried into this week" is a COUNT on the wire with no rows behind
        // it. A rail invented from what is available would be a region that
        // says less than the column beside it.
        aside={
          address.view === "weekly" ? undefined : (
            <>
              {/* The day's own shape leads the rail: what it is booked with,
                  then what is owed. Both are cuts of the SAME worklist answer
                  the work column is drawn from, so the rail cannot name a
                  meeting the queue has already dropped. */}
              <SchedulePanel
                day={worklistQuery.data}
                state={readState(worklistQuery)}
              />
              <PromisesPanel
                day={worklistQuery.data}
                state={readState(worklistQuery)}
              />
              <OvernightPanel />
              <PositionPanel />
              <section id="home-watch">
                <WatchPanel
                  deals={quiet}
                  more={beyondPage}
                  state={readState(dealsQuery)}
                />
              </section>
            </>
          )
        }
      />
    </div>
  );
}
