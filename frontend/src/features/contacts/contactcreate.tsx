import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  contactCreateFields,
  mapContactBody,
} from "../../components/contactformfields";
import { CreateAction, type FormRows } from "../../components/create";
import { useObjectCustomFields } from "../../components/customfields.form";
import { QuickCaptureAction } from "../../components/quickcapture";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { unwrap } from "../../lib/problem";

type Contact = components["schemas"]["Contact"];

async function createContact(
  values: Record<string, string>,
  rows: FormRows | undefined,
  customFields: Record<string, unknown>,
  t: (key: MessageKey) => string,
  idempotencyKey: string,
): Promise<Contact> {
  return unwrap(
    await api.POST("/contacts", {
      params: { header: { "Idempotency-Key": idempotencyKey } },
      body: { ...mapContactBody(values, rows ?? {}), ...customFields },
    }),
    t,
  );
}

// The Contacts list's typed creates; the list adds the vCard import after them.
export function ContactCreateActions() {
  const t = useT();
  const cf = useObjectCustomFields("contact", "create");
  return (
    <>
      <QuickCaptureAction />
      <CreateAction
        label={t("create.contact")}
        invalidate="contacts"
        screen="contacts"
        create={(values, rows, key) =>
          createContact(values, rows, cf.toBody(values), t, key)
        }
        resolveExisting={(_code, id) => ({ screen: "contacts", id })}
        fields={[...contactCreateFields(t), ...cf.formFields]}
      />
    </>
  );
}
