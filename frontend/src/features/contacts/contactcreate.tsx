import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  contactCreateFields,
  mapContactBody,
} from "../../components/contactformfields";
import { CreateAction, type FormRows } from "../../components/create";
import { useObjectCustomFields } from "../../components/customfields.form";
import { leadHeldNoticeFor } from "../../components/leadheldnotice";
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
  extra?: Record<string, unknown>,
): Promise<Contact> {
  return unwrap(
    await api.POST("/contacts", {
      params: { header: { "Idempotency-Key": idempotencyKey } },
      body: {
        ...mapContactBody(values, rows ?? {}),
        ...customFields,
        ...extra,
      },
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
        create={(values, rows, key, extra) =>
          createContact(values, rows, cf.toBody(values), t, key, extra)
        }
        resolveExisting={(_code, id) => ({ screen: "contacts", id })}
        refusalNotice={leadHeldNoticeFor}
        fields={[...contactCreateFields(t), ...cf.formFields]}
      />
    </>
  );
}
