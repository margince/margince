import type { components } from "../api/schema";
import { useCanWriteRecord } from "../app/capability";
import { ContactLink } from "../design-system/contactlink";
import { useT } from "../i18n";
import { useSorMode } from "./common";
import { ADDRESS_FIELDS, addressFrom } from "./companyform";
import { contactEditFields, mapContactUpdate } from "./contactformfields";
import { RecordCustomFields } from "./recordcustomfields";
import { saveRecordEdit } from "./recordedit";
import { RecordFields, rawRecord } from "./recordfields";
import { useRecordOwners } from "./recordreferences";

type Contact = components["schemas"]["Contact"];
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
          emails: contact.emails?.length
            ? contact.emails.map((email) => (
                <ContactLink
                  key={email.email}
                  kind="email"
                  value={email.email}
                  record={{ entityType: "contact", entityId: contact.id }}
                  readOnly={Boolean(contact.archived_at)}
                />
              ))
            : undefined,
          phones: contact.phones?.length
            ? contact.phones.map((phone) => (
                <ContactLink
                  key={phone.phone}
                  kind="phone"
                  value={phone.phone}
                />
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
            options: [
              { value: "workspace", label: t("visibility.team") },
              { value: "owner", label: t("record.visibilityOwner") },
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
