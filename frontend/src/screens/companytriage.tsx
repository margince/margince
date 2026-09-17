// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Badge, Disclosure } from "../design-system/atoms";
import { PanelBody } from "../design-system/panel";
import { reasonText } from "../design-system/pipelineladder";

import { SurfaceState } from "../design-system/surfacestate";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { type Locale, useLocale, useT } from "../i18n";
import { throwProblem } from "./common";
import { SectionSummary } from "./companyrailshared";
import "./companytriage.css";

// GET /companies/{id}/capture-triage — why this company record exists.
//
// The pipeline's company check asks about a mail DOMAIN, and a domain is
// checked once for every message that ever arrives from it. So the per-message
// ladder cannot carry the answer without implying that the message a member
// happens to be looking at was the cause; it names this surface instead, and
// this is where the answer is.
//
// The rungs are the ladder's own vocabulary, resolved through the ladder's own
// catalog (`reasonText`), so the two surfaces cannot come to describe one check
// with two sentences.

type Triage = components["schemas"]["CompanyCaptureTriage"];
type TriagedDomain = components["schemas"]["CompanyTriagedDomain"];

// The tone each status carries here. Only two arrive — a domain is settled or
// it is open — and the map is keyed on the union so a status added upstream is
// a compile error rather than an untoned badge.
const STATUS_TONE: Record<
  TriagedDomain["rung"]["status"],
  "success" | "warn" | "danger" | undefined
> = {
  done: "success",
  skipped: undefined,
  pending: "warn",
  failed: "danger",
  not_applicable: undefined,
  unknown: undefined,
  not_reported: undefined,
};

export function CompanyTriageSection({
  companyId,
}: Readonly<{ companyId: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const query = useQuery({
    queryKey: ["company-capture-triage", companyId],
    queryFn: async () => {
      const { data, error } = await api.GET("/companies/{id}/capture-triage", {
        params: { path: { id: companyId } },
      });
      if (error) {
        throwProblem(error);
      }
      // Rejected rather than defaulted. `?? []` would draw the empty state,
      // which says no domain was ever checked into this company — a claim
      // about how the record came to exist that this response never made.
      if (!data || !Array.isArray(data.domains)) {
        throw new Error("malformed company-capture-triage response");
      }
      return data;
    },
  });

  return (
    // CLOSED by default, unlike the sections above it. This answers "why does
    // this record exist", which a reader asks once and the rest of the column
    // answers every time they open the account — a section that took space on
    // every visit for a question nobody was asking would push the day-to-day
    // slices down the column.
    <Disclosure
      className="co-sect"
      summary={<SectionSummary title={t("companyTriage.title")} />}
    >
      <PanelBody>
        <p className="t-sub">{t("companyTriage.sub")}</p>
        <SurfaceState
          loadingLabel={t("companyTriage.title")}
          state={triageState(query)}
          emptyLabel={t("companyTriage.empty")}
        >
          <ol className="company-triage__list">
            {(query.data?.domains ?? []).map((entry) => (
              <TriagedDomainRow
                key={entry.domain}
                entry={entry}
                locale={locale}
                zone={zone}
              />
            ))}
          </ol>
        </SurfaceState>
      </PanelBody>
    </Disclosure>
  );
}

// EMPTY AND UNAVAILABLE ARE DIFFERENT ANSWERS, and the whole card turns on
// keeping them apart: empty means nobody checked a domain into this company —
// it was typed in or imported, which is ordinary — and unavailable means the
// read failed and we know nothing. Collapsing them would tell a member how
// their record came to exist on the strength of a request that did not answer.
function triageState(
  query: Readonly<{ isPending: boolean; isError: boolean; data?: Triage }>,
): "loading" | "unavailable" | "empty" | "ready" {
  if (query.isPending) {
    return "loading";
  }
  if (query.isError || !query.data) {
    return "unavailable";
  }
  return query.data.domains.length === 0 ? "empty" : "ready";
}

function TriagedDomainRow({
  entry,
  locale,
  zone,
}: Readonly<{ entry: TriagedDomain; locale: Locale; zone: string }>) {
  const t = useT();
  const { rung } = entry;
  return (
    <li>
      <p className="company-triage__head">
        {/* Verbatim: the domain is what a member greps their own mail for, so
            prettifying it costs them the string they actually need. */}
        <span>{entry.domain}</span>
        <Badge tone={STATUS_TONE[rung.status]}>
          {t(`pipeline.status.${rung.status}`)}
        </Badge>
      </p>
      <p className="t-caption">
        {reasonText(rung, t)}
        {/* Only when the report carried one. A line reading "checked" with
            nothing after it states a date this answer never gave. */}
        {rung.at !== null && rung.at !== undefined && (
          <>
            {" · "}
            {t("companyTriage.checkedAt", {
              when: formatDateTime(rung.at, locale, zone),
            })}
          </>
        )}
      </p>
    </li>
  );
}
