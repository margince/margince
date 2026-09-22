import { api } from "../api/client";
import type { components } from "../api/schema";
import { usePageName } from "../app/pagemeta";
import { useRecordZone } from "../app/recordzone";
import { Badge } from "../design-system/atoms";
import { useToast } from "../design-system/toast";
import { normalizeProfileUrl } from "../format/profileurl";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { throwProblem, useViewerId } from "./common";
import { contactCreateFields, mapContactBody } from "./contactformfields";
import { CreateAction, type CreateField, type FormRows } from "./create";
import { useObjectCustomFields } from "./customfields.form";
import { EntityRef } from "./entityref";
import {
  type ListPage,
  type ListQuery,
  ListTable,
  listFetchLimit,
  useListQuery,
  useOwnerChips,
  useTagChips,
} from "./listquery";
import {
  createdColumn,
  lastActivityColumn,
  mineEmptyNote,
  ownerColumn,
  standardViews,
  tagsColumn,
} from "./recordlist";
import { SaveViewAction, useSavedViewTabs } from "./savedviews";
import { listQueryParams } from "./tagfilter";
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
      total: data.page.total,
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
                  <Badge tone="warning">{t("record.archived")}</Badge>
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
            // like every other value in the row — somebody a reader could
            // write to, not an identifier.
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
        saveView={<SaveViewAction resource="contacts" query={state.query} />}
        rowKey={(contact) => contact.id}
        rowRoute={(contact) => ({ screen: "contacts", id: contact.id })}
        dataChips={[...ownerChips, ...tagChips]}
        dataViews={savedViews}
        views={[...standardViews(viewerId)]}
      />
    </div>
  );
}
