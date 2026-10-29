import type { ComponentProps, ReactNode } from "react";
import type { components } from "../../api/schema";
import { AddressCountryMismatchBadge } from "../../components/addresscountrymismatch";
import { ADDRESS_FIELDS } from "../../components/companyform";
import { useEmployerSummary } from "../../components/contactemployersummary";
import { rosterOwnerName } from "../../components/entityref";
import { leadHeldNoticeFor } from "../../components/leadheldnotice";
import { RailField } from "../../components/railfield";
import { RecordCustomFields } from "../../components/recordcustomfields";
import { RecordFields } from "../../components/recordfields";
import { TagsPanel } from "../../components/tagspanel";
import { Badge } from "../../design-system/atoms";
import {
  ContactLink,
  CopyEmail,
  EmailFace,
} from "../../design-system/contactlink";
import { FieldRow } from "../../design-system/fieldgrid";
import { OffsiteLink } from "../../design-system/offsitelink";
import { RecordCard } from "../../design-system/recordcard";
import type {
  DetailRow,
  DetailSection,
} from "../../design-system/recorddetails";
import { countryBeyondCity } from "../../format/address";
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
import { BoughtMark, boughtFields } from "./boughtmarks";
import { BoughtAttributes } from "./contactboughtattributes";
import { useContactEdit } from "./contactedit";
import { ContactSource } from "./contactfacts";
import { type ProfileField, proposedEmployer } from "./employerproposal";
import { ProposedEmployerRow } from "./employerproposalrow";
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
  const edit = useContactEdit(contact);
  const bought = boughtFields(contact);
  const ownerName = useMemberName(contact.owner_id);
  const notSet = t("field.unset");
  const linkedin =
    typeof contact.social?.linkedin === "string" ? contact.social.linkedin : "";
  const linkedinHref = linkedinUrl(linkedin);
  const email = contact.primary_email;
  const emailRow = contact.emails?.find((row) => row.email === email);
  const phone = primaryPhone(contact.phones);
  const address = [
    contact.address?.city,
    countryBeyondCity(contact.address?.city, contact.address?.country),
  ]
    .filter(Boolean)
    .join(", ");
  const employer = currentEmployer(view.employments?.data);
  const proposed = employer
    ? undefined
    : proposedEmployer(view, view.employments?.data ?? []);
  const owner = rosterOwnerName(contact.owner_id, ownerName, t, notSet);
  // Every row but the employer changes in place; an empty one still folds.
  // A handle (an address or number with its marks and glyphs) lays them out.
  const field = (
    key: string,
    term: string,
    present: boolean,
    change: Omit<ComponentProps<typeof RailField>, "edit" | "label">,
  ): DetailRow => {
    const value = (
      <RailField
        {...change}
        edit={edit}
        label={term}
        reading={present ? change.reading : notSet}
      />
    );
    return {
      key,
      term,
      value:
        present && change.link ? (
          <span className="fieldgrid-handle">{value}</span>
        ) : (
          value
        ),
      empty: !present,
    };
  };
  return [
    {
      key: "about",
      label: t("record.details.about"),
      rows: [
        {
          key: "employer",
          term: t("contact.rail.employer"),
          value: employerValue(view, employer, proposed, notSet),
          empty: !employer && !proposed,
          block: Boolean(employer ?? proposed),
        },
        field("email", t("create.email"), Boolean(email), {
          keys: ["emails"],
          link: Boolean(email),
          reading: email && (
            <>
              <ContactLink
                kind="email"
                value={email}
                record={{ entityType: "contact", entityId: contact.id }}
              >
                <EmailFace address={email} />
              </ContactLink>
              <BoughtMark
                bought={emailRow && bought.get(`email:${emailRow.id}`)}
                subject={email}
              />
              <CopyEmail address={email} />
            </>
          ),
        }),
        field("phone", t("create.phone"), Boolean(phone), {
          keys: ["phones"],
          link: Boolean(phone),
          reading: phone && (
            <>
              <ContactLink
                kind="phone"
                value={phone.phone}
                record={{ entityType: "contact", entityId: contact.id }}
              />
              <BoughtMark
                bought={bought.get(`phone:${phone.id}`)}
                subject={phone.phone}
              />
            </>
          ),
        }),
        field("linkedin", t("contact.page.linkedin"), linkedin !== "", {
          keys: ["social.linkedin"],
          // The label claims the destination is LinkedIn, and a crawl or a
          // connector writes this value, so another host gets no link.
          link: Boolean(linkedinHref),
          text: linkedin,
          reading: linkedinHref ? (
            <OffsiteLink href={linkedin}>
              {linkedinHref.pathname.replace(/^\/|\/$/g, "")}
            </OffsiteLink>
          ) : (
            linkedin
          ),
        }),
        field("title", t("create.contactTitle"), Boolean(contact.title), {
          keys: ["title"],
          text: contact.title ?? "",
          reading: contact.title,
          beside: (
            <BoughtMark
              bought={bought.get("title")}
              subject={contact.title ?? ""}
            />
          ),
        }),
        field("address", t("co.address.summary"), address !== "", {
          keys: ADDRESS_FIELDS.map((entry) => entry.key),
          reading: address,
          beside: contact.address_country_mismatch && (
            <>
              {" "}
              <AddressCountryMismatchBadge
                mismatch={contact.address_country_mismatch}
              />
            </>
          ),
        }),
      ],
    },
    {
      key: "ownership",
      label: t("record.details.ownership"),
      rows: [
        field("owner", t("contact.page.owner"), Boolean(contact.owner_id), {
          keys: ["owner_id"],
          // An owner the roster no longer names is still the owner, and the
          // warning ink says the record needs a new one.
          reading:
            owner === t("ref.notInRoster") ? (
              <span className="pe-detail-warn">
                {t("contact.aside.ownerGone")}
              </span>
            ) : (
              owner
            ),
        }),
      ],
    },
  ];
}

function employerValue(
  view: Contact360,
  employer: Employment | undefined,
  proposed: ProfileField | undefined,
  notSet: string,
): ReactNode {
  if (employer) return <EmployerCard employment={employer} />;
  if (proposed) return <ProposedEmployerRow view={view} fact={proposed} />;
  return notSet;
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
  createdBy,
}: Readonly<{
  contact: Contact;
  profiles?: readonly Profile[];
  createdBy?: Contact360["created_by"];
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
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
              <ContactSource contact={contact} createdBy={createdBy} />
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
