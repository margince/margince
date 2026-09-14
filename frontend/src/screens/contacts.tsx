import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { usePageName } from "../app/pagemeta";
import { useRecordZone } from "../app/recordzone";
import { activityTimeline } from "../design-system/activitytimeline";
import { Badge, OverflowMenu, SegmentedControl } from "../design-system/atoms";
import { RecordView } from "../design-system/composed";
import { EmailDetail } from "../design-system/emaildetail";
import {
  useRecordTimeline,
  useTimelineFilters,
} from "../design-system/recordtimeline";
import { TimelineFilterBar } from "../design-system/timelinefilterbar";
import { useToast } from "../design-system/toast";
import { ProvenanceTag } from "../design-system/trust";
import { formatDateTime } from "../format/format";
import { normalizeProfileUrl } from "../format/profileurl";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  LoadMoreButton,
  OverlayUnavailable,
  provenanceOf,
  QueryGate,
  throwProblem,
  useSorMode,
  useViewerId,
} from "./common";
import { ConsentSection } from "./consent";
import {
  type Contact360,
  IdentityRail,
  RelationshipPulse,
  ThinState,
  thinRecord,
  useContact360,
  WhoKnowsThem,
} from "./contact360";
import { EnrichedFields } from "./contactcorrections";
import { ContactDealRooms } from "./contactdealrooms";
import { ContactEditMergeArchive } from "./contacteditmergearchive";
import { contactCreateFields, mapContactBody } from "./contactformfields";
import { ContactNetworkTab } from "./contactnetwork";
import { ContactProjects } from "./contactprojects";
import { RecordContextPanel } from "./context";
import { CreateAction, type CreateField, type FormRows } from "./create";
import { CustomFieldsPanel } from "./customfields.card";
import {
  type ObjectCustomFields,
  useObjectCustomFields,
} from "./customfields.form";
import { EntityRef } from "./entityref";
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
import { LogActivity } from "./logactivity";
import {
  createdColumn,
  lastActivityColumn,
  mineEmptyNote,
  ownerColumn,
  standardViews,
  tagsColumn,
} from "./recordlist";
import { invalidateRecord } from "./recordwritekeys";
import { RelationshipsTab } from "./relationships";
import { SaveViewAction, useSavedViewTabs } from "./savedviews";
import { ShareAction } from "./share";
import { listQueryParams } from "./tagfilter";
import { TimelineActions } from "./timelineactions";
import { groupChronology } from "./timelinegroups";
import { VCardImport } from "./vcard-import";

// Contacts list + contact 360 (B-EP09.10a/b). Every row carries its
// provenance chip (captured_by is server truth); the 360 renders the
// per-purpose consent card and evidence-or-omit fields — absent data is
// omitted, never guessed. Search/filter/sort/pagination (P-14), the rich
// create modal (P-15), the If-Match edit form (P-1), and the dedupe
// view-existing link (P-16) are the four shared blocks wired in here.

type Contact = components["schemas"]["Contact"];

