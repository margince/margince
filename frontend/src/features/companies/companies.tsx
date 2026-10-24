import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  LIFECYCLE_LABELS,
  LIFECYCLE_OPTIONS,
  RELATIONSHIP_TYPE_LABELS,
  SIZE_BAND_OPTIONS,
} from "../../components/companylookups";
import { QueryGate } from "../../design-system/querystates";
import { RecordTabs } from "../../design-system/recordtabs";
import { useT } from "../../i18n";
import { COMPANY_TABS, type CompanyTab } from "../../lib/companytab";
import { unwrap } from "../../lib/problem";
import { scrollPageToTop } from "../../lib/reveal";
import { useAddressedTab } from "../records/recordtab";
import { CompanyPage } from "./companies.page";
import { useAcknowledgeCompanyView, useCompany360 } from "./company360";
import { hasFinance } from "./companyfinance";
import { wholeCount } from "./companyrailshared";
import { REFUSED_WHEN_ARCHIVED, useShownCompanyTab } from "./companytab";

export { CompaniesScreen } from "./companies.list";
export { profileFieldLabel } from "./companies.profile";

export type Company = components["schemas"]["Company"];

// Both vocabularies live in companylookups.ts, the leaf this screen and the rail
// share, because they overlap (`customer` is in both) and only a module holding
// both can tell a header is about to print one word twice. Re-exported so every
// existing caller of this module still resolves.
export { LIFECYCLE_LABELS, LIFECYCLE_OPTIONS, RELATIONSHIP_TYPE_LABELS };

export type Company360View = components["schemas"]["Company360"];

// Lives in companylookups.ts because the rail's Details grid builds a size-band
// picker off the same seven wire bands; re-exported for the same reason as above.
export { SIZE_BAND_OPTIONS };

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
  view?: Company360View,
): Partial<Record<CompanyTab, number>> {
  if (!view) {
    return {};
  }
  const counts: Partial<Record<CompanyTab, number>> = {};
  // The server's exact total where it sent one: the history page is capped,
  // so without it the whole-page rule leaves the tab uncounted.
  const history = view.activities?.page.total ?? wholeCount(view.activities);
  if (history != null) {
    counts.timeline = history;
  }
  const contacts = wholeCount(view.contacts);
  if (contacts != null) {
    counts.contacts = contacts;
  }
  const deals = dealsAndProjectsCount(view);
  if (deals != null) {
    counts.deals = deals;
  }
  const tasks = wholeCount(view.next_steps);
  if (tasks != null) {
    counts.tasks = tasks;
  }
  return counts;
}

// The tab holds both lists, so its figure is their sum, and only while both
// are whole. One cut-short list makes the sum a floor, not a count.
function dealsAndProjectsCount(view: Company360View): number | undefined {
  const deals = wholeCount(view.deals);
  if (
    deals == null ||
    !view.projects ||
    view.projects_page?.has_more !== false
  ) {
    return undefined;
  }
  return deals + view.projects.length;
}

function companyTabsFor(
  company: Company,
  tab: CompanyTab,
): readonly CompanyTab[] {
  // Gated on the relationship type, not on `company.partner`: the Company
  // read does not select the extension row, so that field is always absent
  // and every partner would lose the tab. The type is equivalent and IS
  // returned — a company carries it exactly when it has a programme, which the
  // store enforces in both directions (ADR-0079).
  const isPartner = (company.relationship_types ?? []).includes("partner");
  const drop = new Set<CompanyTab>();
  if (!isPartner && tab !== "partner") {
    drop.add("partner");
  }
  // Finance is absent exactly where its card is absent, on FIN-AC-3's own list
  // — an account nobody has ever invoiced has no money to report, and a tab
  // that opens onto nothing is worse than one that is not there.
  if (!hasFinance(company.lifecycle) && tab !== "finance") {
    drop.add("finance");
  }
  // An archived account's refused tabs would open onto nothing too.
  if (company.archived_at) {
    for (const refused of REFUSED_WHEN_ARCHIVED) drop.add(refused);
  }
  return drop.size === 0
    ? COMPANY_TABS
    : COMPANY_TABS.filter((id) => !drop.has(id));
}

