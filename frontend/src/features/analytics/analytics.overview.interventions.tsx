// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { AppLink } from "../../design-system/applink";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import { SurfaceState } from "../../design-system/surfacestate";
import {
  formatDateAbbrev,
  formatMoneyOrAbsent,
  formatNumber,
} from "../../format/format";
import { type Locale, useLocale, usePlural, useT } from "../../i18n";
import { analyticsSectionHref } from "../../lib/analytics.address";
import { useCan } from "../../lib/capability";
import { isEntityKind } from "../../lib/entity";
import { useMemberNames } from "../../lib/membernames";
import { useRecordZone } from "../../lib/recordzone";
import type { UrlParams } from "../../lib/urlstate";
import { type AttentionState, attentionState } from "./analytics.attention";
import type { AnalyticsScope } from "./analytics.context";
import { useDataCoverage } from "./analytics.coverage";
import { useInputChecks } from "./analytics.forecast.review";
import { checkLabel } from "./inputcheck.facts";

type InputCheck = components["schemas"]["InputCheck"];
type CheckList = NonNullable<ReturnType<typeof useInputChecks>["data"]>;
type CheckGroup = NonNullable<CheckList["groups"]>[number];

// How many findings are named one by one, in the server's order: most severe,
// then most money at stake. The rest are counted in their groups.
const NAMED = 3;

/** The element the summary's sentences about checks move focus to. */
export const INTERVENTIONS_ID = "analytics-interventions";

export type Interventions = Readonly<{
  // The whole list the reader may see, with the server's groups and totals.
  list: CheckList | undefined;
  state: AttentionState;
  // Sources the last check could not read: maintenance, never a sales row.
  unreadSources: number;
}>;

/**
 * The findings Sales review acts on and the sources behind them, read under
 * the keys their own sections read them by.
 */
export function useInterventions(scope: AnalyticsScope): Interventions {
  const canReadForecast = useCan("forecast", "read");
  const checks = useInputChecks(canReadForecast, scope);
  const coverage = useDataCoverage();
  const canReadCoverage = useCan("data_coverage", "read");
  const unread =
    (canReadCoverage &&
      coverage.data?.sources.filter((source) => source.state !== "checked")) ||
    [];
  return {
    // A disabled query still hands back what it cached, so a grant withdrawn
    // mid-session is checked here as well as at the request.
    list: canReadForecast ? checks.data : undefined,
    state: attentionState(canReadForecast, checks),
    unreadSources: unread.length,
  };
}

/**
 * The top findings as verbs a reader can act on, their groups counted over
 * the whole list, and the sources needing maintenance on their own line.
 * Every door carries `dials`, the question the list was read under.
 */
