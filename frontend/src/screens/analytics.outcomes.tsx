import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { StatCard } from "../design-system/atoms";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { StatStrip } from "../design-system/statstrip";
import { SurfaceState } from "../design-system/surfacestate";
import { formatMoneyCompact, formatNumber } from "../format/format";
import { type Locale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { openAnalyticsSection } from "./analytics.address";
import { rowCount, rowMoney } from "./analytics.cells";
import { MEETING_STATUSES } from "./analytics.questions.values";
import { throwProblem } from "./common";

// What a reading says when the row it needs is not there. THREE facts, not one:
// a read in flight resolves by waiting, a failed one never will, and a lens
// that answered with nothing has told the truth. One word over all three said
// "nothing to read yet" over a request that had failed.
function absentReading(
  query: Readonly<{ isLoading: boolean; isError: boolean }>,
): MessageKey {
  if (query.isLoading) return "analytics.readingLoading";
  if (query.isError) return "analytics.readingUnavailable";
  return "analytics.readingNone";
}

type AnalyticsScopeWire = components["schemas"]["AnalyticsScope"];

// The seat's own outcomes: open pipeline and meetings, nothing computed here.
//
// Drawn only under an OWNER default lens. The report engine's population
// default is the caller's own row scope, so for a wider lens the same
// requests would measure a team while the heading said "my" — and there is
// no per-report scope override on the wire to force self. The tab is hidden
// for those lenses; a hand-typed address gets the explanation instead.
export function MyOutcomesView({
  defaultScope,
  locale,
}: Readonly<{ defaultScope: AnalyticsScopeWire; locale: Locale }>) {
  const t = useT();
  const self = defaultScope.kind === "owner" ? (defaultScope.id ?? null) : null;

  const pipelineQuery = useQuery({
    queryKey: ["report", "pipeline-current", "outcomes", self],
    enabled: self != null,
    queryFn: async () => {
      const { data, error } = await api.POST("/reports/{report}", {
        params: { path: { report: "pipeline-current" } },
        body: {
          // The seat pinned EXPLICITLY, not left to the server's default
          // population: the default is also the caller's own today, so the
          // two agree, but this card's heading says "my" and a heading must
          // not be true by a coincidence this file cannot see.
          filters: { owner_id: self },
          aggregates: [
            { fn: "count", as: "deal_count" },
            { fn: "sum", field: "amount_base_minor", as: "raw_minor" },
          ],
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

  const meetingsQuery = useQuery({
    queryKey: ["report", "activities-by-kind", "outcomes", self],
    enabled: self != null,
    queryFn: async () => {
      const { data, error } = await api.POST("/reports/{report}", {
        params: { path: { report: "activities-by-kind" } },
        body: {
          filters: { kind: "meeting", host_user_id: self },
          group_by: ["meeting_status"],
          aggregates: [{ fn: "count", as: "meetings" }],
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

  if (self == null) {
    // A hand-typed address under a manager lens: the numbers this view could
    // fetch would measure the default population, not the contact. `withheld`
    // and not `empty`, which would claim they have no outcomes.
    return (
      <SurfaceState
        state="withheld"
        emptyLabel={t("common.empty")}
        loadingLabel={t("analytics.sectionOutcomes")}
        detail={{ withheldReason: t("analytics.outcomesOwnLensOnly") }}
      >
        {null}
      </SurfaceState>
    );
  }

  const pipelineRow = pipelineQuery.data?.rows[0];
  const baseCurrency = pipelineQuery.data?.base_currency ?? null;
  const noRow = t(absentReading(pipelineQuery));
  const pipelineCount = pipelineRow
    ? formatNumber(rowCount(pipelineRow, "deal_count"), locale)
    : noRow;
  const rawMinor = pipelineRow ? rowMoney(pipelineRow, "raw_minor") : null;
  // The currency names what the figure is IN, so a read with none to name drops
  // the parenthetical. TWO absences follow, never one word for both: no
  // currency is a setting to fill, an absent sum is a row with no priced deal
  // in it — and only the first has a reason worth a detail line.
  const valueLabel = baseCurrency
    ? t("analytics.baseValue", { currency: baseCurrency })
    : t("analytics.baseValueUnnamed");
  const openMoney = !baseCurrency
    ? t("analytics.noBaseCurrency")
    : rawMinor == null
      ? t("analytics.forecastNoAmount")
      : formatMoneyCompact(rawMinor, baseCurrency, locale);
  const meetingRows = meetingsQuery.data?.rows ?? [];
  const meetingsByStatus = new Map(
    meetingRows
      .filter((row) => typeof row.meeting_status === "string")
      .map((row) => [String(row.meeting_status), rowCount(row, "meetings")]),
  );

  return (
    <>
      <Panel title={t("analytics.myPipeline")}>
        <PanelBody>
          <StatStrip>
            {/* Both readings are one row of the pipeline report, and the
                pipeline section draws that report — so the door is that
                section rather than a deal list this view never queried. */}
            <StatCard
              narrow="row"
              label={t("analytics.count")}
              value={pipelineCount}
              onOpen={() => openAnalyticsSection("pipeline")}
            />
            <StatCard
              narrow="row"
              label={valueLabel}
              value={pipelineRow ? openMoney : noRow}
              // WHY there is no figure, where a row came back and no currency
              // names it: an installation that never set one is a setting away
              // from a number, and "No amount" alone reads as a book worth
              // nothing.
              detail={
                pipelineRow && !baseCurrency
                  ? t("analytics.noBaseCurrencyWhy")
                  : undefined
              }
              onOpen={() => openAnalyticsSection("pipeline")}
            />
          </StatStrip>
        </PanelBody>
      </Panel>
      <Panel title={t("analytics.myMeetings")}>
        <PanelBody>
          {/* Current standing, stated as such: a held meeting was once booked
              and the record no longer says so, so these are today's facts and
              not a funnel. */}
          <PanelIntro>{t("analytics.meetingsAsTheyStand")}</PanelIntro>
          <StatStrip>
            {MEETING_STATUSES.map((status) => (
              <StatCard
                key={status.key}
                narrow="row"
                label={t(status.labelKey)}
                value={formatNumber(
                  meetingsByStatus.get(status.key) ?? 0,
                  locale,
                )}
              />
            ))}
          </StatStrip>
        </PanelBody>
      </Panel>
    </>
  );
}
