import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { AddressCountryMismatchBadge } from "../../components/addresscountrymismatch";
import { ADDRESS_FIELDS } from "../../components/companyform";
import { useEmployerSummary } from "../../components/contactemployersummary";
import { rosterOwnerName } from "../../components/entityref";
import { leadHeldNoticeFor } from "../../components/leadheldnotice";
import { LinkedInChip } from "../../components/linkedinchip";
import { RecordCustomFields } from "../../components/recordcustomfields";
import { RecordFields } from "../../components/recordfields";
import { useSectionEdit } from "../../components/sectionedit";
import { TagsPanel } from "../../components/tagspanel";
import { Badge } from "../../design-system/atoms";
import { ContactLink } from "../../design-system/contactlink";
import { FieldRow } from "../../design-system/fieldgrid";
import { reachRow } from "../../design-system/reachcard";
import { RecordCard } from "../../design-system/recordcard";
import type {
  DetailRow,
  DetailSection,
} from "../../design-system/recorddetails";
import { postalLines } from "../../format/address";
import {
  EMAIL_TYPE_LABEL,
  PHONE_TYPE_LABEL,
} from "../../format/contactchannels";
import { primaryPhone } from "../../format/contacturi";
import { formatDateAbbrev } from "../../format/format";
import { industryWords } from "../../format/industry";
import { linkedinUrl } from "../../format/weburl";
import { useLocale, useT } from "../../i18n";
import { useCanWriteRecord } from "../../lib/capability";
import {
  currentEmployer,
  useEmploymentZone,
} from "../../lib/employmentcurrency";
import { labelFor } from "../../lib/labelfor";
import { useMemberName } from "../../lib/membernames";
import { routeHash } from "../../lib/router";
import { useViewerZone } from "../../lib/viewerzone";
import { BoughtMark, boughtFields } from "./boughtmarks";
import { BoughtAttributes } from "./contactboughtattributes";
import { useContactEdit } from "./contactedit";
import { ContactSource } from "./contactfacts";
import { ContactReach } from "./contacthandles";
import { useEmployerChange } from "./employerchange";
import { type ProfileField, proposedEmployer } from "./employerproposal";
import { ProposedEmployerRow } from "./employerproposalrow";

type Contact = components["schemas"]["Contact"];
type Contact360 = components["schemas"]["Contact360"];
type Employment = components["schemas"]["Contact360Employment"];

/**
 * The left column's About, Address and Ownership groups, each with its own
 * Edit. An empty row folds behind "Show N empty fields"; render `element` beside.
 */
export function useContactAboutSections(
  view: Contact360,
): Readonly<{ sections: DetailSection[]; element: ReactNode }> {
  const t = useT();
  const contact = view.contact;
  const bought = boughtFields(contact);
  const ownerName = useMemberName(contact.owner_id);
  const notSet = t("field.unset");
  const linkedin =
    typeof contact.social?.linkedin === "string" ? contact.social.linkedin : "";
  const employmentZone = useEmploymentZone();
  const employer = currentEmployer(view.employments?.data, employmentZone);
  const proposed = employer
    ? undefined
    : proposedEmployer(view, view.employments?.data ?? [], employmentZone);
  const owner = rosterOwnerName(contact.owner_id, ownerName, t, notSet);
  const edits = useSectionEdits(view, employer, proposed);
  const field = (
    key: string,
    term: string,
    reading: ReactNode,
    shape: RowShape = {},
  ): DetailRow => readingRow(key, term, reading, notSet, shape);
  // Every address and number shares one card. A kind with none folds as its
  // own empty row, so "Show empty fields" still offers to add it.
  const email = contact.primary_email;
  const phone = primaryPhone(contact.phones);
  const reachRows: DetailRow[] = [
    ...(email || phone
      ? [
          reachRow(
            t("record.reach.label"),
            <ContactReach contact={contact} bought={bought} />,
          ),
        ]
      : []),
    ...(email ? [] : [field("email", t("create.email"), undefined)]),
    ...(phone ? [] : [field("phone", t("create.phone"), undefined)]),
  ];
  const sections: DetailSection[] = [
    {
      key: "about",
      label: t("record.details.about"),
      edit: edits.about,
      rows: [
        {
          key: "employer",
          term: t("contact.rail.employer"),
          value: employerReading(view, employer, proposed, notSet),
          empty: !employer && !proposed,
          block: Boolean(employer ?? proposed),
        },
        ...reachRows,
        field(
          "linkedin",
          t("contact.page.linkedin"),
          linkedin && <LinkedInChip href={linkedin} name={contact.full_name} />,
          { handle: Boolean(linkedinUrl(linkedin)) },
        ),
        field("title", t("create.contactTitle"), contact.title, {
          beside: (
            <BoughtMark
              bought={bought.get("title")}
              subject={contact.title ?? ""}
            />
          ),
        }),
      ],
    },
    {
      key: "address",
      label: t("co.address.summary"),
      edit: edits.address,
      rows: [
        field(
          "address",
          t("co.address.summary"),
          postalLines(contact.address),
          {
            beside: contact.address_country_mismatch && (
              <>
                {" "}
                <AddressCountryMismatchBadge
                  mismatch={contact.address_country_mismatch}
                />
              </>
            ),
          },
        ),
      ],
    },
    {
      key: "ownership",
      label: t("record.details.ownership"),
      edit: edits.ownership,
      rows: [
        field(
          "owner",
          t("contact.page.owner"),
          // An owner the roster no longer names is still the owner, and the
          // warning ink says the record needs a new one.
          contact.owner_id &&
            (owner === t("ref.notInRoster") ? (
              <span className="pe-detail-warn">
                {t("contact.aside.ownerGone")}
              </span>
            ) : (
              owner
            )),
        ),
      ],
    },
  ];
  return { sections, element: edits.element };
}

