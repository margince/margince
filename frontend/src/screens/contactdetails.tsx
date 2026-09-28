import type { components } from "../api/schema";
import { useCanWriteRecord } from "../app/capability";
import { ContactLink } from "../design-system/contactlink";
import { FieldRow } from "../design-system/fieldgrid";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { BoughtMark, boughtFields } from "./boughtmarks";
import { ADDRESS_FIELDS, addressFrom } from "./companyform";
import { BoughtAttributes } from "./contactboughtattributes";
import { contactEditFields, mapContactUpdate } from "./contactformfields";
import { RecordCustomFields } from "./recordcustomfields";
import { saveRecordEdit } from "./recordedit";
import { RecordFields, rawRecord } from "./recordfields";
import { useRecordOwners } from "./recordreferences";
import { TagsPanel } from "./tagspanel";

type Contact = components["schemas"]["Contact"];
type Profile = components["schemas"]["ContactProviderProfile"];

// The kind of each handle, in the words the edit form's own Type option
// uses, so the card and the form never name one kind two ways.
const EMAIL_TYPE_LABEL: Readonly<Record<string, MessageKey>> = {
  work: "field.emailWork",
  personal: "field.emailPersonal",
  other: "field.emailOther",
};
const PHONE_TYPE_LABEL: Readonly<Record<string, MessageKey>> = {
  work: "field.phoneWork",
  mobile: "field.phoneMobile",
  home: "field.phoneHome",
  other: "field.phoneOther",
};
export function ContactDetails({
  contact,
  profiles,
}: Readonly<{ contact: Contact; profiles?: readonly Profile[] }>) {
  const t = useT();
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
                  <ContactLink kind="phone" value={phone.phone} />
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
        links={
          linkedin
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
            <FieldRow label={t("tags.panelTitle")} align="top">
              <TagsPanel
                entityType="contact"
                entityID={contact.id}
                canEdit={canEdit}
                bare
              />
            </FieldRow>
          </>
        }
        fields={[
          ...contactEditFields(t),
          ...ADDRESS_FIELDS,
          {
            // Still reads visibility, and does not offer it: an owner-private
            // contact must name an owner, so the rule the header's toggle
            // obeys is the rule this field states. The toggle is the one place
            // the value CHANGES — a second control for it here was the same
            // fact in two shapes, one of which said nothing about what the
            // change would do.
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