export function CompanyScreen({ id }: Readonly<{ id: string }>) {
  const t = useT();
  // The tab is scoped to the ACCOUNT being read, the same reason the
  // chronology filter is (useChronologyFilter): the route swaps one company
  // for another without ever unmounting this component, so a reader who opened
  // Partner on one account met it again on the next — and companyTabsFor's own
  // carveout (a reader mid-way through setting up a programme keeps the tab
  // while `tab === "partner"`) has no way to tell "still this account" from
  // "a different one" unless something resets it at the boundary.
  const [tab, setTab] = useAddressedTab("companies", id, COMPANY_TABS);
  // The account itself still comes from its own read, so the header renders
  // whatever the composite is doing.
  const companyQuery = useQuery({
    queryKey: ["company", id],
    queryFn: async () => {
      return unwrap(
        await api.GET("/companies/{id}", {
          params: { path: { id } },
        }),
      );
    },
  });
  // Beside the account read; an archived row stops it, because the server
  // refuses that account's 360.
  const view = useCompany360(id, !companyQuery.data?.archived_at);
  // Only an assembled 360 counts as a visit: a page that never rendered the
  // account is not one the reader saw.
  useAcknowledgeCompanyView(id, view.data !== undefined);

  return (
    <div className="wrap">
      <QueryGate query={companyQuery} pendingLabel={t("nav.companies")}>
        {(company) => (
          <CompanyRecord
            company={company}
            view={view}
            tab={tab}
            onTab={setTab}
            t={t}
          />
        )}
      </QueryGate>
    </div>
  );
}

// CompanyRecord renders the page once the account itself has loaded. Split
// out so the 360's two states — assembling and assembled — are handled in one
// place rather than nested inside the account gate.
function CompanyRecord({
  company,
  view,
  tab: addressed,
  onTab,
  t,
}: Readonly<{
  company: Company;
  view: { data?: Company360View; isPending: boolean; isError: boolean };
  tab: CompanyTab;
  onTab: (next: CompanyTab) => void;
  t: ReturnType<typeof useT>;
}>) {
  const archived = Boolean(company.archived_at);
  const tab = useShownCompanyTab(company.id, archived, addressed);
  const assembled = archived ? undefined : view.data;
  const visibleTabs = companyTabsFor(company, tab);
  const tabs = (
    // A test id rather than a class: journeys walk the page's own tab strip,
    // and nothing draws this wrapper.
    <div data-testid="co-tabs">
      <RecordTabs
        options={visibleTabs}
        value={tab}
        onChange={(next) => {
          onTab(next);
          scrollPageToTop();
        }}
        counts={companyTabCounts(assembled)}
        labels={{
          // "360", not the shared "Overview": this tab is the account's
          // one assembled reading, and the card inside it is named the same
          // — a tab and the thing it opens calling themselves two different
          // words is two places to learn. Its own key rather than a re-worded
          // `tab.overview`, which four other record types render and none of
          // them is this.
          overview: t("tab.overview"),
          contacts: t("tab.contacts"),
          // `tab.dealsProjects` rather than the shared `tab.deals`: the contact
          // page's own Deals tab holds no projects, so renaming that key would
          // rename a tab that did not change. This label now also names the
          // lead's own Deals & projects tab, which reads the same two things.
          deals: t("tab.dealsProjects"),
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
  );

  // Both tabs render inside ONE page. Partner used to be a different
  // component tree with no rails, so switching tab unmounted both side
  // columns and every query behind them: the grid re-columned under the
  // reader and the page refetched itself on the way back. Only the middle
  // column's body changes now.
  return (
    <CompanyPage
      company={company}
      view={assembled}
      loading={!archived && view.isPending}
      failed={!archived && view.isError}
      tab={tab}
      tabs={tabs}
      onTab={onTab}
    />
  );
}
