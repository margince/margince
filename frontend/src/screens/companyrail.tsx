import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { routeHash } from "../app/router";
import { Button, Disclosure } from "../design-system/atoms";
import { AvatarStack } from "../design-system/avatarstack";
import { OffsiteLink } from "../design-system/offsitelink";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { Popover } from "../design-system/popover";
import { RecordCard } from "../design-system/recordcard";
import { type SectionState, SurfaceState } from "../design-system/surfacestate";
import { formatDate, formatNumber } from "../format/format";
import { webUrl } from "../format/weburl";
import { useLocale, useT } from "../i18n";
import { problemCodeOf, throwProblem } from "./common";
import { DealsSection } from "./companyraildeals";
import { DetailsGrid } from "./companyraildetails";
import {
  contactRole,
  contactsSlice,
  RAIL_ROW_LIMIT,
  SectionSummary,
  sectionAnswered,
} from "./companyrailshared";
import { CompanyTagsSection } from "./companyrailtags";
import { CounterpartyHoldRow } from "./counterparty-hold";
import { signalKindLabel, signalTone } from "./record360";
import { RecordTeam } from "./recordteam";
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
// account — its open deals, its contacts, its facts, its lists and tags — in
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

type Company = components["schemas"]["Company"];
type Company360 = components["schemas"]["Company360"];
type Contact = components["schemas"]["Company360Contact"];
type Signal = components["schemas"]["Signal"];

export function CompanyRail({
  companyId,
  company,
  view,
  loading,
  composerOpen,
  onTab,
}: Readonly<{
  companyId: string;
  // The page's own resolved record, read regardless of how the composite
  // read below is doing — Details draws from this whenever the composite
  // has no company slice yet (still loading, or failed), rather than
  // going blank on a read the page already has the answer to.
  company?: Company;
  view?: Company360;
  // The composite read `view` comes off is still in flight. Threaded to the
  // sections that read `view` straight (Deals, Contacts, Tags) so their
  // `sectionState` calls can tell "still loading" apart from "the read
  // failed" — both hand a section an undefined `view`, and without this flag
  // every one of them reads the failed state for as long as the read runs,
  // flashing "could not be loaded" on every ordinary page open.
  loading: boolean;
  // A composer drawer is open in this column. The rail stands down entirely
  // rather than narrowing: squeezed to a third of its width it is a column of
  // broken cards, and no mockup draws the two side by side.
  composerOpen: boolean;
  // Where each panel's header link goes: Deals/Contacts switch the record's own
  // tab strip, Details opens Profile. One callback rather than three, because
  // every use is the same verb aimed at a different tab.
  onTab: (tab: "deals" | "contacts" | "profile") => void;
}>) {
  const t = useT();
  if (composerOpen) {
    return null;
  }
  return (
    // A plain div: the shell's own <aside> is the landmark around this, and a
    // second labelled region inside it would give a reader two names for one
    // column. Inside it ONE pane of named sections (DESIGN.md §6): the
    // account's fields, its deals, its contacts, the hold, its tags — each a
    // disclosure with its own summary, so the column reads as one object with
    // five slices rather than five cards a reader has to assemble.
    <div className="co-rail">
      <Panel>
        {/* Details lead the column: the account's own fields are the first
            thing a reader orients by, and they draw from the page's already-
            resolved record while the composite read below is still arriving. */}
        <Disclosure
          className="co-sect"
          open
          summary={<SectionSummary title={t("co.details.title")} />}
        >
          <PanelBody>
            <DetailsGrid company={view?.company ?? company} />
          </PanelBody>
          <div className="card-actions">
            {/* "All fields", not "Profile": the Profile TAB carries that name a
                few pixels away, and two controls with one accessible name in
                one view is a dead end for anyone moving by name rather than by
                sight. */}
            <Button small variant="ghost" onClick={() => onTab("profile")}>
              {t("co.rail.details.all")}
            </Button>
          </div>
        </Disclosure>
        <RecordTeam recordType="company" recordId={companyId} />
        {/* Both summaries stand on EVERY tab, the open one included: the
            column is the reader's anchor while they move between tabs, and
            each shows only the top RAIL_ROW_LIMIT rows — a summary beside a
            tab is not a duplicate of it, a full copy would be. */}
        <DealsSection view={view} loading={loading} onTab={onTab} />
        <ContactsSection view={view} loading={loading} onTab={onTab} />
        <CompanyHoldSection company={view?.company ?? company} />
        <Disclosure
          className="co-sect"
          open
          summary={<SectionSummary title={t("tags.panelTitle")} />}
        >
          <CompanyTagsSection
            company={view?.company ?? company}
            companyId={companyId}
            bare
          />
        </Disclosure>
      </Panel>
    </div>
  );
}

