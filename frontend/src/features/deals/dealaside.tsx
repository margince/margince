// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { useEmployerSummary } from "../../components/contactemployersummary";
import { EntityRef, useEntityName } from "../../components/entityref";
import { Badge } from "../../design-system/atoms";
import { provenanceLabel } from "../../design-system/provenance";
import { provenanceOf } from "../../design-system/provenanceof";
import { FieldGuard } from "../../design-system/rbac";
import { RecordCard } from "../../design-system/recordcard";
import {
  type DetailRow,
  type DetailSection,
  RecordDetails,
} from "../../design-system/recorddetails";
import { COMPANY_LIFECYCLE_LABELS } from "../../format/companylifecycle";
import { formatDateAbbrev, formatMoneyWhole } from "../../format/format";
import { industryWords } from "../../format/industry";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useAcquisitionSources } from "../../lib/acquisitionsources.queries";
import { useListsAvailable, useRecordLists } from "../../lib/lists.queries";
import { useMemberName } from "../../lib/membernames";
import { useRecordZone } from "../../lib/recordzone";
import { routeHash } from "../../lib/router";
import { useViewerId } from "../../lib/session";
import { useRecordTags } from "../../lib/tags.queries";
import { closeDayOf } from "./closeday";
import {
  MOTION_OPTIONS,
  PRIORITY_OPTIONS,
} from "./deal360/dealcommercialfields";
import { forecastLabel } from "./deal360/dealheaderfacts";
import type { Deal, Stage } from "./deals";
import "./deal360/deal360.css";

type AsideProps = Readonly<{
  deal: Deal;
  // The pipeline's stages, so the details can name the one the deal is on.
  stages: readonly Stage[];
}>;

/**
 * The deal's details under its head: what it is, who of ours has it, how it
 * reached Margince, and the lists it is on. A field with no value folds behind
 * "Show N empty fields"; one that has a value is drawn in its group.
 */
export function DealAside(props: AsideProps) {
  // The server answers no lists for an archived record, so the read is not
  // even asked while lists are off or the deal is archived.
  const listsOn = useListsAvailable() && !props.deal.archived_at;
  return listsOn ? (
    <DealAsideWithLists {...props} />
  ) : (
    <DealAsideBody {...props} lists={[]} />
  );
}

function DealAsideWithLists(props: AsideProps) {
  const lists = useRecordLists("deal", props.deal.id).data?.data ?? [];
  return <DealAsideBody {...props} lists={lists} />;
}

