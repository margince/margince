import type { ReactNode } from "react";
import { CountUp } from "../../design-system/countup";
import { CrawlCanvas } from "../../design-system/crawl-canvas";
import type { MarginceCoreState } from "../../design-system/margince-core";
import {
  OnboardingStage,
  STAGE_CORE_ID,
} from "../../design-system/onboarding-stage";
import { formatAiRunCost } from "../../format/aicost";
import { formatNumber } from "../../format/format";
import { type Locale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { CORE_LABELS } from "../../lib/onboarding-core-label";
import type {
  AiRunSummary,
  CompanySiteRead,
  CompanySiteReadPage,
  GateScan,
  Translate,
} from "./onboarding-gate";
import {
  pageStatusWord,
  reasonOf,
  ScanTickerEntry,
  useLatestArrivedPage,
} from "./onboarding-gate.ticker";

/**
 * What the stage says above the column, for both faces of this screen.
 *
 * A function rather than a branch inside the component: the two faces are one
 * shape with different values. A returned object keeps them from drifting into
 * two layouts, and keeps GateColumn readable: the branching is here, over data,
 * not over markup.
 */
type StageHead = Readonly<{
  core: MarginceCoreState;
  /**
   * What the Core is doing, in words, for the band above.
   *
   * While a read runs it is the read's own phase line, the sentence the theatre
   * writes from `phaseKey`, not a second vocabulary for the same fact. A read
   * with no phase leaves it absent, as the theatre's line does: the band says
   * nothing rather than inventing a fifth message.
   */
  coreLabel?: string;
  /** The part of this stop the reader is in, for the band beside the step. */
  where?: string;
  title: string;
  sub: string;
}>;

// `pages_read` is the server's own tally; where it is absent the fetched pages
// are the same fact counted from the array rather than a guess. One helper, so
// the band and the tally beneath it cannot disagree about how many were read.
function pagesReadOf(read: CompanySiteRead): number {
  return (
    read.pages_read ??
    read.pages.filter((page) => page.status === "fetched").length
  );
}

function stageHead(
  t: Translate,
  scan: GateScan | undefined,
  running: boolean,
  name: string | undefined,
): StageHead {
  if (scan === undefined) {
    // At rest before the press, the orb carries nothing worth announcing, and
    // "idle" over an empty form is chrome describing itself. The band speaks
    // from the moment there is something to say.
    const core = running ? "ingest" : "idle";
    return {
      core,
      coreLabel: running ? t(CORE_LABELS[core]) : undefined,
      title: name ? t("ob.gate.title", { name }) : t("ob.gate.titleAnonymous"),
      sub: t("ob.gate.sub"),
    };
  }
  const settled = SETTLED.has(scan.read.status);
  const core = coreStateFor(scan.read);
  return {
    core,
    coreLabel: t(CORE_LABELS[core]),
    title: settled
      ? t("ob.scan.doneTitle", { host: scan.host })
      : t("ob.scan.title", { host: scan.host }),
    sub: settled
      ? t("ob.scan.doneSub", {
          facts: formatNumber(scan.read.facts.length, scan.locale),
          fields: formatNumber(scan.read.profile_fields.length, scan.locale),
        })
      : t("ob.scan.sub"),
    // The page count belongs in the band and only here. A second line above a
    // tally with the same two-word label would print it twice.
    where: t("ob.scan.pagesRead", {
      pages: formatNumber(pagesReadOf(scan.read), scan.locale),
    }),
  };
}

/**
 * The column both faces share, and the reason they are one component.
 *
 * The Core, the headline and the promise sentence keep their tree positions
 * from the first question to the finished read; only the tail below changes.
 * Two components at one position would be two types there, so React would
 * unmount one subtree and mount the other. The Core would rebuild its WebGL
 * context, restart its float, breathe and sheen loops, and replay its entrance.
 * The most important moment in the flow would flash instead of continuing.
 */
export function GateColumn({
  scan,
  running,
  name,
  children,
}: Readonly<{
  scan?: GateScan;
  running?: boolean;
  name?: string;
  children: ReactNode;
}>) {
  const t = useT();
  const head = stageHead(t, scan, running === true, name);
  const settled = scan !== undefined && SETTLED.has(scan.read.status);
  return (
    <OnboardingStage
      flow={t("ob.stage.flow")}
      // Nothing reaches this screen until first run is done, so a model is
      // always bound by the time anyone is here. The room says so, and it says
      // only that: a second trigger per screen is how one colour acquires two
      // meanings.
      lit
      coreState={head.core}
      // The orb is the subject while the screen asks one question, and steps
      // back once the reader has a read of their own to watch. Same element,
      // same place: it gives ground rather than being replaced.
      coreScale={scan === undefined ? "hero" : "work"}
      // The Core is aria-hidden, so the band says what it is showing. While a
      // read runs, that is the theatre's own phase line, not a second
      // vocabulary for the same fact.
      coreStateLabel={head.coreLabel}
      // A read grows under the reader, tile by tile. Centred, the column would
      // re-centre on every arriving page and carry the line being read upward.
      anchor={scan === undefined ? "center" : "start"}
      // Where the reader is, on the screen they wait on longest (often two
      // minutes). The name only, with no marks: this surface is prop-driven and
      // cannot see the flow around it. So it says what it is and does not invent
      // how many stops the passage has.
      step={t("ob.stop.read")}
      where={scan === undefined ? undefined : head.where}
      title={head.title}
      sub={head.sub}
    >
      <div
        className={`ob-gate${scan === undefined ? "" : " ob-scan"}${
          settled ? " is-settled" : ""
        }`}
      >
        {children}
      </div>
    </OnboardingStage>
  );
}

// A read that has produced its answer. `partial` belongs here: it carries facts
// and fields, and lacks only pages the crawl could not reach. The page strip
// states those, tile by tile.
const SETTLED: ReadonlySet<CompanySiteRead["status"]> = new Set([
  "ready",
  "partial",
  "confirmed",
]);

const BROKEN: ReadonlySet<CompanySiteRead["status"]> = new Set([
  "failed",
  "abandoned",
]);

function coreStateFor(read: CompanySiteRead): MarginceCoreState {
  if (SETTLED.has(read.status)) {
    // A finished run settles back to idle: there is no state of its own for
    // "done".
    return "idle";
  }
  if (BROKEN.has(read.status)) {
    return "error";
  }
  // A site read is intake, pages arriving one after another, until the
  // extracting phase, where the agent works over what it has. Reading the phase
  // too keeps this orb saying what the conversation's own orb says (presence.ts).
  return read.phase === "extracting" ? "working" : "ingest";
}

// The one phase line, from the only two fields that carry a phase. `status`
// wins over `phase` because a queued or deferred read has not started, whatever
// a stale `phase` from an earlier attempt still says. No fifth message is
// invented for the states that carry no phase: the line goes empty instead.
function phaseKey(read: CompanySiteRead): MessageKey | null {
  if (read.status === "queued") {
    return "ob.scan.phaseQueued";
  }
  if (read.status === "deferred") {
    return "ob.scan.phaseDeferred";
  }
  if (read.phase === "crawling") {
    return "ob.scan.phaseCrawling";
  }
  if (read.phase === "extracting") {
    return "ob.scan.phaseExtracting";
  }
  return null;
}

// The path alone, because every row shares the host. A URL the browser cannot
// parse is shown whole rather than dropped. It came from the server, and hiding
// it would lose an operator's one clue about why that page behaved oddly.
function pathOf(url: string): string {
  try {
    return new URL(url).pathname;
  } catch {
    return url;
  }
}

// What happened to one page and why, as one sentence. The crawl picture cannot
// carry this and does not try: it is the accessible statement of the same walk.
function pageLabel(t: Translate, page: CompanySiteReadPage): string {
  const reason = reasonOf(t, page);
  if (page.status === "skipped") {
    return t("ob.scan.pageSkipped", { url: page.url, reason });
  }
  if (page.status === "failed") {
    return t("ob.scan.pageFailed", { url: page.url, reason });
  }
  return t("ob.scan.pageFetched", { url: page.url });
}

function costLine(t: Translate, runtime: AiRunSummary, locale: Locale): string {
  return t("ob.scan.costLine", {
    calls: formatNumber(runtime.call_attempts, locale),
    tokens: formatNumber(runtime.tokens_in + runtime.tokens_out, locale),
    cost: formatAiRunCost(runtime.estimated_cost_microusd, locale),
  });
}

/**
 * The wait: what is happening, what has been read, what it cost.
 *
 * Everything on screen is a field of the polled read. The three regions each
 * hold their own size so an arriving page or a new phase never re-lays out the
 * column.
 */
export function ReadTheatre({
  read,
  host,
  locale,
  configuredModel,
  uncertainCount,
}: Readonly<{
  read: CompanySiteRead;
  host: string;
  locale: Locale;
  configuredModel: string;
  /** See `OnboardingGate`'s own prop of the same name for what this counts
   * and why it is absent rather than zero for most of a read. */
  uncertainCount?: number;
}>) {
  return (
    <GateColumn scan={{ read, host, locale }}>
      <TheatreTail
        read={read}
        locale={locale}
        configuredModel={configuredModel}
        uncertainCount={uncertainCount}
      />
    </GateColumn>
  );
}

// The theatre's own regions: everything below the head the column draws. Split
// out so the gate can swap this in without replacing the column around it;
// GateColumn documents why that matters.
export function TheatreTail({
  read,
  locale,
  configuredModel,
  uncertainCount,
}: Readonly<{
  read: CompanySiteRead;
  locale: Locale;
  configuredModel: string;
  uncertainCount?: number;
}>) {
  const t = useT();
  const settled = SETTLED.has(read.status);
  const phase = phaseKey(read);
  const runtime = read.ai_runtime;
  const pagesRead = pagesReadOf(read);
  const skipped = read.pages.filter((page) => page.status === "skipped").length;
  const latestPage = useLatestArrivedPage(read.pages);

  return (
    <>
      {/* One panel for the whole read: what it is doing, what it has walked,
          where it is now, and what it has found. Four blocks on bare ground
          read as unrelated readouts; inside one card they read as one
          instrument. The spend stays outside: it is the page's disclosure,
          not the read's. */}
      <div className="ob-scan-panel">
        {/* Fixed height, opacity-only crossfade: the phase changes in place. */}
        <p className="ob-scan-phase" aria-live="polite">
          <span className="ob-scan-phase-dot" aria-hidden />
          {phase === null ? null : (
            <span key={phase} className="ob-scan-phase-text">
              {t(phase)}
            </span>
          )}
        </p>

        {/* The site as it is read, in the shape being walked, so a reader
            waiting two minutes can find their own site in it. The list under
            this names every page in words, the ticker names each as it lands,
            and the counters carry the totals. */}
        <CrawlCanvas
          pages={read.pages.map((page) => ({
            path: pathOf(page.url),
            note: pageStatusWord(t, page),
          }))}
          label={t("ob.scan.pageStripLabel")}
          // The evidence flows into the Core, which is the screen's claim: the
          // read is feeding the thing that will answer with it.
          flowToId={STAGE_CORE_ID}
        />
        {/* The same pages in words, for a reader the picture cannot reach. A
            canvas has one summary label, and the ticker below announces only
            the page that just landed. Without this list there is no way to
            review every page walked and its status. */}
        <ul className="sr-only" aria-label={t("ob.scan.pageStripLabel")}>
          {read.pages.map((page) => (
            <li key={page.url}>{pageLabel(t, page)}</li>
          ))}
        </ul>

        {/* The page the crawl walked last, not a growing transcript. Fixed
            height, one occupant at a time: the old page fades out as the new
            one fades in, and under reduced motion the text swaps. Pages that
            arrive faster than the fade are dropped, not queued. The count
            beside it is the tally, not a promise that every page was shown. */}
        <div className="ob-scan-ticker">
          <ul
            className="ob-scan-ticker-row"
            aria-live="polite"
            aria-label={t("ob.scan.logLabel")}
          >
            {latestPage === null ? null : (
              <ScanTickerEntry
                key={latestPage.url}
                page={latestPage}
                facts={read.facts}
                t={t}
              />
            )}
          </ul>
        </div>

        {/* The two figures the read is earning, at the size of the thing
            being waited for. They count up because they are still being
            earned: a climbing number tells a waiting reader the wait is going
            somewhere. No denominator: the crawl does not know how many pages a
            site has, and an invented total would be untrustworthy. */}
        <dl className="ob-scan-tally">
          <div>
            <dt>{t("ob.scan.tallyPages")}</dt>
            <dd>
              <CountUp value={pagesRead} locale={locale} />
            </dd>
          </div>
          <div>
            <dt>{t("ob.scan.tallyFacts")}</dt>
            <dd>
              <CountUp value={read.facts.length} locale={locale} />
            </dd>
          </div>
          {/* Absent, not zero, until `uncertainCount` is set (see its doc
              comment on `OnboardingGate`). A counter sitting at 0 for most of
              the read would say "nothing to decide yet", which this screen
              does not know at that point. */}
          {uncertainCount !== undefined && (
            <div>
              <dt>{t("ob.scan.tallyUncertain")}</dt>
              <dd>
                <CountUp value={uncertainCount} locale={locale} />
              </dd>
            </div>
          )}
        </dl>

        <p className="ob-scan-counts t-caption">
          <span>
            {t("ob.scan.pagesSkipped", {
              count: formatNumber(skipped, locale),
            })}
          </span>
          {settled ? null : <span>{t("ob.scan.stillReading")}</span>}
        </p>
      </div>

      {/* The AI indigo, not the brand accent: this is what the machine spent,
          not something the user is asked to do. The spend and call count sit
          together on the label's line. Which models answered is detail, on its
          own full-width row beneath. */}
      <div className="ob-scan-cost">
        <div className="ob-scan-cost-head">
          <p className="ob-scan-cost-label">{t("ob.scan.transparency")}</p>
          <p className="ob-scan-cost-line t-caption">
            {runtime === undefined ? (
              t("ob.scan.costPending")
            ) : (
              <>
                {costLine(t, runtime, locale)}
                {/* A call billed with no effective rate is missing from the
                    total above, not folded in as a zero. So the figure reads
                    as complete unless this says otherwise. */}
                {runtime.unpriced_calls > 0 && (
                  <small className="ob-scan-cost-unpriced">
                    {t("ob.scan.costUnpriced")}
                  </small>
                )}
              </>
            )}
          </p>
        </div>
        <p className="ob-scan-cost-model t-caption">{configuredModel}</p>
      </div>
    </>
  );
}
