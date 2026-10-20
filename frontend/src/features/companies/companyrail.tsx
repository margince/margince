import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { AvatarStack } from "../../design-system/avatarstack";
import { Disclosure, DisclosureGroup } from "../../design-system/disclosure";
import { OffsiteLink } from "../../design-system/offsitelink";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import { Popover } from "../../design-system/popover";
import { RecordCard } from "../../design-system/recordcard";
import { RecordOriginFacts } from "../../design-system/recordfacts";
import {
  type SectionState,
  SurfaceState,
} from "../../design-system/surfacestate";
import { formatDate, formatNumber } from "../../format/format";
import { webUrl } from "../../format/weburl";
import { useLocale, useT } from "../../i18n";
import { useCanWriteRecord } from "../../lib/capability";
import { problemCodeOf, unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { routeHash } from "../../lib/router";
import { CounterpartyHoldRow } from "../privacy/counterparty-hold";
import { signalKindLabel, signalTone } from "../records/record360";
import { RecordCustomFields } from "../records/recordcustomfields";
import { RecordTeam } from "../records/recordteam";
import { CompanyReachCard } from "./company-reach";
import { CompanyDetails } from "./companydetails";
import { CompanyProfileDetails } from "./companyraildetails";
import { ProjectsSection } from "./companyrailprojects";
import {
  AllRowsLink,
  contactRole,
  contactsSlice,
  RAIL_ROW_LIMIT,
  SectionSummary,
  SectionTitle,
  sectionAnswered,
} from "./companyrailshared";
import { CompanyTriageSection } from "./companytriage";
// The row and card shapes this file draws — co-rowlink, co-row-meta, co-card —
// are defined in company360.css. Imported HERE rather than left to the caller:
// it works today only because the company record page pulls that stylesheet in
// for its own sake, so this file renders unstyled anywhere else.
import "./company360.css";
import { ListsSection } from "./companyraillists";

// The record page's LEFT rail (mockup State A): the account's context,
// beside the work rather than under it. Passed to RecordView's `rail` slot,
// so it takes the wider of the two rail shares (page-zones-rail: 3fr/7fr)
// rather than the narrower `aside` share a right-hand column would get.
//
// Drawn as separate panels, each answering one question about the account:
// its details and tags, its registration, its team, its open deals, its
// projects, its contacts, its hold, in the order a reader works down the
// column, rather than the disclosures the rail used to fold into one card: a
// hairline inside a panel reads as one story about that panel's own subject, and a
// panel's own edge is what tells a reader they have moved on to a different
// one. Tags file under Details rather than in their own panel: a tag is a
// fact about the account, on the same card as the rest of them.
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
}: Readonly<{
  companyId: string;
  // The page's own resolved record, read regardless of how the composite
  // read below is doing — Details draws from this whenever the composite
  // has no company slice yet (still loading, or failed), rather than
  // going blank on a read the page already has the answer to.
  company?: Company;
  view?: Company360;
}>) {
  const t = useT();
  const resolved = view?.company ?? company;
  return (
    // A plain div: the shell's own <aside> is the landmark around this. A
    // second labelled region inside it would give one column two names. It
    // holds what the account is: its fields, origin, registration and lists.
    <div className="co-rail">
      {/* Details lead the column: the account's own fields are the first
            thing a reader orients by, and they draw from the page's already-
            resolved record while the composite read below is still arriving. */}
      {resolved && (
        <>
          <CompanyDetails company={resolved} />
          <RecordCustomFields kind="company" record={resolved} />
          {/* When the row was written and by whom: a fact about the record,
              filed with its fields rather than in the line under its name. */}
          <Panel title={t("record.origin")}>
            <PanelBody>
              {/* The record row the head reads, before the 360's copy of it:
                  the two may carry different provenance mid-refresh. */}
              <RecordOriginFacts record={company ?? resolved} />
            </PanelBody>
          </Panel>
        </>
      )}
      <Panel>
        <DisclosureGroup>
          {resolved && (
            <Disclosure
              className="co-sect"
              summary={<SectionSummary title={t("record.registration")} />}
            >
              <PanelBody>
                <CompanyProfileDetails company={resolved} />
              </PanelBody>
            </Disclosure>
          )}
          {/* The server refuses an archived account's lists. */}
          {!resolved?.archived_at && <ListsSection companyId={companyId} />}
        </DisclosureGroup>
      </Panel>
    </div>
  );
}

