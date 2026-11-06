import { useQueries, useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { DealCard } from "../../design-system/dealcard";
import { Panel, PanelBody } from "../../design-system/panel";
import { StatCard } from "../../design-system/statcard";
import { StatStrip } from "../../design-system/statstrip";
import { SurfaceState, sectionState } from "../../design-system/surfacestate";
import { formatDayMonth, formatNumber } from "../../format/format";
import { type Locale, useLocale, usePlural, useT } from "../../i18n";
import { useCan, useCanWrite } from "../../lib/capability";
import { throwProblem } from "../../lib/problem";
import { navigate, routeHash } from "../../lib/router";
import "./contact360.css";
import { toBoardDeal } from "../../components/boarddeal";
import { openValueWords } from "../../components/openvaluewords";
import { AddRelationshipAction } from "../../components/relationships";
import { closeDayOf } from "../../lib/closeday";
import { openValue } from "../../lib/openvalue";
import { dealRoleLabel } from "../../lib/recordlabels";
import { useViewerZone } from "../../lib/viewerzone";
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
  return useQuery(dealSummaryQuery(dealId));
}

// One read per deal on the tab. The figures and each card share that one
// cache entry.
function dealSummaryQuery(dealId: string) {
  return {
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
  };
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

// One deal this contact sits on, as the board draws a deal, with its stage
// and this contact's seat. Until the deal itself is read, or where it may not
// be, the row that `deal_roles` alone can carry stands in.
function DealTile({ role }: Readonly<{ role: DealRole }>) {
  const t = useT();
  const { locale } = useLocale();
  const readerZone = useViewerZone();
  const deal = useDealSummary(role.deal_id).data;
  const companyRead = useDealCompany(deal?.company_id);
  const company = companyRead.data;
  const roleWord = dealRoleLabel(role.role, t);
  if (!deal) {
    return <DealRow role={role} roleWord={roleWord} />;
  }
  const marks = new Map(
    company
      ? [
          [
            company.id,
            { name: company.display_name, logoUrl: company.logo_url },
          ],
        ]
      : [],
  );
  return (
    <li>
      <DealCard
        deal={toBoardDeal(deal, {
          marks,
          // A company the read failed on, or refused, keeps its row as one
          // not read rather than vanishing from the card.
          unreadable: new Set(
            deal.company_id && (companyRead.isError || company === null)
              ? [deal.company_id]
              : [],
          ),
        })}
        href={routeHash({ screen: "deals", id: deal.id })}
        zone={readerZone}
        closed={
          deal.status === "open"
            ? undefined
            : closeLine(deal, t, locale, readerZone)
        }
        standing={
          <>
            <Badge tone="info">
              {role.deal_stage ?? t("contact.deals.noStage")}
            </Badge>
            {roleWord && <Badge>{roleWord}</Badge>}
          </>
        }
      />
    </li>
  );
}

// The deal by its seat alone: title, stage and close, for a deal not yet read
// or one this reader may not open.
function DealRow({
  role,
  roleWord,
}: Readonly<{ role: DealRole; roleWord: string | null }>) {
  const t = useT();
  const { locale } = useLocale();
  const readerZone = useViewerZone();
  const deal = useDealSummary(role.deal_id).data;
  const meta = [
    role.deal_stage ?? t("contact.deals.noStage"),
    closeLine(deal, t, locale, readerZone),
  ]
    .filter(Boolean)
    .join(" · ");
  return (
    <li className="ct-deal-row">
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
    </li>
  );
}

// The deals in figures: how many are open, what they are worth together, and
// which closes next. Until every deal is read in full, a figure says it is a
// floor or says nothing. It never prints part of the answer.
function DealFigures({ roles }: Readonly<{ roles: DealRole[] }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const readerZone = useViewerZone();
  const reads = useQueries({
    queries: roles.map((role) => dealSummaryQuery(role.deal_id)),
  });
  const settled = reads.every((read) => read.isSuccess);
  const deals = reads
    .map((read) => read.data)
    .filter((deal): deal is Deal => deal != null);
  const complete = settled && deals.length === roles.length;
  const open = deals.filter((deal) => deal.status === "open");
  const next = open
    .filter((deal) => deal.expected_close_date)
    // ISO days sort as plain strings, in no locale.
    .sort((a, b) =>
      (a.expected_close_date ?? "") < (b.expected_close_date ?? "") ? -1 : 1,
    )[0];
  const pending = "…";
  return (
    <StatStrip density="plate" label={t("contact.deals.figures")}>
      <StatCard
        label={t("contact.deals.openCount")}
        value={
          settled
            ? `${formatNumber(open.length, locale)}${complete ? "" : "+"}`
            : pending
        }
        detail={plural("contact.deals.ofAll", roles.length, {
          count: formatNumber(roles.length, locale),
        })}
      />
      <StatCard
        label={t("contact.deals.openValue")}
        value={
          settled
            ? openValueWords(openValue(open, complete), t, locale)
            : pending
        }
      />
      <StatCard
        label={t("contact.deals.nextClose")}
        value={
          !settled
            ? pending
            : next?.expected_close_date
              ? formatDayMonth(next.expected_close_date, locale, readerZone)
              : t("contact.deals.noClose")
        }
        detail={next?.name}
      />
    </StatStrip>
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
          <PanelBody>
            <DealFigures roles={cards} />
            <ul className="ct-deal-grid">
              {cards.map((role) => (
                <DealTile key={role.relationship_id} role={role} />
              ))}
            </ul>
          </PanelBody>
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
