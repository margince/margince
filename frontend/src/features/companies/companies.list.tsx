import { useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import { usePageName } from "../../app/pagemeta";
import { useBulkSelection } from "../../components/bulkverbs";
import {
  companyCreateFields,
  createCompany,
  RELATIONSHIP_TYPE_OPTIONS,
} from "../../components/companyform";
import {
  formatSizeBand,
  LIFECYCLE_LABELS,
  LIFECYCLE_OPTIONS,
  RELATIONSHIP_TYPE_LABELS,
  SIZE_BAND_OPTIONS,
} from "../../components/companylookups";
import { CreateAction } from "../../components/create";
import { useObjectCustomFields } from "../../components/customfields.form";
import {
  type ListPage,
  type ListQuery,
  ListTable,
  listFetchLimit,
  useListQuery,
  useTagChips,
} from "../../components/listquery";
import { useOwnerChips } from "../../components/ownerdials";
import {
  createdColumn,
  lastActivityColumn,
  mineEmptyLine,
  ownerColumn,
  standardViews,
  tagsColumn,
} from "../../components/recordlist";
import { Avatar, Badge, Button } from "../../design-system/atoms";
import type { ListChip } from "../../design-system/listsurface";
import { CellStrip, useListRefusal } from "../../design-system/listtable";
import { useToast } from "../../design-system/toast";
import { industryOptions, industryWords } from "../../format/industry";
import { useLocale, useT } from "../../i18n";
import { useCan } from "../../lib/capability";
import { labelFor } from "../../lib/labelfor";
import {
  KIND_LABELS,
  MEANING_LABELS,
  PARTNER_KINDS,
  PARTNER_STAGE_MEANINGS,
  usePartnerStages,
} from "../../lib/partnerstages.queries";
import { unwrap } from "../../lib/problem";
import { navigate } from "../../lib/router";
import { useViewerId } from "../../lib/session";
import { listQueryParams } from "../../lib/tagfilter";
import { useViewerZone } from "../../lib/viewerzone";
import { useActionRefusal } from "../../lib/writerefusal";
import {
  SavedViewsFailure,
  SaveViewAction,
  useSavedViewTabs,
} from "../lists/savedviews";
import type { Company } from "./companies";
import { DORMANCY_LABELS, DORMANCY_RUNGS } from "./companyactivity";
import { offerDomainContacts } from "./companycontacts/domaincontacts";
import { announceCompanyDuplicates } from "./companyduplicates";
import { displayHost } from "./companyheader";
import { partnerMarkText } from "./companymarks";
import { companyTypeLabel, useCompanyTypeDirectory } from "./companytypes";

// The stages of an account we deliver for, in funnel order. The Customers tab and
// the Lifecycle option that names them hold the same value, so the two agree.
const DELIVERING = ["onboarding", "live", "customer"].join(",");

// The address holds one stage, or several comma-joined. One goes as `status`,
// several as `status_any`. A value that names no stage is dropped, not sent.
function statusParams(joined: string | undefined) {
  const named = joined?.split(",") ?? [];
  const stages = LIFECYCLE_OPTIONS.filter((stage) => named.includes(stage));
  if (stages.length > 1) {
    return { status_any: stages };
  }
  return { status: stages[0] };
}

async function fetchCompaniesPage(
  query: ListQuery,
  cursor: string | null,
  signal?: AbortSignal,
): Promise<ListPage<Company>> {
  const { status, ...filters } = query.filters;
  const data = unwrap(
    await api.GET("/companies", {
      signal,
      params: {
        query: {
          q: query.q || undefined,
          sort: query.sort || undefined,
          include_archived: query.includeArchived || undefined,
          cursor: cursor || undefined,
          limit: listFetchLimit(query.perPage),
          ...listQueryParams(filters),
          ...statusParams(status),
        },
      },
    }),
  );
  return {
    data: data.data,
    page: {
      next_cursor: data.page.next_cursor ?? null,
      has_more: data.page.has_more,
      total: data.page.total,
    },
  };
}

export function CompaniesScreen({
  startCreating = false,
}: Readonly<{ startCreating?: boolean }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const toast = useToast();
  const pageName = usePageName("companies");
  const refusal = useListRefusal(
    useActionRefusal({ object: "company", action: "create" }),
  );
  const { locale } = useLocale();
  const readerZone = useViewerZone();
  const cf = useObjectCustomFields("company", "create");
  const state = useListQuery<Company>({
    key: "companies",
    initialSort: "-created_at",
    fetchPage: fetchCompaniesPage,
  });
  const selection = useBulkSelection({
    rows: state.rows,
    recordType: "company",
    labelOf: (company) => company.display_name,
  });
  // The owner dials name the reader, so they are offered only once /me has
  // answered. The table reads a chip valued "" as "clear this filter", so "My
  // companies" mid-load would narrow nothing. The deal list's owner chip
  // follows the same rule.
  const viewerId = useViewerId();
  const ownerChips = useOwnerChips();
  const tagChips = useTagChips();
  const extensionTypes = useCompanyTypeDirectory().data?.data ?? [];
  const savedViews = useSavedViewTabs("companies");
  // The partner dials read the partner register, which the server refuses a
  // seat without the partner grant.
  const readsPartners = useCan("partner", "read");
  const stages = usePartnerStages(readsPartners).data ?? [];
  const partnerChips: readonly ListChip[] = readsPartners
    ? [
        {
          key: "partner_kind",
          label: t("partner.kinds"),
          allLabel: t("partner.kindsAll"),
          options: PARTNER_KINDS.map((kind) => ({
            value: kind,
            label: t(labelFor(KIND_LABELS, kind)),
          })),
        },
        {
          key: "partner_stage",
          label: t("partner.stage"),
          allLabel: t("partner.stageAll"),
          options: stages.map((stage) => ({
            value: stage.key,
            label: stage.label,
          })),
        },
        {
          key: "partner_stage_meaning",
          label: t("partner.stageMeaning"),
          allLabel: t("partner.stageMeaningAll"),
          options: PARTNER_STAGE_MEANINGS.map((meaning) => ({
            value: meaning,
            label: t(labelFor(MEANING_LABELS, meaning)),
          })),
        },
      ]
    : [];
  // Beside the owner dial rather than in `chips`, because of the option labels.
  // A `chips` entry runs every label through `t()`, so its options must be
  // message keys. A size band is a numeral range that reads the same in
  // English, German and Vietnamese. Seven keys whose three translations all
  // coincide would be noise standing in for meaning. The dial's own name still
  // needs translating, which is what `t()` is doing here.
  const sizeChip: readonly ListChip[] = [
    {
      key: "size_band",
      label: t("company.sizeBand"),
      allLabel: t("company.filterSizeBandAll"),
      options: SIZE_BAND_OPTIONS.map((value) => ({
        value,
        label: formatSizeBand(t, value),
      })),
    },
    {
      key: "industry",
      label: t("company.industry"),
      allLabel: t("company.filterIndustryAll"),
      options: industryOptions(t, locale),
    },
  ];

  return (
    <div className="wrap">
      <ListTable
        title={pageName}
        state={state}
        unit="unit.companies"
        unitOne="unit.company"
        emptyLine={mineEmptyLine({
          t,
          state,
          viewerId,
          unit: "unit.companies",
        })}
        refusal={refusal}
        action={
          <>
            <Button onClick={() => navigate({ screen: "partners" })}>
              {t("nav.partners")}
            </Button>
            <CreateAction
              label={t("create.company")}
              requires={{ object: "company", action: "create" }}
              refusalId={refusal?.id}
              invalidate="companies"
              screen="companies"
              listCreate={{ open: startCreating }}
              create={(values, rows, key) =>
                createCompany(values, rows, cf.toBody(values), t, key)
              }
              resolveExisting={(_code, id) => ({ screen: "companies", id })}
              onCreated={(company) => {
                offerDomainContacts(queryClient, company);
                void announceCompanyDuplicates(
                  queryClient,
                  toast,
                  t,
                  company.id,
                );
              }}
              fields={[...companyCreateFields(t, locale), ...cf.formFields]}
            />
          </>
        }
        columns={[
          {
            key: "name",
            header: t("company.name"),
            cell: (company: Company) => (
              <span className="avatar-row">
                <Avatar
                  identity={company.id}
                  name={company.display_name}
                  src={company.logo_url}
                />
                <strong title={company.display_name}>
                  {company.display_name}
                </strong>
                {company.archived_at && (
                  <Badge tone="warning">{t("record.archived")}</Badge>
                )}
              </span>
            ),
            sort: "display_name",
            fixed: true,
          },
          {
            // What the company does, in its own words from its own site.
            key: "description",
            header: t("company.description"),
            cell: (company: Company) => company.description ?? "",
            sort: "description",
            initiallyHidden: true,
          },
          {
            // Unsorted, as the server orders neither: a code's alphabet is not
            // the reader's, and a size band's text is not its size.
            key: "industry",
            header: t("company.industry"),
            initiallyHidden: true,
            cell: (company: Company) =>
              company.industry ? industryWords(t, company.industry) : null,
          },
          {
            key: "employees",
            header: t("company.employees"),
            initiallyHidden: true,
            cell: (company: Company) =>
              company.size_band ? formatSizeBand(t, company.size_band) : null,
          },
          { ...tagsColumn<Company>(t), initiallyHidden: true },
          {
            key: "website",
            initiallyHidden: true,
            header: t("company.website"),
            // Sorted by the host the cell prints; the scheme is a constant.
            sort: "website_url",
            phone: "caption",
            folded: (company: Company) =>
              company.website_url ? displayHost(company.website_url) : null,
            cell: (company: Company) =>
              company.website_url ? (
                <a
                  href={company.website_url}
                  target="_blank"
                  rel="noreferrer noopener"
                  onClick={(e) => e.stopPropagation()}
                >
                  {displayHost(company.website_url)}
                </a>
              ) : null,
          },
          {
            // How many contacts work here that this reader may see. The server
            // counts under the caller's row scope. Zero is a number: a reader
            // must tell "no contacts" from "not shown".
            key: "contacts",
            header: t("company.contactCount"),
            initiallyHidden: true,
            numeric: true,
            cell: (company: Company) => company.contact_count ?? "",
            // A reader shown no count is ordered by none: such rows go last.
            sort: "contact_count",
          },
          {
            // Withheld (absent key), not zero, for a role without
            // computed_field:read. The company page's pipeline tile follows
            // the same withheld-state rule, so the two never disagree.
            key: "openDeals",
            header: t("company.openDealCount"),
            numeric: true,
            cell: (company: Company) => company.open_deal_count ?? "",
            sort: "open_deal_count",
          },
          {
            key: "class",
            header: t("company.lifecycle"),
            sort: "status",
            phone: "end",
            // Reads status, because nothing writes classification. A column
            // on classification would show stale values forever.
            cell: (company: Company) =>
              company.status && company.status !== "unknown" ? (
                <Badge>{t(labelFor(LIFECYCLE_LABELS, company.status))}</Badge>
              ) : null,
          },
          {
            key: "relationship",
            header: t("company.relationshipTypes"),
            // No `sort`: an account can be a partner and a customer. Ordering
            // by the first member of a set would read as an answer without
            // being one, so the filter above narrows instead.
            //
            // The column lets a reader see why a row matched that filter. It
            // is multi-valued (ADR-0079), because showing only the first type
            // would make the second look untrue.
            cell: (company: Company) =>
              company.relationship_types?.length ||
              company.extension_relationship_types?.length ? (
                <CellStrip>
                  {(company.relationship_types ?? []).map((type) => (
                    <Badge key={type}>
                      {type === "partner"
                        ? partnerMarkText(company, t)
                        : t(labelFor(RELATIONSHIP_TYPE_LABELS, type))}
                    </Badge>
                  ))}
                  {(company.extension_relationship_types ?? []).map((type) => (
                    <Badge key={type}>
                      {companyTypeLabel(type, extensionTypes)}
                    </Badge>
                  ))}
                </CellStrip>
              ) : null,
          },
          ownerColumn<Company>(t),
          lastActivityColumn<Company>(t, locale, readerZone),
          createdColumn<Company>(t, locale, readerZone),
        ]}
        saveView={<SaveViewAction resource="companies" state={state} />}
        notice={<SavedViewsFailure resource="companies" />}
        rowKey={(company) => company.id}
        rowRoute={(company) => ({ screen: "companies", id: company.id })}
        selection={selection}
        dataChips={[...ownerChips, ...sizeChip, ...partnerChips, ...tagChips]}
        chips={[
          {
            key: "status",
            label: "company.lifecycle",
            allLabel: "company.filterLifecycleAll",
            options: [
              ...LIFECYCLE_OPTIONS.map((value) => ({
                value,
                label: labelFor(LIFECYCLE_LABELS, value),
              })),
              {
                value: DELIVERING,
                label: "company.filterLifecycleDelivering" as const,
              },
            ],
          },
          {
            key: "relationship_type",
            label: "company.relationshipTypes",
            allLabel: "company.filterRelTypeAll",
            options: [
              ...RELATIONSHIP_TYPE_OPTIONS.map((value) => ({
                value,
                label: labelFor(RELATIONSHIP_TYPE_LABELS, value),
              })),
              // An option value holding a colon reads as `param:value`. The
              // extension filter is its own parameter, so naming it keeps the
              // whole `ext:<unit>:<type>` value.
              ...extensionTypes.map(({ value, label }) => ({
                value: `extension_relationship_type:${value}`,
                text: label,
              })),
            ],
          },
          {
            // The server's dormancy ladder, read from each company's last
            // activity by the rule the company page uses.
            key: "dormancy",
            label: "company.dormancy",
            allLabel: "company.filterDormancyAll",
            options: DORMANCY_RUNGS.map((value) => ({
              value,
              label: labelFor(DORMANCY_LABELS, value),
            })),
          },
        ]}
        dataViews={savedViews}
        // The two cuts carry no sort, so the reader's own order survives the
        // change of tab.
        views={[
          ...standardViews(viewerId),
          { label: "list.viewCustomers", filters: { status: DELIVERING } },
          { label: "list.viewProspects", filters: { status: "prospect" } },
        ]}
      />
    </div>
  );
}