/**
 * CompanyFiling is how the account is looked after beyond its fields. It holds
 * who is responsible, its projects, who on the team can reach it, and its mail
 * domain's hold. It stands on the Profile tab, so the fields column keeps to
 * what the account is.
 */
export function CompanyFiling({
  companyId,
  company,
  view,
  loading,
  onTab,
}: Readonly<{
  companyId: string;
  company?: Company;
  view?: Company360;
  loading: boolean;
  onTab: (tab: "deals" | "contacts" | "profile") => void;
}>) {
  const t = useT();
  const resolved = view?.company ?? company;
  const canWriteCompany =
    useCanWriteRecord("company", resolved) && !resolved?.archived_at;
  return (
    <Panel>
      <DisclosureGroup>
        <Disclosure
          className="co-sect"
          summary={<SectionSummary title={t("assignments.title")} />}
        >
          <RecordTeam
            recordType="company"
            recordId={companyId}
            readOnly={!canWriteCompany}
            bare
          />
        </Disclosure>
        {/* The server refuses an archived account's 360 and its lists. */}
        {!resolved?.archived_at && (
          <>
            <ProjectsSection view={view} loading={loading} onTab={onTab} />
            {/* Who on the team can reach the contacts not yet on file. */}
            {resolved && <CompanyReachCard companyId={resolved.id} />}
          </>
        )}
        <CompanyHoldSection company={resolved} />
        {/* Beside the hold, and for the same reason: both are about the
            account's mail domain rather than about the work on it. */}
        <CompanyTriageSection companyId={companyId} />
      </DisclosureGroup>
    </Panel>
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
 * ContactsSection is who is on the account: the top of the roster as pills, each
 * opening the contact. The set-role and route-in verbs stay on the Contacts
 * tab's own roster rather than being rebuilt here a second time.
 */
export function ContactsSection({
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
  // Already ranked: the server orders contacts by engagement, strength and id
  // (contacts.RankContacts) before it cuts to twenty-five. Re-sorting here
  // would be a second spelling of that rule.
  const { contacts, count, state } = contactsSlice(view, loading);
  const answered = sectionAnswered(state);
  return (
    <Panel
      title={
        <SectionTitle
          title={t("co.rail.contacts.title")}
          count={
            answered && count != null ? formatNumber(count, locale) : undefined
          }
        />
      }
      titleAction={
        state === "ready" ? (
          <AllRowsLink
            label={t("co.contacts.all")}
            onOpen={() => onTab("contacts")}
          />
        ) : undefined
      }
    >
      <PanelBody>
        {state === "ready" ? (
          <ul className="record-card-list record-card-pills">
            {contacts.slice(0, RAIL_ROW_LIMIT).map((contact) => (
              <li key={contact.contact_id}>
                <ContactCard contact={contact} />
              </li>
            ))}
          </ul>
        ) : (
          <>
            <SurfaceState
              state={state}
              emptyLabel={t("co.rail.contacts.empty")}
              loadingLabel={t("co.rail.contacts.title")}
            >
              {null}
            </SurfaceState>
            {/* The empty state's one verb. */}
            {state === "empty" && (
              <div className="card-actions">
                <Button variant="ghost" onClick={() => onTab("contacts")}>
                  {t("co.rail.contacts.add")}
                </Button>
              </div>
            )}
          </>
        )}
      </PanelBody>
    </Panel>
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
                      identity: route.user_id,
                    }))}
                  />
                </>
              }
            >
              <p>{t("co.rail.contacts.inTouch")}</p>
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
      const data = unwrap(
        await api.GET("/signals", {
          params: {
            query: { company_id: companyId, status: "open", limit: 10 },
          },
        }),
      );
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