// Each group's Edit over the contact's one edit. A proposed employer is
// settled by its own row's verbs, so the dialog offers no employer then.
function useSectionEdits(
  view: Contact360,
  employer: Employment | undefined,
  proposed: ProfileField | undefined,
) {
  const t = useT();
  const edit = useContactEdit(view.contact);
  const employerChange = useEmployerChange(view, employer);
  const changeEmployer = proposed ? undefined : employerChange.open;
  const about = useSectionEdit(edit, {
    label: t("record.details.about"),
    keys: ["employer", "emails", "phones", "social.linkedin", "title"],
    flows: changeEmployer && {
      employer: {
        label: t("contact.rail.employer"),
        reading: employer?.company_name ?? t("field.unset"),
        open: changeEmployer,
      },
    },
  });
  const address = useSectionEdit(edit, {
    label: t("co.address.summary"),
    keys: ADDRESS_FIELDS.map((entry) => entry.key),
  });
  const ownership = useSectionEdit(edit, {
    label: t("record.details.ownership"),
    keys: ["owner_id"],
  });
  return {
    about: about.edit,
    address: address.edit,
    ownership: ownership.edit,
    element: (
      <>
        {employerChange.element}
        {about.element}
        {address.element}
        {ownership.element}
      </>
    ),
  };
}

// A handle (an address or number with its marks and glyphs) lays them out.
type RowShape = Readonly<{ handle?: boolean; beside?: ReactNode }>;

function readingRow(
  key: string,
  term: string,
  reading: ReactNode,
  notSet: string,
  { handle, beside }: RowShape,
): DetailRow {
  return {
    key,
    term,
    value: (
      <>
        {handle && reading ? (
          <span className="fieldgrid-handle">{reading}</span>
        ) : (
          reading || notSet
        )}
        {beside}
      </>
    ),
    empty: !reading,
  };
}

function employerReading(
  view: Contact360,
  employer: Employment | undefined,
  proposed: ProfileField | undefined,
  notSet: string,
): ReactNode {
  if (proposed) return <ProposedEmployerRow view={view} fact={proposed} />;
  return employer ? <EmployerCard employment={employer} /> : notSet;
}

/**
 * Where they work now, as the catalog's record card. The company's mark and
 * name are the way in, "Current" sits at its end, and what the account is to
 * us goes under it. The facts are the company's own, so a company this reader may
 * not open shows its name alone.
 */
function EmployerCard({ employment }: Readonly<{ employment: Employment }>) {
  const t = useT();
  const company = useEmployerSummary(employment.company_id).data;
  const name = company?.display_name ?? employment.company_name ?? "";
  const facts = company
    ? [
        company.lifecycle && company.lifecycle !== "unknown"
          ? t(`company.lifecycle.${company.lifecycle}`)
          : null,
        company.industry ? industryWords(t, company.industry) : null,
        company.address?.city,
      ]
        .filter(Boolean)
        .join(" · ")
    : "";
  return (
    <RecordCard
      kind="company"
      name={name}
      href={routeHash({ screen: "companies", id: employment.company_id })}
      identity={employment.company_id}
      logo={company?.logo_url}
      position={facts || undefined}
      aside={<Badge wrap>{t("rel.current")}</Badge>}
    />
  );
}

