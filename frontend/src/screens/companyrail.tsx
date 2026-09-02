import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { routeHash } from "../app/router";
import { Avatar, Button, Disclosure } from "../design-system/atoms";
import { AvatarStack } from "../design-system/avatarstack";
import { EvidenceMark } from "../design-system/evidencemark";
import { OffsiteLink } from "../design-system/offsitelink";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { Popover } from "../design-system/popover";
import {
  type SectionState,
  SurfaceState,
  sectionState,
} from "../design-system/surfacestate";
import {
  formatDate,
  formatMoneyOrAbsent,
  formatNumber,
} from "../format/format";
import { webUrl } from "../format/weburl";
import { useLocale, useT } from "../i18n";
import { problemCodeOf, throwProblem } from "./common";
import { NewDealAction } from "./companyactions";
import { useCompanyReadOnlyReason } from "./companyheader";
import { DetailsGrid } from "./companyraildetails";
import { SectionSummary, sectionAnswered } from "./companyrailshared";
import { TagsSection } from "./companyrailtags";
import { CounterpartyHoldRow } from "./counterparty-hold";
import { roleOf } from "./provider-status";
import { signalKindLabel, signalTone } from "./record360";
// The row and card shapes this file draws — co-rowlink, co-row-meta, co-card —
// are defined in company360.css. Imported HERE rather than left to the caller:
// it works today only because the company record page pulls that stylesheet in
// for its own sake, so this file renders unstyled anywhere else.
import "./company360.css";

// The record page's LEFT rail (mockup State A): the account's context,
// beside the work rather than under it. Passed to RecordView's `rail` slot,
// so it takes the wider of the two rail shares (page-zones-rail: 3fr/7fr)
// rather than the narrower `aside` share a right-hand column would get.
//
// Drawn as SIX separate panels, each answering one question about the
// account — its open deals, its people, its facts, its lists and tags — in
// the order a reader works down the column, rather than the disclosures the
// rail used to fold into one card: a hairline inside a panel reads as one
// story about that panel's own subject, and a panel's own edge is what tells
// a reader they have moved on to a different one.
//
// Health moved to the readings row above the tabs, so it is not repeated
// here — two copies of the same verdict is a value the reader has to
// reconcile. "Our team on this account" and "Leads from this company" are
// not drawn: neither has a field or an endpoint behind it yet.
//
// The rail is not rendered while the composer is open. That is the page's
// decision rather than this component's: the drawer opens over the page as
// its own overlay, and the rail standing behind it would only be two things
// competing for the same glance.

type Organization = components["schemas"]["Organization"];
type Organization360 = components["schemas"]["Organization360"];
type Contact = components["schemas"]["Organization360Contact"];
type Deal = components["schemas"]["Organization360Deal"];
type Signal = components["schemas"]["Signal"];

