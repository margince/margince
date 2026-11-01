// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { AppLink } from "../../design-system/applink";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import {
  type SectionState,
  SurfaceState,
} from "../../design-system/surfacestate";
import {
  formatMoneyCompact,
  formatNumber,
  MONEY_ABSENT,
} from "../../format/format";
import { formatMoneyOrWord } from "../../format/moneyword";
import { type Locale, useLocale, usePlural, useT } from "../../i18n";
import {
  analyticsSectionHref,
  type Section,
} from "../../lib/analytics.address";
import { useCan } from "../../lib/capability";
import { isEntityKind } from "../../lib/entity";
import { useMemberNames } from "../../lib/membernames";
import { type UrlParams, useUrlParams } from "../../lib/urlstate";
import type { AnalyticsScope } from "./analytics.context";
import { useDataCoverage } from "./analytics.coverage";
import { useInputChecks } from "./analytics.forecast.review";
import { useForecastReadings } from "./forecast.queries";
import { checkLabel } from "./inputcheck.facts";

type InputCheck = components["schemas"]["InputCheck"];

export type AttentionItem = Readonly<{
  key: string;
  title: string;
  detail: string;
  action: string;
  section: Section;
  // The record a finding is about, when the line names one.
  subject?: InputCheck["subject"];
}>;

// How many findings the panel names one by one, in the server's order (most
// severe, then most money at stake). The rest stay one line that opens the
// full review, so the panel stays short enough to read every day.
const NAMED_FINDINGS = 5;

export function AnalyticsAttention({
  scope,
}: Readonly<{ scope: AnalyticsScope }>) {
  const { items } = useAttentionList(scope);
  return <AttentionPanel items={items} />;
}

// Whether the list is the whole answer. `failed` means a source did not
// answer, so a short or empty list says nothing about what is clear.
export type AttentionState = "pending" | "failed" | "ready";

/**
 * The findings as one panel. Given `state`, the panel always draws. It then
 * says whether the list is loading, could not be read, or holds nothing.
 * Each finding links to its section asked with `dials`, the question the
 * list was read under; without them a link carries the address as it stands.
 */
export function AttentionPanel({
  items,
  state,
  emptyLabel,
  dials,
}: Readonly<{
  items: readonly AttentionItem[];
  state?: AttentionState;
  emptyLabel?: string;
  dials?: UrlParams;
}>) {
  const t = useT();
  const [params] = useUrlParams();
  if (items.length === 0 && state === undefined) {
    return null;
  }
  return (
    <Panel title={t("analytics.attention")}>
      {state !== undefined && items.length === 0 && (
        <PanelBody>
          <SurfaceState
            state={surfaceOf(state)}
            emptyLabel={emptyLabel ?? t("common.empty")}
            loadingLabel={t("analytics.attention")}
          >
            {null}
          </SurfaceState>
        </PanelBody>
      )}
      {state === "failed" && items.length > 0 && (
        <PanelBody>
          <p className="t-caption">{t("analytics.attentionIncomplete")}</p>
        </PanelBody>
      )}
      {items.map((item) => (
        <PanelRow key={item.key} className="analytics-attention-row">
          <div className="analytics-attention-text">
            <p className="analytics-attention-title">{item.title}</p>
            {item.subject && isEntityKind(item.subject.type) && (
              <EntityRef
                kind={item.subject.type}
                id={item.subject.id}
                name={item.subject.label}
              />
            )}
            <p className="t-caption">{item.detail}</p>
          </div>
          <AppLink
            className="btn btn-ghost"
            href={analyticsSectionHref(item.section, dials ?? params)}
          >
            {item.action}
          </AppLink>
        </PanelRow>
      ))}
    </Panel>
  );
}

function surfaceOf(state: AttentionState): SectionState {
  if (state === "pending") return "loading";
  return state === "failed" ? "unavailable" : "empty";
}

