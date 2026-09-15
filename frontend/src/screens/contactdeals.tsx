import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import { Avatar } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { SurfaceState, sectionState } from "../design-system/surfacestate";
import { formatDayMonth, formatMoneyCompact } from "../format/format";
import { useLocale, useT } from "../i18n";
import { throwProblem } from "./common";
import "./contact360.css";
import { readableRole } from "./contactcards";
import { AddRelationshipAction } from "./relationships";

type Contact360 = components["schemas"]["Contact360"];
type Deal = components["schemas"]["Deal"];
type Company = components["schemas"]["Company"];

// --- Deals ------------------------------------------------------------------

type DealRole = NonNullable<Contact360["deal_roles"]>["data"][number];
type CommitteeMember = NonNullable<
  Contact360["commercial"]
>["committee"][number];

// The deal's own record: `deal_roles` carries only the title, the stage word
// and the seat, and the money and the close date live on the deal itself.
// 403/404 read as "nothing to show" rather than an error: the same degrade
// `useEmployerSummary` (contactemployers.tsx) gives a company this reader may
// not open. Not shared with that hook: it is private to its own file, and
// this task's touch scope stops at this one.
function useDealSummary(dealId: string) {
  return useQuery({
    queryKey: ["deal", dealId],
    queryFn: async (): Promise<Deal | null> => {
      const { data, error, response } = await api.GET("/deals/{id}", {
        params: { path: { id: dealId } },
      });
      if (error) {
        if (response.status === 403 || response.status === 404) {
          return null;
        }
        throwProblem(error);
      }
      return data;
    },
    staleTime: 5 * 60_000,
  });
}

// `Deal.company_id` names the deal's company but carries no display name of
// its own, so the card asks for it the same way `useDealSummary` asks for the
// deal: by id, with the same 403/404 degrade. Shares its cache key with
// `useEmployerSummary`'s, so a company already read for the Companies panel
// is not fetched twice.
function useDealCompany(companyId: string | null | undefined) {
  return useQuery({
    queryKey: ["company", companyId ?? null],
    enabled: companyId != null,
    queryFn: async (): Promise<Company | null> => {
      if (!companyId) {
        return null;
      }
      const { data, error, response } = await api.GET("/companies/{id}", {
        params: { path: { id: companyId } },
      });
      if (error) {
        if (response.status === 403 || response.status === 404) {
          return null;
        }
        throwProblem(error);
      }
      return data;
    },
    staleTime: 5 * 60_000,
  });
}