export function InterventionPanel({
  interventions,
  dials,
  emptyLabel,
}: Readonly<{
  interventions: Interventions;
  dials: UrlParams;
  emptyLabel: string;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const { list, state, unreadSources } = interventions;
  const findings = list?.data ?? [];
  const forecast = (extra?: readonly [string, string]) =>
    analyticsSectionHref(
      "forecast",
      new Map([...dials, ...(extra ? [extra] : [])]),
    );
  return (
    <div id={INTERVENTIONS_ID} tabIndex={-1}>
      <Panel
        title={t("analytics.attention")}
        footer={
          findings.length > 0 && (
            <AppLink href={forecast()}>
              {t("analytics.intervention.viewAll")}
            </AppLink>
          )
        }
      >
        <PanelBody>
          {/* The checks read every pipeline and close date of the scope. */}
          <p className="t-caption">{t("analytics.intervention.scope")}</p>
          {findings.length === 0 && (
            <SurfaceState
              state={
                state === "pending"
                  ? "loading"
                  : state === "failed"
                    ? "unavailable"
                    : "empty"
              }
              emptyLabel={emptyLabel}
              loadingLabel={t("analytics.attention")}
            >
              {null}
            </SurfaceState>
          )}
          {state === "failed" && findings.length > 0 && (
            <p className="t-caption">{t("analytics.attentionIncomplete")}</p>
          )}
        </PanelBody>
        <FindingRows
          findings={findings.slice(0, NAMED)}
          href={(id) => forecast(["finding", id])}
        />
        {list?.totals && list.totals.findings > 0 && (
          <Groups
            groups={list.groups ?? []}
            totals={list.totals}
            href={(type) => forecast(["check", type])}
          />
        )}
        {unreadSources > 0 && (
          <PanelBody>
            <p className="analytics-attention-title">
              {t("analytics.intervention.maintenance")}
            </p>
            <p className="t-caption">
              {plural("analytics.intervention.maintenanceBody", unreadSources, {
                count: formatNumber(unreadSources, locale),
              })}{" "}
              <AppLink href={analyticsSectionHref("coverage", dials)}>
                {t("analytics.attentionCoverageAction")}
              </AppLink>
            </p>
          </PanelBody>
        )}
      </Panel>
    </div>
  );
}

function FindingRows({
  findings,
  href,
}: Readonly<{
  findings: readonly InputCheck[];
  href: (id: string) => string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const owners = useMemberNames(
    findings.flatMap((check) => check.owner_id ?? []),
  );
  return findings.map((check) => (
    <PanelRow key={check.id} className="analytics-attention-row">
      <div className="analytics-attention-text">
        <p className="analytics-attention-title">
          <VerbAndObject check={check} />
        </p>
        <p className="t-caption">{t(checkLabel(check.type))}</p>
        <p className="t-caption">
          {findingFacts(check, owners, t, locale, zone)}
        </p>
      </div>
      <AppLink className="btn btn-ghost" href={href(check.id)}>
        {t(
          check.destination?.mode === "read"
            ? "analytics.intervention.open"
            : "analytics.intervention.answer",
        )}
      </AppLink>
    </PanelRow>
  ));
}

// The verb and the record it is about, the record as a link to itself. The
// sentence is translated whole, so the link sits where each language puts it.
function VerbAndObject({ check }: Readonly<{ check: InputCheck }>) {
  const t = useT();
  const { subject } = check;
  if (!subject?.label) return t("analytics.intervention.reviewFinding");
  const [before = "", after = ""] = t("analytics.intervention.review", {
    subject: SLOT,
  }).split(SLOT);
  return (
    <>
      {before}
      {isEntityKind(subject.type) ? (
        <EntityRef kind={subject.type} id={subject.id} name={subject.label} />
      ) : (
        subject.label
      )}
      {after}
    </>
  );
}

const SLOT = "\u0000";

// Who answers for a finding, since when it is open and what is at stake. An
// owner whose name has not arrived is left out, never called absent. A deal
// with no owner is said to have none; any other subject's owner is unknown.
function findingFacts(
  check: InputCheck,
  owners: ReadonlyMap<string, string>,
  t: ReturnType<typeof useT>,
  locale: Locale,
  zone: string,
): string {
  const owner = check.owner_id
    ? owners.get(check.owner_id)
    : check.subject_kind === "deal"
      ? t("analytics.intervention.unassigned")
      : undefined;
  const since = t("analytics.intervention.since", {
    date: formatDateAbbrev(check.first_seen_at, locale, zone),
  });
  const amount =
    check.affected_minor == null
      ? undefined
      : t("analytics.attentionAtStake", {
          amount: formatMoneyOrAbsent(
            check.affected_minor,
            check.currency,
            locale,
          ),
        });
  const outOfDate = check.inputs_changed
    ? t("analytics.attentionOutOfDate")
    : undefined;
  return [owner, since, amount, outOfDate].filter(Boolean).join(" · ");
}

function Groups({
  groups,
  totals,
  href,
}: Readonly<{
  groups: readonly CheckGroup[];
  totals: NonNullable<CheckList["totals"]>;
  href: (type: string) => string;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const counted = (group: {
    findings: number;
    deals: number;
    other_findings: number;
  }) =>
    [
      plural("analytics.intervention.findings", group.findings, {
        count: formatNumber(group.findings, locale),
      }),
      plural("analytics.intervention.deals", group.deals, {
        count: formatNumber(group.deals, locale),
      }),
      group.other_findings > 0
        ? plural("analytics.intervention.other", group.other_findings, {
            count: formatNumber(group.other_findings, locale),
          })
        : undefined,
    ]
      .filter(Boolean)
      .join(" · ");
  return (
    <>
      <PanelBody>
        <p className="analytics-attention-title">
          {t("analytics.intervention.allChecks", { counts: counted(totals) })}
        </p>
      </PanelBody>
      {groups.map((group) => (
        <PanelRow key={group.type} className="analytics-attention-row">
          <div className="analytics-attention-text">
            <p>{t(checkLabel(group.type))}</p>
            <p className="t-caption">
              {[
                counted(group),
                // One amount per currency, never added across them.
                ...group.exposure.map((money) =>
                  formatMoneyOrAbsent(
                    money.amount_minor,
                    money.currency,
                    locale,
                  ),
                ),
              ].join(" · ")}
            </p>
          </div>
          <AppLink className="btn btn-ghost" href={href(group.type)}>
            {t("analytics.intervention.show")}
          </AppLink>
        </PanelRow>
      ))}
    </>
  );
}