export function CompanyRail({
  orgId,
  org,
  view,
  loading,
  composerOpen,
  onTab,
}: Readonly<{
  orgId: string;
  // The page's own resolved record, read regardless of how the composite
  // read below is doing — Details draws from this whenever the composite
  // has no organization slice yet (still loading, or failed), rather than
  // going blank on a read the page already has the answer to.
  org?: Organization;
  view?: Organization360;
  // The composite read `view` comes off is still in flight. Threaded to the
  // sections that read `view` straight (Deals, People, Tags) so their
  // `sectionState` calls can tell "still loading" apart from "the read
  // failed" — both hand a section an undefined `view`, and without this flag
  // every one of them reads the failed state for as long as the read runs,
  // flashing "could not be loaded" on every ordinary page open.
  loading: boolean;
  // A composer drawer is open in this column. The rail stands down entirely
  // rather than narrowing: squeezed to a third of its width it is a column of
  // broken cards, and no mockup draws the two side by side.
  composerOpen: boolean;
  // Where each panel's header link goes: Deals/People switch the record's own
  // tab strip, Details opens Profile. One callback rather than three, because
  // every use is the same verb aimed at a different tab.
  onTab: (tab: "deals" | "people" | "profile") => void;
}>) {
  const t = useT();
  if (composerOpen) {
    return null;
  }
  return (
    // A plain div: RecordView's own <aside> is the landmark around this, and a
    // second labelled region inside it would give a reader two names for one
    // column.
    <div className="co-rail">
      {/* Details lead the column: the account's own fields are the first
          thing a reader orients by, and they draw from the page's already-
          resolved record while the composite read below is still arriving. */}
      <Panel
        title={t("co.details.title")}
        titleAction={
          // "All fields", not "Profile": the Profile TAB carries that name a
          // few pixels away, and two controls with one accessible name in one
          // view is a dead end for anyone moving by name rather than by sight.
          // It also reads as the sibling cards' "All N" does — this card shows
          // a few of the account's fields, the tab shows every one.
          <Button small variant="ghost" onClick={() => onTab("profile")}>
            {t("co.rail.details.all")}
          </Button>
        }
      >
        <PanelBody>
          <DetailsGrid organization={view?.organization ?? org} />
        </PanelBody>
      </Panel>
      {/* Both summaries stand on EVERY tab, the open one included: the rail
          is the reader's anchor while they move between tabs, and each card
          shows only the top RAIL_ROW_LIMIT rows — a summary beside a tab is
          not a duplicate of it, a full copy would be. */}
      <DealsSection view={view} loading={loading} onTab={onTab} />
      <PeopleSection view={view} loading={loading} onTab={onTab} />
      <CompanyHoldSection organization={view?.organization ?? org} />
      <TagsSection view={view} orgId={orgId} loading={loading} />
    </div>
  );
}

// How many rows a rail card shows before pointing at the tab. The rail is a
// glance, and a twenty-row card beside the work column is a second page, not
// an anchor — the "All N" header verb is the way to the rest.
const RAIL_ROW_LIMIT = 3;

// Keeping a whole account's correspondence private, from the account page.
//
// The account's own domain is what a hold names here — an advisory firm answers
// from whichever address picked up the file, so the domain is the unit that
// actually covers the relationship. Drawn only when the account HAS a domain:
// a hold has to name something, and `website_url` is derived from the primary
// domain row, so its absence means there is nothing to name.
function CompanyHoldSection({
  organization,
}: Readonly<{ organization: Organization | undefined }>) {
  const t = useT();
  const host = hostOf(organization?.website_url);
  if (!host) {
    return null;
  }
  return (
    <Panel title={t("hold.sectionTitle")}>
      <PanelBody>
        {/* The row takes an ADDRESS and derives the domain from it, which is
            what every person page hands it. An account has only the domain, so
            it is handed a bare address at that domain — the same value the
            row's own domain verb would compute. */}
        <CounterpartyHoldRow email={`x@${host}`} />
      </PanelBody>
    </Panel>
  );
}

// The registrable host inside a derived website URL, or nothing when the value
// is absent or not a URL this can read. Never throws at the caller: an account
// with an unparseable website simply offers no hold, rather than taking the
// rail down with it.
function hostOf(website: string | null | undefined): string | undefined {
  if (!website) {
    return undefined;
  }
  try {
    return new URL(website).hostname.replace(/^www\./, "").toLowerCase();
  } catch {
    return undefined;
  }
}

/**
 * DealsSection is the account's open pipeline at a glance: the top
 * RAIL_ROW_LIMIT deals, each with its stage, expected close, and the deal's
 * own reason for needing attention ahead of everything else about it.
 * `view.deals.data` is already open-only (the 360's own contract — closed
 * deals are reported through `won_lifetime` and `lost_count`, never listed),
 * so nothing here filters on `status` a second time.
 *
 * "Top" means: a deal carrying an attention flag before one without, and the
 * larger amount before the smaller — the row a rep would want surfaced is
 * the one that needs a move or carries the money.
 */