// One deal this contact sits on: the seat `deal_roles` carries, and the
// money, the close date and the company `useDealSummary`/`useDealCompany`
// fetch for it. `committee` is passed only for the one deal the commercial
// section is tracking as open: the other stakeholders in the room, which no
// per-deal fetch here carries.
function DealCard({
  role,
  committee,
}: Readonly<{
  role: DealRole;
  committee?: CommitteeMember[];
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const deal = useDealSummary(role.deal_id).data;
  const company = useDealCompany(deal?.company_id).data;
  const roleWord = readableRole(role.role);
  const meta = [
    deal?.amount_minor != null && deal.currency
      ? formatMoneyCompact(deal.amount_minor, deal.currency, locale)
      : null,
    role.deal_stage ?? t("contact.deals.noStage"),
    deal?.expected_close_date
      ? t("contact.commercial.closes", {
          // The record's own zone: a close date is a date-only wire value
          // with no instant to localize, and a reader west of UTC rendering
          // it in their own would quote the day before to a colleague
          // quoting the right one.
          date: formatDayMonth(deal.expected_close_date, locale, recordZone),
        })
      : null,
    company?.display_name ?? null,
  ]
    .filter(Boolean)
    .join(" · ");
  return (
    <div className="pe-deal-card">
      <span className="pe-deal-card-title">
        <button
          type="button"
          className="pe-meta-link"
          onClick={() => navigate({ screen: "deals", id: role.deal_id })}
        >
          {role.deal_title ?? t("contact.deals.untitled")}
        </button>
      </span>
      <span className="pe-deal-card-meta t-caption">{meta}</span>
      {roleWord && <span className="t-caption">{roleWord}</span>}
      {committee && committee.length > 0 && (
        <div className="pe-deal-card-committee">
          <span className="t-caption">{t("contact.commercial.committee")}</span>
          {committee.map((member) => (
            <div className="pe-deal-card-committee-row" key={member.contact_id}>
              <Avatar name={member.full_name} src={member.photo_url} />
              <span>{member.full_name}</span>
              <span className="t-caption">{readableRole(member.role)}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

// The rows `deal_roles` carries. The open deal can sit past `deal_roles`' own
// page cut: present in `commercial` with no row above to carry its seat.
// Appended rather than dropped, because losing it here would be the seat and
// the committee disappearing off the edge of a page cut the reader never
// asked about.
function dealCards(view?: Contact360): DealRole[] {
  const roles = view?.deal_roles?.data ?? [];
  const openDeal = view?.commercial?.deal;
  if (openDeal && !roles.some((role) => role.deal_id === openDeal.deal_id)) {
    return [
      ...roles,
      {
        relationship_id: openDeal.deal_id,
        deal_id: openDeal.deal_id,
        deal_title: openDeal.title,
        deal_stage: openDeal.stage,
        role: view?.commercial?.role ?? "",
      },
    ];
  }
  return roles;
}

/**
 * ContactDealsTab is one list of cards: every deal this contact is recorded
 * on, each carrying its seat, its money, its stage and the company it is
 * with, plus (for the one deal the commercial section is tracking as open)
 * the buying committee besides. The committee used to live a second time in a
 * panel of its own below the list; folded into the matching card instead,
 * because a second wording of the same deal is how the two started to
 * disagree.
 */
export function ContactDealsTab({
  view,
  loading = false,
}: Readonly<{ view?: Contact360; loading?: boolean }>) {
  const t = useT();
  // The object half of the gate, asked as the server asks it. The row half —
  // whether this caller may write THIS contact — is the anchor's own, and the
  // server applies it to the write; a tab that second-guessed it here would
  // withhold the verb on a record the write would have accepted.
  const canSeat = useCanWrite("relationship", "create");
  const cards = dealCards(view);
  const state = sectionState(
    view,
    "deal_roles",
    // A card appended from the commercial deal counts as presence too, or
    // an absent deal_roles section would hide a deal the page can show.
    Boolean(view?.deal_roles) || cards.length > 0,
    cards.length,
    loading,
  );
  return (
    <div className="record-stack">
      <Panel
        title={t("tab.deals")}
        // THE TAB'S OWN VERB, and the same one the relationships tab offers —
        // reached from where a reader is already asking the question rather
        // than spelled a second time. Narrowed to the deal edge: a kind
        // selector offering "employment" on a Deals tab would ask the reader to
        // answer what the tab already answered.
        //
        // Withheld outright without the grant, as the relationships tab does:
        // there is no fact about this contact to report, so a disabled button
        // would be an affordance that says nothing.
        titleAction={
          view?.contact.id && canSeat ? (
            <AddRelationshipAction
              scope={{ contact_id: view.contact.id }}
              only={{ kind: "deal_stakeholder", label: "rel.seatOnDeal" }}
            />
          ) : undefined
        }
      >
        <PanelBody>
          <SurfaceState
            state={state}
            emptyLabel={t("contact.deals.empty")}
            loadingLabel={t("tab.deals")}
          >
            {cards.map((role) => (
              <DealCard
                key={role.relationship_id}
                role={role}
                committee={
                  view?.commercial?.deal?.deal_id === role.deal_id
                    ? view.commercial.committee
                    : undefined
                }
              />
            ))}
          </SurfaceState>
        </PanelBody>
      </Panel>
    </div>
  );
}
