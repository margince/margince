// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The pure, non-JSX half of the contact-methods editor — row shaping, staging
// and the small edits every row list makes — split out of personcontactedit.tsx
// to keep that file under the fe-file-length ceiling. Nothing here touches the
// DOM, so it is exercised the same way from the component and from a unit test
// without either standing up React.

import type { components } from "../api/schema";
import type { SelectOption } from "../design-system/select";
import type { useT } from "../i18n";

type Person = components["schemas"]["Person"];
type PersonEmailInput = components["schemas"]["PersonEmailInput"];
type PersonPhoneInput = components["schemas"]["PersonPhoneInput"];
export type EmailType = PersonEmailInput["email_type"];
export type PhoneType = PersonPhoneInput["phone_type"];

// A staged row carries a stable client-side KEY for React's list identity —
// never the value, which changes on every keystroke, and never the array
// index, which shifts under a row the moment an earlier one is removed. An
// existing row's server id is already stable and unique, so it doubles as the
// key; a freshly appended row has no id yet and takes a fresh one of its own.
export type StagedEmail = PersonEmailInput & Readonly<{ key: string }>;
export type StagedPhone = PersonPhoneInput & Readonly<{ key: string }>;

export function seedEmails(person: Person): StagedEmail[] {
  return (person.emails ?? []).map((row) => ({
    key: row.id,
    email: row.email,
    email_type: row.email_type,
    is_primary: row.is_primary,
    position: row.position,
  }));
}

export function seedPhones(person: Person): StagedPhone[] {
  return (person.phones ?? []).map((row) => ({
    key: row.id,
    phone: row.phone,
    phone_type: row.phone_type,
    is_primary: row.is_primary,
    position: row.position,
  }));
}

// The small edits every row list makes, shared between emails and phones so
// the two do not grow two answers to one question.
export function replaceRow<TRow extends Readonly<{ key: string }>>(
  rows: readonly TRow[],
  key: string,
  patch: Partial<TRow>,
): TRow[] {
  return rows.map((row) => (row.key === key ? { ...row, ...patch } : row));
}

export function removeRow<TRow extends Readonly<{ key: string }>>(
  rows: readonly TRow[],
  key: string,
): TRow[] {
  return rows.filter((row) => row.key !== key);
}

// Swaps a row with its neighbour in the given direction; a no-op at either
// end of the list rather than wrapping, because "move up" on the first row
// wrapping to the bottom would surprise a reader far more than doing nothing.
export function moveRow<TRow extends Readonly<{ key: string }>>(
  rows: readonly TRow[],
  key: string,
  direction: "up" | "down",
): TRow[] {
  const index = rows.findIndex((row) => row.key === key);
  if (index === -1) {
    return [...rows];
  }
  const target = direction === "up" ? index - 1 : index + 1;
  if (target < 0 || target >= rows.length) {
    return [...rows];
  }
  const next = [...rows];
  [next[index], next[target]] = [next[target], next[index]];
  return next;
}

// Marks one row primary and clears every SIBLING OF THE SAME TYPE — the
// server enforces at most one primary per type (DB-enforced), so a picker
// that let two "work" rows both claim primary would stage a state the save
// could never actually produce.
export function selectPrimary<
  TRow extends Readonly<{ key: string; is_primary: boolean }>,
>(rows: readonly TRow[], key: string, kindOf: (row: TRow) => string): TRow[] {
  const target = rows.find((row) => row.key === key);
  if (!target) {
    return [...rows];
  }
  const kind = kindOf(target);
  return rows.map((row) =>
    kindOf(row) === kind ? { ...row, is_primary: row.key === key } : row,
  );
}

export function isEmailType(value: string): value is EmailType {
  return value === "work" || value === "personal" || value === "other";
}

export function isPhoneType(value: string): value is PhoneType {
  return (
    value === "work" ||
    value === "mobile" ||
    value === "home" ||
    value === "other"
  );
}

export function emailTypeOptions(t: ReturnType<typeof useT>): SelectOption[] {
  return [
    { value: "work", label: t("person.contactType.work") },
    { value: "personal", label: t("person.contactType.personal") },
    { value: "other", label: t("person.contactType.other") },
  ];
}

export function phoneTypeOptions(t: ReturnType<typeof useT>): SelectOption[] {
  return [
    { value: "work", label: t("person.contactType.work") },
    { value: "mobile", label: t("person.contactType.mobile") },
    { value: "home", label: t("person.contactType.home") },
    { value: "other", label: t("person.contactType.other") },
  ];
}

export function toEmailInputs(
  rows: readonly StagedEmail[],
): PersonEmailInput[] {
  return rows
    .filter((row) => row.email.trim() !== "")
    .map((row, index) => ({
      email: row.email.trim(),
      email_type: row.email_type,
      is_primary: row.is_primary,
      position: index,
    }));
}

export function toPhoneInputs(
  rows: readonly StagedPhone[],
): PersonPhoneInput[] {
  return rows
    .filter((row) => row.phone.trim() !== "")
    .map((row, index) => ({
      phone: row.phone.trim(),
      phone_type: row.phone_type,
      is_primary: row.is_primary,
      position: index,
    }));
}

export function blankEmail(position: number): StagedEmail {
  return {
    key: crypto.randomUUID(),
    email: "",
    email_type: "work",
    is_primary: false,
    position,
  };
}

export function blankPhone(position: number): StagedPhone {
  return {
    key: crypto.randomUUID(),
    phone: "",
    phone_type: "work",
    is_primary: false,
    position,
  };
}
