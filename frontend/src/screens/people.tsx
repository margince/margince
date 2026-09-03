import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import { activityTimeline } from "../design-system/activitytimeline";
import { Badge, SegmentedControl } from "../design-system/atoms";
import { RecordView } from "../design-system/composed";
import {
  useRecordTimeline,
  useTimelineFilters,
} from "../design-system/recordtimeline";
import { TimelineFilterBar } from "../design-system/timelinefilterbar";
import { useToast } from "../design-system/toast";
import { ProvenanceTag } from "../design-system/trust";
import { normalizeProfileUrl } from "../format/profileurl";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { ArchiveAction } from "./archive";
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
import { RecordContextPanel } from "./context";
import { CreateAction, type CreateField, type FormRows } from "./create";
import { CustomFieldsCard } from "./customfields.card";
import {
  type ObjectCustomFields,
  useObjectCustomFields,
} from "./customfields.form";
import { EditAction } from "./edit";
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
import { MergeAction } from "./merge";
import {
  IdentityRail,
  type Person360,
  RelationshipPulse,
  ThinState,
  thinRecord,
  usePerson360,
  WhoKnowsThem,
} from "./person360";
import { EnrichedFields } from "./personcorrections";
import { PersonDealRooms } from "./persondealrooms";
import { PersonNetworkTab } from "./personnetwork";
import { PersonProjects } from "./personprojects";
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

// Contacts list + person 360 (B-EP09.10a/b). Every row carries its
// provenance chip (captured_by is server truth); the 360 renders the
// per-purpose consent card and evidence-or-omit fields — absent data is
// omitted, never guessed. Search/filter/sort/pagination (P-14), the rich
// create modal (P-15), the If-Match edit form (P-1), and the dedupe
// view-existing link (P-16) are the four shared blocks wired in here.

type Person = components["schemas"]["Person"];
type CreatePersonRequest = components["schemas"]["CreatePersonRequest"];
type UpdatePersonRequest = components["schemas"]["UpdatePersonRequest"];

