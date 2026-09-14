// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { normalizeProfileUrl } from "../format/profileurl";
import type { useT } from "../i18n";
import { addressPatch } from "./companyform";
import type { CreateField, FormRows } from "./create";

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
      email: row.email.trim(),
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
export function mapContactBody(
  values: Record<string, string>,
  rows: FormRows,
): CreateContactRequest {
  const linkedin = values["social.linkedin"]?.trim();
  const emails = contactEmailInputs(rows);
  const phones = contactPhoneInputs(rows);
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
    { key: "title", label: "create.contactTitle" },
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

// Creation and Details share the same address types and primary semantics.
export function contactEditFields(t: ReturnType<typeof useT>): CreateField[] {
  return contactCreateFields(t);
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
