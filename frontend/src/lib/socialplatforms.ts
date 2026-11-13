// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The keys a contact's `social` map takes. The server refuses any other key, so
// this is a declared mirror of the backend's socialplatform.Keys(), and
// backend/gates/frontendsocialplatforms_test.go fails when the two differ.
export const SOCIAL_PLATFORMS = ["github", "linkedin", "twitter"] as const;

// The listed links of a stored social map. A key written before the list
// existed stays on the record and is never sent back, because the server
// refuses it on a write.
export function listedSocial(social: unknown): Record<string, unknown> {
  if (!social || typeof social !== "object" || Array.isArray(social)) {
    return {};
  }
  const listed: readonly string[] = SOCIAL_PLATFORMS;
  return Object.fromEntries(
    Object.entries(social).filter(([key]) => listed.includes(key)),
  );
}
