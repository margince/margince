// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { stripEveryKeyTag } from "../projects/projectrecord";
import type { MailDraft, SavedDraftFields } from "./composesaveddraftcalls";
import { fieldsOfDraft, sameFields } from "./composesaveddraftfields";
import { replySubject } from "./replysubject";

// The saved draft's rules that draw nothing: what counts as written, what a
// close does, and when the draft has had its chance. The hook is composesaveddraft.tsx.

type Activity = components["schemas"]["Activity"];

// Whether the reader put anything in the composer it did not put there itself.
// The thread's counterparty, the reply subject and a caller's opening message
// (a forward's quote) are the composer's own defaults. A composer opened and
// closed untouched saves nothing. A file, a reason or a send time is the
// reader's choice, and is kept like a word.
export function wroteSomething(
  fields: SavedDraftFields,
  defaults: Readonly<{
    recipient?: string;
    subject: string;
    body: string;
    intent?: string;
  }>,
): boolean {
  const subject = stripEveryKeyTag(fields.subject).trim();
  return (
    fields.body.trim() !== defaults.body.trim() ||
    (fields.editorState?.intent ?? "").trim() !==
      (defaults.intent ?? "").trim() ||
    fields.cc.length > 0 ||
    fields.bcc.length > 0 ||
    fields.to.some((address) => address !== defaults.recipient) ||
    (subject !== "" && subject !== defaults.subject.trim()) ||
    fields.files.length > 0 ||
    fields.context !== "" ||
    fields.sendAt !== ""
  );
}

// A file the save left out stays on screen as the reader chose it, so this
// composer's own save is measured against what it sent.
export function unsavedWords(
  state: Readonly<{
    fields: SavedDraftFields;
    held: MailDraft | null;
    save: Readonly<{
      data: MailDraft | undefined;
      variables: Readonly<{ fields: SavedDraftFields }> | undefined;
    }>;
    defaults: Parameters<typeof wroteSomething>[1];
  }>,
): boolean {
  const { held, save } = state;
  if (!held) return wroteSomething(state.fields, state.defaults);
  const ownSave =
    save.data?.id === held.id && save.data.version === held.version;
  const sent = ownSave ? save.variables?.fields : undefined;
  return !sameFields(state.fields, sent ?? fieldsOfDraft(held));
}

// What a close does: waits for a save out, discards only the very words a
// refused close tried to keep, and keeps anything newer first.
export function closeStep(
  state: Readonly<{
    save: Readonly<{
      isPending: boolean;
      variables: Readonly<{ fields: SavedDraftFields }> | undefined;
    }>;
    refusedClose: boolean;
    fields: SavedDraftFields;
    unsaved: boolean;
  }>,
): "wait" | "discard" | "save" | "close" {
  if (state.save.isPending) return "wait";
  const asRefused =
    JSON.stringify(state.save.variables?.fields) ===
    JSON.stringify(state.fields);
  if (state.refusedClose && asRefused) return "discard";
  return state.unsaved ? "save" : "close";
}

// The saved draft has had its chance: read, and back on screen when there was
// one. Until then an empty body is not yet the reader's last word.
export function savedDraftSettled(
  state: Readonly<{
    anchored: boolean;
    read: Readonly<{
      isError: boolean;
      isSuccess: boolean;
      isFetching: boolean;
    }>;
    held: boolean;
    restored: boolean;
    // A read answering after the first keystroke restores nothing.
    typed: boolean;
  }>,
): boolean {
  const { read } = state;
  // A failed read leaves open whether a draft is held, so it never settles.
  if (!state.anchored) return true;
  if (!read.isSuccess || read.isFetching) return false;
  return !state.held || state.restored || state.typed;
}

// What the composer opens with: a draft is measured against it, and deleting
// one puts it back.
export function composerDefaults(input: {
  initialIntent?: string;
  initialMessage?: Readonly<{ subject: string; body: string }>;
  offeredRecipient: string | undefined;
  replyTo: Activity | undefined;
}) {
  const replying =
    input.replyTo?.kind === "email" &&
    input.replyTo.content_state !== "withheld";
  return {
    intent: input.initialIntent ?? "",
    recipient: input.offeredRecipient,
    subject: replying
      ? replySubject(input.replyTo?.subject ?? "")
      : (input.initialMessage?.subject ?? ""),
    body: input.initialMessage?.body ?? "",
  };
}
