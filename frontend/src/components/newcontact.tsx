// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { api } from "../api/client";
import type { components } from "../api/schema";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { unwrap } from "../lib/problem";
import {
  contactCompanyField,
  contactCreateFields,
  type HomeCompany,
  mapContactBody,
} from "./contactformfields";
import { CreateAction, type FormRows } from "./create";
import { useObjectCustomFields } from "./customfields.form";
import { leadHeldNoticeFor } from "./leadheldnotice";

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

// The full contact form. On a company's page the company is already chosen,
// so its box is left out and the company goes as `company_id`.
export function NewContactAction({
  company,
}: Readonly<{ company?: HomeCompany }>) {
  const t = useT();
  const cf = useObjectCustomFields("contact", "create");
  const fields = contactCreateFields(t).filter(
    (field) => company === undefined || field.key !== contactCompanyField.key,
  );
  return (
    <CreateAction
      label={t("create.contact")}
      invalidate={company?.listKey ?? "contacts"}
      screen="contacts"
      create={(values, rows, key, extra) =>
        createContact(
          company ? { ...values, company_id: company.id } : values,
          rows,
          cf.toBody(values),
          t,
          key,
          extra,
        )
      }
      resolveExisting={(_code, id) => ({ screen: "contacts", id })}
      refusalNotice={leadHeldNoticeFor}
      fields={[...fields, ...cf.formFields]}
    />
  );
}