function DealAsideBody({
  deal,
  stages,
  lists,
}: AsideProps & Readonly<{ lists: readonly { id: string; name: string }[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const facts: Facts = {
    deal,
    t,
    locale,
    zone: useRecordZone(),
    row: rowMaker(deal.masked_fields ?? [], t),
  };
  const owner = useMemberName(deal.owner_id).data;
  const reads = {
    viewerId: useViewerId(),
    sources: useAcquisitionSources().data ?? [],
    tags: useRecordTags("deal", deal.id, !deal.archived_at).data,
  };
  const sections: DetailSection[] = [
    {
      key: "about",
      label: t("record.details.about"),
      rows: aboutRows(facts, stages),
    },
    {
      key: "ownership",
      label: t("record.details.ownership"),
      rows: [
        facts.row("owner", "list.owner", owner, "owner_id"),
        {
          key: "partner",
          term: t("deal.aside.partner"),
          value: <DealPartner deal={deal} />,
        },
      ],
    },
    {
      key: "registration",
      label: t("record.registration"),
      rows: registrationRows(facts, reads),
    },
    ...listSection(lists, t),
  ];
  return (
    <RecordDetails
      sections={sections}
      footnote={t("deal.details.closingFootnote")}
    />
  );
}

type RowMaker = (
  key: string,
  term: MessageKey,
  value: ReactNode,
  maskKey?: string,
) => DetailRow;

type Facts = Readonly<{
  deal: Deal;
  t: Translator;
  locale: Locale;
  zone: string;
  row: RowMaker;
}>;

// A field the reader may not see says so; one nobody filled folds away.
function rowMaker(masked: readonly string[], t: Translator): RowMaker {
  return (key, term, value, maskKey = key) => {
    if (masked.includes(maskKey)) {
      return { key, term: t(term), value: <FieldGuard mode="masked" /> };
    }
    if (value === null || value === undefined || value === "") {
      return { key, term: t(term), value: t("field.unset"), empty: true };
    }
    return { key, term: t(term), value };
  };
}

function money(
  amountMinor: number | null | undefined,
  currency: string | null | undefined,
  locale: Locale,
): string | null {
  return amountMinor == null || !currency
    ? null
    : formatMoneyWhole(amountMinor, currency, locale);
}

function day(value: string | null | undefined, facts: Facts) {
  return value ? formatDateAbbrev(value, facts.locale, facts.zone) : null;
}

function aboutRows(facts: Facts, stages: readonly Stage[]): DetailRow[] {
  const { deal, t, locale, row } = facts;
  const account: DetailRow =
    deal.company_id && !deal.masked_fields?.includes("company_id")
      ? {
          key: "account",
          term: t("deal.aside.account"),
          value: <DealAccount companyId={deal.company_id} />,
          block: true,
        }
      : row("account", "deal.aside.account", null, "company_id");
  return [
    account,
    row(
      "value",
      "deals.amount",
      money(deal.amount_minor, deal.currency, locale),
      "amount_minor",
    ),
    row(
      "expected_arr",
      "deal.expectedArr",
      money(deal.expected_arr_minor, deal.currency, locale),
      "expected_arr_minor",
    ),
    row(
      "stage",
      "deals.stage",
      stages.find((stage) => stage.id === deal.stage_id)?.name,
    ),
    row(
      "close",
      "deal.standing.close",
      day(closeDayOf(deal), facts),
      "expected_close_date",
    ),
    row(
      "forecast",
      "deals.filterForecast",
      deal.forecast_category && forecastLabel(deal.forecast_category, t),
      "forecast_category",
    ),
    row("wait_until", "deal.waitUntil", day(deal.wait_until, facts)),
    row(
      "motion",
      "deal.motion",
      optionLabel(MOTION_OPTIONS, deal.commercial_motion, t),
      "commercial_motion",
    ),
    row(
      "priority",
      "deal.priority",
      optionLabel(PRIORITY_OPTIONS, deal.priority, t),
    ),
  ];
}

function registrationRows(
  facts: Facts,
  reads: Readonly<{
    viewerId: string | undefined;
    sources: readonly { key: string; label: string }[];
    tags: ReturnType<typeof useRecordTags>["data"];
  }>,
): DetailRow[] {
  const { deal, t, row } = facts;
  const tags =
    reads.tags && !reads.tags.withheld ? (reads.tags.data ?? []) : [];
  const source = deal.acquisition_source;
  return [
    row(
      "source",
      "history.field.source",
      provenanceLabel(
        provenanceOf(
          deal.captured_by,
          reads.viewerId,
          deal.author,
          deal.source,
          deal.imported_from,
        ),
        t,
        undefined,
      ),
    ),
    row(
      "acquisition",
      "deal.acquisitionSource",
      source &&
        (reads.sources.find((one) => one.key === source)?.label ?? source),
      "acquisition_source",
    ),
    row("created", "list.created", day(deal.created_at, facts)),
    row(
      "tags",
      "tags.panelTitle",
      tags.length > 0 ? (
        <span className="deal-aside-chips">
          {tags.map((tag) => (
            <Badge key={tag.tag_id}>{tag.name}</Badge>
          ))}
        </span>
      ) : null,
    ),
  ];
}

// The partner and what it did: a sourced deal earns commission, an influenced
// one does not, so the two must not read alike. No partner is an answer.
function DealPartner({ deal }: Readonly<{ deal: Deal }>) {
  const t = useT();
  const name = useEntityName("company", deal.partner_company_id).name;
  if (deal.masked_fields?.includes("partner_company_id")) {
    return <FieldGuard mode="masked" />;
  }
  if (!deal.partner_company_id) {
    return <>{t("deal.aside.partnerNone")}</>;
  }
  const role =
    deal.partner_attribution === "sourced"
      ? t("deal.attributionSourced")
      : deal.partner_attribution === "influenced"
        ? t("deal.attributionInfluenced")
        : undefined;
  return (
    <a
      className="deal-aside-link"
      href={routeHash({ screen: "companies", id: deal.partner_company_id })}
      title={role}
    >
      {[name ?? t("field.unset"), role].filter(Boolean).join(" · ")}
    </a>
  );
}

// A stored option in the reader's words; one this build does not list reads
// as itself.
function optionLabel(
  options: readonly { value: string; label: MessageKey }[],
  value: string | null | undefined,
  t: Translator,
): string | undefined {
  if (!value) {
    return undefined;
  }
  const option = options.find((one) => one.value === value);
  return option ? t(option.label) : value;
}

function listSection(
  lists: readonly { id: string; name: string }[],
  t: Translator,
): DetailSection[] {
  if (lists.length === 0) {
    return [];
  }
  return [
    {
      key: "lists",
      label: t("lists.record.title"),
      children: (
        <span className="deal-aside-chips">
          {lists.map((list) => (
            <a
              key={list.id}
              className="deal-aside-chip"
              href={routeHash({ screen: "lists", id: list.id })}
            >
              {list.name}
            </a>
          ))}
        </span>
      ),
    },
  ];
}

// The account is the same card as a contact in Way in. Mark and name are one
// link, its facts sit under them and its stage at the edge.
function DealAccount({ companyId }: Readonly<{ companyId: string }>) {
  const t = useT();
  const company = useEmployerSummary(companyId).data;
  const name = company?.display_name;
  const stage =
    company?.lifecycle && company.lifecycle !== "unknown"
      ? t(COMPANY_LIFECYCLE_LABELS[company.lifecycle])
      : undefined;
  const facts = [
    stage,
    company?.industry ? industryWords(t, company.industry) : undefined,
    company?.address?.city,
  ].filter((part): part is string => Boolean(part));
  if (!name) {
    return <EntityRef kind="company" id={companyId} />;
  }
  return (
    <RecordCard
      kind="company"
      name={name}
      identity={companyId}
      href={routeHash({ screen: "companies", id: companyId })}
      logo={company?.logo_url}
      position={facts.length > 0 ? facts.join(" · ") : undefined}
      aside={stage && <Badge wrap>{stage}</Badge>}
    />
  );
}
