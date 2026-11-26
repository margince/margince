// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

type NameableLead = Readonly<{
  full_name?: string | null;
  email?: string | null;
}>;

// Mirrors the server's leadIdentityName so blank names use the trimmed address.
// Callers choose their own fallback when neither field names the lead.
export function leadIdentityName(lead: NameableLead): string {
  const name = lead.full_name?.trim();
  if (name) {
    return name;
  }
  return lead.email?.trim() ?? "";
}
