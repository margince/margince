// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** The two fields that can name a lead. Structural rather than the contract's
 *  `Lead`, because the record-reference reader answers with the same pair from
 *  a different read and is naming the same lead. */
type NameableLead = Readonly<{
  full_name?: string | null;
  email?: string | null;
}>;

/**
 * What a lead is called: its own name if it has one, otherwise the email
 * address that is the only other thing naming it, and the empty string when it
 * has neither.
 *
 * `??` is not this rule. A `full_name` that is PRESENT and EMPTY is not a name,
 * and nothing between a `CreateLead` body and the stored row refuses one — so a
 * screen falling back on null alone renders such a lead blank, while the server
 * promotes the very same lead into a person named by its address.
 *
 * Both fields are trimmed, and for one reason: padding is not identity. The
 * server's address arrives through `values.ParseEmail`, which trims it there —
 * so trimming here is what keeps the two agreeing rather than a rule of our
 * own. A field that is nothing but padding names nobody, and answering it would
 * hand a caller a truthy string their own `|| fallback` then never reaches.
 *
 * This mirrors `leadIdentityName` in the people module, which is where the
 * server and its SQL answer the question. Return `""` and let the caller pick
 * its own last resort — a list row falls back to the id, a page heading to the
 * word "Leads" — because what to say about a lead nothing names is the
 * caller's to know.
 */
export function leadIdentityName(lead: NameableLead): string {
  const name = lead.full_name?.trim();
  if (name) {
    return name;
  }
  return lead.email?.trim() ?? "";
}
