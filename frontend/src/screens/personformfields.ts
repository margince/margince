// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { useT } from "../i18n";
import type { CreateField, FormRows } from "./create";

// The person form's field schema and its two request-body mappers — create
// and edit both describe an address the same way, because ONE mapper for
// each request is what stops the two coming to disagree about what a
// PersonEmailInput or a repeatable phone row means. Its own file so
// contacts.tsx's ContactsScreen (create) and personeditmergearchive.tsx's
// PersonEditMergeArchive (edit, shared by both PersonScreen and
// PersonPageV2) can each read it without importing one another.

type CreatePersonRequest = components["schemas"]["CreatePersonRequest"];
type UpdatePersonRequest = components["schemas"]["UpdatePersonRequest"];

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
// ONE MAPPER FOR BOTH REQUESTS, because `PersonEmailInput` is one schema for
// both: create and update describing an address differently is exactly what
// that shared schema exists to prevent.
function personEmailInputs(rows: FormRows) {
  return (rows.emails ?? [])
    .filter((row) => (row.email ?? "").trim().length > 0)
    .map((row, index) => ({
      email: row.email.trim(),
      email_type: asEmailType(row.email_type),
      is_primary: row.is_primary === "true",
      position: index,
    }));
}

// The phone rows, the same way and for the same reason.
function personPhoneInputs(rows: FormRows) {
  return (rows.phones ?? [])
    .filter((row) => (row.phone ?? "").trim().length > 0)
    .map((row, index) => ({
      phone: row.phone.trim(),
      phone_type: asPhoneType(row.phone_type),
      is_primary: row.is_primary === "true",
      position: index,
    }));
}

// Builds the create-contact request body: scalar fields trim to undefined
// when blank (never sent rather than sent empty), `social.linkedin` folds
// into the `social` object, and each repeatable row becomes an
// emails/phones entry keyed by its position in the list.
export function mapPersonBody(
  values: Record<string, string>,
  rows: FormRows,
): CreatePersonRequest {
  const linkedin = values["social.linkedin"]?.trim();
  const emails = personEmailInputs(rows);
  const phones = personPhoneInputs(rows);
  return {
    full_name: values.full_name.trim(),
    first_name: values.first_name?.trim() || undefined,
    last_name: values.last_name?.trim() || undefined,
    title: values.title?.trim() || undefined,
    social: linkedin ? { linkedin } : undefined,
    emails: emails.length > 0 ? emails : undefined,
    phones: phones.length > 0 ? phones : undefined,
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
// person form always has both, so the sets go out on every save.
export function mapPersonUpdate(
  values: Record<string, unknown>,
  rows?: FormRows,
): UpdatePersonRequest {
  const linkedin = stringField(values["social.linkedin"]).trim();
  return {
    full_name: stringField(values.full_name).trim() || undefined,
    first_name: stringField(values.first_name).trim() || undefined,
    last_name: stringField(values.last_name).trim() || undefined,
    title: stringField(values.title).trim() || undefined,
    social: linkedin ? { linkedin } : undefined,
    emails: rows ? personEmailInputs(rows) : undefined,
    phones: rows ? personPhoneInputs(rows) : undefined,
  };
}

// Takes t rather than resolving its own strings because the email/phone
// "Type" options are display text, not raw values — fieldControl (create.tsx)
// renders option.label verbatim, so the human-readable string has to be
// resolved via useT() before it reaches CreateField, unlike companies.tsx's
// size_band options, which are already display-ready raw labels ("1-10").
export function contactCreateFields(t: ReturnType<typeof useT>): CreateField[] {
  return [
    { key: "full_name", label: "create.fullName", required: true },
    { key: "first_name", label: "create.firstName" },
    { key: "last_name", label: "create.lastName" },
    { key: "title", label: "create.personTitle" },
    { key: "social.linkedin", label: "create.linkedin" },
    {
      key: "emails",
      label: "create.email",
      type: "repeatable",
      addLabel: "field.addEmail",
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
          options: [
            { value: "work", label: t("field.emailWork") },
            { value: "personal", label: t("field.emailPersonal") },
            { value: "other", label: t("field.emailOther") },
          ],
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
      rowFields: [
        { key: "phone", label: "create.phone", required: true },
        {
          key: "phone_type",
          label: "field.phoneType",
          type: "select",
          options: [
            { value: "work", label: t("field.phoneWork") },
            { value: "mobile", label: t("field.phoneMobile") },
            { value: "home", label: t("field.phoneHome") },
            { value: "other", label: t("field.phoneOther") },
          ],
        },
      ],
      primaryKey: "is_primary",
      typeKey: "phone_type",
      typeDefault: "work", // matches asPhoneType's own fallback
    },
  ];
}

// The edit form is contactCreateFields WITHOUT the create-only parts, so the
// two cannot come to describe an address differently — the same reason one
// mapper serves both request bodies.
//
// It carries the email and phone rows because nothing else in the product can
// change them. A bounced send names the address that refused it and sends the
// reader here; a form that omitted the field left that reader at a page which
// reported the failure and could not fix it.
//
// Moving the primary marker between two addresses of the SAME type is refused
// by the server with a bare 409 today. Not a limit of this form and not
// introduced here — the same PATCH has answered that way since the field
// existed — but the primary radio is the first control that reaches it, so the
// conflict is shown rather than swallowed. Correcting an address, adding one
// and removing one all work.
export function personEditFields(t: ReturnType<typeof useT>): CreateField[] {
  return contactCreateFields(t);
}
