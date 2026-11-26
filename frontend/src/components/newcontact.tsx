// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { unwrap } from "../lib/problem";
import { useAddedNotice } from "./addednotice";
import {
  contactCompanyField,
  contactCreateFields,
  type HomeCompany,
  mapContactBody,
  refreshCompaniesAfterContact,
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
// The reader stays on the company to add the next one.
export function NewContactAction({
  company,
  listCreate,
  refusalId,
}: Readonly<{
  company?: HomeCompany;
  listCreate?: Readonly<{ open: boolean }>;
  // See CreateAction: the page's one sentence saying why.
  refusalId?: string;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const added = useAddedNotice();
  const cf = useObjectCustomFields("contact", "create");
  const fields = contactCreateFields(t).filter(
    (field) => company === undefined || field.key !== contactCompanyField.key,
  );
  return (
    <CreateAction
      label={t("create.contact")}
      requires={{ object: "contact", action: "create" }}
      refusalId={refusalId}
      invalidate={company?.listKey ?? "contacts"}
      screen="contacts"
      stay={company !== undefined}
      listCreate={listCreate}
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
      onCreated={(contact) => {
        void refreshCompaniesAfterContact(queryClient);
        if (company)
          added(contact.full_name, { screen: "contacts", id: contact.id });
      }}
      resolveExisting={(_code, id) => ({ screen: "contacts", id })}
      refusalNotice={leadHeldNoticeFor}
      fields={[...fields, ...cf.formFields]}
    />
  );
}