function DealsSection({
  view,
  loading,
  onTab,
}: Readonly<{
  view?: Organization360;
  loading: boolean;
  onTab: (tab: "deals") => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const deals = view?.deals;
  const rows = [...(deals?.data ?? [])].sort(byDealWeight);
  const state = sectionState(
    view,
    "deals",
    Boolean(deals),
    rows.length,
    loading,
  );
  const answered = sectionAnswered(state);
  // Closed history vs. never having had a deal: the two empty accounts read
  // differently — one has simply not started, the other has already been
  // through a cycle and stands between two of them.
  const hasClosedHistory = Boolean(
    deals &&
      ((deals.won_lifetime?.amount_minor ?? 0) > 0 || deals.lost_count > 0),
  );
  return (
    <Panel
      title={t("co.rail.deals.title")}
      titleAction={
        answered ? (
          <Button small variant="ghost" onClick={() => onTab("deals")}>
            {rows.length > 0
              ? t("co.rail.all", { count: formatNumber(rows.length, locale) })
              : t("co.rail.add")}
          </Button>
        ) : undefined
      }
    >
      {state === "ready" ? (
        rows
          .slice(0, RAIL_ROW_LIMIT)
          .map((deal) => <DealRailRow key={deal.deal_id} deal={deal} />)
      ) : (
        <PanelBody>
          <SurfaceState
            state={state}
            emptyLabel={
              hasClosedHistory
                ? t("co.rail.deals.emptyClosedOnly")
                : t("co.rail.deals.empty")
            }
          >
            {null}
          </SurfaceState>
          {state === "empty" && !hasClosedHistory && view?.organization && (
            <DealsCreateVerb organization={view.organization} />
          )}
        </PanelBody>
      )}
    </Panel>
  );
}

// The create-deal verb, gated on writability the same way TagsSection's own
// add-tag verb is: `useCompanyReadOnlyReason` needs a resolved
// Organization, so this is its own component mounted only once one exists,
// rather than a conditional hook call inside DealsSection itself.
function DealsCreateVerb({
  organization,
}: Readonly<{ organization: Organization }>) {
  const readOnlyReason = useCompanyReadOnlyReason(organization);
  if (readOnlyReason) {
    return null;
  }
  return (
    <div className="co-card-actions">
      <NewDealAction
        orgId={organization.id}
        orgName={organization.display_name}
      />
    </div>
  );
}

// The rail's own ranking: a deal that needs a move outranks one that does
// not, and past that the money decides. Stable for equal weights, so the
// server's own order — the one every other deal surface shows — is what
// breaks ties rather than an accident of the sort.
function byDealWeight(a: Deal, b: Deal): number {
  const needsMove = (deal: Deal) => (deal.attention || deal.stalled ? 1 : 0);
  const moved = needsMove(b) - needsMove(a);
  if (moved !== 0) {
    return moved;
  }
  return (b.amount?.amount_minor ?? 0) - (a.amount?.amount_minor ?? 0);
}

// One flag a deal can carry ahead of its stage and close date: an overdue
// task beats a stall, because a stall is the absence of a reason and an
// overdue task IS one — the same precedence the work list draws its own
// attention line by.
function dealFlag(deal: Deal, t: ReturnType<typeof useT>): string | undefined {
  if (deal.attention) {
    return deal.attention.kind === "overdue_task"
      ? t("co.rail.deals.attentionOverdue")
      : t("co.rail.deals.attentionCommitment");
  }
  return deal.stalled ? t("deal.stalled") : undefined;
}

function DealRailRow({ deal }: Readonly<{ deal: Deal }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const closes = deal.expected_close_date
    ? t("co.work.closes", {
        date: formatDate(deal.expected_close_date, locale, recordZone),
      })
    : t("co.rail.deals.noCloseDate");
  const flag = dealFlag(deal, t);
  const note = [flag, deal.stage_name ?? t("co.deals.noStage"), closes]
    .filter(Boolean)
    .join(" · ");
  return (
    <PanelRow className="co-row">
      <a
        className="co-rowlink co-rowcover"
        href={routeHash({ screen: "deals", id: deal.deal_id })}
      >
        {deal.name}
      </a>
      <span className="t-mono">
        {formatMoneyOrAbsent(
          deal.amount?.amount_minor,
          deal.amount?.currency,
          locale,
        )}
      </span>
      <p className="co-row-meta">{note}</p>
    </PanelRow>
  );
}

/**
 * PeopleSection is a glance at the roster: who is here, how they have
 * answered, and, where the graph read supports it, the colleagues already in
 * contact with them. The set-role and route-in verbs stay on the People tab's
 * own roster rather than being rebuilt here a second time.
 */
function PeopleSection({
  view,
  loading,
  onTab,
}: Readonly<{
  view?: Organization360;
  loading: boolean;
  onTab: (tab: "people") => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  // Already ranked. The server orders the people section by engagement, then
  // relationship strength, then id (people.RankContacts) BEFORE it cuts to
  // twenty-five, so re-sorting here would be a second spelling of that rule —
  // and the copy that drifts, since only one of the two is what chose which
  // twenty-five arrived.
  const contacts = view?.people?.data ?? [];
  const state = sectionState(
    view,
    "people",
    Boolean(view?.people),
    contacts.length,
    loading,
  );
  const answered = sectionAnswered(state);
  return (
    <Panel
      title={t("co.rail.people.title")}
      titleAction={
        answered ? (
          <Button small variant="ghost" onClick={() => onTab("people")}>
            {contacts.length > 0
              ? t("co.rail.all", {
                  count: formatNumber(contacts.length, locale),
                })
              : t("co.rail.add")}
          </Button>
        ) : undefined
      }
    >
      {state === "ready" ? (
        // The top of the byReach order: the rail glances at who matters most
        // on the account, and the People tab is the full roster.
        contacts.slice(0, RAIL_ROW_LIMIT).map((contact) => (
          <PanelRow key={contact.person_id} className="co-person-row">
            <PersonRow contact={contact} />
          </PanelRow>
        ))
      ) : (
        <PanelBody>
          <SurfaceState state={state} emptyLabel={t("co.rail.people.empty")}>
            {null}
          </SurfaceState>
          {state === "empty" && (
            <div className="co-card-actions">
              <Button small variant="ghost" onClick={() => onTab("people")}>
                {t("co.rail.people.add")}
              </Button>
            </div>
          )}
        </PanelBody>
      )}
    </Panel>
  );
}

function PersonRow({ contact }: Readonly<{ contact: Contact }>) {
  const t = useT();
  const colleagues = contact.routes?.top ?? [];
  return (
    <>
      <Avatar name={contact.full_name} />
      <span className="co-person-id">
        {/* Same target the People tab offers, and the same reach: the row is
            the link, not the eight characters of a short name. */}
        <a
          className="co-rowlink co-person-name co-rowcover"
          href={routeHash({ screen: "contacts", id: contact.person_id })}
        >
          {contact.full_name}
        </a>
        {/* The same fallback the tab makes, because a rail that disagreed
            with the tab about somebody's role is worse than neither showing
            one. The server decides the precedence; both surfaces read it.
            Branched on the VALUE, not on title_source — that field is
            optional, and a server sending a purchased title without it would
            otherwise render an empty, padded span. */}
        {roleOf(contact) && (
          <span className="co-person-role">
            {contact.title_source === "provider" ? (
              <EvidenceMark
                value={roleOf(contact)}
                source={{
                  provenance: { kind: "connector", connector: "provider" },
                }}
              />
            ) : (
              roleOf(contact)
            )}
          </span>
        )}
      </span>
      {colleagues.length > 0 && (
        <span className="co-person-routes">
          {/* A bare monogram is a mark only its owner recognises, so the
              stack opens to the sentence it stands for: which colleagues are
              already in touch with this person. Hover for a passing reader,
              click and focus for everyone a hover never reaches; the sr-only
              names double as the trigger's accessible name. */}
          <Popover
            onHover
            label={
              <>
                <span className="sr-only">
                  {colleagues.map((route) => route.display_name).join(", ")}
                </span>
                <AvatarStack
                  people={colleagues.map((route) => ({
                    name: route.display_name,
                  }))}
                />
              </>
            }
          >
            <p className="t-caption">{t("co.rail.people.inTouch")}</p>
            <ul className="co-person-routes-list">
              {colleagues.map((route) => (
                <li key={route.display_name}>{route.display_name}</li>
              ))}
            </ul>
          </Popover>
        </span>
      )}
    </>
  );
}

/**
 * SourcePageLink opens the page a signal was read off, when it names one.
 *
 * The address rides `evidence` rather than a field of its own: a signal cites
 * its source, and a citation is per-claim. `source_type: "page"` is the one
 * kind that is a web address — the others point at rows inside this product,
 * which a reader reaches by other means — so it is the only one linked here.
 *
 * Renders nothing when no evidence names a page, which is the ordinary case
 * for a signal derived from the product's own records.
 */
function SourcePageLink({ signal }: Readonly<{ signal: Signal }>) {
  const t = useT();
  // Array-checked rather than `?? []`: the client validates no response body,
  // so a server ahead of this tab could send a shape `.find` cannot walk, and
  // the throw would take the whole account page down over one row's citation.
  const cited = Array.isArray(signal.evidence) ? signal.evidence : [];
  // The first citation that is actually reachable, not the first one CLAIMING
  // to be a page: a malformed address in the first slot would otherwise hide a
  // good one behind it, and the row would fall silently back to no link.
  const page = cited.find(
    (one) =>
      one?.source_type === "page" &&
      typeof one.source_id === "string" &&
      webUrl(one.source_id) !== null,
  );
  if (!page?.source_id) {
    return null;
  }
  return (
    <OffsiteLink href={page.source_id} className="co-rowlink co-signal-link">
      {t("co.signals.openSource")}
    </OffsiteLink>
  );
}

/**
 * SignalsSection reads the account-filtered signals, same endpoint and same
 * withheld/failed handling SignalsCard used. Signals are a separately
 * governed surface, not a 360 section, so this runs its own query rather
 * than reading a slice of `view`.
 *
 * Not mounted by CompanyRail — signals moved out of the rail to sit beside the
 * account's other readings, and the company record's overview stack mounts it
 * there.
 */
export function SignalsSection({ orgId }: Readonly<{ orgId: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const query = useQuery({
    queryKey: ["signals", "organization", orgId],
    queryFn: async () => {
      const { data, error } = await api.GET("/signals", {
        params: {
          query: { organization_id: orgId, status: "open", limit: 10 },
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data.data;
    },
  });
  const signals: Signal[] = query.data ?? [];
  const withheld =
    query.isError && problemCodeOf(query.error) === "permission_denied";
  let state: SectionState = "ready";
  if (withheld) {
    state = "withheld";
  } else if (query.isError) {
    state = "failed";
  } else if (query.isPending) {
    state = "loading";
  } else if (signals.length === 0) {
    state = "empty";
  }
  return (
    <Disclosure
      className="co-sect"
      open
      summary={
        <SectionSummary
          title={t("co.signals.title")}
          count={sectionAnswered(state) ? signals.length : undefined}
        />
      }
    >
      {state === "ready" ? (
        signals.map((signal) => (
          <PanelRow key={signal.id} className="co-signal-row">
            <span
              className={`co-dot${signalTone(signal.severity) ? ` co-dot-${signalTone(signal.severity)}` : ""}`}
              aria-hidden="true"
            />
            <span className="co-signal-body">
              <span className="co-signal-title">
                {signalKindLabel(signal.kind, t)}
              </span>
              <span className="co-signal-summary">{signal.summary}</span>
              {/* A signal ABOUT one of the account's projects sends the
                  reader to that project: the summary names it, the link
                  opens it. An account- or person-subject signal already
                  sits on the page it is about. */}
              {signal.entity_type === "project" && signal.entity_id && (
                <a
                  className="co-rowlink co-signal-link"
                  href={routeHash({ screen: "projects", id: signal.entity_id })}
                >
                  {t("co.signals.openProject")}
                </a>
              )}
              {/* The page the claim was read off. A newsroom signal cites the
                  article rather than copying it, so the headline on this row
                  is the whole of what we hold — without the address, a reader
                  who wants the announcement itself has nowhere to go and the
                  citation proves nothing. */}
              <SourcePageLink signal={signal} />
            </span>
            <span className="co-row-meta">
              {formatDate(signal.detected_at, locale, recordZone)}
            </span>
          </PanelRow>
        ))
      ) : (
        <PanelBody>
          <SurfaceState
            state={state}
            emptyLabel={t("co.signals.empty")}
            emptyDetail={t("co.signals.emptyDetail")}
            detail={
              state === "failed" ? { onRetry: () => void query.refetch() } : {}
            }
          >
            {null}
          </SurfaceState>
        </PanelBody>
      )}
    </Disclosure>
  );
}