async function fetchPeoplePage(
  query: ListQuery,
  cursor: string | null,
): Promise<ListPage<Person>> {
  const { data, error } = await api.GET("/people", {
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

// Merge-target search (P-2): reuses the list read, mapped down to the
// {id, name} shape MergeAction renders — the caller filters out the source
// row since this fetch has no notion of "the record being merged away".
async function searchPeopleTargets(
  q: string,
): Promise<{ id: string; name: string }[]> {
  const { data, error } = await api.GET("/people", {
    params: { query: { q, limit: 10 } },
  });
  if (error) {
    throwProblem(error);
  }
  return data.data.map((candidate) => ({
    id: candidate.id,
    name: candidate.full_name,
  }));
}

function asEmailType(value: string | undefined): "work" | "personal" | "other" {
  return value === "personal" || value === "other" ? value : "work";
}

function asPhoneType(
  value: string | undefined,
): "work" | "mobile" | "home" | "other" {
  return value === "mobile" || value === "home" || value === "other"
    ? value
    : "work";
}

// Builds the create-contact request body: scalar fields trim to undefined
// when blank (never sent rather than sent empty), `social.linkedin` folds
// into the `social` object, and each repeatable row becomes an
// emails/phones entry keyed by its position in the list.
export function mapPersonBody(
  values: Record<string, string>,
  rows: FormRows,
): CreatePersonRequest {
  const linkedin = values["social.linkedin"]?.trim();
  const emails = (rows.emails ?? [])
    .filter((row) => (row.email ?? "").trim().length > 0)
    .map((row, index) => ({
      email: row.email.trim(),
      email_type: asEmailType(row.email_type),
      is_primary: row.is_primary === "true",
      position: index,
    }));
  const phones = (rows.phones ?? [])
    .filter((row) => (row.phone ?? "").trim().length > 0)
    .map((row, index) => ({
      phone: row.phone.trim(),
      phone_type: asPhoneType(row.phone_type),
      is_primary: row.is_primary === "true",
      position: index,
    }));
  return {
    full_name: values.full_name.trim(),
    first_name: values.first_name?.trim() || undefined,
    last_name: values.last_name?.trim() || undefined,
    title: values.title?.trim() || undefined,
    social: linkedin ? { linkedin } : undefined,
    emails: emails.length > 0 ? emails : undefined,
    phones: phones.length > 0 ? phones : undefined,
    source: "manual",
  };
}

function stringField(value: unknown): string {
  return typeof value === "string" ? value : "";
}

// Builds the PATCH body: only the UpdatePersonRequest fields (never
// emails/phones — not in the contract's update shape).
export function mapPersonUpdate(
  values: Record<string, unknown>,
): UpdatePersonRequest {
  const linkedin = stringField(values["social.linkedin"]).trim();
  return {
    full_name: stringField(values.full_name).trim() || undefined,
    first_name: stringField(values.first_name).trim() || undefined,
    last_name: stringField(values.last_name).trim() || undefined,
    title: stringField(values.title).trim() || undefined,
    social: linkedin ? { linkedin } : undefined,
  };
}

// Built inside ContactsScreen (not module-level) because the email/phone
// "Type" options are display text, not raw values — fieldControl (create.tsx)
// renders option.label verbatim, so the human-readable string has to be
// resolved via useT() before it reaches CreateField, unlike organizations.tsx's
// size_band options, which are already display-ready raw labels ("1-10").
function contactCreateFields(t: ReturnType<typeof useT>): CreateField[] {
  return [
    { key: "full_name", label: "create.fullName", required: true },
    { key: "first_name", label: "create.firstName" },
    { key: "last_name", label: "create.lastName" },
    { key: "title", label: "create.personTitle" },
    { key: "social.linkedin", label: "create.linkedin" },
    {
      key: "emails",
      label: "create.email",
      type: "repeatable",
      addLabel: "field.addEmail",
      rowFields: [
        {
          key: "email",
          label: "create.email",
          type: "email",
          required: true,
        },
        {
          key: "email_type",
          label: "field.emailType",
          type: "select",
          options: [
            { value: "work", label: t("field.emailWork") },
            { value: "personal", label: t("field.emailPersonal") },
            { value: "other", label: t("field.emailOther") },
          ],
        },
      ],
      primaryKey: "is_primary",
    },
    {
      key: "phones",
      label: "create.phone",
      type: "repeatable",
      addLabel: "field.addPhone",
      rowFields: [
        { key: "phone", label: "create.phone", required: true },
        {
          key: "phone_type",
          label: "field.phoneType",
          type: "select",
          options: [
            { value: "work", label: t("field.phoneWork") },
            { value: "mobile", label: t("field.phoneMobile") },
            { value: "home", label: t("field.phoneHome") },
            { value: "other", label: t("field.phoneOther") },
          ],
        },
      ],
      primaryKey: "is_primary",
    },
  ];
}

const personEditFields: CreateField[] = [
  { key: "full_name", label: "create.fullName", required: true },
  { key: "first_name", label: "create.firstName" },
  { key: "last_name", label: "create.lastName" },
  { key: "title", label: "create.personTitle" },
  { key: "social.linkedin", label: "create.linkedin" },
];

async function createContact(
  values: Record<string, string>,
  rows: FormRows | undefined,
  customFields: Record<string, unknown>,
  t: (key: MessageKey) => string,
): Promise<Person> {
  const { data, error } = await api.POST("/people", {
    body: { ...mapPersonBody(values, rows ?? {}), ...customFields },
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
    { key: "title", label: "create.personTitle" },
    { key: "organization_name", label: "create.companyName" },
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

async function quickCapturePerson(
  values: Record<string, string>,
  t: (key: MessageKey) => string,
): Promise<Person> {
  const { data, error } = await api.POST("/people/quick-capture", {
    body: {
      full_name: values.full_name?.trim() ?? "",
      title: statedValue(values, "title"),
      organization_name: statedValue(values, "organization_name"),
      // Normalized here rather than server-side for the same reason the person
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
  return data.person;
}

function profileUrlOrUndefined(raw: string | undefined) {
  const stated = raw?.trim();
  return stated ? normalizeProfileUrl(stated) : undefined;
}

/**
 * PersonAside is the relationship column, and in overlay mode it SAYS it
 * cannot answer rather than disappearing.
 *
 * Both panels read the interaction projection, which is folded from natively
 * captured participants — a mirror-backed workspace has none. Rendering
 * nothing would let the page read as "nobody here knows them", which is a lie
 * about the relationship rather than an empty answer about the data.
 */
function PersonAside({
  view,
  overlay,
}: Readonly<{ view?: Person360; overlay: boolean }>) {
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
  const emails = (view.person.emails ?? []).map((e) => e.email);
  return (
    <>
      <RelationshipPulse view={view} />
      <PersonProjects
        personId={view.person.id}
        projects={view.projects}
        readOnly={Boolean(view.person.archived_at)}
      />
      <WhoKnowsThem view={view} />
      {emails.length > 0 ? <PersonDealRooms emails={emails} /> : null}
    </>
  );
}

export function ContactsScreen() {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  // Offered only once /me has answered: a chip whose value is still "" reads
  // as "clear this filter", so a half-built owner dial narrows nothing.
  const viewerId = useViewerId();
  const ownerChips = useOwnerChips();
  const tagChips = useTagChips();
  const savedViews = useSavedViewTabs("people");
  const cf = useObjectCustomFields("person");
  // The form that never closes gives no other feedback: without this, six
  // saved people look exactly like six that failed silently.
  const toast = useToast();
  const state = useListQuery<Person>({
    key: "people",
    initialSort: "-created_at",
    fetchPage: fetchPeoplePage,
  });

  return (
    <div className="wrap">
      <ListTable
        state={state}
        unit="unit.contacts"
        emptyNote={mineEmptyNote({ t, state, viewerId, unit: "unit.contacts" })}
        action={
          <>
            <CreateAction
              label={t("create.quickCapture")}
              invalidate="people"
              screen="contacts"
              testId="quick-capture"
              keepOpen
              create={(values) => quickCapturePerson(values, t)}
              onCreated={(person) =>
                toast.show(
                  t("create.quickCaptureSaved", { name: person.full_name }),
                )
              }
              resolveExisting={(_code, id) => ({ screen: "contacts", id })}
              fields={quickCaptureFields()}
            />
            <CreateAction
              label={t("create.contact")}
              invalidate="people"
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
            header: t("people.name"),
            cell: (person: Person) => (
              <span>
                <strong>{person.full_name}</strong>
                {person.title && (
                  <span className="t-caption"> · {person.title}</span>
                )}
                {person.archived_at && (
                  <Badge tone="warn">{t("record.archived")}</Badge>
                )}
              </span>
            ),
            sort: "full_name",
            fixed: true,
          },
          {
            key: "email",
            header: t("people.email"),
            cell: (person: Person) => (
              <span className="t-mono">
                {person.emails?.find((email) => email.is_primary)?.email ??
                  person.emails?.[0]?.email ??
                  ""}
              </span>
            ),
          },
          tagsColumn<Person>(t),
          ownerColumn<Person>(t),
          lastActivityColumn<Person>(t, locale, recordZone),
          createdColumn<Person>(t, locale, recordZone),
        ]}
        tools={<SaveViewAction resource="people" query={state.query} />}
        rowKey={(person) => person.id}
        rowRoute={(person) => ({ screen: "contacts", id: person.id })}
        dataChips={[...ownerChips, ...tagChips]}
        dataViews={savedViews}
        views={[
          ...standardViews(viewerId),
          { label: "list.viewAZ", sort: "full_name" },
        ]}
      />
    </div>
  );
}

const PERSON_TABS = ["overview", "relationships", "history"] as const;
type PersonTab = (typeof PERSON_TABS)[number];

// The verbs a person record offers, and the two the overlay withholds.
//
// Extracted from the 360 render so that render carries the record's SHAPE and
// this carries what may be done to it: the mode branch and the archive branch
// are both about the verbs, and reading either one no longer means holding the
// whole page.
function PersonActionBadges({
  person,
  cf,
  archivedReasonId,
}: Readonly<{
  person: Person;
  // Read at screen level and handed down, so the schema request runs BESIDE the
  // person's rather than after it. Started here, it would begin only once the
  // record had landed and the strip first rendered — and an edit opened in that
  // gap would offer a form with no custom fields on it.
  cf: ObjectCustomFields;
  // Minted once for the page by the caller, because the archive is a fact
  // about the record rather than about any one verb that refuses.
  archivedReasonId: string;
}>) {
  const t = useT();
  const overlay = useSorMode() === "overlay";
  const viewerId = useViewerId();
  const id = person.id;
  return (
    <>
      <ProvenanceTag provenance={provenanceOf(person.captured_by, viewerId)} />
      {/* Where this contact came from, when it came from a lead
          (ADR-0119/A170). The pointer runs person → lead and the
          lead's page is a terminal record of the promotion, so the
          chip is a link rather than a label: a rep asking "was this
          a merge or a new contact?" reads the answer there. */}
      {person.converted_from_lead_id && (
        <Badge tone="accent">
          {t("person.fromLead")}{" "}
          <EntityRef kind="lead" id={person.converted_from_lead_id} />
        </Badge>
      )}
      {person.archived_at && <Badge tone="warn">{t("record.archived")}</Badge>}
      {/* An archived record is read-only: the backend rejects
          edit/merge/archive on a non-live row (there is no
          unarchive path). The verbs stay VISIBLE and refused,
          pointing at the page's one sentence about the archive
          (STATE-4a): a missing control says nothing about the
          record, while a refused one names the reason. */}
      <EditAction<Person>
        disabledReasonId={person.archived_at ? archivedReasonId : undefined}
        label={t("record.edit")}
        savedMessage={(saved) =>
          t("record.saveDone", { name: saved.full_name })
        }
        notice={overlay ? t("overlay.partialWriteBack") : undefined}
        fields={[...personEditFields, ...cf.formFields]}
        record={{
          id: person.id,
          version: person.version,
          full_name: person.full_name,
          first_name: person.first_name ?? "",
          last_name: person.last_name ?? "",
          title: person.title ?? "",
          "social.linkedin": stringField(person.social?.linkedin),
          ...cf.recordSlice(person),
        }}
        update={async (values, _rows, opened) => {
          const { data, error } = await api.PATCH("/people/{id}", {
            params: {
              path: { id },
              ...ifMatch(requireVersion(opened?.version)),
            },
            body: {
              ...mapPersonUpdate(values),
              // A diff against what the form prefilled from: a
              // snapshot sends `null` for every empty custom field,
              // and the API reads that as clearing a column nobody
              // touched.
              ...cf.toPatch(values, opened ?? {}),
            },
          });
          if (error) {
            throwProblem(error);
          }
          return data;
        }}
        invalidate="people"
        recordKey="person"
      />
      {/* Merge has no incumbent-first projection — the seam
          refuses it outright (overlay/provider_writes.go
          Merge) — unlike edit/archive below, which it
          serves, so it stays hidden here. */}
      {!overlay && (
        <MergeAction
          disabledReasonId={person.archived_at ? archivedReasonId : undefined}
          label={t("merge.person")}
          sourceId={person.id}
          sourceName={person.full_name}
          searchTargets={searchPeopleTargets}
          merge={async (targetId) => {
            const { data, error } = await api.POST("/people/{id}/merge", {
              params: {
                path: { id: person.id },
                ...ifMatch(requireVersion(person.version)),
              },
              body: { target_id: targetId },
            });
            if (error) {
              throwProblem(error, t);
            }
            return data;
          }}
          invalidate="people"
          recordKey="person"
          survivorRoute={(targetId) => ({
            screen: "contacts",
            id: targetId,
          })}
        />
      )}
      <ArchiveAction
        disabledReasonId={person.archived_at ? archivedReasonId : undefined}
        label={t("record.archive")}
        confirmText={t("record.archiveConfirm")}
        archivedMessage={t("record.archiveDone", {
          name: person.full_name,
        })}
        archive={async () => {
          const { data, error } = await api.DELETE("/people/{id}", {
            params: { path: { id } },
          });
          if (error) {
            throwProblem(error);
          }
          return data;
        }}
        invalidate="people"
        recordKey="person"
        onArchived={() => navigate({ screen: "contacts" })}
      />
      {/* A record grant probes the native row via
          auth.EnsureLinkTarget, which a mirrored record has
          no row for — sharing stays hidden in overlay
          regardless of record type (see deals.tsx's
          DealBadges). */}
      {!overlay && (
        <ShareAction
          recordType="person"
          recordId={person.id}
          disabledReasonId={person.archived_at ? archivedReasonId : undefined}
        />
      )}
    </>
  );
}

// What the chosen tab shows. One component per screen rather than one per tab:
// the panels share the record and differ only in which of it they draw, and a
// component apiece would put five files between a reader and that fact.
function PersonTabPanels({
  tab,
  person,
  view,
}: Readonly<{
  tab: PersonTab;
  person: Person;
  view?: Person360;
}>) {
  const queryClient = useQueryClient();
  const id = person.id;
  return (
    <>
      {tab === "overview" && thinRecord(view) && view && (
        <ThinState view={view} />
      )}
      {/* Consent renders on a thin record too: it is not an absence
          but a guard — what you may send is a live fact whether or
          not anyone has written to them yet. */}
      {tab === "overview" && <ConsentSection personId={person.id} />}
      {tab === "overview" && view && (
        <EnrichedFields personId={id} view={view} />
      )}
      {tab === "overview" && !thinRecord(view) && (
        <>
          <CustomFieldsCard object="person" record={person} />
          <RecordContextPanel entityType="person" id={person.id} />
          <LogActivity entityType="person" entityId={person.id} />
        </>
      )}
      {tab === "relationships" && (
        <div style={{ display: "grid", gap: "var(--space-4)" }}>
          <PersonNetworkTab personId={id} />
          <RelationshipsTab scope={{ person_id: person.id }} />
        </div>
      )}
      {tab === "history" && (
        <RecordHistoryTab
          kind="person"
          id={person.id}
          restore={{
            version: person.version,
            onRestored: () =>
              invalidateRecord(queryClient, "person", person.id),
          }}
        />
      )}
    </>
  );
}

export function PersonScreen({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const recordZone = useRecordZone();
  const cf = useObjectCustomFields("person");
  // ONE sentence about this contact being archived, minted here and pointed at
  // by every verb the archive refuses. Said once for the page rather than
  // beside each of four buttons.
  const archivedReasonId = useId();
  const [tab, setTab] = useState<PersonTab>("overview");
  const personQuery = useQuery({
    queryKey: ["person", id],
    queryFn: async () => {
      const { data, error } = await api.GET("/people/{id}", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
  const [timelineFilters, setTimelineFilters] = useTimelineFilters(id);
  const timelineQuery = useRecordTimeline("person", id, {
    filters: timelineFilters,
  });
  const view360 = usePerson360(id);
  // The composite is only usable once it carries its mandatory root record.
  // Guarding on the whole payload would let a partial or error-shaped body
  // through and crash the rail on a person that is not there.
  const view = view360.data?.person ? view360.data : undefined;
  const overlay = useSorMode() === "overlay";
  const viewerId = useViewerId();
  const timelineEntries = activityTimeline(
    timelineQuery.activities,
    viewerId,
    (activity) => (
      <TimelineActions
        activity={activity}
        entityType="person"
        entityId={id}
        personId={id}
      />
    ),
  );

  return (
    <div className="wrap">
      <QueryGate query={personQuery}>
        {(person) => (
          <RecordView
            name={person.full_name}
            subtitle={person.title ?? undefined}
            zone={recordZone}
            badges={
              <PersonActionBadges
                person={person}
                cf={cf}
                archivedReasonId={archivedReasonId}
              />
            }
            // The archive is a fact about the whole record, so it is stated
            // once across the header rather than repeated beside each verb it
            // refuses. The rail says the same thing about its own inline
            // edits (personrail.tsx) off this same key, so the two cannot
            // drift into two spellings of one fact. Absent while the contact
            // is live: a line always reserved would read as a record with
            // something to say about itself and nothing said.
            band={
              person.archived_at ? (
                <p id={archivedReasonId} className="t-caption">
                  {t("person.rail.archivedReadOnly")}
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
            aside={<PersonAside view={view} overlay={overlay} />}
          >
            <div style={{ marginBottom: 16 }}>
              <SegmentedControl
                options={PERSON_TABS}
                value={tab}
                onChange={setTab}
                labels={{
                  overview: t("tab.overview"),
                  relationships: t("tab.relationships"),
                  history: t("tab.history"),
                }}
              />
            </div>
            <PersonTabPanels tab={tab} person={person} view={view} />
          </RecordView>
        )}
      </QueryGate>
    </div>
  );
}
