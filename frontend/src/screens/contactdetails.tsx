import type { components } from "../api/schema";
import { useCanWriteRecord } from "../app/capability";
import { ContactLink } from "../design-system/contactlink";
import { FieldRow } from "../design-system/fieldgrid";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useSorMode } from "./common";
import { ADDRESS_FIELDS, addressFrom } from "./companyform";
import { contactEditFields, mapContactUpdate } from "./contactformfields";
import { RecordCustomFields } from "./recordcustomfields";
import { saveRecordEdit } from "./recordedit";
import { RecordFields, rawRecord } from "./recordfields";
import { useRecordOwners } from "./recordreferences";
import { TagsPanel } from "./tagspanel";

type Contact = components["schemas"]["Contact"];

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
export function ContactDetails({ contact }: Readonly<{ contact: Contact }>) {
  const t = useT();
  const canEdit = useCanWriteRecord("contact", contact) && !contact.archived_at;
  const owners = useRecordOwners(contact.owner_id);
  const overlay = useSorMode() === "overlay";
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
                </span>
              ))
            : undefined,
        }}
        links={
          typeof contact.social?.linkedin === "string" &&
          contact.social?.linkedin
            ? {
                "social.linkedin": {
                  href: contact.social?.linkedin,
                  label: t("record.openProfile"),
                },
              }
            : undefined
        }
        canEdit={canEdit}
        // Tags as one more row of the card: filing beside the facts it files.
        extraRows={
          <FieldRow label={t("tags.panelTitle")} align="top">
            <TagsPanel
              entityType="contact"
              entityID={contact.id}
              canEdit={
                useCanWriteRecord("contact", contact) && !contact.archived_at
              }
              bare
            />
          </FieldRow>
        }
        notice={overlay ? t("overlay.partialWriteBack") : undefined}
        fields={[
          ...contactEditFields(t),
          ...ADDRESS_FIELDS,
          {
            key: "owner_id",
            required: contact.visibility === "owner",
            label: "list.owner",
            type: "select",
            options: owners,
          },
          {
            key: "visibility",
            label: "history.field.visibility",
            type: "select",
            required: true,
            // The same words the header's VisibilityBadge says for the same
            // states, so the rail and the head never name one fact two ways.
            options: [
              { value: "workspace", label: t("visibility.team") },
              { value: "owner", label: t("visibility.private") },
            ],
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
          "social.linkedin":
            typeof contact.social?.linkedin === "string"
              ? contact.social.linkedin
              : "",
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
