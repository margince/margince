// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

type NamedContact = Readonly<{
  full_name: string;
  first_name?: string | null;
}>;

/**
 * The word a contact page uses for the contact: the stored first name, else the
 * whole display name. The first word of a display name is the family name for a
 * family-name-first writer, so it is never used as the name.
 */
export function contactFirstName(contact: NamedContact): string {
  const first = contact.first_name?.trim();
  return first || contact.full_name;
}
