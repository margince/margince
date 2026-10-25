import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { AddressCountryMismatchBadge } from "../../components/addresscountrymismatch";
import { ADDRESS_FIELDS, addressFrom } from "../../components/companyform";
import { useEmployerSummary } from "../../components/contactemployersummary";
import {
  contactEditFields,
  mapContactUpdate,
} from "../../components/contactformfields";
import { rosterOwnerName } from "../../components/entityref";
import { RecordCustomFields } from "../../components/recordcustomfields";
import { saveRecordEdit } from "../../components/recordedit";
import { RecordFields, rawRecord } from "../../components/recordfields";
import { TagsPanel } from "../../components/tagspanel";
import { Badge } from "../../design-system/atoms";
import { ContactLink } from "../../design-system/contactlink";
import { FieldRow } from "../../design-system/fieldgrid";
import { OffsiteLink } from "../../design-system/offsitelink";
import { RecordCard } from "../../design-system/recordcard";
import type {
  DetailRow,
  DetailSection,
} from "../../design-system/recorddetails";
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
import { useMemberName } from "../../lib/membernames";
import { useRecordZone } from "../../lib/recordzone";
import { routeHash } from "../../lib/router";
import { useRecordOwners } from "../records/recordreferences";
import { BoughtMark, boughtFields } from "./boughtmarks";
import { BoughtAttributes } from "./contactboughtattributes";
import { ContactSource } from "./contactfacts";
import { currentEmployer } from "./employmentcurrency";

type Contact = components["schemas"]["Contact"];
type Contact360 = components["schemas"]["Contact360"];
type Employment = components["schemas"]["Contact360Employment"];

/**
 * The left column's About and Ownership groups. Every field the design
 * lists has a row; an empty one folds behind "Show N empty fields" with the
 * word for its empty state. The rows read; the Research tab's details change them.
 */
export function useContactAboutSections(view: Contact360): DetailSection[] {
  const t = useT();
  const contact = view.contact;
  const bought = boughtFields(contact);
  const ownerName = useMemberName(contact.owner_id);
  const notSet = t("field.unset");
  const linkedin =
    typeof contact.social?.linkedin === "string" ? contact.social.linkedin : "";
  const linkedinHref = linkedinUrl(linkedin);
  const email = contact.primary_email;
  const emailRow = contact.emails?.find((row) => row.email === email);
  const phone = primaryPhone(contact.phones);
  const address = [contact.address?.city, contact.address?.country]
    .filter(Boolean)
    .join(", ");
  const employer = currentEmployer(view.employments?.data);
  const owner = rosterOwnerName(contact.owner_id, ownerName, t, notSet);
  const field = (
    key: string,
    term: string,
    value: ReactNode,
    present: boolean,
  ): DetailRow => ({
    key,
    term,
    value: present ? value : notSet,
    empty: !present,
  });
  return [
    {
      key: "about",
      label: t("record.details.about"),
      rows: [
        {
          key: "employer",
          term: t("contact.rail.employer"),
          value: employer ? <EmployerCard employment={employer} /> : notSet,
          empty: !employer,
          block: Boolean(employer),
        },
        field(
          "email",
          t("create.email"),
          email && (
            <>
              <ContactLink
                kind="email"
                value={email}
                record={{ entityType: "contact", entityId: contact.id }}
              />
              <BoughtMark
                bought={emailRow && bought.get(`email:${emailRow.id}`)}
                subject={email}
              />
            </>
          ),
          Boolean(email),
        ),
        field(
          "phone",
          t("create.phone"),
          phone && (
            <span className="fieldgrid-handle">
              <ContactLink
                kind="phone"
                value={phone.phone}
                record={{ entityType: "contact", entityId: contact.id }}
              />
              <BoughtMark
                bought={bought.get(`phone:${phone.id}`)}
                subject={phone.phone}
              />
            </span>
          ),
          Boolean(phone),
        ),
        field(
          "linkedin",
          t("contact.page.linkedin"),
          // The label claims the destination is LinkedIn, and a crawl or a
          // connector writes this value, so another host gets no link.
          linkedinHref ? (
            <OffsiteLink href={linkedin}>
              {linkedinHref.pathname.replace(/^\/|\/$/g, "")}
            </OffsiteLink>
          ) : (
            linkedin
          ),
          linkedin !== "",
        ),
        field(
          "title",
          t("create.contactTitle"),
          <>
            {contact.title}
            <BoughtMark
              bought={bought.get("title")}
              subject={contact.title ?? ""}
            />
          </>,
          Boolean(contact.title),
        ),
        field(
          "address",
          t("co.address.summary"),
          <>
            {address}
            {contact.address_country_mismatch && (
              <>
                {" "}
                <AddressCountryMismatchBadge
                  mismatch={contact.address_country_mismatch}
                />
              </>
            )}
          </>,
          address !== "",
        ),
      ],
    },
    {
      key: "ownership",
      label: t("record.details.ownership"),
      rows: [
        field(
          "owner",
          t("contact.page.owner"),
          // An owner the roster no longer names is still the owner, and the
          // warning ink says the record needs a new one.
          owner === t("ref.notInRoster") ? (
            <span className="pe-detail-warn">
              {t("contact.aside.ownerGone")}
            </span>
          ) : (
            owner
          ),
          Boolean(contact.owner_id),
        ),
      ],
    },
  ];
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
 * Every field of the contact, each editing in place, with its tags, when it
 * was created and who wrote it. The Research tab draws it: the left column
 * reads the same fields, and this is where they change.
 */
export function ContactDetails({
  contact,
  profiles,
}: Readonly<{ contact: Contact; profiles?: readonly Profile[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const canEdit = useCanWriteRecord("contact", contact) && !contact.archived_at;
  const owners = useRecordOwners(contact.owner_id);
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
                    {t(EMAIL_TYPE_LABEL[email.email_type ?? "work"])}
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
                    {t(PHONE_TYPE_LABEL[phone.phone_type ?? "work"])}
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
        canEdit={canEdit}
        // Tags as one more row of the card: filing beside the facts it files.
        extraRows={
          <>
            {/* What a provider sold that no field holds: shown as bought facts
                beside the record, never copied into the address. */}
            <BoughtAttributes profiles={profiles} />
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
        fields={[
          ...contactEditFields(t),
          ...ADDRESS_FIELDS,
          {
            // Still reads visibility, and does not offer it. An owner-private
            // contact must name an owner, and this field states the rule the
            // header's toggle obeys.
            key: "owner_id",
            required: contact.visibility === "owner",
            label: "list.owner",
            type: "select",
            options: owners,
          },
        ]}
        groups={[
          {
            label: t("co.address.summary"),
            keys: ADDRESS_FIELDS.map((field) => field.key),
          },
        ]}
        record={{
          ...contact,
          original: contact,
          ...addressFrom(contact.address),
          "social.linkedin": linkedin,
        }}
        resolveExisting={(_code, id) => ({ screen: "contacts", id })}
        save={async (values, rows, opened) => {
          return saveRecordEdit(
            "contact",
            rawRecord(opened),
            mapContactUpdate(values, rows, opened),
          );
        }}
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
