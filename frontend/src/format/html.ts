// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

const ENTITIES: Readonly<Record<string, string>> = {
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&quot;",
};

/** Text made safe to place in markup, as element content or a quoted attribute. */
export function escapeHtml(text: string): string {
  return text.replace(
    /[&<>"]/g,
    (character) => ENTITIES[character] ?? character,
  );
}
