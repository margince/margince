import { EMAIL_TYPE_LABEL, PHONE_TYPE_LABEL } from "../format/contactchannels";
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { api } from "../api/client";
import type { components } from "../api/schema";
import type { useT } from "../i18n";
import { unwrap } from "../lib/problem";
import { addressPatch } from "./companyform";
import type { CreateField, FormRows } from "./create";
import type { NameOffer } from "./create.offered";
import { searchCompanies } from "./filterreference";
import { normalizeProfileUrl } from "./profileurl";

// Creation and Details share the field definitions and collection mappers.
// The update mapper sends only fields submitted by the active editor.

type CreateContactRequest = components["schemas"]["CreateContactRequest"];
type UpdateContactRequest = components["schemas"]["UpdateContactRequest"];

function asEmailType(value: string | undefined): "work" | "personal" | "other" {
  return value === "personal" || value === "other" ? value : "work";
}

function asPhoneType(
  value: string | undefined,
): "work" | "mobile" | "home" | "other" {
  return value === "mobile" || value === "home" || value === "other"
    ? value
    : "work";
}

// The email rows a writer supplied, blank ones dropped and position taken from
// where each one sits in the list.
//
// ONE MAPPER FOR BOTH REQUESTS, because `ContactEmailInput` is one schema for
// both: create and update describing an address differently is exactly what
// that shared schema exists to prevent.
function contactEmailInputs(rows: FormRows) {
  return (rows.emails ?? [])
    .filter((row) => (row.email ?? "").trim().length > 0)
    .map((row, index) => ({
      email: (row.email ?? "").trim(),
      email_type: asEmailType(row.email_type),
      is_primary: row.is_primary === "true",
      position: index,
    }));
}

// The phone rows, the same way and for the same reason.
function contactPhoneInputs(rows: FormRows) {
  return (rows.phones ?? [])
    .filter((row) => (row.phone ?? "").trim().length > 0)
    .map((row, index) => ({
      phone: (row.phone ?? "").trim(),
      phone_type: asPhoneType(row.phone_type),
      is_primary: row.is_primary === "true",
      position: index,
    }));
}

// The domain of the first work address typed, the primary one first. Which
// domains name a company is the server's answer, so free mail is not judged here.
export function workEmailDomain(rows: FormRows): string | undefined {
  const work = (rows.emails ?? []).filter(
    (row) => asEmailType(row.email_type) === "work",
  );
  const [first] = [...work].sort(
    (a, b) => Number(b.is_primary === "true") - Number(a.is_primary === "true"),
  );
  const domain = /^[^@\s]+@([^@\s]+\.[^@\s]+)$/.exec(
    (first?.email ?? "").trim(),
  )?.[1];
  return domain?.toLowerCase();
}

async function companyOnDomain(domain: string): Promise<readonly NameOffer[]> {
  const data = unwrap(
    await api.GET("/companies", { params: { query: { domain, limit: 1 } } }),
  );
  return data.data.map((company) => ({
    value: company.id,
    label: company.display_name,
  }));
}

// The employer box of every contact create: a pick sends `company_id`, a typed
// name creates a company, and an untouched box takes a work email's company.
export const contactCompanyField: CreateField = {
  key: "company_name",
  label: "create.companyName",
  offers: {
    search: searchCompanies,
    pickedKey: "company_id",
    pickedHint: "create.companyPicked",
    newHint: "create.companyNew",
    prefill: {
      from: workEmailDomain,
      find: companyOnDomain,
      hint: "create.companyPrefilled",
    },
  },
};

// The company whose page a create runs on, which files the contact under it.
export type HomeCompany = Readonly<{
  id: string;
  // The list on the company page that shows the new contact.
  listKey: string;
}>;

// The contract refuses both together, so the request says what the reader chose.
export function companyChoice(values: Record<string, string>) {
  const companyId = values.company_id?.trim();
  return companyId
    ? { company_id: companyId }
    : { company_name: values.company_name?.trim() || undefined };
}

// Builds the create-contact request body: scalar fields trim to undefined
// when blank (never sent rather than sent empty), `social.linkedin` folds
// into the `social` object, and each repeatable row becomes an
// emails/phones entry keyed by its position in the list.
export function mapContactBody(
  values: Record<string, string>,
  rows: FormRows,
): CreateContactRequest {
  const linkedin = values["social.linkedin"]?.trim();
  const emails = contactEmailInputs(rows);
  const phones = contactPhoneInputs(rows);
  return {
    full_name: (values.full_name ?? "").trim(),
    first_name: values.first_name?.trim() || undefined,
    last_name: values.last_name?.trim() || undefined,
    title: values.title?.trim() || undefined,
    social: linkedin ? { linkedin } : undefined,
    emails: emails.length > 0 ? emails : undefined,
    phones: phones.length > 0 ? phones : undefined,
    ...companyChoice(values),
    source: "manual",
  };
}