// Keeping a whole account's correspondence private, from the account page.
//
// The account's own domain is what a hold names here — an advisory firm answers
// from whichever address picked up the file, so the domain is the unit that
// actually covers the relationship. Drawn only when the account HAS a domain:
// a hold has to name something, and `website_url` is derived from the primary
// domain row, so its absence means there is nothing to name.
function CompanyHoldSection({
  company,
}: Readonly<{ company: Company | undefined }>) {
  const t = useT();
  const host = hostOf(company?.website_url);
  if (!host) {
    return null;
  }
  return (
    <Disclosure
      className="co-sect"
      summary={<SectionSummary title={t("hold.sectionTitle")} />}
    >
      <PanelBody>
        {/* The row takes an ADDRESS and derives the domain from it, which is
            what every contact page hands it. An account has only the domain, so
            it is handed a bare address at that domain — the same value the
            row's own domain verb would compute. */}
        <CounterpartyHoldRow email={`x@${host}`} />
      </PanelBody>
    </Disclosure>
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
 * ContactsSection is a glance at the roster: who is here, how they have
 * answered, and, where the graph read supports it, the colleagues already in
 * contact with them. The set-role and route-in verbs stay on the Contacts tab's
 * own roster rather than being rebuilt here a second time.
 */
function ContactsSection({
  view,
  loading,
  onTab,
}: Readonly<{
  view?: Company360;
  loading: boolean;
  onTab: (tab: "contacts") => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  // Already ranked. The server orders the contacts section by engagement, then
  // relationship strength, then id (contacts.RankContacts) BEFORE it cuts to
  // twenty-five, so re-sorting here would be a second spelling of that rule —
  // and the copy that drifts, since only one of the two is what chose which
  // twenty-five arrived.
  const { contacts, count, state } = contactsSlice(view, loading);
  const answered = sectionAnswered(state);
  return (
    <Disclosure
      className="co-sect"
      open
      summary={
        <SectionSummary
          title={t("co.rail.contacts.title")}
          count={answered ? count : undefined}
        />
      }
    >
      {state === "ready" ? (
        // The top of the byReach order: the rail glances at who matters most
        // on the account, and the Contacts tab is the full roster.
        <ul className="record-card-list">
          {contacts.slice(0, RAIL_ROW_LIMIT).map((contact) => (
            <li key={contact.contact_id}>
              <ContactCard contact={contact} />
            </li>
          ))}
        </ul>
      ) : (
        <PanelBody>
          <SurfaceState
            state={state}
            emptyLabel={t("co.rail.contacts.empty")}
            loadingLabel={t("co.rail.contacts.title")}
          >
            {null}
          </SurfaceState>
          {/* The empty state's one verb; the foot below stands only once
              there are rows, so an empty roster never offers the same
              tab twice under two names. */}
          {state === "empty" && (
            <div className="card-actions">
              <Button small variant="ghost" onClick={() => onTab("contacts")}>
                {t("co.rail.contacts.add")}
              </Button>
            </div>
          )}
        </PanelBody>
      )}
      {state === "ready" && (
        <div className="card-actions">
          <Button small variant="ghost" onClick={() => onTab("contacts")}>
            {count != null
              ? t("co.rail.all", { count: formatNumber(count, locale) })
              : t("co.rail.allUncounted")}
          </Button>
        </div>
      )}
    </Disclosure>
  );
}

function ContactCard({ contact }: Readonly<{ contact: Contact }>) {
  const t = useT();
  const colleagues = contact.routes?.top ?? [];
  return (
    <RecordCard
      kind="contact"
      name={contact.full_name}
      identity={contact.contact_id}
      href={routeHash({ screen: "contacts", id: contact.contact_id })}
      position={contactRole(contact)}
      email={contact.primary_email ?? undefined}
      aside={
        colleagues.length > 0 && (
          /* The rail's own wrapper: whether the stack's trigger wears a
             pointer is this surface's decision, not the card's.

             A bare monogram is a mark only its owner recognises, so the stack
             opens to the sentence it stands for: which colleagues are already
             in touch with this contact. Hover for a passing reader, click and
             focus for everyone a hover never reaches; the sr-only names
             double as the trigger's accessible name. */
          <span className="co-contact-routes">
            <Popover
              onHover
              label={
                <>
                  <span className="sr-only">
                    {colleagues.map((route) => route.display_name).join(", ")}
                  </span>
                  <AvatarStack
                    contacts={colleagues.map((route) => ({
                      name: route.display_name,
                    }))}
                  />
                </>
              }
            >
              <p className="t-caption">{t("co.rail.contacts.inTouch")}</p>
              <ul className="co-contact-routes-list">
                {colleagues.map((route) => (
                  // Keyed on the id, not the name: two colleagues can share a
                  // display name, and a name key would fold their rows.
                  <li key={route.user_id}>{route.display_name}</li>
                ))}
              </ul>
            </Popover>
          </span>
        )
      }
    />
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
export function SignalsSection({ companyId }: Readonly<{ companyId: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const query = useQuery({
    queryKey: ["signals", "company", companyId],
    queryFn: async () => {
      const { data, error } = await api.GET("/signals", {
        params: {
          query: { company_id: companyId, status: "open", limit: 10 },
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
              <span className="co-signal-summary t-caption">
                {signal.summary}
              </span>
              {/* A signal ABOUT one of the account's projects sends the
                  reader to that project: the summary names it, the link
                  opens it. An account- or contact-subject signal already
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
            <span className="co-row-meta t-caption">
              {formatDate(signal.detected_at, locale, recordZone)}
            </span>
          </PanelRow>
        ))
      ) : (
        <PanelBody>
          <SurfaceState
            loadingLabel={t("co.signals.title")}
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
