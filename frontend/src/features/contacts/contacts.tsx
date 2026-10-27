import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { usePageName } from "../../app/pagemeta";
import { useBulkSelection } from "../../components/bulkverbs";
import { EntityRef } from "../../components/entityref";
import {
  type ListPage,
  type ListQuery,
  ListTable,
  listFetchLimit,
  useListQuery,
} from "../../components/listquery";
import {
  createdColumn,
  lastActivityColumn,
  mineEmptyNote,
  ownerColumn,
  standardViews,
  tagsColumn,
  useOwnerTagChips,
} from "../../components/recordlist";
import { Badge } from "../../design-system/atoms";
import { Avatar } from "../../design-system/avatar";
import { ContactLink } from "../../design-system/contactlink";
import { useLocale, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { useViewerId } from "../../lib/session";
import { listQueryParams } from "../../lib/tagfilter";
import { VCardImportAction, VCardImportPage } from "../import/vcard-import";
import { SaveViewAction, useSavedViewTabs } from "../lists/savedviews";
import { ContactCreateActions } from "./contactcreate";

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
  const data = unwrap(
    await api.GET("/contacts", {
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
    }),
  );
  return {
    data: data.data,
    page: {
      next_cursor: data.page.next_cursor ?? null,
      has_more: data.page.has_more,
      total: data.page.total,
    },
  };
}

// `#/contacts/import` is this screen's reserved segment (app/nav.ts), so the
// list's reads never start for the import page.
export function ContactsScreen({
  importing = false,
}: Readonly<{ importing?: boolean }>) {
  return importing ? <VCardImportPage /> : <ContactsList />;
}

function ContactsList() {
  const t = useT();
  const pageName = usePageName("contacts");
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  // Offered only once /me has answered: a chip whose value is still "" reads
  // as "clear this filter", so a half-built owner dial narrows nothing.
  const viewerId = useViewerId();
  const ownerChips = useOwnerTagChips();
  const savedViews = useSavedViewTabs("contacts");
  const state = useListQuery<Contact>({
    key: "contacts",
    initialSort: "-created_at",
    fetchPage: fetchContactsPage,
  });
  const selection = useBulkSelection({
    rows: state.rows,
    recordType: "contact",
    labelOf: (contact) => contact.full_name,
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
            <ContactCreateActions />
            {/* Last, because it is the one a reader reaches for with a stack
                of cards already in hand rather than while typing one contact. */}
            <VCardImportAction />
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
            cell: (contact: Contact) =>
              contact.primary_email ? (
                <ContactLink
                  kind="email"
                  value={contact.primary_email}
                  record={{ entityType: "contact", entityId: contact.id }}
                  readOnly={Boolean(contact.archived_at)}
                />
              ) : null,
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
                <span className="avatar-row">
                  <Avatar
                    identity={contact.employer.company_id}
                    name={contact.employer.company_name}
                    src={contact.employer.logo_url}
                  />
                  <EntityRef
                    kind="company"
                    id={contact.employer.company_id}
                    name={contact.employer.company_name}
                  />
                </span>
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
        selection={selection}
        dataChips={ownerChips}
        dataViews={savedViews}
        views={[...standardViews(viewerId)]}
      />
    </div>
  );
}