// Who answers for a finding, what is at stake, and whether the records moved
// since the check found it. An absent part is left out, never drawn empty.
function findingDetail(
  check: InputCheck,
  owner: string | undefined,
  t: ReturnType<typeof useT>,
  locale: Locale,
): string {
  const amount =
    check.affected_minor == null
      ? undefined
      : t("analytics.attentionAtStake", {
          amount: formatMoneyOrWord(
            check.affected_minor,
            check.currency,
            locale,
            MONEY_ABSENT,
            formatMoneyCompact,
          ),
        });
  const outOfDate = check.inputs_changed
    ? t("analytics.attentionOutOfDate")
    : undefined;
  return [owner, amount, outOfDate].filter(Boolean).join(" · ");
}

// Each source is read under the key its own section reads it by, so opening
// that section shows the same answer this list summarised. A source that fails
// to load drops its line instead of the panel. The section it links to states
// the failure, and `state` says the list may be short.
// `withReadings` false leaves out the unpriced-deals line and its read. A page
// whose headline says how much of the pipeline is priced needs neither.
export function useAttentionList(
  scope: AnalyticsScope,
  withReadings = true,
): Readonly<{ items: AttentionItem[]; state: AttentionState }> {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const canReadForecast = useCan("forecast", "read");
  const checks = useInputChecks(canReadForecast, scope);
  // An undefined scope holds the read, which is what a reader without the
  // forecast grant needs: no request, rather than a refused one.
  const readings = useForecastReadings(
    canReadForecast && withReadings ? scope : undefined,
    "quarter",
  );
  const coverage = useDataCoverage();
  const canReadCoverage = useCan("data_coverage", "read");

  // A disabled query still hands back what it cached, so a grant withdrawn
  // mid-session is checked here as well as at the request.
  const items: AttentionItem[] = [];
  const findings = (canReadForecast && checks.data?.data) || [];
  const owners = useMemberNames(
    findings.flatMap((check) => check.owner_id ?? []),
  );
  for (const check of findings.slice(0, NAMED_FINDINGS)) {
    // An owner whose name has not arrived is left out, never called absent.
    const owner = check.owner_id
      ? owners.get(check.owner_id)
      : t("analytics.attentionNoOwner");
    items.push({
      key: `check:${check.id}`,
      title: t(checkLabel(check.type)),
      subject: check.subject,
      detail: findingDetail(check, owner, t, locale),
      action: t("analytics.attentionReview"),
      section: "forecast",
    });
  }
  if (findings.length > NAMED_FINDINGS) {
    items.push({
      key: "checks",
      title: plural("analytics.attentionChecks", findings.length, {
        count: formatNumber(findings.length, locale),
      }),
      detail: t("analytics.attentionChecksDetail"),
      action: t("analytics.attentionChecksAction"),
      section: "forecast",
    });
  }
  const forecast = canReadForecast && withReadings ? readings.data : undefined;
  if (forecast && forecast.priced_count < forecast.eligible_count) {
    items.push({
      key: "unpriced",
      title: t("analytics.attentionUnpriced", {
        priced: formatNumber(forecast.priced_count, locale),
        eligible: formatNumber(forecast.eligible_count, locale),
      }),
      detail: t("analytics.attentionUnpricedDetail"),
      action: t("analytics.attentionUnpricedAction"),
      section: "forecast",
    });
  }
  const unread =
    (canReadCoverage &&
      coverage.data?.sources.filter((source) => source.state !== "checked")) ||
    [];
  if (unread.length > 0) {
    items.push({
      key: "coverage",
      title: plural("analytics.attentionCoverage", unread.length, {
        count: formatNumber(unread.length, locale),
      }),
      detail: t("reporting.coverageAction"),
      action: t("analytics.attentionCoverageAction"),
      section: "coverage",
    });
  }
  return { items, state: attentionState(canReadForecast, checks) };
}

// Whether the findings list is loading, failed or answered; a reader without
// the forecast grant has nothing to wait for.
export function attentionState(
  reads: boolean,
  checks: Readonly<{ isPending: boolean; isError: boolean }>,
): AttentionState {
  if (!reads) return "ready";
  if (checks.isError) return "failed";
  return checks.isPending ? "pending" : "ready";
}
