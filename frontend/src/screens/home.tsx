import { useMemo } from "react";
import type { components } from "../api/schema";
import { navigate } from "../app/router";
import type { DecisionDeckItem } from "../design-system/decisiondeck";
import { PageZones } from "../design-system/pagezones";
import type { SectionState } from "../design-system/surfacestate";
import { calendarDay } from "../format/calendarday";
import { formatMoneyOrAbsent } from "../format/format";
import { useNow } from "../format/now";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { useMe } from "./common";
import { DecisionsSection } from "./home.decisions";
import {
  type GlanceBrief,
  type GlanceOvernight,
  HomeGlance,
} from "./home.glance";
import {
  type Deal,
  type MorningBrief,
  type MorningDigest,
  openDeals,
  quietDeals,
  useHomeDeals,
  useMorningBrief,
  useMorningDigest,
  usePipelineValue,
} from "./home.queries";
import { OvernightPanel, PositionPanel, WatchPanel } from "./home.rail";
import { HomeReadingsStrip } from "./home.readings";
import { TodaySection } from "./home.today";
import { useApprovalTokenSink, usePendingApprovals } from "./inbox";
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

/** How many pending decisions stop waiting today, in the reader's own day. */
function expiringToday(approvals: readonly Approval[], nowMs: number): number {
  const zone = viewerZone();
  const today = calendarDay(new Date(nowMs), zone);
  return approvals.filter((approval) => {
    const expires = approval.expires_at;
    return expires != null && calendarDay(new Date(expires), zone) === today;
  }).length;
}

/** The top-ranked item's deal, named only if the loaded page names it. */
function topDeal(
  brief: MorningBrief | null,
  deals: readonly Deal[],
): Deal | null {
  const first = brief?.items[0];
  if (!first) {
    return null;
  }
  return deals.find((deal) => deal.id === first.deal_id) ?? null;
}

/** The top composite as whole percent, or null when there is no run. */
function topPct(brief: MorningBrief | null): number | null {
  const first = brief?.items[0];
  return first ? Math.round(first.composite * 100) : null;
}

/**
 * Move the page to one of its own sections.
 *
 * The briefing's numerals are the way into the work, and on a page this tall the
 * way in has to actually move. `scrollIntoView` on the section rather than a
 * hash link: a hash would put a second history entry between the reader and
 * wherever they came from, for a move within one page.
 */
function goToSection(id: string): void {
  document.getElementById(id)?.scrollIntoView({
    // The one motion the reader asked for by pressing a control. Honoured as
    // instant under reduced motion, which the media query below reports.
    behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches
      ? "auto"
      : "smooth",
    block: "start",
  });
}

/**
 * The work column, in the order the day sets: a deadline leads until there is no
 * deadline left, and then the plan does.
 */
function HomeWork({
  items,
  nowMs,
  deckState,
  brief,
  deals,
  briefQueryReady,
  onApproved,
  onAlreadyDecided,
}: Readonly<{
  items: readonly DecisionDeckItem[];
  nowMs: number;
  deckState: SectionState;
  brief: MorningBrief | null;
  deals: readonly Deal[];
  briefQueryReady: boolean;
  onApproved: (approvalId: string, token: string) => void;
  onAlreadyDecided: () => void;
}>) {
  const decisions = (
    <DecisionsSection
      key="decisions"
      items={items}
      nowMs={nowMs}
      state={deckState}
      onApproved={onApproved}
      onAlreadyDecided={onAlreadyDecided}
    />
  );
  const today = (
    <TodaySection
      key="today"
      brief={brief}
      deals={deals}
      nowMs={nowMs}
      ready={briefQueryReady}
    />
  );
  // A KEYED array, not two fragments. Positional children of a fragment are
  // matched by slot, so swapping the order unmounted the deck and mounted a
  // fresh one — and everything it holds locally went with it: the staging tray,
  // the reader's Deck/List choice, and the tally behind the cleared plate. Which
  // meant the one state the flip exists to reveal, "deck clear, N sent", could
  // never be reached by clearing the deck. Keys make React reorder instead.
  return items.length > 0 ? [decisions, today] : [today, decisions];
}

/**
 * The top-ranked deal as the briefing states it — both halves or neither.
 *
 * A deal named without its figure reads as one nobody priced; a figure with no
 * name belongs to no deal at all.
 */
function briefGlance(
  answered: boolean,
  brief: MorningBrief | null,
  leader: Deal | null,
  locale: Parameters<typeof formatMoneyOrAbsent>[2],
): GlanceBrief | null {
  if (!answered || !brief) {
    return null;
  }
  return {
    ranked: brief.items.length,
    topDeal: leader?.name ?? null,
    topAmount: leader
      ? formatMoneyOrAbsent(leader.amount_minor, leader.currency, locale)
      : null,
  };
}

