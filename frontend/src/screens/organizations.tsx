import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { watchStartedAiRun } from "../app/ai-activity";
import { PageAsideToggle, usePageAside } from "../app/pageaside";
import { usePageName } from "../app/pagemeta";
import { useRecordZone } from "../app/recordzone";
import { navigate, useRoute } from "../app/router";
import {
  Avatar,
  Badge,
  Button,
  Card,
  EmptyState,
  Modal,
  Skeleton,
} from "../design-system/atoms";
import {
  RecordView,
  type TimelineEntry,
  type TimelineGroup,
} from "../design-system/composed";
import type { ListChip } from "../design-system/listsurface";
import { OpenEmailDrawer } from "../design-system/openemaildrawer";
import { OverlayFallback } from "../design-system/overlayfallback";
import { Panel, PanelBody } from "../design-system/panel";
import { liveProjects } from "../design-system/projectpicker";
import { RecordTabs } from "../design-system/recordtabs";
import {
  hasTimelineFilters,
  useRecordTimeline,
  useTimelineFilters,
} from "../design-system/recordtimeline";
import { sectionState } from "../design-system/surfacestate";
import { TimelineFilterBar } from "../design-system/timelinefilterbar";
import { AutonomyDot } from "../design-system/trust";
import { formatDateTime, formatMoney, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { taskWriteKeys } from "./activitykeys";
import { AssistantPanel } from "./assistant";
import {
  coldFieldLabel,
  problemMessageOf,
  QueryGate,
  throwProblem,
  useSorMode,
  useViewerId,
} from "./common";
import { MoneyPane, PeopleChips, ThreadFold } from "./company/glance";
import {
  DealsCard,
  NextSteps,
  type Org360Result,
  recordNamesIn,
  StateStrip,
  type SuggestionAction,
  useAcknowledgeOrganizationView,
  useOrganization360,
} from "./company360";
import { NewDealAction } from "./companyactions";
import { CompanyApprovalsPanel } from "./companyapprovals";
import { CompanyContractState, CompanyLastOffer } from "./companycommercial";
import { CompanyContractsCard } from "./companycontracts";
import { CompanyDocumentsCard } from "./companydocuments";
import { DossierPanel } from "./companydossier";
import { type CitedRecord, EvidenceModal } from "./companyevidence";
import { CompanyFinanceCard, hasFinance } from "./companyfinance";
import { GrowthFitPanel } from "./companygrowthfit";
import {
  CompanyActionBadges,
  CompanyIdentityLine,
  CompanyLifecycleControl,
  CompanyPrimaryActions,
  CompanyRelationshipBadges,
  displayHost,
} from "./companyheader";
import {
  LIFECYCLE_LABELS,
  LIFECYCLE_OPTIONS,
  RELATIONSHIP_TYPE_LABELS,
  SIZE_BAND_OPTIONS,
} from "./companylookups";
import { CompanyPeopleList } from "./companypeople/contacts";
import { CoverageBand } from "./companypeople/summary";
import { CompanyProfileForm } from "./companyprofiletab";
import { CompanyRail, SignalsSection } from "./companyrail";
import { wholeCount } from "./companyrailshared";
import {
  COMPANY_TABS,
  type CompanyTab,
  companyTabRoute,
  isCompanyTab,
} from "./companytab";
import { TechnicalProfileCard } from "./companytechnical";
import { Company360Call, NeedsList, useTodayReading } from "./companytoday";
import { hasWorkInFlight, sinceLastVisitFooter } from "./companywork";
import { ComposeModal } from "./compose";
import {
  CreateAction,
  type CreateField,
  type FormRows,
  splitMultiselectValue,
} from "./create";
import { CustomFieldsCard } from "./customfields.card";
import { useObjectCustomFields } from "./customfields.form";
import { useRoster } from "./entityref";
import { RecordHistoryTab } from "./history";
import {
  type ListPage,
  type ListQuery,
  ListTable,
  listFetchLimit,
  useListQuery,
  useOwnerChips,
  useTagChips,
} from "./listquery";
import { PersonMeetingBrief } from "./meetingbrief";
import { useOpenEmail } from "./openemail";
import { PartnerTab } from "./partners";
import { RecordSpine } from "./record360";
import {
  ChronologyFilter,
  ChronologyFooter,
  chronologyNotice,
  useChronologyFilter,
  useRecordChronology,
} from "./recordchronology";
import { ConversationList } from "./recordconversations";
import {
  createdColumn,
  lastActivityColumn,
  mineEmptyNote,
  ownerColumn,
  standardViews,
  tagsColumn,
} from "./recordlist";
import { RelationshipsTab } from "./relationships";
import { SaveViewAction, useSavedViewTabs } from "./savedviews";
import { listQueryParams } from "./tagfilter";
import {
  TaskDetailModal,
  TaskQuickActions,
  useTaskUpdate,
} from "./taskactions";
import { TimelineActions } from "./timelineactions";
import { groupChronology } from "./timelinegroups";
// The row and card shapes this file draws — co-rowlink, co-row-meta, co-card —
// are defined in company360.css. Imported HERE rather than left to the caller:
// it works today only because the company record page pulls that stylesheet in
// for its own sake, so this file renders unstyled anywhere else.
import "./company360.css";
import { useAccountScan } from "./accountscan";
import { LogActivityAction } from "./logactivity";
import { invalidateRecord } from "./recordwritekeys";

// Companies list + company 360 (B-EP09.10a/b). Firmographics render
// evidence-or-omit: a field with no stored value is absent, never guessed.
// Search/filter/sort/pagination (P-14), the rich create modal (P-15), the
// If-Match edit form (P-1), and the dedupe view-existing link (P-16) are
// wired in here the same way as contacts (people.tsx) — the enrich flow,
// firmographics card, and timeline stay exactly as they were.

type Organization = components["schemas"]["Organization"];

// Where the account stands with us (ADR-0079/A124), in the words a reader
// sees. Lives in companylookups.ts, the leaf both this screen and the rail
// import, so the two cannot drift onto two different label sets for the same
// enum. Re-exported: every existing caller of `LIFECYCLE_LABELS` from this
// module still resolves, and this file still reads it below as its own.
// What it is TO US, multi-valued (ADR-0079/A124). Moved beside
// LIFECYCLE_LABELS in companylookups.ts because the two vocabularies OVERLAP —
// `customer` is a member of both — and only a module holding both can tell
// that the header is about to print one word twice. Re-exported for the same
// reason LIFECYCLE_LABELS is: every existing caller still resolves.
export { LIFECYCLE_LABELS, RELATIONSHIP_TYPE_LABELS };

type CreateOrganizationRequest =
  components["schemas"]["CreateOrganizationRequest"];
type UpdateOrganizationRequest =
  components["schemas"]["UpdateOrganizationRequest"];
type Organization360View = components["schemas"]["Organization360"];

// Lives in companylookups.ts, same reason as LIFECYCLE_LABELS above: the
// rail's Details grid (companyraildetails.tsx) builds a size-band picker off
// the same seven wire bands, and a second copy here is the value neither
// screen's TypeScript catches drifting. Re-exported for the same reason too:
// every existing caller of `SIZE_BAND_OPTIONS` from this module still
// resolves.
export { SIZE_BAND_OPTIONS };

async function fetchOrganizationsPage(
  query: ListQuery,
  cursor: string | null,
): Promise<ListPage<Organization>> {
  const { data, error } = await api.GET("/organizations", {
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
  });
  if (error) {
    throwProblem(error);
  }
  return {
    data: data.data,
    page: {
      next_cursor: data.page.next_cursor ?? null,
      has_more: data.page.has_more,
    },
  };
}

function stringField(value: unknown): string {
  return typeof value === "string" ? value : "";
}

// Merge-target search (P-2): mirrors searchPeopleTargets (people.tsx) — the
// caller filters out the source row.
export async function searchOrgTargets(
  q: string,
): Promise<{ id: string; name: string }[]> {
  const { data, error } = await api.GET("/organizations", {
    params: { query: { q, limit: 10 } },
  });
  if (error) {
    throwProblem(error);
  }
  return data.data.map((candidate) => ({
    id: candidate.id,
    name: candidate.display_name,
  }));
}

function asSizeBand(
  value: string | undefined,
): CreateOrganizationRequest["size_band"] {
  return (SIZE_BAND_OPTIONS as readonly string[]).includes(value ?? "")
    ? (value as CreateOrganizationRequest["size_band"])
    : undefined;
}

// The repeatable `domains` rows → the wire `domains[]` shape, shared by the
// create body and the edit patch: blank rows drop out, the domain lowercases,
// and the row's primary radio (a string "true"/"") becomes the boolean flag.
// An empty result is `undefined` — on create that means "no domains", on
// update the field is omitted so the stored set stays untouched (never
// silently cleared).
function mapDomainRows(rows: FormRows): CreateOrganizationRequest["domains"] {
  const domains = mapDomainRowsReplaceSet(rows);
  return domains.length > 0 ? domains : undefined;
}

type DomainPatch = NonNullable<UpdateOrganizationRequest["domains"]>;

// The edit-patch form of the repeatable domains field: always the concrete
// desired set (possibly empty), so a caller can send [] to clear every domain.
// Blank rows drop; the primary radio ("true"/"") becomes the boolean flag.
function mapDomainRowsReplaceSet(rows: FormRows): DomainPatch {
  return (rows.domains ?? [])
    .filter((row) => (row.domain ?? "").trim().length > 0)
    .map((row) => ({
      domain: row.domain.trim().toLowerCase(),
      is_primary: row.is_primary === "true",
    }));
}

// Order-independent set equality: an edit that leaves the domains untouched
// omits the field (sparse PATCH), while any real change — including clearing
// to empty — sends the replace-set.
function sameDomainSet(a: DomainPatch, b: DomainPatch): boolean {
  if (a.length !== b.length) {
    return false;
  }
  const key = (d: DomainPatch[number]) => `${d.domain}:${d.is_primary ? 1 : 0}`;
  const seen = new Set(a.map(key));
  return b.every((d) => seen.has(key(d)));
}

// Builds the create-company request body: `domains[]` rows carry
// `{domain, is_primary}` keyed off the repeatable rows channel, scalar
// fields trim to undefined when blank.
export function mapOrgBody(
  values: Record<string, string>,
  rows: FormRows,
): CreateOrganizationRequest {
  return {
    display_name: values.display_name.trim(),
    legal_name: values.legal_name?.trim() || undefined,
    industry: values.industry?.trim() || undefined,
    size_band: asSizeBand(values.size_band),
    domains: mapDomainRows(rows),
    source: "manual",
  };
}

// Builds the PATCH body: the scalar UpdateOrganizationRequest fields plus the
// domains replace-set from the edit modal's repeatable rows. Domains are sent
// only when the set actually changed from `currentDomains` — an untouched edit
// omits the field (sparse PATCH), and clearing every row sends [] (clear all),
// the two cases the contract's "absent = untouched" vs "[] = clear" distinguish.
export function mapOrgUpdate(
  values: Record<string, unknown>,
  rows: FormRows,
  currentDomains: Organization["domains"] = [],
): UpdateOrganizationRequest {
  const desired = mapDomainRowsReplaceSet(rows);
  const current: DomainPatch = (currentDomains ?? []).map((domain) => ({
    domain: domain.domain,
    is_primary: domain.is_primary,
  }));
  const body: UpdateOrganizationRequest = {
    display_name: stringField(values.display_name).trim() || undefined,
    legal_name: stringField(values.legal_name).trim() || undefined,
    industry: stringField(values.industry).trim() || undefined,
    size_band: asSizeBand(stringField(values.size_band)),
    owner_id: stringField(values.owner_id).trim() || undefined,
  };
  if (!sameDomainSet(desired, current)) {
    body.domains = desired;
  }
  const lifecycle = stringField(values.lifecycle).trim();
  if (lifecycle) {
    body.lifecycle = lifecycle as NonNullable<
      UpdateOrganizationRequest["lifecycle"]
    >;
  }
  // Always sent when the field was rendered, even empty: this is a replace-set,
  // and "the user cleared every type" is an edit, not an absence. The form
  // channel joins a multiselect into one comma string, so an empty string is
  // the honest empty set.
  if (values.relationship_types !== undefined) {
    body.relationship_types = splitMultiselectValue(
      stringField(values.relationship_types),
    ) as NonNullable<UpdateOrganizationRequest["relationship_types"]>;
  }
  // Nullable rather than trim-to-undefined, and for the same reason the
  // relationship set is: clearing a LinkedIn URL is an edit. `|| undefined`
  // would read a deletion as "the caller did not mention it" and put the old
  // value straight back.
  if (values.linkedin_url !== undefined) {
    body.linkedin_url = stringField(values.linkedin_url).trim() || null;
  }
  const address = addressPatch(values);
  if (address) {
    body.address = address;
  }
  return body;
}

// The six columns behind Address, flattened into form fields. The wire shape is
// one nested object; the form channel is flat string values, so the two are
// mapped at the boundary (addressFrom / addressPatch) rather than teaching the
// form about nesting for one record type.
const ADDRESS_FIELDS: CreateField[] = [
  { key: "address_line1", label: "create.addressLine1" },
  { key: "address_line2", label: "create.addressLine2" },
  { key: "address_postal_code", label: "create.postalCode" },
  { key: "address_city", label: "create.city" },
  { key: "address_region", label: "create.region" },
  { key: "address_country", label: "create.country" },
];

// addressFrom prefills the six flat fields from the record's nested address.
export function addressFrom(
  address: Organization["address"],
): Record<string, string> {
  return {
    address_line1: address?.line1 ?? "",
    address_line2: address?.line2 ?? "",
    address_postal_code: address?.postal_code ?? "",
    address_city: address?.city ?? "",
    address_region: address?.region ?? "",
    address_country: address?.country ?? "",
  };
}

// addressPatch folds the six flat fields back into the wire's nested object.
//
// A cleared field is sent as null rather than omitted: the caller had the value
// on screen and erased it, which is an edit. Omitting it would silently keep
// what the record held — the failure mode where a user deletes a line, saves,
// and finds it back on reload.
//
// The whole object is omitted only when the form never rendered the fields at
// all, so a surface that does not offer the address cannot blank one.
function addressPatch(
  values: Record<string, unknown>,
): UpdateOrganizationRequest["address"] | undefined {
  if (values.address_line1 === undefined) {
    return undefined;
  }
  const field = (key: string) => stringField(values[key]).trim() || null;
  return {
    line1: field("address_line1"),
    line2: field("address_line2"),
    postal_code: field("address_postal_code"),
    city: field("address_city"),
    region: field("address_region"),
    // ISO-3166 alpha-2, and the server compares on the canonical spelling, so
    // "de" typed in lower case is the same country as "DE".
    country: stringField(values.address_country).trim().toUpperCase() || null,
  };
}

const companyCreateFields: CreateField[] = [
  { key: "display_name", label: "create.displayName", required: true },
  { key: "legal_name", label: "create.legalName" },
  { key: "industry", label: "create.industry" },
  {
    key: "size_band",
    label: "create.sizeBand",
    type: "select",
    options: SIZE_BAND_OPTIONS.map((band) => ({ value: band, label: band })),
  },
  {
    key: "domains",
    label: "org.domains",
    type: "repeatable",
    addLabel: "field.addDomain",
    rowFields: [{ key: "domain", label: "field.domain", required: true }],
    primaryKey: "is_primary",
  },
];

// The edit form, built per-render because the owner options are the live user
// roster.
//
// Stage and relationship types ARE here now: the retired classification could
// not be edited from anywhere, because the update contract carried no such
// field.
// Where the account stands with us: lives in companylookups.ts, same reason
// as LIFECYCLE_LABELS and SIZE_BAND_OPTIONS above — the rail's Details grid
// builds a lifecycle picker off the same wire order, and a second copy here
// is the value neither screen's TypeScript catches drifting. Re-exported so
// every existing caller of `LIFECYCLE_OPTIONS` from this module still
// resolves.
export { LIFECYCLE_OPTIONS };

// What it is to us has no rail counterpart today, so it stays local.
export const RELATIONSHIP_TYPE_OPTIONS = [
  "customer",
  "partner",
  "supplier",
  "investor",
  "portfolio_company",
  "competitor",
  "other",
] as const;

// t is threaded in because the option LABELS are catalog keys, not words: the
// field renderer prints option.label as given, so an untranslated key reaches
// the reader as "org.lifecycle.customer".
export function companyEditFields(
  owners: readonly { id: string; display_name: string }[],
  hasOwner: boolean,
  t: (key: MessageKey) => string,
): CreateField[] {
  return [
    { key: "display_name", label: "create.displayName", required: true },
    { key: "legal_name", label: "create.legalName" },
    { key: "industry", label: "create.industry" },
    {
      key: "size_band",
      label: "create.sizeBand",
      type: "select",
      options: SIZE_BAND_OPTIONS.map((band) => ({ value: band, label: band })),
    },
    // Who is accountable for this account. It defaults to whoever created the
    // record and stays there until someone changes it — which, until now,
    // nothing on this page let them do.
    //
    // Required exactly when the account HAS an owner: an optional select
    // offers a blank option, and `UpdateOrganizationRequest.owner_id` cannot
    // carry "unassign" — a null is indistinguishable from an omitted field on
    // the wire. Offering the blank would take the answer and drop it. An
    // account with no owner yet keeps the blank, because there it is the
    // truthful current state rather than an edit we cannot make.
    {
      key: "owner_id",
      label: "co.pulse.owner",
      type: "select",
      required: hasOwner,
      options: owners.map((user) => ({
        value: user.id,
        label: user.display_name,
      })),
    },
    // Where the account stands, and what it is to us — the two questions the
    // retired classification tried to answer with one value, and the reason
    // neither was editable from this page at all.
    {
      key: "lifecycle",
      label: "org.lifecycle",
      type: "select",
      options: LIFECYCLE_OPTIONS.map((value) => ({
        value,
        label: t(LIFECYCLE_LABELS[value]),
      })),
    },
    {
      key: "relationship_types",
      label: "org.relationshipTypes",
      type: "multiselect",
      options: RELATIONSHIP_TYPE_OPTIONS.map((value) => ({
        value,
        label: t(RELATIONSHIP_TYPE_LABELS[value]),
      })),
    },
    // The company's own LinkedIn page. A canonical column since ADR-0085/A130,
    // not a custom field, because it carries identity semantics — matching,
    // dedupe, enrichment — and the person side already treats it that way. The
    // server normalizes what is pasted, so a URL copied from any tab of the
    // company page resolves to the one spelling.
    { key: "linkedin_url", label: "create.linkedinUrl" },
    // Where the company actually is. It has been in the API since the record
    // existed and reachable from no form on this page, so a rep who knew the
    // address had nowhere to put it.
    ...ADDRESS_FIELDS,
    {
      key: "domains",
      label: "org.domains",
      type: "repeatable",
      addLabel: "field.addDomain",
      rowFields: [{ key: "domain", label: "field.domain", required: true }],
      primaryKey: "is_primary",
    },
  ];
}

async function createCompany(
  values: Record<string, string>,
  rows: FormRows | undefined,
  customFields: Record<string, unknown>,
  t: (key: MessageKey) => string,
): Promise<Organization> {
  const { data, error } = await api.POST("/organizations", {
    body: { ...mapOrgBody(values, rows ?? {}), ...customFields },
  });
  if (error) {
    throwProblem(error, t);
  }
  return data;
}

export function CompaniesScreen() {
  const t = useT();
  const pageName = usePageName("companies");
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const cf = useObjectCustomFields("organization");
  const state = useListQuery<Organization>({
    key: "organizations",
    initialSort: "-created_at",
    fetchPage: fetchOrganizationsPage,
  });
  // The owner dials name the reader, so they are offered only once /me has
  // answered. A chip whose value is still "" reads as "clear this filter" to
  // the table, so offering "My companies" mid-load would quietly narrow
  // nothing — the same reason the deal list builds its owner chip this way.
  const viewerId = useViewerId();
  const ownerChips = useOwnerChips();
  const tagChips = useTagChips();
  const savedViews = useSavedViewTabs("organizations");
  // Beside the owner dial rather than in `chips`, and the reason is the option
  // labels. A `chips` entry runs every label through `t()`, so its options must
  // be message keys — and a size band is a numeral range that reads identically
  // in English, German and Vietnamese. Seven keys whose three translations all
  // coincide would be noise standing in for meaning. The dial's own name still
  // needs translating, which is what `t()` is doing here.
  const sizeChip: readonly ListChip[] = [
    {
      key: "size_band",
      label: t("org.sizeBand"),
      allLabel: t("org.filterSizeBandAll"),
      options: SIZE_BAND_OPTIONS.map((value) => ({ value, label: value })),
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
            <Button small onClick={() => navigate({ screen: "partners" })}>
              {t("nav.partners")}
            </Button>
            <CreateAction
              label={t("create.company")}
              invalidate="organizations"
              screen="companies"
              create={(values, rows) =>
                createCompany(values, rows, cf.toBody(values), t)
              }
              resolveExisting={(_code, id) => ({ screen: "companies", id })}
              fields={[...companyCreateFields, ...cf.formFields]}
            />
          </>
        }
        columns={[
          {
            key: "name",
            header: t("org.name"),
            cell: (org: Organization) => (
              <span className="avatar-row">
                <Avatar
                  identity={org.id}
                  name={org.display_name}
                  src={org.logo_url}
                  shape="organization"
                />
                <strong>{org.display_name}</strong>
                {org.archived_at && (
                  <Badge tone="warn">{t("record.archived")}</Badge>
                )}
              </span>
            ),
            sort: "display_name",
            fixed: true,
          },
          {
            // What the company DOES, in their own words from their own site.
            // This replaced industry and size: in a real import size_band was
            // null for every company and industry for most, so the list's two
            // widest columns were reliably empty.
            key: "description",
            header: t("org.description"),
            cell: (org: Organization) => org.description ?? "",
          },
          tagsColumn<Organization>(t),
          {
            key: "website",
            header: t("org.website"),
            cell: (org: Organization) =>
              org.website_url ? (
                <a
                  href={org.website_url}
                  target="_blank"
                  rel="noreferrer noopener"
                  onClick={(e) => e.stopPropagation()}
                >
                  {displayHost(org.website_url)}
                </a>
              ) : null,
          },
          {
            // AC-companies-2/3: how many people work here that this reader may
            // see — the server counts under the caller's row scope. Zero is a
            // number: a reader must tell "no contacts" from "not shown".
            key: "contacts",
            header: t("org.contactCount"),
            numeric: true,
            cell: (org: Organization) => org.contact_count ?? "",
          },
          {
            // Withheld (absent key), not zero, for a role without
            // computed_field:read — the same STATE-4 rule the company page's
            // pipeline tile follows, so the two never disagree.
            key: "openDeals",
            header: t("org.openDealCount"),
            numeric: true,
            cell: (org: Organization) => org.open_deal_count ?? "",
          },
          {
            key: "class",
            header: t("org.lifecycle"),
            // classification is retired and no longer written by anything,
            // so a column reading it would show whatever it happened to
            // hold when the split shipped, forever.
            cell: (org: Organization) =>
              org.lifecycle && org.lifecycle !== "unknown" ? (
                <Badge>{t(LIFECYCLE_LABELS[org.lifecycle])}</Badge>
              ) : null,
          },
          {
            key: "relationship",
            header: t("org.relationshipTypes"),
            // A filter with no column to read it back on is a list that cannot
            // say why a row matched. Multi-valued on purpose (ADR-0079/A124):
            // an account can be a partner AND a customer, and showing only the
            // first would make the second look untrue.
            cell: (org: Organization) =>
              org.relationship_types?.length ? (
                <span
                  style={{
                    display: "flex",
                    flexWrap: "wrap",
                    gap: "var(--space-1)",
                  }}
                >
                  {org.relationship_types.map((type) => (
                    <Badge key={type}>
                      {t(RELATIONSHIP_TYPE_LABELS[type])}
                    </Badge>
                  ))}
                </span>
              ) : null,
          },
          ownerColumn<Organization>(t),
          lastActivityColumn<Organization>(t, locale, recordZone),
          createdColumn<Organization>(t, locale, recordZone),
        ]}
        tools={<SaveViewAction resource="organizations" query={state.query} />}
        rowKey={(org) => org.id}
        rowRoute={(org) => ({ screen: "companies", id: org.id })}
        dataChips={[...ownerChips, ...sizeChip, ...tagChips]}
        chips={[
          {
            key: "lifecycle",
            label: "org.lifecycle",
            allLabel: "org.filterLifecycleAll",
            options: LIFECYCLE_OPTIONS.filter(
              (value) => value !== "unknown",
            ).map((value) => ({ value, label: LIFECYCLE_LABELS[value] })),
          },
          {
            key: "relationship_type",
            label: "org.relationshipTypes",
            allLabel: "org.filterRelTypeAll",
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

type SiteReadReport = components["schemas"]["SiteReadReport"];

const SITE_READ_STATUS_LABELS: Record<SiteReadReport["status"], MessageKey> = {
  queued: "deepread.statusQueued",
  deferred: "deepread.statusDeferred",
  running: "deepread.statusRunning",
  done: "deepread.statusDone",
  partial: "deepread.statusPartial",
  cancelled: "deepread.statusCancelled",
  failed: "deepread.statusFailed",
};

// How far a read has got, as the two stages the engine actually reports.
//
// TWO, not the four a reader might imagine — the server's `phase` says
// `crawling` or `extracting` and nothing finer, and a ladder with invented
// rungs would be a progress bar that moves on its own schedule. Two honest
// stages tell a reader more than four made-up ones.
export type ScanState = "done" | "running" | "queued";

const SCAN_STAGES = ["crawling", "extracting"] as const;

type ScanStage = (typeof SCAN_STAGES)[number];

const SCAN_STAGE_LABELS: Record<ScanStage, MessageKey> = {
  crawling: "deepread.stage.crawling",
  extracting: "deepread.stage.extracting",
};

/**
 * Where each stage stands, read off the report rather than timed.
 *
 * A read that has not started has both stages waiting; one in `extracting` has
 * finished crawling by definition, which is the only inference here and it is
 * the engine's own ordering rather than a guess about elapsed time.
 */
export function scanStates(
  report: SiteReadReport,
): Record<ScanStage, ScanState> {
  if (report.status === "queued" || report.status === "deferred") {
    return { crawling: "queued", extracting: "queued" };
  }
  // `phase` is null the moment a read goes terminal, so a running read with no
  // phase is one whose first stage is under way and has not said so yet.
  return report.phase === "extracting"
    ? { crawling: "done", extracting: "running" }
    : { crawling: "running", extracting: "queued" };
}

/**
 * The stages of a read in flight, as a ladder.
 *
 * Drawn ONLY while the read is still going. Once it is done, stopped or
 * failed, the panel beside this says so with its own reason, and a ladder of
 * ticks under a failure would be the page congratulating itself.
 */
function ScanSteps({ report }: Readonly<{ report: SiteReadReport }>) {
  const t = useT();
  // Every state the ladder can answer for, and only those. A read waiting on
  // budget is still a read in flight — the deferral note beside this says how
  // long — while a finished or failed one has its own reason to give, and a
  // column of ticks under a failure would be the page congratulating itself.
  const inFlight =
    report.status === "queued" ||
    report.status === "deferred" ||
    report.status === "running";
  if (!inFlight) {
    return null;
  }
  const states = scanStates(report);
  return (
    <ol className="deepread-steps">
      {SCAN_STAGES.map((stage) => (
        <li key={stage} className={`deepread-step t-sub is-${states[stage]}`}>
          <span className="deepread-step-mark" aria-hidden="true" />
          {t(SCAN_STAGE_LABELS[stage])}
          {/* The state in words as well as in the mark: three tinted circles
              are three tinted circles to a reader who cannot tell them
              apart. */}
          <span className="deepread-step-state t-caption">
            {t(`deepread.step.${states[stage]}`)}
          </span>
        </li>
      ))}
    </ol>
  );
}

const SITE_READ_STOP_LABELS: Record<
  NonNullable<SiteReadReport["stopped_reason"]>,
  MessageKey
> = {
  budget: "deepread.stopBudget",
  page_cap: "deepread.stopPageCap",
  byte_cap: "deepread.stopByteCap",
  deadline: "deepread.stopDeadline",
};

function SiteReadDeferral({ report }: Readonly<{ report: SiteReadReport }>) {
  const t = useT();
  const { locale } = useLocale();
  if (report.status !== "deferred") {
    return null;
  }
  return (
    <p className="t-caption" style={{ margin: "var(--space-2) 0 0" }}>
      {report.status_detail}
      {report.next_attempt_at && (
        <>
          {" "}
          {t("deepread.resumesAt", {
            when: formatDateTime(report.next_attempt_at, locale, viewerZone()),
          })}
        </>
      )}
    </p>
  );
}

// The polled half of the deep read: renders progress while the crawl is in
// flight (3s poll, stops on a terminal status) and the full account when it
// ends — pages read, pages SKIPPED and why, and the stop reason when the
// crawl ended early. The skip/stop rendering is the transparency surface: a
// truncated crawl must never read as complete.
function SiteReadPanel({
  orgId,
  readId,
}: Readonly<{ orgId: string; readId: string }>) {
  const plural = usePlural();
  const t = useT();
  const { locale } = useLocale();
  const reportQuery = useQuery({
    queryKey: ["site-read", orgId, readId],
    queryFn: async () => {
      const { data, error } = await api.GET(
        "/organizations/{id}/site-reads/{readId}",
        { params: { path: { id: orgId, readId } } },
      );
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      if (status === "queued" || status === "running") {
        return 3000;
      }
      return status === "deferred" ? 60_000 : false;
    },
  });

  if (reportQuery.isPending) {
    return <Skeleton width="60%" />;
  }
  if (reportQuery.isError) {
    return (
      <p className="t-caption" style={{ color: "var(--danger)" }}>
        {problemMessageOf(reportQuery.error, t)}
      </p>
    );
  }

  const report = reportQuery.data;
  const terminal =
    report.status === "done" ||
    report.status === "partial" ||
    report.status === "failed";

  return (
    <div style={{ marginTop: "var(--space-3)" }}>
      <p
        style={{
          display: "flex",
          alignItems: "center",
          gap: "var(--space-2)",
          flexWrap: "wrap",
          margin: 0,
        }}
      >
        <Badge tone={report.status === "failed" ? "danger" : undefined}>
          {t(SITE_READ_STATUS_LABELS[report.status])}
        </Badge>
        <span className="t-caption">
          {plural("deepread.pagesSoFar", report.pages.length, {
            count: formatNumber(report.pages.length, locale),
          })}
        </span>
        {terminal && (
          <span className="t-caption">
            {plural("deepread.factCount", report.fact_count ?? 0, {
              count: formatNumber(report.fact_count ?? 0, locale),
            })}
          </span>
        )}
      </p>
      <ScanSteps report={report} />
      <SiteReadDeferral report={report} />
      {report.stopped_reason && (
        <p style={{ margin: "var(--space-2) 0 0" }}>
          <Badge tone="warn">
            {t("deepread.stoppedEarly", {
              reason: t(SITE_READ_STOP_LABELS[report.stopped_reason]),
            })}
          </Badge>
        </p>
      )}
      {terminal && report.proposal_ids.length > 0 && (
        <p
          style={{
            display: "flex",
            alignItems: "center",
            gap: "var(--space-2)",
            flexWrap: "wrap",
            margin: "var(--space-3) 0 0",
          }}
        >
          <AutonomyDot tier="confirm" />
          <span className="t-caption">
            {plural("deepread.proposals", report.proposal_ids.length, {
              count: formatNumber(report.proposal_ids.length, locale),
            })}
          </span>
          <Button small onClick={() => navigate({ screen: "worklist" })}>
            {t("enrich.toInbox")}
          </Button>
        </p>
      )}
    </div>
  );
}

// The whole-site deep read (A102/R2), the enrich verb's big sibling: one
// click starts (or joins — idempotent per org+url) a background crawl of the
// company's own site; findings stage as 🟡 proposals for the inbox, nothing
// writes to the record here. 422 (no website on file) and 501 (crawl seam
// unwired) surface their honest cause instead of a generic failure.
function DeepReadCard({ orgId }: Readonly<{ orgId: string }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [readId, setReadId] = useState<string | null>(null);
  // A read id lives only in the tab that started the crawl, so a read that
  // ended after the rep navigated away used to be unfindable — and an account
  // whose crawl FAILED then looked exactly like one nobody had tried to
  // enrich. 404 is the honest "never read" and leaves the card offering a
  // first crawl.
  const latest = useQuery({
    queryKey: ["site-read-latest", orgId],
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/organizations/{id}/site-reads/latest",
        { params: { path: { id: orgId } } },
      );
      if (response.status === 404) {
        return null;
      }
      if (error) {
        throwProblem(error);
      }
      return data ?? null;
    },
  });
  const shownReadId = readId ?? latest.data?.read_id ?? null;
  const start = useMutation({
    mutationKey: ["site-read", orgId],
    mutationFn: async () => {
      const { data, error, response } = await api.POST(
        "/organizations/{id}/deep-read",
        { params: { path: { id: orgId } } },
      );
      if (error) {
        // 501 means the crawl seam is unwired, which the server states in its
        // own terms and the card states in the reader's. Either way this stays
        // a problem, so the render below can tell it from a bug in here.
        throwProblem(
          response.status === 501
            ? { title: t("deepread.unavailable") }
            : error,
        );
      }
      return data;
    },
    onSuccess: (started) => {
      setReadId(started.read_id);
      // The started read IS the latest one, so say so rather than leaving the
      // cached answer to expire. Without this the card holds a 30s stale
      // "never read" (FE-PARAM-1) that a rep who navigates away and back
      // inside the window still sees — the same invisible-crawl state this
      // query was added to end.
      queryClient.invalidateQueries({
        queryKey: ["site-read-latest", orgId],
      });
      // The read is queued the moment the 202 lands, and the rail's feed is
      // what draws the crawl on the Core — but the occurrence reaches that feed
      // through the outbox, so it is not there yet. This is what makes the rail
      // watch for it rather than meet it on its next idle poll.
      watchStartedAiRun(queryClient);
    },
  });

  return (
    <Card
      title={t("deepread.title")}
      sub={t("deepread.sub")}
      actions={
        <Button
          small
          pending={start.isPending}
          busyLabel={t("deepread.starting")}
          onClick={() => start.mutate()}
        >
          {t("deepread.cta")}
        </Button>
      }
      style={{ marginBottom: "var(--space-4)" }}
    >
      {start.isError && (
        <p className="t-caption" style={{ color: "var(--danger)" }}>
          {problemMessageOf(start.error, t)}
        </p>
      )}
      {shownReadId && <SiteReadPanel orgId={orgId} readId={shownReadId} />}
    </Card>
  );
}

type OrganizationHierarchyRollup =
  components["schemas"]["OrganizationHierarchyRollup"];

// A missing stored FX rate fails the whole rollup read with 422
// fx_rate_unavailable (never a rate-of-1 substitute, never zeros) — this
// marker lets the render branch on that ONE cause without re-parsing the
// problem body a second time.
class FxUnavailableError extends Error {}

async function fetchHierarchyRollup(
  orgId: string,
): Promise<OrganizationHierarchyRollup> {
  const { data, error } = await api.GET(
    "/organizations/{id}/hierarchy-rollup",
    {
      params: { path: { id: orgId }, query: { scope: "tree" } },
    },
  );
  if (error) {
    if (error.code === "fx_rate_unavailable") {
      throw new FxUnavailableError();
    }
    throwProblem(error);
  }
  return data;
}

// P-7: the org hierarchy roll-up (weighted pipeline, current-quarter
// closed-won, 30-day activity, aggregated account count), read-only. Money
// renders only when both amount_minor and currency are present (Money's
// fields are individually optional on the wire) — never a hand-formatted or
// zero-filled figure.
function HierarchyRollupCard({ orgId }: Readonly<{ orgId: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const rollupQuery = useQuery({
    queryKey: ["rollup", orgId],
    queryFn: () => fetchHierarchyRollup(orgId),
  });

  if (rollupQuery.isPending) {
    return (
      <div
        style={{
          display: "flex",
          flexDirection: "column",
          gap: "var(--space-3)",
        }}
      >
        <Skeleton width="60%" />
        <Skeleton width="90%" />
        <Skeleton width="75%" />
      </div>
    );
  }
  if (rollupQuery.isError) {
    if (rollupQuery.error instanceof FxUnavailableError) {
      return <EmptyState>{t("rollup.fxUnavailable")}</EmptyState>;
    }
    return <EmptyState>{problemMessageOf(rollupQuery.error, t)}</EmptyState>;
  }

  const rollup = rollupQuery.data;
  const money = (value: OrganizationHierarchyRollup["weighted_pipeline"]) =>
    value.amount_minor != null && value.currency
      ? formatMoney(value.amount_minor, value.currency, locale)
      : "—";

  return (
    <Card title={t("tab.rollup")} style={{ marginBottom: "var(--space-4)" }}>
      <dl className="firmo">
        <div>
          <dt className="t-eyebrow">{t("rollup.weightedPipeline")}</dt>
          <dd className="t-mono">{money(rollup.weighted_pipeline)}</dd>
        </div>
        <div>
          <dt className="t-eyebrow">{t("rollup.closedWon")}</dt>
          <dd className="t-mono">{money(rollup.closed_won)}</dd>
        </div>
        <div>
          <dt className="t-eyebrow">{t("rollup.activity30d")}</dt>
          <dd>{formatNumber(rollup.activity_count_30d, locale)}</dd>
        </div>
        <div>
          <dt className="t-eyebrow">{t("rollup.accounts")}</dt>
          <dd>{formatNumber(rollup.aggregated_account_count, locale)}</dd>
        </div>
      </dl>
      {rollup.restricted_excluded.length > 0 && (
        <p className="t-caption" style={{ marginTop: "var(--space-2)" }}>
          {t("rollup.excluded", {
            count: formatNumber(rollup.restricted_excluded.length, locale),
          })}
        </p>
      )}
      <p className="t-caption" style={{ marginTop: "var(--space-2)" }}>
        {t("rollup.computedAt", {
          when: formatDateTime(rollup.computed_at, locale, recordZone),
        })}
      </p>
    </Card>
  );
}

// One confirmed profile field (S-E02): the human field label, the value, and
// a footer that names where it came from — provenance, confidence when the
// read carried one, and the grounding evidence snippet. These are values that
// LANDED on the record, whatever lane wrote them.
// PROFILE_FIELD_LABELS names the profile fields as statements ABOUT a company.
//
// The same fields are asked of the reader during onboarding, where the second
// person is right — "What do you sell?" is a question to us. On a prospect's
// record that framing put the reader in the wrong chair: the page appeared to
// be interviewing us about a company we are trying to sell to.
const PROFILE_FIELD_LABELS: Record<string, MessageKey> = {
  display_name: "co.profileField.display_name",
  offer_summary: "co.profileField.offer_summary",
  icp: "co.profileField.icp",
  buying_center: "co.profileField.buying_center",
  value_proposition: "co.profileField.value_proposition",
  usp: "co.profileField.usp",
  customer_pains: "co.profileField.customer_pains",
  desired_outcomes: "co.profileField.desired_outcomes",
  buying_intents: "co.profileField.buying_intents",
  common_objections: "co.profileField.common_objections",
  sales_motion: "co.profileField.sales_motion",
  legal_name: "co.profileField.legal_name",
  registered_address: "co.profileField.registered_address",
  register_vat: "co.profileField.register_vat",
  legal_form: "co.profileField.legal_form",
  register_court: "co.profileField.register_court",
  register_number: "co.profileField.register_number",
  industry: "co.profileField.industry",
  history: "co.profileField.history",
};

// The onboarding wording is the fallback, so a field added there still reads
// as words rather than as a column name here.
export function profileFieldLabel(
  field: string,
  t: ReturnType<typeof useT>,
): string {
  const key = PROFILE_FIELD_LABELS[field];
  return key ? t(key) : coldFieldLabel(field, t);
}

// Overview · People · Deals · Tasks · History · Documents · Profile, the
// mockup's strip. `timeline` IS the mockup's History — it presents as
// "Verlauf"/"History" and carries the account's chronology; the id stays as
// it is because it is in every saved URL. The audit spine is still not a tab:
// it is an inspection of the record rather than part of its story, and it
// opens from the header's overflow menu.
//
// Deals, Tasks and Documents are tabs rather than only cards because the
// mockup gives each its own place in the strip, and the pipeline, the open
// tasks and the file table each want more room than a card in a grid can
// spare. Profile carries the account's own reference material — its filed
// fields, its facts, who it is connected to, and the one-off tools — so a
// reader who wants any of that checks one tab instead of a scatter of
// disclosures under every other one. Partner stays a tab: it is a form, not
// a reading of this account.

// Two tabs are conditional on the account, and both follow the same shape: a
// tab that is empty on nearly every record teaches the reader to skip the tab
// strip, so it is absent rather than empty — and each keeps a carveout for the
// reader who is already standing on it, so nothing can strand them.
//
// Partner renders the partner programme — certification, role, margin tier —
// a form about a commercial arrangement the overwhelming majority of accounts
// do not have. It shows for an account that HAS one, and for the reader who
// just asked to set one up (the overflow menu switches the tab, which is what
// `tab` already carries), so the only path to a first partner row stays open.
// How much is behind each tab, read ONLY from the composite the page already
// has. A count is not worth a request: a badge that costs a round trip per tab
// turns opening an account into eight of them, and the figure it paints is
// worth less than the read it spends.
//
// A tab whose section the 360 did not return gets NO count rather than a zero.
// The two are different claims — a zero says the account has none, and absence
// says this reader was not shown the section (`sections_omitted`) or the read
// has not landed yet. Printing "0" for either is how a grant boundary comes to
// read as an empty account.
//
// 360 and Profile carry none by design: neither is a list of things, so a
// figure beside them would count something the reader cannot see under them.
//
// A PAGED section counts only when its page is the whole set — `wholeCount`
// spells that rule once for the badges here and the rail's own summaries.
function companyTabCounts(
  view?: Organization360View,
): Partial<Record<CompanyTab, number>> {
  if (!view) {
    return {};
  }
  const counts: Partial<Record<CompanyTab, number>> = {};
  const people = wholeCount(view.people);
  if (people != null) {
    counts.people = people;
  }
  const deals = wholeCount(view.deals);
  if (deals != null) {
    counts.deals = deals;
  }
  const tasks = wholeCount(view.next_steps);
  if (tasks != null) {
    counts.tasks = tasks;
  }
  return counts;
}

function companyTabsFor(
  org: Organization,
  tab: CompanyTab,
): readonly CompanyTab[] {
  // Gated on the relationship type, not on `org.partner`: the Organization
  // read does not select the extension row, so that field is always absent
  // and every partner would lose the tab. The type is equivalent and IS
  // returned — an org carries it exactly when it has a programme, which the
  // store enforces in both directions (ADR-0079/A124).
  const isPartner = (org.relationship_types ?? []).includes("partner");
  const drop = new Set<CompanyTab>();
  if (!isPartner && tab !== "partner") {
    drop.add("partner");
  }
  // Finance is absent exactly where its card is absent, on FIN-AC-3's own list
  // — an account nobody has ever invoiced has no money to report, and a tab
  // that opens onto nothing is worse than one that is not there.
  if (!hasFinance(org.lifecycle) && tab !== "finance") {
    drop.add("finance");
  }
  return drop.size === 0
    ? COMPANY_TABS
    : COMPANY_TABS.filter((id) => !drop.has(id));
}

// useCompanyTab is scoped to the ACCOUNT being read, the same reason the
// chronology filter is (useChronologyFilter): the route swaps one company
// for another without ever unmounting this component, so a reader who opened
// Partner on one account met it again on the next — and companyTabsFor's own
// carveout (a reader mid-way through setting up a programme keeps the tab
// while `tab === "partner"`) has no way to tell "still this account" from
// "a different one" unless something resets it at the boundary.
function useCompanyTab(
  recordId: string,
): [CompanyTab, (next: CompanyTab) => void] {
  const route = useRoute();
  // Read off the ADDRESS rather than held beside it, so the tab a reader is on
  // is the tab the URL names — and the per-record reset this used to do by
  // hand is gone with it: a tab belongs to the account it is addressed with,
  // so swapping accounts cannot carry one along.
  const addressed =
    route.screen === "companies" && route.id === recordId
      ? route.id2
      : undefined;
  return [
    isCompanyTab(addressed) ? addressed : "overview",
    // A PUSH, so Back steps between the tabs a reader opened rather than
    // leaving the account altogether — the same thing the contact page's strip
    // does. The per-record reset this used to hold is now the address's: a tab
    // belongs to the account it names, so moving to another account cannot
    // carry one along.
    (next: CompanyTab) => navigate(companyTabRoute(recordId, next)),
  ];
}

// openTaskId is scoped to the ACCOUNT being read, the same reason
// useCompanyTab is: the route swaps one company for another without ever
// unmounting this component, so a task detail modal opened on one account
// would keep rendering over the next one.
//
// It is scoped to the TAB for the same reason at one level down. The detail
// modal only renders on Tasks, so a tab change takes the dialog off screen
// without its own `onClose` ever running — and an id that outlives the surface
// holding it reopens the dialog by itself when the reader comes back to Tasks,
// having closed nothing. That the reader cannot currently reach a tab pill
// behind the open modal's backdrop is a property of Modal, not of this state:
// resetting here is what makes the invariant this page's own.
function useOpenTaskId(
  recordId: string,
  tab: CompanyTab,
): [string | null, (next: string | null) => void] {
  const [openTaskId, setOpenTaskId] = useState<string | null>(null);
  const [openTaskFor, setOpenTaskFor] = useState(recordId);
  const [openTaskOn, setOpenTaskOn] = useState(tab);
  if (openTaskFor !== recordId || openTaskOn !== tab) {
    setOpenTaskFor(recordId);
    setOpenTaskOn(tab);
    setOpenTaskId(null);
  }
  return [openTaskId, setOpenTaskId];
}

// The company 360 badge/action bar. Archived records are read-only: the
// backend rejects edit/merge/archive on a non-live row (there is no unarchive
// path), so those buttons would only 404 — the Archived badge is the whole
// affordance. Extracted from CompanyScreen so its render stays legible.
// The company's edit form. Its own component because it owns three reads the
// rest of the action bar has no use for — the custom-field catalogue, the user
// roster behind the owner picker, and the record slice they prefill.
export function CompanyScreen({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const [tab, setTab] = useCompanyTab(id);
  const view = useOrganization360(id);
  // Only an assembled 360 counts as a visit: in overlay mode there is no
  // baseline to advance, and a page that never rendered the account is not
  // one the reader saw.
  useAcknowledgeOrganizationView(id, view.data?.state === "ready");
  // The account itself still comes from its own read: the 360 refuses
  // entirely in overlay mode, and the header must render either way.
  const orgQuery = useQuery({
    queryKey: ["organization", id],
    queryFn: async () => {
      const { data, error } = await api.GET("/organizations/{id}", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

  return (
    <div className="wrap">
      <QueryGate query={orgQuery} pendingLabel={t("nav.companies")}>
        {(org) => (
          <CompanyRecord org={org} view={view} tab={tab} onTab={setTab} t={t} />
        )}
      </QueryGate>
    </div>
  );
}

// CompanyRecord renders the page once the account itself has loaded. Split
// out so the 360's three states — assembling, assembled, refused because the
// workspace reads elsewhere — are handled in one place rather than nested
// inside the account gate.
function CompanyRecord({
  org,
  view,
  tab,
  onTab,
  t,
}: Readonly<{
  org: Organization;
  view: { data?: Org360Result; isPending: boolean; isError: boolean };
  tab: CompanyTab;
  onTab: (next: CompanyTab) => void;
  t: ReturnType<typeof useT>;
}>) {
  const sorMode = useSorMode();
  const assembled = view.data?.state === "ready" ? view.data.view : undefined;
  // Two sources say "this workspace reads elsewhere", and either is enough.
  // The 360 refuses with a 422, and /me reports the mode directly — a
  // workspace that flipped mode after this page cached its read would
  // otherwise keep serving a native-looking company view.
  const overlay = view.data?.state === "overlay" || sorMode === "overlay";
  const visibleTabs = companyTabsFor(org, tab);
  // One tab is not a choice. A strip with a single tab is a control that does
  // nothing, so it disappears entirely rather than asking the reader to pick
  // the page they are already on.
  const tabs =
    visibleTabs.length > 1 ? (
      <div className="co-tabs">
        <RecordTabs
          options={visibleTabs}
          value={tab}
          onChange={onTab}
          counts={companyTabCounts(assembled)}
          // The switch for the account's own details column, at the end of
          // the tab row: it chooses what the page shows beside the work, so it
          // stands with the controls that choose what the work column shows,
          // and never in the head among the record's verbs.
          trailing={<PageAsideToggle />}
          labels={{
            // "360", not the shared "Overview": this tab is the account's
            // one assembled reading, and the card inside it is named the same
            // — a tab and the thing it opens calling themselves two different
            // words is two places to learn. Its own key rather than a re-worded
            // `tab.overview`, which four other record types render and none of
            // them is this.
            overview: t("tab.overview"),
            people: t("tab.people"),
            deals: t("tab.deals"),
            tasks: t("tab.tasks"),
            timeline: t("tab.timeline"),
            // The tab's own key rather than `finance.title`, which the card
            // inside varies by lifecycle ("Finance (historical)"). A tab label
            // names a place and does not qualify it; sharing one key would tie
            // the strip to a title that changes under it.
            finance: t("tab.finance"),
            documents: t("tab.documents"),
            profile: t("tab.profile"),
            partner: t("tab.partner"),
          }}
        />
      </div>
    ) : null;

  // Both tabs render inside ONE page. Partner used to be a different
  // component tree with no rails, so switching tab unmounted both side
  // columns and every query behind them: the grid re-columned under the
  // reader and the page refetched itself on the way back. Only the middle
  // column's body changes now.
  return (
    <CompanyPage
      org={org}
      view={assembled}
      overlay={overlay}
      loading={view.isPending}
      failed={view.isError}
      tab={tab}
      tabs={tabs}
      onTab={onTab}
    />
  );
}

// The four RecordView slots the chronology section fills: the list, the
// filter above it, the load-more and disclosure below it, and the notice that
// replaces the list when there is nothing honest to draw. Assembled here so
// the page's render reads as a layout rather than as four nested ternaries.
type ChronologySlots = {
  timeline?: TimelineEntry[];
  timelineGroups?: readonly TimelineGroup[];
  timelineHeader?: ReactNode;
  timelineFooter?: ReactNode;
  timelineNotice?: ReactNode;
  onOpenThread?: (threadKey: string) => void;
};

function useChronologySlots({
  org,
  view,
  overlay,
  loading,
  failed,
  active,
}: Readonly<{
  org: Organization;
  view?: Organization360View;
  overlay: boolean;
  loading: boolean;
  failed: boolean;
  // Whether the chronology is on screen at all. The Partner tab is a form,
  // so it renders no timeline rather than an empty one.
  active: boolean;
}>): {
  slots: ChronologySlots;
  showChanges: () => void;
  // The open message and its setter travel OUT of this hook rather than the
  // drawer being mounted in a timeline slot. A record-level dialog belongs to
  // the page, not to a tab — the same rule the audit modal below states — and
  // a drawer that unmounts with the Timeline tab leaves its id behind, so
  // coming back to that tab can put a second dialog over an open one.
  openEmail: string | null;
  setOpenEmail: (activityId: string | null) => void;
} {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  // The workspace roster, for the ids a change row stores. Read here rather
  // than inside the adapter: the roster is a workspace query and the adapter
  // is a pure mapping, which is what lets it be tested without one.
  const roster = useRoster("user", true);
  const colleagues = new Map(
    (roster.data ?? []).flatMap((entry) =>
      "display_name" in entry ? [[entry.id, entry.display_name] as const] : [],
    ),
  );
  // One resolver for every id the chronology can meet: a colleague from the
  // workspace roster, or one of the account's own records from the 360 it was
  // read with. Keyed by id alone because a stored value carries no type — an
  // owner field holds a uuid and nothing else — and the two id spaces do not
  // collide.
  const records = recordNamesIn(view);
  const colleagueName = (id: string) =>
    colleagues.get(id) ??
    records("person", id) ??
    records("deal", id) ??
    records("organization", id);
  const [filter, setFilter] = useChronologyFilter(org.id);
  const [filters, setFilters] = useTimelineFilters(org.id);
  // The 360's own page seeds the list; older pages and every narrowed read
  // come from the activity list itself.
  const timeline = useRecordTimeline("organization", org.id, {
    filters,
    firstPage: view?.activities,
  });

  const [openEmail, setOpenEmail] = useOpenEmail();
  const history = useRecordChronology({
    onOpenEmail: setOpenEmail,
    kind: "organization",
    recordId: org.id,
    filter,
    // A narrowed read is a question about what was said, so the record's own
    // edits stand down: they are not meetings, and not what the reader asked.
    narrowed: hasTimelineFilters(filters),
    activities: timeline.activities,
    activitiesHaveMore: timeline.hasNextPage,
    loadMore: timeline,
    // What a stored value MEANS, rather than the shape it is kept in: a date
    // read at the record's own zone, a list as its words, and a colleague's id
    // as their name. The resolver reaches the workspace roster because the ids
    // a change row carries are ours — an owner moving from one colleague to
    // another is the change a reader most often comes here for.
    // An account holds no currency of its own: a minor-unit column on this
    // record says so rather than printing a bare integer under a currency it
    // was never denominated in.
    values: {
      currency: null,
      locale,
      zone: recordZone,
      nameOf: colleagueName,
    },
    renderActions: (activity) => (
      <TimelineActions
        activity={activity}
        entityType="organization"
        entityId={org.id}
      />
    ),
  });
  // An evidence mark asks "where did this value come from" — the answer is
  // the record's change history, so the mark turns the timeline to Changes
  // rather than opening a screen of its own.
  const showChanges = () => setFilter("changes");

  if (!active) {
    return {
      slots: { timelineNotice: <span /> },
      showChanges,
      openEmail,
      setOpenEmail,
    };
  }
  // In overlay mode the refusal is stated once, in the body: repeating it over
  // the timeline would read as two separate things being unavailable rather
  // than one page not being assembled.
  if (overlay) {
    return {
      slots: { timeline: history.entries, timelineNotice: <span /> },
      showChanges,
      openEmail,
      setOpenEmail,
    };
  }
  return {
    showChanges,
    openEmail,
    setOpenEmail,
    slots: {
      timeline: history.entries,
      // Conversations, not messages. The account's timeline is where the same
      // exchange showed up three times — a product update to three contacts
      // was three rows, and a five-message thread was five.
      timelineGroups: groupChronology(history.entries, timeline.hasNextPage),
      timelineHeader: (
        <>
          <ChronologyFilter
            filter={filter}
            conversations
            onFilter={setFilter}
          />
          {filter !== "changes" && (
            <TimelineFilterBar value={filters} onChange={setFilters} />
          )}
        </>
      ),
      timelineFooter: <ChronologyFooter filter={filter} chronology={history} />,
      // Every cut renders through the ONE chronicle: Changes draws the same
      // change rows the All view interleaves and Conversations the same
      // thread rows, so no cut is a second rendering of rows another cut
      // already shows. The By-field reading and the put-back control live in
      // the record's Full history (the header's overflow menu), the one
      // surface that carries the restore write.
      timelineNotice:
        chronologyNotice(
          filter === "conversations"
            ? "chronology.conversationsEmpty"
            : "co.timeline.empty",
          {
            // The two feeds are read together rather than per filter.
            loading: loading || history.loading || timeline.isPending,
            failed: failed || history.failed || timeline.isError,
            // A narrowed read is the list's own and is assembled once it
            // answers; the unfiltered one is the 360's section.
            assembled: hasTimelineFilters(filters)
              ? timeline.isSuccess
              : Boolean(view?.activities),
            filter,
          },
          history.entries.length,
          t,
        ) ??
        // The Conversations cut narrows the chronicle to the exchanges
        // somebody can answer, drawn through the same grouped list as every
        // other cut, standing where the unfiltered list would.
        (filter === "conversations" ? (
          <ConversationList
            groups={groupChronology(history.entries, timeline.hasNextPage)}
            zone={recordZone}
          />
        ) : undefined),
    },
  };
}

// CompanyPage is the page itself: identity and verbs at the top, then three
// zones — what this company IS on the left, what is HAPPENING in the middle,
// the BUSINESS around it on the right.
//
// All three tabs render here. The rails belong to the ACCOUNT, not to the
// overview, so they stay mounted whichever tab is open and the reader keeps
// the firmographics and the business context while reading the partner form
// or the change history.
// The receipt drawer's state: which claim is open, and the ordered list it
// steps through.
//
// The ORDER belongs to the card that offered the chip, not to the drawer. A
// reader who clicked the third citation in a sentence expects "next" to mean
// the fourth citation in THAT sentence — a drawer that built its own order
// would step somewhere they cannot predict. A card with no ordering to give
// passes none, and the drawer draws no arrows rather than guessing one.
function useCitedReceipt() {
  const [cited, setCited] = useState<CitedRecord | null>(null);
  const [list, setList] = useState<readonly CitedRecord[]>([]);
  // The message a citation opened, held HERE beside the receipt it opens for
  // the other kinds: both answer "what did this chip open", and splitting them
  // would leave a caller wiring two states for one question.
  const [email, setEmail] = useState<string | null>(null);
  const open = (
    entityType: string,
    entityId: string,
    siblings?: readonly CitedRecord[],
  ) => {
    // An activity opens the message itself. It used to fall through both
    // branches below and land nowhere — the account's commitment rows have
    // been passing `source_activity_id` into a button that did nothing since
    // the day they were written, because an activity had no detail route.
    // It has one now.
    if (citationOpensEmail(entityType)) {
      setEmail(entityId);
      return;
    }
    if (citationOpensRecord(entityType)) {
      openCitation(entityType, entityId);
      return;
    }
    if (citationHasReceipt(entityType)) {
      setCited({ entityType, entityId });
      setList(siblings ?? []);
    }
  };
  // Wrapping at each end: a reader walking a sentence's citations should not
  // hit a dead stop and have to close the drawer to reach the first one again.
  const step = (direction: -1 | 1) => {
    if (!cited) {
      return;
    }
    const at = list.findIndex(
      (each) =>
        each.entityType === cited.entityType &&
        each.entityId === cited.entityId,
    );
    if (at < 0) {
      return;
    }
    setCited(list[(at + direction + list.length) % list.length]);
  };
  return {
    cited,
    email,
    open,
    close: () => setCited(null),
    closeEmail: () => setEmail(null),
    step: list.length > 1 ? step : undefined,
  };
}

// What the composer is anchored on. A reply answers the message it names; an
// account-started message names the person it is TO, because it has no thread
// to inherit a recipient from.
type ComposeAnchor =
  | { kind: "reply"; id: string }
  | { kind: "account"; id: string }
  // The next step the advice says is missing: the task form, opened on a
  // task. No id, because the rule that asks for it names the account and not
  // one deal — with several open it would be guessing which.
  | { kind: "task" };

// A suggestion action kind the page has no handler for. Reached only if the
// contract grows a kind before this page does, which TypeScript refuses at the
// switch that calls it — the runtime throw is for a payload the build never saw.
function unreachableAction(kind: never): never {
  throw new Error(`no surface performs the suggestion action ${String(kind)}`);
}

// The composer, opened on whichever anchor the page holds. Extracted so the
// page does not carry a branch per anchor kind in its own JSX.
function AccountComposer({
  anchor,
  orgId,
  onClose,
}: Readonly<{
  anchor: ComposeAnchor;
  orgId: string;
  onClose: () => void;
}>) {
  if (anchor.kind === "task") {
    // The header's own Add-task form, opened already on a task: the advice
    // shortcuts to it rather than inventing a second way to set a next step.
    return (
      <LogActivityAction
        entityType="organization"
        entityId={orgId}
        askedKind="task"
        triggerLabel="log.addTask"
        openOnMount
        onClose={onClose}
      />
    );
  }
  const reply = anchor.kind === "reply";
  return (
    <ComposeModal
      activityId={reply ? anchor.id : undefined}
      personId={reply ? undefined : anchor.id}
      entityType="organization"
      entityId={orgId}
      kind="email"
      open
      onClose={onClose}
    />
  );
}

function CompanyPage({
  org,
  view,
  overlay,
  loading,
  failed,
  tab,
  tabs,
  onTab,
}: Readonly<{
  org: Organization;
  view?: Organization360View;
  overlay: boolean;
  loading: boolean;
  // The composite read failed. Distinct from "still loading" and from "the
  // account is empty", because all three would otherwise draw the same
  // blank page and only one of them is a fact about the account.
  failed: boolean;
  tab: CompanyTab;
  // The rendered tab bar. It is handed down to the body rather than drawn
  // here, so the strip can lead and the bar sit beneath it.
  tabs: ReactNode;
  // An evidence mark can be clicked from the Partner tab, where the timeline
  // it wants to filter is not on screen; the page has to come back to the
  // Overview before the filter means anything.
  onTab: (next: CompanyTab) => void;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const recordZone = useRecordZone();
  const archivedParagraphId = useId();
  // Only when the paragraph below is actually rendered — the raw `useId()`
  // value is always truthy, so passing IT unconditionally told
  // CompanyActionBadges a sentence was already drawn for every account,
  // archived or not, and left its own fallback (the "not yours to change"
  // case) pointing `aria-describedby` at an id nothing on the page carries.
  const archivedReasonId = org.archived_at ? archivedParagraphId : undefined;
  // ONE composer, opened two ways. Anchored on a timeline message it answers
  // that message; anchored on a person it starts a new one and grounds on the
  // account instead of a thread (ADR-0087 §1). Two pieces of state would let
  // both open at once, which is two composers over each other.
  const [composing, setComposing] = useState<ComposeAnchor | null>(null);
  // The header's own Write-email drawer. Separate state from `composing`,
  // which anchors on a message or a person — but the same consequence for the
  // layout, because both open into the rail's column.
  const [writingEmail, setWritingEmail] = useState(false);
  const [decisionsOpen, setDecisionsOpen] = useState(false);
  const [auditOpen, setAuditOpen] = useState(false);
  // Which open task the Tasks tab has expanded into its detail modal, keyed
  // by activity id rather than a bare boolean because the row that opened it
  // is also what the modal reads.
  const [openTaskId, setOpenTaskId] = useOpenTaskId(org.id, tab);
  const receipt = useCitedReceipt();
  // An archived company takes no new activity, so completing or snoozing a
  // task from here would only be refused server-side.
  const readOnly = Boolean(org.archived_at);
  // Shared by the Tasks tab's row verbs and its detail modal, so a complete
  // fired from one and a snooze fired from the other land on the same
  // mutation and invalidate the same reads — the account's own timeline (the
  // 360 renders it, not a `["activities", …]` query of its own), the
  // workspace-wide queue, and the task's own detail.
  const taskUpdate = useTaskUpdate(taskWriteKeys("organization", org.id));
  // Either composer holds the rail's column, and the rail does not care
  // which. Computed once so both `CompanyRail` and the layout below share
  // the one decision, rather than the page rendering a rail element that
  // itself returns null while `RecordView` still reserves the column for it.
  const composerOpen = Boolean(composing) || writingEmail;
  // The details pane yields to the composer, which opens as its own drawer
  // (ComposeModal's `placement="right"` is a portalled overlay): no pane at
  // all while one is open, so the column is absent for exactly as long as the
  // drawer holds that space, rather than standing open around nothing.
  const details = usePageAside(!composerOpen);
  // The rail, or nothing while a composer holds its column. Both drawers open
  // into this space, so a rail beside them would be two things in one place —
  // and absent rather than narrowed, because a rail squeezed to a third of its
  // width is a column of broken cards.
  const rail = (
    <CompanyRail
      orgId={org.id}
      org={org}
      view={view}
      // The composite read still in flight vs. it having failed: without
      // this the rail's own sections cannot tell the two apart from an
      // undefined `view` alone, and both drawing the loading skeleton is not
      // the same defect as both drawing "could not be loaded".
      loading={loading}
      composerOpen={composerOpen}
      onTab={onTab}
    />
  );
  const {
    slots,
    showChanges: filterToChanges,
    openEmail,
    setOpenEmail,
  } = useChronologySlots({
    org,
    view,
    overlay,
    loading,
    failed,
    active: tab === "timeline",
  });
  const showChanges = () => {
    onTab("timeline");
    filterToChanges();
  };
  return (
    <RecordView
      name={org.display_name}
      avatarSrc={org.logo_url}
      // The account's standing, read on the name's own line rather than
      // folded into the meta line below with everything else it carries —
      // the one value here a reader looks for first.
      nameBadge={
        <>
          <CompanyLifecycleControl org={org} />
          {/* What the account IS to us, beside where it stands. Both are tags
              ON the record, so both belong with its name. */}
          <CompanyRelationshipBadges org={org} />
        </>
      }
      zone={recordZone}
      // What the account IS, then where it STANDS, both on the identity's own
      // lines: the meta line (domain, industry, owner, last exchange) and the
      // standing under it (open pipeline, work in flight, owner).
      //
      // The prose description is NOT here. It is the one thing in the header a
      // reader cannot act on, it is unbounded in length, and on an enriched
      // account it repeats the industry two lines above it. It reads in the
      // details grid, where the rest of the account's filed fields are.
      pulse={<CompanyIdentityLine org={org} view={view} loading={loading} />}
      // The composer opens from a button rather than standing open above the
      // page: a whole form in the header's action strip pushed the account's
      // own story below the fold before a word of it was read.
      actions={
        <>
          {/* One sentence for the whole strip. Both action groups below refuse
              for the same reason, so the reason belongs to the page rather than
              to whichever group is drawing — stated in each, an archived
              account said the same thing twice as soon as the menu opened. */}
          {org.archived_at && (
            <p className="t-caption" id={archivedParagraphId}>
              {t("record.archivedReadOnly")}
            </p>
          )}
          <CompanyPrimaryActions
            org={org}
            composerOpen={writingEmail}
            onComposerOpen={setWritingEmail}
            archivedReasonId={archivedReasonId}
          />
          {/* Last in the row, after the verbs it holds the remainder of: a
              menu of everything-else read as the first thing to press when it
              led them. */}
          <CompanyActionBadges
            org={org}
            archivedReasonId={archivedReasonId}
            view={view}
            onOpenHistory={() => setAuditOpen(true)}
            onSetUpPartner={() => onTab("partner")}
            onOpenDecisions={
              tab === "overview" ? () => setDecisionsOpen(true) : undefined
            }
          />
        </>
      }
      actionsInline
      // The account's context, beside the work under the tab row: what is
      // true of the ACCOUNT does not belong to whichever part of it is open,
      // so the pane stays put when a tab changes.
      aside={details.open ? rail : undefined}
      // The bar that chooses which part of the account to read, across the
      // page above the columns: the details pane opens under it, from the
      // control at its end.
      tabs={tabs}
      // A company's mark is its logo, so it is drawn on a square the way a
      // logo is rather than round the way a face is.
      markShape="organization"
      // The chronology is the account's story and belongs to the overview.
      // The Partner tab is a form, so it does not repeat it under itself.
      {...slots}
    >
      <CompanyRecordBody
        org={org}
        view={view}
        overlay={overlay}
        loading={loading}
        failed={failed}
        tab={tab}
        onTab={onTab}
        t={t}
        receipt={receipt}
        composing={composing}
        onCompose={setComposing}
        onPerform={(action) => {
          // Total over the kinds the server can name: a kind this page
          // cannot perform is a compile error here, never a button that
          // swallows the click.
          switch (action.kind) {
            case "draft_reply":
              if (action.activity_id) {
                setComposing({ kind: "reply", id: action.activity_id });
              }
              return;
            case "open_deal":
              if (action.deal_id) {
                navigate({ screen: "deals", id: action.deal_id });
              }
              return;
            case "add_task":
              setComposing({ kind: "task" });
              return;
            default:
              unreachableAction(action.kind);
          }
        }}
        decisionsOpen={decisionsOpen}
        onDecisionsOpen={setDecisionsOpen}
        readOnly={readOnly}
        openTaskId={openTaskId}
        onOpenTask={setOpenTaskId}
        taskUpdate={taskUpdate}
        onOpenHistory={showChanges}
      />
      {/* The email drawer, on the same rule as the audit spine below: it
          belongs to the RECORD. Mounted in the Timeline tab's own slot it
          unmounted with that tab and left its id behind, so returning to
          Timeline could put it over an already-open dialog. */}
      <OpenEmailDrawer
        activityId={openEmail}
        zone={recordZone}
        onClose={() => setOpenEmail(null)}
      />
      {/* The audit spine, opened from the header's overflow menu. It belongs
          to the RECORD, not to a tab, so it opens over whichever tab is up. */}
      <Modal
        open={auditOpen}
        onClose={() => setAuditOpen(false)}
        labelledBy="co-audit-title"
        size="wide"
      >
        <h2 id="co-audit-title" className="t-h2 modal-title">
          {t("record.fullHistory")}
        </h2>
        {/* Mounted only while open: the two history reads behind it are the
            page's most expensive, and nobody who never opens the panel should
            pay for them. */}
        {auditOpen && (
          <RecordHistoryTab
            kind="organization"
            id={org.id}
            restore={{
              version: org.version,
              onRestored: () =>
                invalidateRecord(queryClient, "organization", org.id),
            }}
          />
        )}
        {/* card-actions, not form-actions: what stands above this row is a
            history timeline, which sets its own top margin to 0 and carries no
            bottom one — so the form's row, which brings no top margin because a
            field above it normally does, put Close against the last entry. */}
        <div className="card-actions">
          <Button onClick={() => setAuditOpen(false)}>
            {t("common.close")}
          </Button>
        </div>
      </Modal>
    </RecordView>
  );
}

// CompanyRecordBody is every tab's content below the strip and the tab bar.
// Split out of CompanyPage so that render stays a layout — the account's
// header and its rail — rather than the seven-way branch beneath it.
function CompanyRecordBody({
  org,
  view,
  overlay,
  loading,
  failed,
  tab,
  onTab,
  t,
  receipt,
  composing,
  onCompose,
  onPerform,
  decisionsOpen,
  onDecisionsOpen,
  readOnly,
  openTaskId,
  onOpenTask,
  taskUpdate,
  onOpenHistory,
}: Readonly<{
  org: Organization;
  view?: Organization360View;
  overlay: boolean;
  // The composite read's own pending flag, threaded to every card below that
  // reads `view` directly with no skeleton guard of its own — see
  // sectionState's own doc for why "undefined view" is not one fact.
  loading: boolean;
  failed: boolean;
  tab: CompanyTab;
  onTab: (next: CompanyTab) => void;
  t: ReturnType<typeof useT>;
  receipt: ReturnType<typeof useCitedReceipt>;
  composing: ComposeAnchor | null;
  onCompose: (anchor: ComposeAnchor | null) => void;
  onPerform: (action: SuggestionAction) => void;
  decisionsOpen: boolean;
  onDecisionsOpen: (open: boolean) => void;
  readOnly: boolean;
  openTaskId: string | null;
  onOpenTask: (activityId: string | null) => void;
  taskUpdate: ReturnType<typeof useTaskUpdate>;
  onOpenHistory: () => void;
}>) {
  // Whether this company is this reader's to change. It used to be
  // `!org.archived_at`, which answered a different question: an archived
  // company is read-only for everyone, but a LIVE company somebody else owns is
  // read-only too, and the page had no way to know that until the record
  // started carrying the answer.
  // The meeting whose brief is open. "Prepare meeting" used to open the
  // composer on the meeting, which is a reply to a room nobody has sat in
  // yet; the brief drawer is what prepares a reader for one.
  const [preparing, setPreparing] = useState<string | null>(null);
  return (
    <>
      {/* Overlay refuses the whole company page, not one tab of it: the
          partner extension and the field history are native records the
          mirror does not hold, so switching tabs must not walk around the
          refusal into reads that can only fail. */}
      {overlay && <OverlayFallback />}
      {!overlay && tab === "partner" && <PartnerTab organizationId={org.id} />}
      {/* The partial-read notice stands IN the overview's stack rather than
          above it: the sentence and the column it qualifies are one body, and
          on the record's own ground the notice's card met the first pane of
          the stack at the border. */}
      {tab === "overview" && (
        <div className="record-stack">
          {failed && <EmptyState>{t("co.partial")}</EmptyState>}
          {/* What needs a person, before anything that merely reports state. It
              is assembled from sections the page already read — open tasks, the
              calendar, what changed since the last visit, the suggestions — put
              in the order a rep works them, with facts, assessments and
              recommendations labelled apart. */}
          <CompanyOverviewStack
            org={org}
            view={view}
            overlay={overlay}
            loading={loading}
            failed={failed}
            readOnly={readOnly}
            onAllDeals={() => onTab("deals")}
            onOpenHistory={onOpenHistory}
            onOpenTab={onTab}
            onOpenRecord={receipt.open}
            onOpenTasks={() => onTab("tasks")}
            onPrepareMeeting={setPreparing}
            onDraftTo={(id) => onCompose({ kind: "account", id })}
            onPerform={onPerform}
          />
        </div>
      )}
      <PersonMeetingBrief
        activityId={preparing}
        open={preparing !== null}
        onClose={() => setPreparing(null)}
        projects={liveProjects(view?.projects)}
      />
      {/* Deals and Tasks, pulled off the overview: a reader who came for the
          commercial picture or the open work should not scroll past the
          day's brief to find either. */}
      {!overlay && (
        <CompanyDealsAndTasksTabs
          tab={tab}
          org={org}
          view={view}
          failed={failed}
          readOnly={readOnly}
          openTaskId={openTaskId}
          onOpenTask={onOpenTask}
          taskUpdate={taskUpdate}
        />
      )}
      {/* The composer, anchored on the message a draft_reply suggestion named.
          It is the same modal the timeline's own Reply opens — the advice
          shortcuts to it rather than inventing a second way to answer. */}
      {composing && (
        <AccountComposer
          anchor={composing}
          orgId={org.id}
          onClose={() => onCompose(null)}
        />
      )}
      {/* The People tab gives the account team the whole middle column, with
          room for the title and the last exchange beside each name. The
          rail's capped summary stands beside it — a top-3 glance is the
          reader's anchor across tabs, not a second copy of the roster. */}
      {tab === "people" && (
        <div className="co-overview-stack">
          {/* The account's people, ranked and paged. One representation, not
              three: the roster card, the connections card and its diagram all
              answered "who works here" again in a different shape, and the
              reader's question is which of them to write to. */}
          {!overlay && !view?.sections_omitted?.includes("people") && (
            <CompanyPeopleList
              orgId={org.id}
              bandSlot={(narrow) => (
                <CoverageBand
                  orgId={org.id}
                  accountName={org.display_name}
                  onNarrow={narrow}
                />
              )}
            />
          )}
        </div>
      )}
      {/* Files get the whole column on their own tab, which is what the mockup
          gives them. The grid keeps its compact card for the reader who only
          wants to know whether there IS paperwork. */}
      {/* The money gets the whole column: what is overdue, what has been
          invoiced over the year, and how this account pays are three readings
          a rep opens the page WITH a question about, rather than meets on the
          way past the day's brief.
          The card keeps its own lifecycle-varying title inside the tab — a
          former customer's figures still read under "Finance (historical)",
          which is the one thing the tab strip above cannot say, because a tab
          label names a place and does not qualify it. */}
      {!overlay && tab === "finance" && (
        <CompanyFinanceCard orgId={org.id} lifecycle={org.lifecycle} />
      )}
      {!overlay && tab === "documents" && (
        // The same stacked column the overview uses, so the two panels are
        // spaced like every other pair of panels in this record rather than
        // touching at the border.
        <div className="co-overview-stack">
          {/* The agreements come first: contract paper is what a reader opens
              this tab for, and the library beneath it is everything else —
              literally everything else, since the library withholds the paper
              already read on an agreement's own row. Two panels, and no file on
              both of them. */}
          <CompanyContractsCard orgId={org.id} />
          <CompanyDocumentsCard orgId={org.id} />
        </div>
      )}
      {/* The decision queue belongs to the OVERVIEW. Leaving it standing over
          Partner put a panel from one tab on top of another, and a reader who
          switched tabs to get rid of it could not. */}
      {decisionsOpen && tab === "overview" && (
        <CompanyApprovalsPanel
          orgId={org.id}
          view={view}
          onClose={() => onDecisionsOpen(false)}
        />
      )}
      {receipt.cited && (
        <EvidenceModal
          orgId={org.id}
          cited={receipt.cited}
          onClose={receipt.close}
          onStep={receipt.step}
        />
      )}
      {/* The message a citation opened. Beside the receipt modal above and for
          the same reason: both are what a chip on this page opens into. */}
      <OpenEmailDrawer
        activityId={receipt.email}
        zone={viewerZone()}
        onClose={receipt.closeEmail}
      />
      {!overlay && (
        <CompanyProfileTab
          active={tab === "profile"}
          org={org}
          offerOnOverview={nothingOnFile(view)}
          onOpenHistory={onOpenHistory}
          t={t}
        />
      )}
    </>
  );
}

// The overview's own stack: one vertical column of same-shaped panels, in the
// order a rep works them — what is worth doing next, then the account itself,
// what it is worth to us, the commercial picture, the money, and what
// happened lately. Extracted from CompanyPage because each section used to be
// its own `tab === "overview" && view &&` branch there, and the page had
// become a list of conditions rather than a layout.
// Whether this account has anything captured against it at all.
//
// Three sections, and ALL must be empty: a company with one contact and no
// mail is still a company somebody has started on, and offering to go and
// research it would talk over the work they have already done.
//
// A section the reader may not see is NOT empty — `sections_omitted` names
// those, and an absent-because-withheld section read as "nothing here" would
// offer a crawl of an account that is already full, to the one reader who
// cannot see that it is.
function nothingOnFile(view?: Organization360View): boolean {
  if (!view) {
    return false;
  }
  const withheld = new Set(view.sections_omitted ?? []);
  const empty = (
    section: "people" | "deals" | "activities",
    rows: number | undefined,
  ) => !withheld.has(section) && rows === 0;
  return (
    empty("people", view.people?.data.length) &&
    empty("deals", view.deals?.data.length) &&
    empty("activities", view.activities?.data.length)
  );
}

// The create verb the work card carries, only where the reader may actually
// read the section. Undefined for an archived record, which takes no new work
// at all, and undefined for a reader whose grant does not reach the deals —
// a button that can only end in a refusal is worse than no button, and
// mounting one is a disclosure of its own.
function workVerbs({
  view,
  org,
  readOnly,
}: Readonly<{
  view?: Organization360View;
  org: Organization;
  readOnly: boolean;
}>): { deal?: ReactNode } | undefined {
  if (readOnly) {
    return undefined;
  }
  return {
    deal: !view?.sections_omitted?.includes("deals") ? (
      <NewDealAction orgId={org.id} orgName={org.display_name} />
    ) : undefined,
  };
}

function CompanyOverviewStack({
  org,
  view,
  overlay,
  loading,
  failed,
  readOnly,
  onAllDeals,
  onOpenHistory,
  onOpenRecord,
  onOpenTasks,
  onPrepareMeeting,
  onDraftTo,
  onOpenTab,
  onPerform,
}: Readonly<{
  org: Organization;
  view?: Organization360View;
  overlay: boolean;
  // The composite read's own pending flag — see CompanyRecordBody's own doc.
  loading: boolean;
  failed: boolean;
  // An archived company takes no new deal, task or role, so the panels below
  // show no verb that would only be refused.
  readOnly: boolean;
  onAllDeals: () => void;
  onOpenHistory: () => void;
  // Where a cited chip leads. Owned by the page, because the profile tab cites
  // the same records and two owners would mean two receipts open over each
  // other.
  onOpenRecord: (entityType: string, entityId: string) => void;
  onOpenTasks: () => void;
  // Opens the meeting brief for the day's meeting — not the composer.
  onPrepareMeeting: (activityId: string) => void;
  onDraftTo: (personId: string) => void;
  onOpenTab: (tab: CompanyTab) => void;
  onPerform: (action: SuggestionAction) => void;
}>) {
  // The names the reading resolves ids against: the account's own records, and
  // the workspace roster for the colleague who held a meeting. Read here rather
  // than inside the thread, because the roster is a workspace read and the
  // thread is a presentational component that holds none of its own.
  const roster = useRoster("user", !overlay);
  const colleagues = new Map(
    (roster.data ?? []).flatMap((entry) =>
      "display_name" in entry ? [[entry.id, entry.display_name] as const] : [],
    ),
  );
  const records = recordNamesIn(view);
  const nameOf = (entityType: string, entityId: string) =>
    entityType === "user"
      ? colleagues.get(entityId)
      : records(entityType, entityId);
  // The reader's scan of this account, asked for on open. Only once the 360
  // has answered natively: the scan is read from the same records, and an
  // overlay workspace has none of them here.
  const scan = useAccountScan(org.id, !overlay && view !== undefined);
  // ONE reading of the account, drawn in two panes: the 360's call at the
  // full measure, and the needs list in the left column under it. Computed
  // here, once, so the verdict and the queue cannot disagree.
  const reading = useTodayReading({
    orgId: org.id,
    view,
    loading,
    failed,
    onPrepareMeeting,
    onDraftTo,
    onOpenRecord,
    onPerform,
    scan,
  });
  return (
    <div className="co-overview-stack">
      {/* The readings lead, under the bar that chose this tab: five doors into
          the tabs that hold their rows. They belong to THIS tab rather than to
          the record — the People tab is a roster and the Documents tab is a
          filing cabinet, and a row of account readings over either is a header
          for a page it is not describing. */}
      {!overlay && view && (
        <StateStrip orgId={org.id} view={view} onOpenTab={onOpenTab} />
      )}
      {/* An account with nothing on file is asked a different question from a
          running one: not "what should you do here" but "shall Margince go and
          find out who these people are". So the research offer LEADS the
          column on such an account, and stands down the moment there is
          anything to read: on a live account the lead is the 360 below, and
          two leads is none. */}
      {!overlay && nothingOnFile(view) && <DeepReadCard orgId={org.id} />}
      {/* The 360 as the first pane, at the full measure (DESIGN.md §7): the
          word, the sentence it rests on, the three dimensions, the spine, and
          the thread folded under it. What moved since this reader was last
          here rides in its foot — the account's own clock, on the account's
          own reading. */}
      {!overlay && (
        <Company360Call
          reading={reading}
          name={org.display_name}
          footer={sinceLastVisitFooter(view)}
        >
          <RecordSpine
            source={view}
            commercial={view?.state_strip?.commercial}
            // The thread names the people on each conversation off the
            // account's own roster: the links carry ids, and an id is not a
            // person a reader recognises. Colleagues come from the workspace
            // roster rather than the account — the person who held a meeting
            // is one of ours, and the account's own people are the other side
            // of it.
            nameOf={nameOf}
            // The page's own router, which already sends an `activity` to the
            // email drawer for every cited chip on this account
            // (citationOpensEmail). The thread takes that same door rather
            // than a second opener somebody would have to keep in step.
            onOpenEmail={(activityId) => onOpenRecord("activity", activityId)}
          />
          {/* Keyed on the account, so its fold is the account's own. The page
              stays mounted while the route swaps organizations, and without
              this a thread a reader OPENED on one company arrives open on the
              next — carrying one account's reading into another's, and
              spending the glance the fold is closed to protect. */}
          <ThreadFold
            key={org.id}
            view={view}
            loading={loading}
            onOpenHistory={onOpenHistory}
            onOpenRecord={onOpenRecord}
          />
        </Company360Call>
      )}
      {/* Two columns under the 360. Left: what needs a person, then the money.
          Right: Ask, then what the account is, then who is there. Each is one
          pane, and the order is the order a rep works them. */}
      {!overlay && (
        <div className="co-glance-cols">
          <div className="co-glance-col">
            <NeedsList reading={reading} onOpenTasks={onOpenTasks} />
            <MoneyPane
              organizationId={org.id}
              view={view}
              loading={loading}
              readOnly={readOnly}
              onAllDeals={onAllDeals}
              onOpenRecord={onOpenRecord}
              // The verbs ride with the WORK rather than with the figures:
              // this is the pane that names every open deal, so it is where a
              // reader is standing when they notice one is missing. Each is
              // gated on its own section being READABLE, and the guard is here
              // rather than inside the verb: NewDealAction reads the pipelines
              // the moment it mounts, which is itself a disclosure to a reader
              // who may not see deals.
              verbs={workVerbs({ view, org, readOnly })}
            />
          </div>
          <div className="co-glance-col">
            {/* The prepared questions are the ones the 360 answers in prose,
                and both are written server-side from this reader's own 360
                and cite records through the same receipt. Beside the reading
                rather than at the foot of the page, so a reader does not
                discover at the bottom that they could have asked at the top. */}
            <div className="co-glance-ask">
              <AssistantPanel
                orgId={org.id}
                enabled={!overlay}
                onOpenRecord={onOpenRecord}
                projects={view?.projects}
              />
            </div>
            {/* The account in prose, beside the reading of it: the 360 answers
                what to DO, this answers what the account IS, in sentences with
                their sources under them. */}
            <DossierPanel
              orgId={org.id}
              enabled
              nameOf={records}
              onOpenRecord={onOpenRecord}
            />
            {/* Is this an account we should be selling to at all — the
                question an account with nothing in flight is actually asking.
                Its own card rather than a section of the money pane: it
                carries its own attribution and its own reassess verb in a
                footer band. */}
            {!hasWorkInFlight(view) && (
              <GrowthFitPanel
                orgId={org.id}
                enabled={!overlay}
                onOpenRecord={onOpenRecord}
              />
            )}
            {/* What Margince noticed on this account that nobody asked it to
                look for — promises made, blockers named, risks read out of
                meetings, mail and invoices. */}
            <Panel className="co-signals">
              <SignalsSection orgId={org.id} />
            </Panel>
            <PeopleChips view={view} loading={loading} onOpenTab={onOpenTab} />
          </div>
        </div>
      )}
    </div>
  );
}

// CompanyDealsAndTasksTabs holds both new tab bodies plus the modal either
// can open. Split out of CompanyPage purely to keep that render legible —
// the two tabs are mutually exclusive and share nothing but the record.
function CompanyDealsAndTasksTabs({
  tab,
  org,
  view,
  failed,
  readOnly,
  openTaskId,
  onOpenTask,
  taskUpdate,
}: Readonly<{
  tab: CompanyTab;
  org: Organization;
  view?: Organization360View;
  failed: boolean;
  readOnly: boolean;
  openTaskId: string | null;
  onOpenTask: (activityId: string | null) => void;
  taskUpdate: ReturnType<typeof useTaskUpdate>;
}>) {
  const t = useT();
  return (
    <>
      {tab === "deals" && (
        // The record's own stack, which is what every tab body with more than
        // one panel takes: the work column draws its children with no interval
        // of its own, so two panels rendered as bare siblings meet at the
        // border and read as one card with a rule through it.
        <div className="record-stack">
          {/* Beside the deals card, not inside it. The card's `extra` slot only
              renders when the deals section itself is readable, so a reader
              holding the contract grant and not the deal grant would never see
              what the account is under contract for — a section withheld by
              somebody else's permission.
              In its own Panel: the block renders PanelBody rows, and standing
              bare over the deals card it read as a line that fell out of one.
              The Panel is drawn only when the reader holds the contract grant
              (the same `contracts` slice the block itself reads), so a reader
              without it gets no empty card. */}
          {view?.state_strip?.contracts && (
            <Panel title={t("co.commercial.title")}>
              <CompanyContractState view={view} />
            </Panel>
          )}
          <CompanyDealsTab
            org={org}
            view={view}
            failed={failed}
            readOnly={readOnly}
          />
        </div>
      )}
      {tab === "tasks" && (
        <CompanyTasksTab
          view={view}
          failed={failed}
          readOnly={readOnly}
          onOpenTask={onOpenTask}
          update={taskUpdate}
        />
      )}
      {tab === "tasks" && openTaskId && (
        <TaskDetailModal
          activityId={openTaskId}
          readOnly={readOnly}
          onClose={() => onOpenTask(null)}
          update={taskUpdate}
        />
      )}
    </>
  );
}

// CompanyDealsTab: the pipeline plus the last commercial exchange, both cited
// evidence-or-omit off the composite read like every other business card.
// Its own component (rather than inlined in CompanyPage) so it can hold the
// same loading/failed split CompanyBusinessGrid does — a card told there is
// no view either says "could not load" or "not yet", and only the caller
// knows which is true.
//
// ONE Panel, not two: the last offer is read off the SAME open deals the
// pipeline list above it already shows, so it renders as `DealsCard`'s
// `extra` slot rather than as a second card repeating "this account's deals"
// under a different heading.
function CompanyDealsTab({
  org,
  view,
  failed,
  readOnly,
}: Readonly<{
  org: Organization;
  view?: Organization360View;
  failed: boolean;
  // An archived company takes no new deal, so it shows no verb that would
  // only be refused.
  readOnly: boolean;
}>) {
  if (!view && !failed) {
    return (
      <Card className="co-card">
        <Skeleton width="100%" height={96} />
      </Card>
    );
  }
  return (
    <DealsCard
      view={view}
      actions={
        readOnly ? undefined : (
          <NewDealAction orgId={org.id} orgName={org.display_name} />
        )
      }
      extra={<CompanyLastOffer view={view} />}
    />
  );
}

// CompanyTasksTab: the account's open tasks, with the same tick-to-complete
// verb the standing work queue offers (taskactions.tsx) — one mutation, so a
// task finished here is finished on the queue too.
//
// `NextSteps` renders `null` on a withheld section (it is a middle-column
// block there, and dropping it is right for that layout) — as a whole TAB
// body that would be a blank tab, exactly what the four-states rule forbids,
// so the withheld case is caught here before `NextSteps` ever sees it.
function CompanyTasksTab({
  view,
  failed,
  readOnly,
  onOpenTask,
  update,
}: Readonly<{
  view?: Organization360View;
  failed: boolean;
  // An archived company takes no new activity, so completing or snoozing a
  // task from here would only be refused server-side.
  readOnly: boolean;
  onOpenTask: (activityId: string) => void;
  update: ReturnType<typeof useTaskUpdate>;
}>) {
  const t = useT();
  if (!view && !failed) {
    return (
      <Card className="co-card">
        <Skeleton width="100%" height={96} />
      </Card>
    );
  }
  if (!view) {
    return <EmptyState>{t("co.partial")}</EmptyState>;
  }
  const steps = view.next_steps?.data ?? [];
  if (
    sectionState(view, "next_steps", Boolean(view.next_steps), steps.length) ===
    "withheld"
  ) {
    // Same chrome as every other state of this tab. A refusal drawn as a bare
    // plate names no section, and the account facts strip above says the same
    // sentence about its own withheld halves — so a reader who lands on an
    // unheaded one cannot tell which of the two they are being refused.
    return (
      <Panel title={t("co.next.title")}>
        <PanelBody>
          <p className="surfacestate-withheld">{t("co.section.restricted")}</p>
        </PanelBody>
      </Panel>
    );
  }
  return (
    <NextSteps
      view={view}
      onOpenTask={(step) => onOpenTask(step.activity_id)}
      update={readOnly ? undefined : update}
      renderAction={
        readOnly
          ? undefined
          : (step) => (
              <TaskQuickActions
                activityId={step.activity_id}
                dueAt={step.due_at}
                update={update}
                showComplete={false}
              />
            )
      }
    />
  );
}

// openCitation routes a cited record to its own screen. The brief, the
// prepared answers and the suggestions all cite the same records, so they
// share one route — a second copy would drift and send one card's reader to
// the wrong screen.
// A citation goes to one of two places. A deal or a person has a screen of its
// own; a fact or a profile field has no screen, but it does have a receipt —
// where the value came from and what could not be recorded about it — which is
// what the reader wanted when they clicked the chip.
function citationOpensRecord(entityType: string): boolean {
  return entityType === "deal" || entityType === "person";
}

// An activity opens the MESSAGE, in the account page's own email drawer.
//
// Its own named decision rather than a bare comparison inside the hook, so the
// rule can be asserted without mounting the page: the account's commitment rows
// have passed `source_activity_id` into a button since the day they were
// written, and it did nothing because an activity fell through both branches
// above. A rule with no name is a rule with no test.
export function citationOpensEmail(entityType: string): boolean {
  return entityType === "activity";
}

// The kinds a receipt can be written for. Narrowing HERE rather than asserting
// at the fetch is what keeps the modal's contract honest: a kind that grows a
// receipt upstream fails to compile until this decision learns about it.
function citationHasReceipt(
  entityType: string,
): entityType is CitedRecord["entityType"] {
  return entityType === "fact" || entityType === "profile_field";
}

function openCitation(entityType: string, entityId: string) {
  if (entityType === "deal") {
    navigate({ screen: "deals", id: entityId });
  }
  if (entityType === "person") {
    navigate({ screen: "contacts", id: entityId });
  }
}

// The reference material a reader opens when the summary above is not enough.
//
// It renders whatever state the 360 is in, because none of it comes from the
// 360: each card runs its own read. That is the rule to keep as the layout
// moves — a failed composite read hides what the 360 answered, not the
// company's profile, its facts or its relationships.
// CompanyProfileTab: the account's own reference material — what it looks
// like in its own words and ours, its filed fields, its facts, who it is
// connected to and the one-off tools. None of it comes from the 360; each
// card runs its own read, so it renders whichever state that read is in
// rather than following the composite's. The tab is gated on overlay at the
// call site — the page has already refused once there.
function CompanyProfileTab({
  active,
  org,
  // Whether the 360 is already leading with the research offer. It renders in
  // exactly ONE place: an account with nothing on file meets it at the top of
  // its 360, and every other account finds it here among the record's tools.
  // Both at once is two buttons that start the same crawl.
  offerOnOverview,
  onOpenHistory,
  t,
}: Readonly<{
  active: boolean;
  org: Organization;
  offerOnOverview: boolean;
  onOpenHistory: () => void;
  t: ReturnType<typeof useT>;
}>) {
  if (!active) {
    return null;
  }
  return (
    <ReferenceDisclosures
      org={org}
      offerOnOverview={offerOnOverview}
      onOpenHistory={onOpenHistory}
      t={t}
    />
  );
}

function ReferenceDisclosures({
  org,
  offerOnOverview,
  onOpenHistory,
  t,
}: Readonly<{
  org: Organization;
  offerOnOverview: boolean;
  onOpenHistory: () => void;
  t: ReturnType<typeof useT>;
}>): ReactNode {
  return (
    <CompanyProfileForm
      org={org}
      onOpenHistory={onOpenHistory}
      tools={
        <>
          {/* Documents are deliberately NOT here: they have their own tab, and a
              reader given the same list in two places has two lists to
              reconcile. */}
          <Panel title={t("co.relationships.title")}>
            <PanelBody>
              <RelationshipsTab scope={{ organization_id: org.id }} />
            </PanelBody>
          </Panel>
          <Panel title={t("co.tools.title")}>
            <PanelBody>
              <CustomFieldsCard object="organization" record={org} />
              <HierarchyRollupCard orgId={org.id} />
              {/* Only where the Brief is not already offering it: an account
                  with nothing on file meets the offer at the top of its own
                  column, and two offers to research the same company is none. */}
              {!offerOnOverview && <DeepReadCard orgId={org.id} />}
              {/* What the company RUNS, beside what it SAYS — read from public
                  records the company never wrote for us: DNS, certificates, the
                  markup of their own homepage. It sits under the read that
                  produces it rather than in a section of its own, because the
                  site read above is what queues it. */}
              <TechnicalProfileCard orgId={org.id} />
            </PanelBody>
          </Panel>
        </>
      }
    />
  );
}
