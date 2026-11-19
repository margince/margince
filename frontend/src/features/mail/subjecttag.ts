// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";

// The project key a mail subject carries, so a customer's reply files itself
// under the project. Capture reads `[KEY]` from an inbound subject and matches
// it against live project keys; this writes the same shape on the way out.

type Project = components["schemas"]["Project"];

// A project with no key gets no tag. An archived project's key may belong to a
// newer live project by now, so writing it would file the reply there.
export function subjectTag(project: Project | null): string {
  if (!project?.key || project.archived_at) {
    return "";
  }
  return `[${project.key}]`;
}

// `subject` with `tag` once, at the front. It runs on every edit, so a second
// call must not stack the tag. The rest of the subject is the rep's typing and
// comes back unchanged.
export function withSubjectTag(subject: string, tag: string): string {
  if (!tag) {
    return subject;
  }
  return prefixed(stripEveryKeyTag(subject), tag);
}

// `subject` with every key-shaped `[...]` group removed, wherever it sits. The
// matcher reads every group, and two live keys file the reply under neither.
// A group that is not key-shaped, such as `[FYI]`, is prose and stays.
export function stripEveryKeyTag(subject: string): string {
  let out = "";
  let cursor = 0;
  for (const group of bracketedGroups(subject)) {
    // A removed tag takes one adjacent space with it, the separator this module
    // writes. Any other spacing is the rep's and stays as typed.
    const leading = group.start > cursor && subject[group.start - 1] === " ";
    const trailing = subject[group.end] === " ";
    const from = leading ? group.start - 1 : group.start;
    out += subject.slice(cursor, from);
    cursor = trailing ? group.end + 1 : group.end;
    if (!keyShaped(group.inner)) {
      out += subject.slice(from, cursor);
      continue;
    }
    // A tag between two words leaves one space; a tag at either end leaves none.
    if (leading && trailing) {
      out += " ";
    }
  }
  return out + subject.slice(cursor);
}

/** One `[...]` group: where it sits in the subject, and what it holds. */
type BracketGroup = Readonly<{
  /** Index of the `[`. */
  start: number;
  /** Index just past the `]`. */
  end: number;
  /** The text between the brackets, untrimmed. */
  inner: string;
}>;

// Every `[...]` group, read as capture's tokenizer reads it: each `[` to the
// next `]`, and an unclosed `[` ends the scan. One pass of `indexOf`, because a
// regular expression here costs quadratic time on a hostile subject.
function bracketedGroups(subject: string): BracketGroup[] {
  const groups: BracketGroup[] = [];
  let open = subject.indexOf("[");
  while (open >= 0) {
    const shut = subject.indexOf("]", open + 1);
    if (shut < 0) {
      return groups;
    }
    groups.push({
      start: open,
      end: shut + 1,
      inner: subject.slice(open + 1, shut),
    });
    open = subject.indexOf("[", shut + 1);
  }
  return groups;
}

// `subject` without a leading `tag` and the one space written after it. A
// second space is the rep's own and stays.
export function stripSubjectTag(subject: string, tag: string): string {
  if (!tag || !subject.startsWith(tag)) {
    return subject;
  }
  const rest = subject.slice(tag.length);
  return rest.startsWith(" ") ? rest.slice(1) : rest;
}

// `body` already lacks the tag, so its leading spaces are the rep's and stay.
function prefixed(body: string, tag: string): string {
  return body === "" ? tag : `${tag} ${body}`;
}

// Whether a bracketed token could be a project key: letter first, then letters,
// digits, underscores and hyphens. A bare number such as `[2026]` is prose.
function keyShaped(token: string): boolean {
  // The pattern projectkey.CheckPattern states; a backend gate holds the two equal.
  return /^[A-Za-z][A-Za-z0-9_-]{1,23}$/.test(token.trim());
}
