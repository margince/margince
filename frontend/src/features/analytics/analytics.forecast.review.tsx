import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Badge, Disclosure } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { type Locale, useLocale, useT } from "../../i18n";
import { throwProblem, unwrap } from "../../lib/problem";
import { type AnalyticsScope, scopeQuery } from "./analytics.context";
import { CheckTable } from "./analytics.forecast.checktable";
import { EnforcementSwitch } from "./analytics.forecast.enforcement";
import { FirstCheck, Recheck } from "./analytics.forecast.firstcheck";
import {
  FreshnessLine,
  OutOfDateBadge,
  outOfDate,
  verdictStands,
} from "./analytics.forecast.freshness";

type Assurance = components["schemas"]["ForecastAssurance"];

// What should be checked before the call.
//
// Two different statements sit here and must not be read as one. The RECORD
// problems are things the pipeline got wrong; the COVERAGE line is how much of
// the pipeline could be looked at. A run that could not open the mailbox and
// found nothing is not a clean pipeline, and folding the two into one count is
// the misreading this panel exists to prevent.
export function ForecastReview({
  scope,
}: Readonly<{ scope?: AnalyticsScope }>) {
  const t = useT();
  const { locale } = useLocale();

  const assurance = useQuery({
    queryKey: ["forecast-assurance"],
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/forecast/assurance",
        {},
      );
      // 404 is this endpoint's ANSWER, not its failure: the contract says "no
      // run has completed yet. A fresh installation has not been checked."
      // Thrown as a problem it reached the error gate and the panel read
      // "Couldn't load this view. not found" — which tells a reader the screen
      // is broken when what happened is that nothing has looked yet, and those
      // are opposite instructions about whether to trust the numbers above.
      if (response.status === 404) {
        return null;
      }
      if (error) {
        throwProblem(error);
      }
      return data ?? null;
    },
  });

  return (
    <QueryGate query={assurance} pendingLabel={t("review.title")}>
      {(run) =>
        run === null ? (
          // 404 here is a workspace nobody has started, not a wait: the nightly
          // pass skips it until somebody does. So the panel ASKS, with the
          // preview that makes the ask answerable.
          <FirstCheck title={t("review.title")} />
        ) : (
          <ReviewPanel
            run={run}
            locale={locale}
            title={t("review.title")}
            scope={scope}
          />
        )
      }
    </QueryGate>
  );
}

// The open findings this reader may see, under the one key every reader of
// them shares, so answering one here clears it everywhere it is counted.
// The findings, for one record scope when given; without one, every finding
// the reader may see. The answer carries the server's groups and totals.
export function useInputChecks(enabled = true, scope?: AnalyticsScope) {
  const query = scope ? scopeQuery(scope) : {};
  return useQuery({
    enabled,
    queryKey: ["input-checks", query],
    // The signal stops the read of a scope the reader has already left.
    queryFn: async ({ signal }) => {
      const data = unwrap(
        await api.GET("/forecast/assurance/exceptions", {
          params: { query },
          signal,
        }),
      );
      return data;
    },
  });
}

function ReviewPanel({
  run,
  locale,
  title,
  scope,
}: Readonly<{
  run: Assurance;
  locale: Locale;
  title: string;
  scope?: AnalyticsScope;
}>) {
  const t = useT();
  // The page's scope, so a door from Sales review lists what it counted.
  const checks = useInputChecks(true, scope);

  return (
    <Panel
      title={title}
      titleAction={
        <>
          {outOfDate(run) && <OutOfDateBadge />}
          <ReadinessBadge run={run} />
        </>
      }
    >
      <PanelBody>
        <FreshnessLine freshness={run.freshness} />
        {run.sources?.some((source) => source.state !== "checked") ? (
          <CoverageLine sources={run.sources} />
        ) : (
          <Disclosure summary={t("analytics.sectionCoverage")}>
            <CoverageLine sources={run.sources} />
          </Disclosure>
        )}
        {/* Beside the coverage line rather than in the header: a recheck is an
            answer to what the coverage line just said, and the header already
            carries the verdict. */}
        <Recheck runId={run.run_id} />
        <EnforcementSwitch />
      </PanelBody>
      <QueryGate query={checks} pendingLabel={title}>
        {(found) => (
          <CheckTable checks={found.data} locale={locale} title={title} />
        )}
      </QueryGate>
    </Panel>
  );
}

// The verdict, as a word rather than a count.
//
// `checks_incomplete` is not a worse `needs_review`. One says the pipeline has
// problems, the other says we could not look, and they ask for different
// things. A verdict on records that changed since, or may have, keeps its
// word and loses its tone. "Ready" in green over a moved pipeline would be a
// false healthy reading.
function ReadinessBadge({ run }: Readonly<{ run: Assurance }>) {
  const t = useT();
  if (!run.readiness) {
    return null;
  }

  const tone = {
    ready: "success",
    ready_with_exceptions: "accent",
    needs_review: "warning",
    checks_incomplete: "warning",
  } as const;
  const label = {
    ready: "review.ready",
    ready_with_exceptions: "review.readyWithExceptions",
    needs_review: "review.needsReview",
    checks_incomplete: "review.checksIncomplete",
  } as const;
  return (
    <Badge tone={verdictStands(run) ? tone[run.readiness] : "default"}>
      {t(label[run.readiness])}
    </Badge>
  );
}

// How much of the pipeline the run could read.
//
// Named sources rather than a count: "2 of 6 sources" tells a reader a number,
// and which two is what they need to fix it.
export function CoverageLine({
  sources,
}: Readonly<{ sources: Assurance["sources"] | undefined }>) {
  const t = useT();
  const unread = (sources ?? []).filter((source) => source.state !== "checked");
  if (unread.length === 0) {
    return <p className="sub">{t("review.allSourcesRead")}</p>;
  }
  return (
    <p className="sub">
      {t("review.sourcesUnread", {
        sources: unread
          .map((source) => sourceName(source.source, t))
          .join(", "),
      })}
    </p>
  );
}

// A source's name in the reader's language.
//
// The wire carries the server's own vocabulary — "mail", "offers" — and the
// line printed it verbatim, so a German reader was told which sources went
// unread in English, in words that name a table rather than a thing they
// recognise. An unknown source falls back to its wire key: a source this
// release has not heard of is better named badly than not named at all, since
// the whole point of the line is which one to go and fix.
// Exported for the Data coverage table, which draws the same source enum:
// two screens naming one vocabulary share one translator.
export function sourceName(source: string, t: ReturnType<typeof useT>): string {
  switch (source) {
    case "mail":
      return t("review.source.mail");
    case "offers":
      return t("review.source.offers");
    case "calendar":
      return t("review.source.calendar");
    case "documents":
      return t("review.source.documents");
    case "contracts":
      return t("review.source.contracts");
    case "incumbent":
      return t("review.source.incumbent");
    default:
      return source;
  }
}
