import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import { SurfaceState, sectionState } from "../../design-system/surfacestate";
import { formatDayMonth, formatMoneyCompact } from "../../format/format";
import { type Locale, useLocale, useT } from "../../i18n";
import { useCan, useCanWrite } from "../../lib/capability";
import { throwProblem } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { navigate } from "../../lib/router";
import "./contact360.css";
import { dealRoleLabel } from "../../lib/recordlabels";
import { closeDayOf } from "../deals/closeday";
import { AddRelationshipAction } from "../records/relationships";
import { ContactDealRooms } from "./contactdealrooms";
import "./contacttabs.css";

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

// The close day the deal header states, worded for where the deal stands.
// Uses the record's own zone. A forecast is a date-only value with no instant
// to localize, and a reader west of UTC would otherwise quote the day before.
function closeLine(
  deal: Deal | null | undefined,
  t: ReturnType<typeof useT>,
  locale: Locale,
  zone: string,
): string | null {
  const day = deal ? closeDayOf(deal) : null;
  if (!deal || !day) {
    return null;
  }
  const closed = deal.status === "won" || deal.status === "lost";
  return t(closed ? "contact.commercial.closed" : "contact.commercial.closes", {
    date: formatDayMonth(day, locale, zone),
  });
}

// One deal this contact sits on: the seat `deal_roles` carries, and the
// money, the close date and the company `useDealSummary`/`useDealCompany`
// fetch for it.
function DealRow({ role }: Readonly<{ role: DealRole }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const deal = useDealSummary(role.deal_id).data;
  const company = useDealCompany(deal?.company_id).data;
  const roleWord = dealRoleLabel(role.role, t);
  const meta = [
    deal?.amount_minor != null && deal.currency
      ? formatMoneyCompact(deal.amount_minor, deal.currency, locale)
      : null,
    role.deal_stage ?? t("contact.deals.noStage"),
    closeLine(deal, t, locale, recordZone),
    company?.display_name ?? null,
  ]
    .filter(Boolean)
    .join(" · ");
  return (
    <PanelRow record className="ct-item">
      <div className="ct-item-main">
        <button
          type="button"
          className="ct-item-link"
          onClick={() => navigate({ screen: "deals", id: role.deal_id })}
        >
          {role.deal_title ?? t("contact.deals.untitled")}
        </button>
        <span className="t-caption">{meta}</span>
      </div>
      {roleWord && <Badge>{roleWord}</Badge>}
    </PanelRow>
  );
}

// The other seats on the one deal the commercial section tracks as open, said
// once under the list. A seat with no recorded role shows the name alone.
function Committee({ members }: Readonly<{ members: CommitteeMember[] }>) {
  const t = useT();
  return (
    <p className="t-caption ct-committee">
      <span>{t("contact.commercial.committee")}</span>
      {members.map((member) => (
        <span className="ct-seat" key={member.contact_id}>
          <span className="ct-seat-name">{member.full_name}</span>
          {member.role && (
            <>
              {" "}
              <span className="ct-seat-role">
                {dealRoleLabel(member.role, t)}
              </span>
            </>
          )}
        </span>
      ))}
    </p>
  );
}

// The rows `deal_roles` carries. The open deal can sit past `deal_roles`' own
// page cut: present in `commercial` with no row above to carry its seat.
// Appended rather than dropped, because losing it here would be the seat and
// the committee disappearing off the edge of a page cut the reader never
// asked about.
export function dealCards(view?: Contact360): DealRole[] {
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
 * ContactDealsTab is one list of every deal this contact is recorded on. Each
 * row carries its money, its stage, its close date and its company, with the
 * seat at the far edge. The buying committee of the one deal the commercial
 * section tracks as open is said once, under the list.
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
  const committee = view?.commercial?.committee ?? [];
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
        footer={
          state === "ready" && committee.length > 0 ? (
            <Committee members={committee} />
          ) : undefined
        }
      >
        {state === "ready" ? (
          cards.map((role) => (
            <DealRow key={role.relationship_id} role={role} />
          ))
        ) : (
          <PanelBody>
            <SurfaceState
              state={state}
              emptyLabel={t("contact.deals.empty")}
              loadingLabel={t("tab.deals")}
            >
              {null}
            </SurfaceState>
          </PanelBody>
        )}
      </Panel>
      <ContactRooms view={view} />
    </div>
  );
}

// The rooms this contact holds a seat in, under the deals the rooms belong to.
// Withheld without the room grant, whose list read would only be refused, and
// for a contact with no address, since a seat is held by address.
function ContactRooms({ view }: Readonly<{ view?: Contact360 }>) {
  const mayRead = useCan("deal_room", "read");
  const emails = (view?.contact.emails ?? []).map((e) => e.email);
  if (!mayRead || emails.length === 0) {
    return null;
  }
  return <ContactDealRooms emails={emails} />;
}
