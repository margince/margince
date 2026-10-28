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
  mineEmptyNote,
  ownerColumn,
  standardViews,
  tagsColumn,
} from "../../components/recordlist";
import { Avatar, Badge, Button } from "../../design-system/atoms";
import type { ListChip } from "../../design-system/listsurface";
import { CellStrip } from "../../design-system/listtable";
import { useLocale, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { navigate } from "../../lib/router";
import { useViewerId } from "../../lib/session";
import { listQueryParams } from "../../lib/tagfilter";
import { SaveViewAction, useSavedViewTabs } from "../lists/savedviews";
import type { Company } from "./companies";
import { offerDomainContacts } from "./companycontacts/domaincontacts";
import { displayHost } from "./companyheader";

async function fetchCompaniesPage(
  query: ListQuery,
  cursor: string | null,
): Promise<ListPage<Company>> {
  const data = unwrap(
    await api.GET("/companies", {
      params: {
        query: {
          q: query.q || undefined,
          sort: query.sort || undefined,
          include_archived: query.includeArchived || undefined,
          cursor: cursor || undefined,
          limit: listFetchLimit(query.perPage),
          ...listQueryParams(query.filters),
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

export function CompaniesScreen() {
  const t = useT();
  const pageName = usePageName("companies");
  const { locale } = useLocale();
  const recordZone = useRecordZone();
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
  const savedViews = useSavedViewTabs("companies");
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
  ];

  return (
    <div className="wrap">
      <ListTable
        title={pageName}
        state={state}
        unit="unit.companies"
        emptyNote={mineEmptyNote({
          t,
          state,
          viewerId,
          unit: "unit.companies",
        })}
        action={
          <>
            <Button onClick={() => navigate({ screen: "partners" })}>
              {t("nav.partners")}
            </Button>
            <CreateAction
              label={t("create.company")}
              invalidate="companies"
              screen="companies"
              create={(values, rows, key) =>
                createCompany(values, rows, cf.toBody(values), t, key)
              }
              resolveExisting={(_code, id) => ({ screen: "companies", id })}
              onCreated={offerDomainContacts}
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
            // What the company does, in its own words from its own site. It
            // stands in for industry and size: imports leave size_band null for
            // every company and industry for most, so those columns sit empty.
            key: "description",
            header: t("company.description"),
            cell: (company: Company) => company.description ?? "",
            sort: "description",
          },
          tagsColumn<Company>(t),
          {
            key: "website",
            header: t("company.website"),
            // Sorted by the host the cell prints; the scheme is a constant.
            sort: "website_url",
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
            sort: "lifecycle",
            // Reads lifecycle, because nothing writes classification. A column
            // on classification would show stale values forever.
            cell: (company: Company) =>
              company.lifecycle && company.lifecycle !== "unknown" ? (
                <Badge>{t(LIFECYCLE_LABELS[company.lifecycle])}</Badge>
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
              company.relationship_types?.length ? (
                <CellStrip>
                  {company.relationship_types.map((type) => (
                    <Badge key={type}>
                      {t(RELATIONSHIP_TYPE_LABELS[type])}
                    </Badge>
                  ))}
                </CellStrip>
              ) : null,
          },
          ownerColumn<Company>(t),
          lastActivityColumn<Company>(t, locale, recordZone),
          createdColumn<Company>(t, locale, recordZone),
        ]}
        saveView={<SaveViewAction resource="companies" query={state.query} />}
        rowKey={(company) => company.id}
        rowRoute={(company) => ({ screen: "companies", id: company.id })}
        selection={selection}
        dataChips={[...ownerChips, ...sizeChip, ...tagChips]}
        chips={[
          {
            key: "lifecycle",
            label: "company.lifecycle",
            allLabel: "company.filterLifecycleAll",
            options: LIFECYCLE_OPTIONS.filter(
              (value) => value !== "unknown",
            ).map((value) => ({ value, label: LIFECYCLE_LABELS[value] })),
          },
          {
            key: "relationship_type",
            label: "company.relationshipTypes",
            allLabel: "company.filterRelTypeAll",
            options: RELATIONSHIP_TYPE_OPTIONS.map((value) => ({
              value,
              label: RELATIONSHIP_TYPE_LABELS[value],
            })),
          },
        ]}
        dataViews={savedViews}
        views={[
          ...standardViews(viewerId),
          {
            label: "list.viewCustomers",
            sort: "display_name",
            filters: { lifecycle: "customer" },
          },
          {
            label: "list.viewProspects",
            sort: "display_name",
            filters: { lifecycle: "prospect" },
          },
        ]}
      />
    </div>
  );
}
