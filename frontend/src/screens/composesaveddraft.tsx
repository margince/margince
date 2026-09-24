// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type QueryClient,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch } from "../api/version";
import { Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { paragraphsFrom } from "../design-system/richtext";
import { type Toast, useToast } from "../design-system/toast";
import { replySubject } from "../format/replysubject";
import { useT } from "../i18n";
import { problemCodeOf, problemMessageOf, throwProblem } from "./common";
import type { RelinkKind } from "./composerelink";
import { stripEveryKeyTag } from "./projectrecord";

// The rep's own unsent message, kept on the server so closing the composer does
// not lose it. Not the AI draft (`composedraftband.tsx`): that one is words a
// model served, this one is whatever the rep had written when they stepped away.

type Activity = components["schemas"]["Activity"];
type MailDraft = components["schemas"]["MailDraft"];
type MailDraftAnchorType = components["schemas"]["MailDraftAnchorType"];

export type SavedDraftAnchor = Readonly<{
  type: MailDraftAnchorType;
  id: string;
}>;

/** The composer's fields as a draft keeps them. */
export type SavedDraftFields = Readonly<{
  to: readonly string[];
  cc: readonly string[];
  bcc: readonly string[];
  subject: string;
  body: string;
  html: string;
}>;

// A reply keeps its draft on the message it answers, a new message on the
// record it was started from. A channel reply keeps none: the draft is a mail's
// fields, and a chat message has no subject or addressee to hold.
export function savedDraftAnchor(input: {
  answering: string | undefined;
  entityType: RelinkKind;
  entityId: string;
  isChannelReply: boolean;
}): SavedDraftAnchor | null {
  if (input.isChannelReply) return null;
  if (input.answering) return { type: "activity", id: input.answering };
  return input.entityId ? { type: input.entityType, id: input.entityId } : null;
}

function draftKey(anchor: SavedDraftAnchor | null) {
  return ["mail-draft", anchor?.type ?? "", anchor?.id ?? ""] as const;
}

// A 404 is the ordinary answer: no draft here, or an anchor this reader can no
// longer see, which the server reports as one thing on purpose.
async function readDraft(anchor: SavedDraftAnchor): Promise<MailDraft | null> {
  const { data, error, response } = await api.GET("/mail-drafts", {
    params: { query: { anchor_type: anchor.type, anchor_id: anchor.id } },
  });
  if (response.status === 404) return null;
  if (!response.ok) throwProblem(error || {});
  return isMailDraft(data) ? data : null;
}

// A 200 is read as a draft only when it has a draft's shape, so an answer that
// is not one (a proxy's page, a fixture's catch-all) restores nothing.
function isMailDraft(value: MailDraft | undefined): value is MailDraft {
  return (
    typeof value?.id === "string" &&
    typeof value.body === "string" &&
    typeof value.subject === "string" &&
    Array.isArray(value.to) &&
    Array.isArray(value.cc) &&
    Array.isArray(value.bcc)
  );
}

async function deleteDraft(id: string): Promise<void> {
  const { error, response } = await api.DELETE("/mail-drafts/{id}", {
    params: { path: { id } },
  });
  // Already gone is the outcome the reader asked for.
  if (!response.ok && response.status !== 404) {
    throwProblem(error || {});
  }
}

/** The words a saved draft puts back in the composer. */
export function fieldsOfDraft(draft: MailDraft): SavedDraftFields {
  return {
    to: draft.to,
    cc: draft.cc,
    bcc: draft.bcc,
    subject: draft.subject,
    body: draft.body,
    html: draft.html_body ?? paragraphsFrom(draft.body),
  };
}

function sameList(a: readonly string[], b: readonly string[]): boolean {
  return a.length === b.length && a.every((value, index) => value === b[index]);
}

function sameFields(a: SavedDraftFields, b: SavedDraftFields): boolean {
  return (
    sameList(a.to, b.to) &&
    sameList(a.cc, b.cc) &&
    sameList(a.bcc, b.bcc) &&
    a.subject === b.subject &&
    a.body === b.body &&
    a.html === b.html
  );
}

// Whether the reader wrote anything the composer did not put there itself. The
// thread's counterparty in To and the reply subject are the composer's own
// defaults, so a reply opened and closed untouched saves nothing.
export function wroteSomething(
  fields: SavedDraftFields,
  defaults: Readonly<{ recipient?: string; subject: string }>,
): boolean {
  const subject = stripEveryKeyTag(fields.subject).trim();
  return (
    fields.body.trim() !== "" ||
    fields.cc.length > 0 ||
    fields.bcc.length > 0 ||
    fields.to.some((address) => address !== defaults.recipient) ||
    (subject !== "" && subject !== defaults.subject.trim())
  );
}

type SaveAsk = Readonly<{
  anchor: SavedDraftAnchor;
  fields: SavedDraftFields;
  version: number | undefined;
}>;

async function writeDraft(ask: SaveAsk): Promise<MailDraft> {
  const { data, error, response } = await api.PUT("/mail-drafts", {
    // No version is a create: the server refuses it 409 when a draft already
    // exists here, so an unpinned write can never replace one unseen.
    params: { ...(ask.version === undefined ? {} : ifMatch(ask.version)) },
    body: {
      anchor_type: ask.anchor.type,
      anchor_id: ask.anchor.id,
      to: [...ask.fields.to],
      cc: [...ask.fields.cc],
      bcc: [...ask.fields.bcc],
      subject: ask.fields.subject,
      body: ask.fields.body,
      html_body: ask.fields.html.trim() === "" ? null : ask.fields.html,
    },
  });
  if (!response.ok || !data) {
    throwProblem(error || {});
  }
  return data;
}

// The toast's Delete outlives the composer it came from, so it is a plain call
// on the client rather than a mutation owned by a component that may be gone.
function deleteFromToast(input: {
  queryClient: QueryClient;
  toast: Toast;
  anchor: SavedDraftAnchor;
  id: string;
  t: ReturnType<typeof useT>;
  onDeleted: () => void;
}) {
  deleteDraft(input.id).then(
    () => {
      input.queryClient.setQueryData(draftKey(input.anchor), null);
      input.onDeleted();
      input.toast.show(input.t("compose.savedDraftDeleted"));
    },
    (error: unknown) => {
      input.toast.show(problemMessageOf(error, input.t), {
        tone: "danger",
        sticky: true,
      });
    },
  );
}

export type SavedDraft = ReturnType<typeof useSavedDraft>;

export function useSavedDraft(input: {
  where: Parameters<typeof savedDraftAnchor>[0];
  open: boolean;
  fields: SavedDraftFields;
  /** The message being answered, whose subject the composer offers. */
  replyTo: Activity | undefined;
  /** The address the composer offered into To on its own. */
  offeredRecipient: string | undefined;
  /** Puts words in the composer: a saved draft's, or its opening defaults. */
  onRestore: (fields: SavedDraftFields) => void;
  onClose: () => void;
}) {
  const t = useT();
  const toast = useToast();
  const queryClient = useQueryClient();
  const { open, fields } = input;
  const anchor = savedDraftAnchor(input.where);
  // What the composer opens with: a draft is measured against it, and deleting
  // one puts it back.
  const defaults = {
    recipient: input.offeredRecipient,
    subject:
      input.replyTo?.kind === "email" &&
      input.replyTo.content_state !== "withheld"
        ? replySubject(input.replyTo.subject ?? "")
        : "",
  };
  const key = draftKey(anchor);
  // Only this composer's own writes move the held version. A refetch on focus
  // could adopt another window's save, and the next write here would replace
  // it without the 409 that is the only warning the reader gets.
  const read = useQuery({
    queryKey: key,
    queryFn: () => (anchor ? readDraft(anchor) : null),
    enabled: open && anchor !== null,
    staleTime: Number.POSITIVE_INFINITY,
    refetchOnMount: "always",
    refetchOnWindowFocus: false,
    refetchInterval: false,
  });
  const held = anchor ? (read.data ?? null) : null;
  const [restoredKey, setRestoredKey] = useState<string | null>(null);
  const [changedElsewhere, setChangedElsewhere] = useState(false);
  const keyText = key.join(":");
  // Restored once per opening and per anchor, and never over words the reader
  // has already started: a read answering after the first keystroke leaves the
  // text alone and only holds the version, so the next save replaces it. A
  // composer reopened still holding the saved words is a restored one too.
  const decided = useRef(new Set<string>());
  // The "Draft saved" toast is sticky, and every later toast queues behind it.
  // Reopening the composer puts the draft back on screen with its own Delete,
  // so the toast has said its piece and would otherwise hold the queue.
  const savedToastUp = useRef(false);
  useEffect(() => {
    if (!open || !savedToastUp.current) return;
    savedToastUp.current = false;
    toast.dismiss();
  }, [open, toast]);
  const onRestore = input.onRestore;
  const typed = wroteSomething(fields, defaults);
  useEffect(() => {
    if (!open) {
      decided.current.clear();
      setRestoredKey(null);
      setChangedElsewhere(false);
      return;
    }
    if (!read.isSuccess || read.isFetching || decided.current.has(keyText))
      return;
    decided.current.add(keyText);
    if (!read.data) return;
    const saved = fieldsOfDraft(read.data);
    if (!typed) onRestore(saved);
    if (!typed || sameFields(fields, saved)) setRestoredKey(keyText);
  }, [
    open,
    read.isSuccess,
    read.isFetching,
    read.data,
    keyText,
    typed,
    fields,
    onRestore,
  ]);

  const dirty = held
    ? !sameFields(fields, fieldsOfDraft(held))
    : wroteSomething(fields, defaults);

  const onCleared = () =>
    onRestore({
      to: defaults.recipient ? [defaults.recipient] : [],
      cc: [],
      bcc: [],
      subject: defaults.subject,
      body: "",
      html: "",
    });
  const onClose = input.onClose;
  const save = useMutation({
    mutationFn: writeDraft,
    onSuccess: (saved, ask) => {
      queryClient.setQueryData(draftKey(ask.anchor), saved);
      setChangedElsewhere(false);
      savedToastUp.current = true;
      toast.show(t("compose.savedDraftSaved"), {
        action: {
          label: t("compose.savedDraftDelete"),
          onAct: () => {
            savedToastUp.current = false;
            deleteFromToast({
              queryClient,
              toast,
              anchor: ask.anchor,
              id: saved.id,
              t,
              onDeleted: onCleared,
            });
          },
        },
      });
      onClose();
    },
    // The save stays pending until the re-read lands, so the notice never
    // offers to load a version this composer has not seen yet.
    onError: async (error, ask) => {
      if (problemCodeOf(error) !== "version_skew") return;
      try {
        await queryClient.fetchQuery({
          queryKey: draftKey(ask.anchor),
          staleTime: 0,
          queryFn: () => readDraft(ask.anchor),
        });
      } catch (reread: unknown) {
        toast.show(problemMessageOf(reread, t), { tone: "danger" });
      }
      setChangedElsewhere(true);
    },
  });
  const remove = useMutation({
    mutationFn: async (
      target: Readonly<{ anchor: SavedDraftAnchor; id: string }>,
    ) => {
      await deleteDraft(target.id);
      return target;
    },
    onSuccess: (target) => {
      queryClient.setQueryData(draftKey(target.anchor), null);
      setRestoredKey(null);
      setChangedElsewhere(false);
      onCleared();
      toast.show(t("compose.savedDraftDeleted"));
    },
  });

  const saveNow = () => {
    if (!anchor || save.isPending) return;
    save.mutate({ anchor, fields, version: held?.version });
  };
  return {
    enabled: anchor !== null,
    held,
    restored: restoredKey === keyText && held !== null,
    changedElsewhere,
    saving: save.isPending,
    // A conflict is not a failure line: the notice above names it and offers
    // the way out.
    error:
      save.isError && problemCodeOf(save.error) !== "version_skew"
        ? problemMessageOf(save.error, t)
        : remove.isError
          ? problemMessageOf(remove.error, t)
          : null,
    save: saveNow,
    // Closing with words the draft does not hold yet keeps them first; the
    // close lands when the save does, and a refused save leaves the composer
    // open over the text rather than dropping it.
    requestClose: () => {
      if (save.isPending) return;
      if (anchor && dirty) saveNow();
      else onClose();
    },
    remove: () => {
      if (anchor && held) remove.mutate({ anchor, id: held.id });
    },
    removing: remove.isPending,
    loadSaved: () => {
      if (!held) return;
      onRestore(fieldsOfDraft(held));
      setChangedElsewhere(false);
    },
    // The send discarded it server-side in the same transaction.
    sent: () => {
      if (anchor) queryClient.setQueryData(draftKey(anchor), null);
    },
  };
}

// What the composer says about the saved draft it is holding: that one was put
// back, or that another window moved it while this one was writing.
export function SavedDraftNotices({ draft }: Readonly<{ draft: SavedDraft }>) {
  const t = useT();
  if (draft.changedElsewhere) {
    return (
      <Callout
        tone="warning"
        kind="outcome"
        title={t("compose.savedDraftChangedTitle")}
        actions={
          draft.held ? (
            <Button onClick={draft.loadSaved}>
              {t("compose.savedDraftLoad")}
            </Button>
          ) : undefined
        }
      >
        {t(
          draft.held
            ? "compose.savedDraftChangedBody"
            : "compose.savedDraftGoneBody",
        )}
      </Callout>
    );
  }
  if (!draft.restored) return null;
  return (
    <Callout
      kind="standing"
      title={t("compose.savedDraftRestored")}
      actions={
        <Button onClick={draft.remove} pending={draft.removing}>
          {t("compose.savedDraftRemove")}
        </Button>
      }
    />
  );
}