function stringField(value: unknown): string {
  return typeof value === "string" ? value : "";
}

// Builds the PATCH body.
//
// `emails` and `phones` REPLACE their sets rather than adding to them, which is
// what a correction needs: a bounced address is fixed by sending the set that
// should stand, and an append-only field could never remove the one that is
// dead. So an empty list is SENT, not omitted — a contact whose last address
// was wrong and is now gone is a real answer, and omitting it would silently
// keep the address the reader just deleted.
//
// `rows` is absent only where a caller has no repeatable fields at all; the
// contact form always has both, so the sets go out on every save.
export function mapContactUpdate(
  values: Record<string, unknown>,
  rows?: FormRows,
  opened: Record<string, unknown> = {},
): UpdateContactRequest {
  const scalar = (key: string) =>
    Object.hasOwn(values, key)
      ? stringField(values[key]).trim() || null
      : undefined;
  const social = opened.social;
  return {
    full_name: stringField(values.full_name).trim() || undefined,
    first_name: scalar("first_name"),
    last_name: scalar("last_name"),
    title: scalar("title"),
    owner_id: scalar("owner_id"),
    visibility:
      values.visibility === "workspace" || values.visibility === "owner"
        ? values.visibility
        : undefined,
    social: Object.hasOwn(values, "social.linkedin")
      ? {
          ...(social && typeof social === "object" && !Array.isArray(social)
            ? social
            : {}),
          linkedin: scalar("social.linkedin")
            ? normalizeProfileUrl(stringField(values["social.linkedin"]))
            : null,
        }
      : undefined,
    address: addressPatch(values),
    emails:
      rows && Object.hasOwn(rows, "emails")
        ? contactEmailInputs(rows)
        : undefined,
    phones:
      rows && Object.hasOwn(rows, "phones")
        ? contactPhoneInputs(rows)
        : undefined,
  };
}

// Takes t rather than resolving its own strings, because the email/phone
// "Type" options are display text. fieldControl (create.controls.tsx) renders
// option.label as given, so the readable string is resolved via useT() before
// it reaches CreateField. companies.tsx's size_band options differ: they are
// raw labels already fit to show ("1-10").
export function contactCreateFields(t: ReturnType<typeof useT>): CreateField[] {
  return [
    { key: "full_name", label: "create.fullName", required: true },
    { key: "first_name", label: "create.firstName" },
    { key: "last_name", label: "create.lastName" },
    { key: "title", label: "create.contactTitle" },
    contactCompanyField,
    { key: "social.linkedin", label: "create.linkedin" },
    {
      key: "emails",
      label: "create.email",
      type: "repeatable",
      addLabel: "field.addEmail",
      editLabel: "field.editEmails",
      rowFields: [
        {
          key: "email",
          label: "create.email",
          type: "email",
          required: true,
        },
        {
          key: "email_type",
          label: "field.emailType",
          type: "select",
          options: Object.entries(EMAIL_TYPE_LABEL).map(([value, label]) => ({
            value,
            label: t(label),
          })),
        },
      ],
      primaryKey: "is_primary",
      typeKey: "email_type",
      typeDefault: "work", // matches asEmailType's own fallback
    },
    {
      key: "phones",
      label: "create.phone",
      type: "repeatable",
      addLabel: "field.addPhone",
      editLabel: "field.editPhones",
      rowFields: [
        { key: "phone", label: "create.phone", required: true },
        {
          key: "phone_type",
          label: "field.phoneType",
          type: "select",
          options: Object.entries(PHONE_TYPE_LABEL).map(([value, label]) => ({
            value,
            label: t(label),
          })),
        },
      ],
      primaryKey: "is_primary",
      typeKey: "phone_type",
      typeDefault: "work", // matches asPhoneType's own fallback
    },
  ];
}

// The split name parts are taken once, for a mail merge's greeting. The
// employer is an edge, edited on the Company card.
const CREATE_ONLY = new Set([
  "first_name",
  "last_name",
  contactCompanyField.key,
]);

export function contactEditFields(t: ReturnType<typeof useT>): CreateField[] {
  return contactCreateFields(t).filter((field) => !CREATE_ONLY.has(field.key));
}

// Compare replace-sets in their request shape; row ids and server metadata
// are not editable values. Position comes from the displayed row order.
export function contactEditComparison(
  record: components["schemas"]["Contact"],
) {
  return {
    ...record,
    emails: (record.emails ?? []).map((row, position) => ({
      email: row.email,
      email_type: row.email_type ?? "work",
      is_primary: row.is_primary,
      position,
    })),
    phones: (record.phones ?? []).map((row, position) => ({
      phone: row.phone,
      phone_type: row.phone_type ?? "work",
      is_primary: row.is_primary,
      position,
    })),
  };
}