async function fetchContactsPage(
  query: ListQuery,
  cursor: string | null,
): Promise<ListPage<Contact>> {
  const { data, error } = await api.GET("/contacts", {
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
    // A LIST read's honest-error path only needs a message to render — the
    // dedupe "view existing" link is a create/update-only concern.
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

async function createContact(
  values: Record<string, string>,
  rows: FormRows | undefined,
  customFields: Record<string, unknown>,
  t: (key: MessageKey) => string,
): Promise<Contact> {
  const { data, error } = await api.POST("/contacts", {
    body: { ...mapContactBody(values, rows ?? {}), ...customFields },
  });
  if (error) {
    throwProblem(error, t);
  }
  return data;
}

// Quick capture: the six things somebody reading a public profile in another
// window can state, and nothing else. Deliberately NOT contactCreateFields
// trimmed — the repeatable email and phone rows are two clicks each before a
// value can be typed, which is the cost this path exists to remove.
//
// The company is one box. A picker would be the better control for attaching
// an EXISTING company, and it is what the full form should grow; here it would
// put a search-and-wait between two keystrokes, so the name creates a company
// and the reader merges later if they typed a company that already exists.
function quickCaptureFields(): CreateField[] {
  return [
    { key: "full_name", label: "create.fullName", required: true },
    { key: "title", label: "create.contactTitle" },
    { key: "company_name", label: "create.companyName" },
    { key: "profile_url", label: "create.linkedin" },
    { key: "email", label: "create.email", type: "email" },
    { key: "phone", label: "create.phone" },
  ];
}

// A blank box is a field the reader left alone, which is not the same as a
// value they cleared — this path only ever creates, so an empty string is
// simply omitted rather than sent as an explicit null.
function statedValue(values: Record<string, string>, key: string) {
  const value = values[key]?.trim();
  return value ? value : undefined;
}

async function quickCaptureContact(
  values: Record<string, string>,
  t: (key: MessageKey) => string,
): Promise<Contact> {
  const { data, error } = await api.POST("/contacts/quick-capture", {
    body: {
      full_name: values.full_name?.trim() ?? "",
      title: statedValue(values, "title"),
      company_name: statedValue(values, "company_name"),
      // Normalized here rather than server-side for the same reason the contact
      // rail normalizes on save: a bare `linkedin.com/in/jdoe` is an address
      // somebody typed, and storing it unusable makes the row permanently
      // unlinkable on every surface that reads it.
      profile_url: profileUrlOrUndefined(values.profile_url),
      email: statedValue(values, "email"),
      phone: statedValue(values, "phone"),
    },
  });
  if (error) {
    throwProblem(error, t);
  }
  return data.contact;
}

function profileUrlOrUndefined(raw: string | undefined) {
  const stated = raw?.trim();
  return stated ? normalizeProfileUrl(stated) : undefined;
}

/**
 * ContactAside is the relationship column, and in overlay mode it SAYS it
 * cannot answer rather than disappearing.
 *
 * Both panels read the interaction projection, which is folded from natively
 * captured participants — a mirror-backed workspace has none. Rendering
 * nothing would let the page read as "nobody here knows them", which is a lie
 * about the relationship rather than an empty answer about the data.
 */
function ContactAside({
  view,
  overlay,
}: Readonly<{ view?: Contact360; overlay: boolean }>) {
  if (overlay) {
    return (
      <>
        <OverlayUnavailable />
        <OverlayUnavailable />
      </>
    );
  }
  if (!view) {
    return undefined;
  }
  // Every address the contact has: a seat may have been invited on a
  // secondary one, and a card that checked only the primary would tell an
  // admin the contact is out of every room when they are not.
  const emails = (view.contact.emails ?? []).map((e) => e.email);
  return (
    <>
      <RelationshipPulse view={view} />
      <ContactProjects
        contactId={view.contact.id}
        projects={view.projects}
        readOnly={Boolean(view.contact.archived_at)}
      />
      <WhoKnowsThem view={view} />
      {emails.length > 0 ? <ContactDealRooms emails={emails} /> : null}
    </>
  );
}

export function ContactsScreen() {
  const t = useT();
  const pageName = usePageName("contacts");
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  // Offered only once /me has answered: a chip whose value is still "" reads
  // as "clear this filter", so a half-built owner dial narrows nothing.
  const viewerId = useViewerId();
  const ownerChips = useOwnerChips();
  const tagChips = useTagChips();
  const savedViews = useSavedViewTabs("contacts");
  const cf = useObjectCustomFields("contact");
  // The form that never closes gives no other feedback: without this, six
  // saved contacts look exactly like six that failed silently.
  const toast = useToast();
  const state = useListQuery<Contact>({
    key: "contacts",
    initialSort: "-created_at",
    fetchPage: fetchContactsPage,
  });

  return (
    <div className="wrap">
      <ListTable
        title={pageName}
        state={state}
        unit="unit.contacts"
        emptyNote={mineEmptyNote({ t, state, viewerId, unit: "unit.contacts" })}
        action={
          <>
            <CreateAction
              label={t("create.quickCapture")}
              invalidate="contacts"
              screen="contacts"
              testId="quick-capture"
              keepOpen
              create={(values) => quickCaptureContact(values, t)}
              onCreated={(contact) =>
                toast.show(
                  t("create.quickCaptureSaved", { name: contact.full_name }),
                )
              }
              resolveExisting={(_code, id) => ({ screen: "contacts", id })}
              fields={quickCaptureFields()}
            />
            <CreateAction
              label={t("create.contact")}
              invalidate="contacts"
              screen="contacts"
              create={(values, rows) =>
                createContact(values, rows, cf.toBody(values), t)
              }
              resolveExisting={(_code, id) => ({ screen: "contacts", id })}
              fields={[...contactCreateFields(t), ...cf.formFields]}
            />
            {/* Last of the three, because it is the one a reader reaches for
                with a stack of cards already in hand rather than while typing
                one contact. Beside the others all the same: a handed-over card
                is a way a contact comes to exist, and every one of those
                belongs on this list. */}
            <VCardImport />
          </>
        }
        columns={[
          {
            key: "name",
            header: t("contacts.name"),
            cell: (contact: Contact) => (
              <span>
                <strong>{contact.full_name}</strong>
                {contact.title && (
                  <span className="t-caption"> · {contact.title}</span>
                )}
                {contact.archived_at && (
                  <Badge tone="warn">{t("record.archived")}</Badge>
                )}
              </span>
            ),
            sort: "full_name",
            fixed: true,
          },
          {
            key: "email",
            header: t("contacts.email"),
            // The SERVER's pick, not a rule re-made here. Which of a
            // contact's addresses they are reachable at is one question, and
            // answering it in the browser meant the list could not be ordered
            // by it and each surface that needed the address answered again.
            //
            // An address is words a reader reads, so it takes the body face
            // like every other value in the row. The mono face is for a
            // machine name — a name and a domain set in it read as an
            // identifier rather than as somebody a reader could write to.
            cell: (contact: Contact) => contact.primary_email ?? "",
            sort: "primary_email",
          },
          {
            // Who they work for TODAY, from the row itself: the wire carries
            // the current primary employment resolved to its account, so the
            // column costs no lookup and shows no company the reader has no
            // grant for.
            //
            // Empty is not "works nowhere" — the field is also absent when the
            // caller may not read edges or that account — so the cell states
            // nothing rather than drawing a dash a reader would take for an
            // answer.
            key: "company",
            header: t("create.relatedCompany"),
            // By the company's NAME, walking the same edge the row walked. A
            // reader who may see no employer at all is ordered by none.
            sort: "employer",
            cell: (contact: Contact) =>
              contact.employer ? (
                // A real link to the COMPANY, in a cell that is not the row's
                // identity cell — so it nests inside no other anchor and the
                // markup stays valid. It used to be text, on the reasoning that
                // the row already links somewhere; but the row links to the
                // CONTACT, and a reader looking at a list of contacts who works
                // for whom had no way to reach the company without opening a
                // contact first. The name comes with the row, so this resolves
                // nothing.
                <EntityRef
                  kind="company"
                  id={contact.employer.company_id}
                  name={contact.employer.company_name}
                />
              ) : null,
          },
          tagsColumn<Contact>(t),
          ownerColumn<Contact>(t),
          lastActivityColumn<Contact>(t, locale, recordZone),
          createdColumn<Contact>(t, locale, recordZone),
        ]}
        tools={<SaveViewAction resource="contacts" query={state.query} />}
        rowKey={(contact) => contact.id}
        rowRoute={(contact) => ({ screen: "contacts", id: contact.id })}
        dataChips={[...ownerChips, ...tagChips]}
        dataViews={savedViews}
        views={[...standardViews(viewerId)]}
      />
    </div>
  );
}

const CONTACT_TABS = ["overview", "relationships", "history"] as const;
type ContactTab = (typeof CONTACT_TABS)[number];

// The verbs a contact record offers, and the two the overlay withholds.
//
// Extracted from the 360 render so that render carries the record's SHAPE and
// this carries what may be done to it: the mode branch and the archive branch
// are both about the verbs, and reading either one no longer means holding the
// whole page.
function ContactActionBadges({
  contact,
  cf,
  archivedReasonId,
}: Readonly<{
  contact: Contact;
  cf: ObjectCustomFields;
  // Minted once for the page by the caller, because the archive is a fact
  // about the record rather than about any one verb that refuses.
  archivedReasonId: string;
}>) {
  const t = useT();
  const overlay = useSorMode() === "overlay";
  const viewerId = useViewerId();
  const disabledReasonId = contact.archived_at ? archivedReasonId : undefined;
  return (
    <>
      <ProvenanceTag provenance={provenanceOf(contact.captured_by, viewerId)} />
      {/* Where this contact came from, when it came from a lead
          (ADR-0119). The pointer runs contact → lead and the
          lead's page is a terminal record of the promotion, so the
          chip is a link rather than a label: a rep asking "was this
          a merge or a new contact?" reads the answer there. */}
      {contact.converted_from_lead_id && (
        <Badge tone="accent">
          {t("contact.fromLead")}{" "}
          <EntityRef kind="lead" id={contact.converted_from_lead_id} />
        </Badge>
      )}
      {contact.archived_at && <Badge tone="warn">{t("record.archived")}</Badge>}
      {/* The record's verbs behind one control, the shape the contact page
          carries (contactactions.tsx). An archived record is read-only: the
          backend rejects edit/merge/archive on a non-live row (there is no
          unarchive path). The verbs stay VISIBLE and refused inside the menu,
          pointing at the page's one sentence about the archive (STATE-4a): a
          missing control says nothing about the record, while a refused one
          names the reason. */}
      <OverflowMenu label={t("record.moreActions")}>
        <ContactEditMergeArchive
          contact={contact}
          cf={cf}
          disabledReasonId={disabledReasonId}
          overlay={overlay}
          beforeArchive={
            // A record grant probes the native row via
            // auth.EnsureLinkTarget, which a mirrored record has no row for —
            // sharing stays hidden in overlay regardless of record type (see
            // deals.tsx's DealBadges).
            !overlay && (
              <ShareAction
                recordType="contact"
                recordId={contact.id}
                disabledReasonId={disabledReasonId}
              />
            )
          }
        />
      </OverflowMenu>
    </>
  );
}

// What the chosen tab shows. One component per screen rather than one per tab:
// the panels share the record and differ only in which of it they draw, and a
// component apiece would put five files between a reader and that fact.
function ContactTabPanels({
  tab,
  contact,
  view,
}: Readonly<{
  tab: ContactTab;
  contact: Contact;
  view?: Contact360;
}>) {
  const queryClient = useQueryClient();
  const id = contact.id;
  return (
    // The rhythm between the tab's panels belongs to the column that holds
    // them, not to the panels: the work column is an `.arrive-stack` and
    // carries no interval of its own, so bare siblings would meet at the
    // border and a panel spacing itself would space only its own side.
    <div className="record-stack">
      {tab === "overview" && thinRecord(view) && view && (
        <ThinState view={view} />
      )}
      {/* Consent renders on a thin record too: it is not an absence
          but a guard — what you may send is a live fact whether or
          not anyone has written to them yet. */}
      {tab === "overview" && (
        <ConsentSection contactId={contact.id} contact={contact} />
      )}
      {tab === "overview" && view && (
        <EnrichedFields contactId={id} view={view} />
      )}
      {tab === "overview" && !thinRecord(view) && (
        <>
          <CustomFieldsPanel object="contact" record={contact} />
          <RecordContextPanel entityType="contact" id={contact.id} />
          <LogActivity entityType="contact" entityId={contact.id} />
        </>
      )}
      {tab === "relationships" && (
        <div style={{ display: "grid", gap: "var(--space-4)" }}>
          <ContactNetworkTab contactId={id} />
          <RelationshipsTab scope={{ contact_id: contact.id }} />
        </div>
      )}
      {tab === "history" && (
        <RecordHistoryTab
          kind="contact"
          id={contact.id}
          restore={{
            version: contact.version,
            onRestored: () =>
              invalidateRecord(queryClient, "contact", contact.id),
          }}
        />
      )}
    </div>
  );
}

export function ContactScreen({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const recordZone = useRecordZone();
  const cf = useObjectCustomFields("contact");
  // ONE sentence about this contact being archived, minted here and pointed at
  // by every verb the archive refuses. Said once for the page rather than
  // beside each of four buttons.
  const archivedReasonId = useId();
  const [tab, setTab] = useState<ContactTab>("overview");
  const contactQuery = useQuery({
    queryKey: ["contact", id],
    queryFn: async () => {
      const { data, error } = await api.GET("/contacts/{id}", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
  const [timelineFilters, setTimelineFilters] = useTimelineFilters(id);
  const timelineQuery = useRecordTimeline("contact", id, {
    filters: timelineFilters,
  });
  const view360 = useContact360(id);
  // The composite is only usable once it carries its mandatory root record.
  // Guarding on the whole payload would let a partial or error-shaped body
  // through and crash the rail on a contact that is not there.
  const view = view360.data?.contact ? view360.data : undefined;
  const overlay = useSorMode() === "overlay";
  const viewerId = useViewerId();
  // Which message the drawer is showing. The page owns it rather than the row,
  // because one drawer over the record is the point: a drawer per row would be
  // a dialog stack, and the record behind it is what the reader is working on.
  const [openEmail, setOpenEmail] = useState<string | null>(null);
  const { locale } = useLocale();
  const timelineEntries = activityTimeline(
    timelineQuery.activities,
    viewerId,
    (activity) => (
      <TimelineActions
        activity={activity}
        entityType="contact"
        entityId={id}
        contactId={id}
      />
    ),
  ).map((entry) =>
    entry.emailSummary
      ? { ...entry, onOpenEmail: () => setOpenEmail(entry.id) }
      : entry,
  );

  return (
    <div className="wrap">
      <QueryGate query={contactQuery} pendingLabel={t("nav.contacts")}>
        {(contact) => (
          <RecordView
            name={contact.full_name}
            subtitle={contact.title ?? undefined}
            zone={recordZone}
            badges={
              <ContactActionBadges
                contact={contact}
                cf={cf}
                archivedReasonId={archivedReasonId}
              />
            }
            // The archive is a fact about the whole record, so it is stated
            // once across the header rather than repeated beside each verb it
            // refuses. The rail says the same thing about its own inline
            // edits (contactrail.tsx) off this same key, so the two cannot
            // drift into two spellings of one fact. Absent while the contact
            // is live: a line always reserved would read as a record with
            // something to say about itself and nothing said.
            band={
              contact.archived_at ? (
                <p id={archivedReasonId} className="t-caption">
                  {t("contact.rail.archivedReadOnly")}
                </p>
              ) : undefined
            }
            timeline={timelineEntries}
            // Conversations, not messages: a thread the contact is on reads
            // as one exchange rather than one row per reply.
            timelineGroups={groupChronology(
              timelineEntries,
              timelineQuery.hasNextPage,
            )}
            // The filter sits ABOVE the timeline; the notice REPLACES it
            // (composed.tsx renders `timelineNotice ?? the list`). Putting the
            // filter in the notice slot hid every activity row behind it.
            timelineHeader={
              overlay ? undefined : (
                <TimelineFilterBar
                  value={timelineFilters}
                  onChange={setTimelineFilters}
                />
              )
            }
            timelineFooter={<LoadMoreButton query={timelineQuery} />}
            timelineNotice={overlay ? <OverlayUnavailable /> : undefined}
            rail={view ? <IdentityRail view={view} /> : undefined}
            aside={<ContactAside view={view} overlay={overlay} />}
          >
            <div style={{ marginBottom: "var(--space-4)" }}>
              <SegmentedControl
                options={CONTACT_TABS}
                value={tab}
                onChange={setTab}
                labels={{
                  overview: t("tab.overview"),
                  relationships: t("tab.relationships"),
                  history: t("tab.history"),
                }}
              />
            </div>
            <ContactTabPanels tab={tab} contact={contact} view={view} />
          </RecordView>
        )}
      </QueryGate>
      {/* One drawer over the record, not one per row: the account stays behind
          it because that is what the reader is working on. */}
      {openEmail && (
        <EmailDetail
          activityId={openEmail}
          onClose={() => setOpenEmail(null)}
          formatWhen={(iso) => formatDateTime(iso, locale, recordZone)}
        />
      )}
    </div>
  );
}