/**
 * Every field of the contact, each editing from its value, with its tags, when it
 * was created and who wrote it. The Research tab draws it; the left column edits
 * its own rows the same way.
 */
export function ContactDetails({
  contact,
  profiles,
}: Readonly<{
  contact: Contact;
  profiles?: readonly Profile[];
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const edit = useContactEdit(contact);
  const bought = boughtFields(contact);
  const linkedin =
    typeof contact.social?.linkedin === "string" ? contact.social.linkedin : "";
  return (
    <>
      <RecordFields
        title={t("contact.rail.detailsTitle")}
        kind="contact"
        renderValues={{
          // Each handle with its kind beside it (work, mobile, personal), the
          // same words the edit form offers, so a reader with two numbers
          // knows which one rings a desk.
          emails: contact.emails?.length
            ? contact.emails.map((email) => (
                <span key={email.email} className="fieldgrid-handle">
                  <ContactLink
                    kind="email"
                    value={email.email}
                    record={{ entityType: "contact", entityId: contact.id }}
                    readOnly={Boolean(contact.archived_at)}
                  />
                  <span className="t-caption">
                    {t(labelFor(EMAIL_TYPE_LABEL, email.email_type ?? "work"))}
                  </span>
                  <BoughtMark
                    bought={bought.get(`email:${email.id}`)}
                    subject={email.email}
                  />
                </span>
              ))
            : undefined,
          phones: contact.phones?.length
            ? contact.phones.map((phone) => (
                <span key={phone.phone} className="fieldgrid-handle">
                  <ContactLink
                    kind="phone"
                    value={phone.phone}
                    record={{ entityType: "contact", entityId: contact.id }}
                  />
                  <span className="t-caption">
                    {t(labelFor(PHONE_TYPE_LABEL, phone.phone_type ?? "work"))}
                  </span>
                  <BoughtMark
                    bought={bought.get(`phone:${phone.id}`)}
                    subject={phone.phone}
                  />
                </span>
              ))
            : undefined,
        }}
        // The label claims the destination is LinkedIn, and a crawl or a
        // connector writes this value, so another host gets no link.
        links={
          linkedinUrl(linkedin)
            ? {
                "social.linkedin": {
                  href: linkedin,
                  label: t("record.openProfile"),
                },
              }
            : undefined
        }
        marks={{
          title: (
            <BoughtMark
              bought={bought.get("title")}
              subject={contact.title ?? ""}
            />
          ),
          "social.linkedin": (
            <BoughtMark bought={bought.get("linkedin")} subject={linkedin} />
          ),
        }}
        canEdit={edit.canEdit}
        // Tags as one more row of the card: filing beside the facts it files.
        extraRows={
          <>
            {/* What a provider sold that no field holds: shown as bought facts
                beside the record, never copied into the address. */}
            <BoughtAttributes
              profiles={profiles}
              runs={contact.bought_fields}
            />
            {contact.archived_at ? null : (
              <FieldRow label={t("tags.panelTitle")} align="top">
                <ContactTagsSection contact={contact} />
              </FieldRow>
            )}
            <FieldRow label={t("list.created")}>
              {formatDateAbbrev(contact.created_at, locale, zone)}
            </FieldRow>
            <FieldRow label={t("history.field.source")} align="middle">
              <ContactSource contact={contact} />
            </FieldRow>
          </>
        }
        fields={[...edit.fields]}
        groups={[
          {
            label: t("co.address.summary"),
            keys: ADDRESS_FIELDS.map((field) => field.key),
          },
        ]}
        record={edit.record}
        resolveExisting={edit.resolveExisting}
        refusalNotice={leadHeldNoticeFor}
        save={edit.save}
      />
      <RecordCustomFields kind="contact" record={contact} />
    </>
  );
}

type Profile = components["schemas"]["ContactProviderProfile"];

/**
 * The contact's tags in the shared panel. The verb asks what the server asks
 * before it writes: grant, seat, the row's `writable`, and not archived.
 * Exported so its test drives this mount, not a literal `canEdit`.
 */
export function ContactTagsSection({
  contact,
}: Readonly<{ contact: Contact }>) {
  const canEdit = useCanWriteRecord("contact", contact) && !contact.archived_at;
  return (
    <TagsPanel
      entityType="contact"
      entityID={contact.id}
      canEdit={canEdit}
      employer={contact.employer}
      bare
    />
  );
}
