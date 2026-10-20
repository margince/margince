import type { components } from "../../api/schema";
import { RecordFields, rawRecord } from "../../components/recordfields";
import { ContactLink } from "../../design-system/contactlink";
import { FieldRow } from "../../design-system/fieldgrid";
import {
  EMAIL_TYPE_LABEL,
  PHONE_TYPE_LABEL,
} from "../../format/contactchannels";
import { useT } from "../../i18n";
import { useCanWriteRecord } from "../../lib/capability";
import { ADDRESS_FIELDS, addressFrom } from "../companies/companyform";
import { TagsPanel } from "../lists/tagspanel";
import { RecordCustomFields } from "../records/recordcustomfields";
import { saveRecordEdit } from "../records/recordedit";
import { useRecordOwners } from "../records/recordreferences";
import { BoughtMark, boughtFields } from "./boughtmarks";
import { BoughtAttributes } from "./contactboughtattributes";
import { contactEditFields, mapContactUpdate } from "./contactformfields";

type Contact = components["schemas"]["Contact"];
type Profile = components["schemas"]["ContactProviderProfile"];
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
                archived={Boolean(contact.archived_at)}
                canEdit={canEdit}
                employer={contact.employer}
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