/** What the night shift did, once the digest has answered. */
function overnightGlance(
  answered: boolean,
  digest: MorningDigest | null,
): GlanceOvernight | null {
  if (!answered || !digest) {
    return null;
  }
  return {
    captured: digest.capture.messages_synced ?? 0,
    duplicates: digest.review.dedupe_open ?? 0,
  };
}

export function HomeScreen() {
  const t = useT();
  const { locale } = useLocale();
  // One clock for the whole page, ticking by the minute: the deck's countdowns
  // read in days and hours, so a per-second tick would re-render every card on
  // the page for a digit none of them draw.
  const nowMs = useNow(60_000);
  const me = useMe();

  // Approving mints an approval token and can 409 already-decided; both must
  // outlive the deck's re-render on invalidation, so Home uses the same shared
  // sink the Decisions screen does.
  const { onApproved, onAlreadyDecided, tokenModal, decidedNote } =
    useApprovalTokenSink();
  const approvalsQuery = usePendingApprovals();
  const briefQuery = useMorningBrief();
  const digestQuery = useMorningDigest();
  const dealsQuery = useHomeDeals();
  const pipelineQuery = usePipelineValue();

  const approvals = approvalsQuery.data?.data ?? [];
  const items = useMemo(() => deckItems(approvals), [approvals]);
  const deals = dealsQuery.data ?? [];
  const quiet = quietDeals(deals);
  const brief = briefQuery.data ?? null;
  const digest = digestQuery.data ?? null;
  const leader = topDeal(brief, deals);

  // `QueryLike` has no `isSuccess`: settled-and-answered is the absence of both
  // other states, and the difference matters here — a reading that is still in
  // flight must stay null rather than count an empty page as "nothing waiting".
  const approvalsReady = !approvalsQuery.isPending && !approvalsQuery.isError;
  // The deck carries its own read state rather than the work column being gated
  // as a whole: gating the column would hide a healthy ranked queue behind a
  // failed decisions read, which is exactly the coupling five separate reads
  // exist to avoid.
  const deckState: SectionState = approvalsQuery.isError
    ? "failed"
    : approvalsQuery.isPending
      ? "loading"
      : "ready";
  const decisionReadings = approvalsReady
    ? {
        pending: approvals.length,
        expiringToday: expiringToday(approvals, nowMs),
      }
    : null;
  // Both halves have to have answered: the count of open deals comes from the
  // deals page and the count of currencies from the report, and half a reading
  // is a tile that states one fact and implies another.
  const openReading =
    dealsQuery.isSuccess && pipelineQuery.isSuccess
      ? {
          deals: openDeals(deals).length,
          currencies: pipelineQuery.data.rows.length,
        }
      : null;
  const rankedReading = briefQuery.isSuccess
    ? { count: brief?.items.length ?? 0, topPct: topPct(brief) }
    : null;

  return (
    <div className="wrap">
      <HomeGlance
        firstName={firstNameOf(me.data?.user?.display_name)}
        now={new Date(nowMs)}
        decisions={decisionReadings}
        brief={briefGlance(briefQuery.isSuccess, brief, leader, locale)}
        overnight={overnightGlance(digestQuery.isSuccess, digest)}
        stalled={dealsQuery.isSuccess ? quiet.length : null}
        onGoToDecisions={() => goToSection("home-decisions")}
        onGoToToday={() => goToSection("home-today")}
        onGoToDuplicates={() => navigate({ screen: "dedupe" })}
        onGoToWatch={() => goToSection("home-watch")}
      />
      <HomeReadingsStrip
        decisions={decisionReadings}
        open={openReading}
        ranked={rankedReading}
        quiet={dealsQuery.isSuccess ? quiet.length : null}
      />
      {/* Screen-level so it survives the deck re-rendering under it. */}
      {decidedNote}
      <PageZones
        shape="aside"
        mainClassName="home-main"
        main={
          <HomeWork
            items={items}
            nowMs={nowMs}
            deckState={deckState}
            brief={brief}
            deals={deals}
            briefQueryReady={briefQuery.isSuccess}
            onApproved={onApproved}
            onAlreadyDecided={onAlreadyDecided}
          />
        }
        asideClassName="home-rail"
        asideLabel={t("home.rail")}
        aside={
          <>
            <OvernightPanel />
            <PositionPanel />
            <section id="home-watch">
              <WatchPanel deals={quiet} pending={dealsQuery.isPending} />
            </section>
          </>
        }
      />
      {tokenModal}
    </div>
  );
}
