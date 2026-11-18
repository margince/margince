// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type QueryClient,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useCallback, useEffect, useRef, useState } from "react";
import type { components } from "../../api/schema";
import { paragraphsFrom } from "../../design-system/richtext";
import {
  type Toast,
  undoAction,
  useOwnToast,
  useToast,
} from "../../design-system/toast";
import { useT } from "../../i18n";
import { problemCodeOf, problemMessageOf } from "../../lib/problem";
import type { ChosenFile } from "./composeattachments";
import { type DraftKeeper, saveOutcome, writeKept } from "./composeautosave";
import {
  deleteDraft,
  draftKey,
  groundedFiling,
  readDraft,
  type SavedDraftAnchor,
  type SavedDraftFields,
  savedDraftAnchor,
  writeDraft,
} from "./composesaveddraftcalls";
import { fieldsOfDraft, sameFields } from "./composesaveddraftfields";
import {
  closeStep,
  composerDefaults,
  savedDraftSettled,
  unsavedWords,
  wroteSomething,
} from "./composesaveddraftrules";

export { SavedDraftNotices } from "./composesaveddraftnotices";

// The rep's own unsent message, kept on the server so closing the composer does
// not lose it. Not the AI draft (`composedraftband.tsx`): that one is words a
// model served, this one is whatever the rep had written when they stepped away.

type Activity = components["schemas"]["Activity"];

/**
 * Putting a saved draft back in the composer.
 *
 * The words, the files, the reason and the send time, the subject the project
 * filing adopts, and the filing the draft was grounded in. Seeded rather than
 * merged at send: a deal the reader picks after restoring is their choice and
 * has to win.
 */
export function useDraftRestore(input: {
  settle: () => void;
  restoreSaved: (saved: SavedDraftFields) => void;
  adoptSavedSubject: (subject: string) => void;
  setFiles: (
    update: (current: readonly ChosenFile[]) => readonly ChosenFile[],
  ) => void;
  account: {
    setDealId: (id: string) => void;
    setProjectId: (id: string) => void;
  };
  setOverrideExplanation?: (text: string) => void;
}): (saved: SavedDraftFields) => void {
  const { settle, restoreSaved, adoptSavedSubject, setFiles } = input;
  const { setOverrideExplanation } = input;
  const { setDealId, setProjectId } = input.account;
  return useCallback(
    (saved: SavedDraftFields) => {
      settle();
      const hasFiling = saved.editorState?.filing !== undefined;
      const filing = groundedFiling(
        saved.editorState?.filing?.map((ground) => ({
          type: ground.anchor_type,
          id: ground.anchor_id,
        })) ?? saved.grounding,
      );
      if (hasFiling || filing.dealId) setDealId(filing.dealId);
      if (hasFiling || filing.projectId) setProjectId(filing.projectId);
      restoreSaved(saved);
      setFiles(() => saved.files);
      adoptSavedSubject(saved.subject);
      setOverrideExplanation?.(saved.overrideExplanation ?? "");
    },
    [
      setOverrideExplanation,
      settle,
      restoreSaved,
      adoptSavedSubject,
      setFiles,
      setDealId,
      setProjectId,
    ],
  );
}

// The toast's Delete outlives the composer it came from, so it is a plain call
// on the client rather than a mutation owned by a component that may be gone.
function deleteFromToast(input: {
  queryClient: QueryClient;
  toast: Toast;
  anchor: SavedDraftAnchor;
  id: string;
  t: ReturnType<typeof useT>;
  fields: SavedDraftFields;
  sends: number;
  onDeleted: () => void;
  onRestored: (fields: SavedDraftFields) => void;
}) {
  deleteDraft(input.id).then(
    () => {
      input.queryClient.setQueryData(draftKey(input.anchor), null);
      input.onDeleted();
      input.toast.show(input.t("compose.savedDraftDeleted"), {
        action: undoAction(input.t("common.undo"), () => restoreDeleted(input)),
      });
    },
    (error: unknown) => {
      input.toast.show(problemMessageOf(error, input.t), {
        tone: "danger",
        sticky: true,
      });
    },
  );
}

// A new draft with the same words: the server keeps no copy of a deleted one.
// A draft written meanwhile refuses this as a conflict, which the toast names.
function restoreDeleted(input: {
  queryClient: QueryClient;
  toast: Toast;
  t: ReturnType<typeof useT>;
  anchor: SavedDraftAnchor;
  fields: SavedDraftFields;
  sends: number;
  onRestored: (fields: SavedDraftFields) => void;
}) {
  writeDraft({
    anchor: input.anchor,
    fields: input.fields,
    version: undefined,
    sends: input.sends,
  }).then(
    (saved) => {
      input.queryClient.setQueryData(draftKey(input.anchor), saved);
      input.onRestored(input.fields);
      input.toast.show(input.t("compose.savedDraftRestored"));
    },
    (error: unknown) =>
      input.toast.show(problemMessageOf(error, input.t), { tone: "danger" }),
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
  initialIntent?: string;
  /** The words a caller opened the composer with, such as a forward's quote. */
  initialMessage?: Readonly<{ subject: string; body: string }>;
  /** Puts words in the composer: a saved draft's, or its opening defaults. */
  onRestore: (fields: SavedDraftFields) => void;
  onClose: () => void;
}) {
  const t = useT();
  const toast = useToast();
  const queryClient = useQueryClient();
  const { open, fields } = input;
  const anchor = savedDraftAnchor(input.where);
  const defaults = composerDefaults(input);
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
  // Which words win is open: a draft found under other words ("found"), or one
  // another window moved after this composer read it ("moved").
  const [conflict, setConflict] = useState<"found" | "moved" | null>(null);
  const changedElsewhere = conflict !== null;
  const keyText = key.join(":");
  // Restored once per opening and per anchor, and never over words the reader
  // has already started. A draft found under other words on screen asks which
  // words win, so no save replaces words the reader never saw. A composer
  // reopened still holding the saved words is a restored one too.
  const decided = useRef(new Set<string>());
  // Reopening the composer puts the draft back on screen with its own Delete,
  // so the "Draft saved" toast, if it is still up, has said its piece.
  const savedToast = useOwnToast();
  const sends = useRef(0);
  useEffect(() => {
    if (open) savedToast.withdraw();
  }, [open, savedToast]);
  const onRestore = input.onRestore;
  const typed = wroteSomething(fields, defaults);
  useEffect(() => {
    if (!open) {
      decided.current.clear();
      setRestoredKey(null);
      setConflict(null);
      return;
    }
    if (!read.isSuccess || read.isFetching || decided.current.has(keyText))
      return;
    decided.current.add(keyText);
    if (!read.data) return;
    const saved = fieldsOfDraft(read.data);
    if (!typed) onRestore(saved);
    if (!typed || sameFields(fields, saved)) setRestoredKey(keyText);
    else setConflict("found");
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

  const onCleared = () =>
    onRestore({
      to: defaults.recipient ? [defaults.recipient] : [],
      cc: [],
      bcc: [],
      subject: defaults.subject,
      body: defaults.body,
      html: defaults.body ? paragraphsFrom(defaults.body) : "",
      files: [],
      context: "",
      sendAt: "",
    });
  const onClose = input.onClose;
  const save = useMutation({
    mutationFn: writeDraft,
    onSuccess: (saved, ask) => {
      // A send that left after this save began discarded the draft, so the
      // save's answer must not bring it back.
      if (ask.sends !== sends.current) return;
      queryClient.setQueryData(draftKey(ask.anchor), saved);
      setConflict(null);
      if (ask.quiet) return;
      savedToast.show(t("compose.savedDraftSaved"), {
        action: {
          kind: "undo",
          label: t("compose.savedDraftDelete"),
          onAct: () => {
            deleteFromToast({
              queryClient,
              toast,
              anchor: ask.anchor,
              id: saved.id,
              t,
              fields: ask.fields,
              sends: sends.current,
              onDeleted: onCleared,
              onRestored: onRestore,
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
      let landed = false;
      try {
        const now = await queryClient.fetchQuery({
          queryKey: draftKey(ask.anchor),
          staleTime: 0,
          queryFn: () => readDraft(ask.anchor),
        });
        landed = now !== null && sameFields(fieldsOfDraft(now), ask.fields);
      } catch (reread: unknown) {
        toast.show(problemMessageOf(reread, t), { tone: "danger" });
      }
      // An earlier save of these words landed and its answer was lost: the
      // server holds them, and no other window moved anything.
      if (landed) {
        if (!ask.quiet) onClose();
        return;
      }
      setConflict("moved");
    },
  });
  const remove = useMutation({
    mutationFn: async (
      target: Readonly<{
        anchor: SavedDraftAnchor;
        id: string;
        fields: SavedDraftFields;
      }>,
    ) => {
      await deleteDraft(target.id);
      return target;
    },
    onSuccess: (target) => {
      queryClient.setQueryData(draftKey(target.anchor), null);
      setRestoredKey(null);
      setConflict(null);
      onCleared();
      toast.show(t("compose.savedDraftDeleted"), {
        action: undoAction(t("common.undo"), () =>
          restoreDeleted({
            queryClient,
            toast,
            t,
            anchor: target.anchor,
            fields: target.fields,
            sends: sends.current,
            onRestored: onRestore,
          }),
        ),
      });
    },
  });

  const dirty = unsavedWords({ fields, held, save, defaults });
  const keeper: DraftKeeper = {
    anchor,
    read: read.isSuccess && !read.isFetching,
    conflicted: changedElsewhere,
    unsaved: open && anchor !== null && dirty,
    unkept: open && dirty,
    save,
    fields,
    askOf: (quiet) =>
      anchor
        ? {
            anchor,
            fields,
            version: held?.version,
            sends: sends.current,
            quiet,
          }
        : null,
  };
  const keeping = saveOutcome(save);
  const { failed: saveFailed, quietly: failedQuietly } = keeping;
  const write = (quiet: boolean) => writeKept(keeper, quiet);
  const closeAfterSave = useRef(false);
  const requestClose = () => {
    // A found draft was never on screen: the notice asks before a close may save over it.
    if (conflict === "found") return;
    const step = closeStep({
      save,
      refusedClose: saveFailed && !failedQuietly,
      fields,
      unsaved: anchor !== null && dirty,
    });
    if (step === "wait") closeAfterSave.current = true;
    else if (step === "discard") {
      save.reset();
      onCleared();
      onClose();
    } else if (step === "save") write(false);
    else onClose();
  };
  const latestClose = useRef(requestClose);
  useEffect(() => {
    latestClose.current = requestClose;
  });
  // A close pressed while a save is out runs once it settles, unless a
  // conflict now asks the reader which words win.
  useEffect(() => {
    if (save.isPending || !closeAfterSave.current) return;
    closeAfterSave.current = false;
    if (!changedElsewhere) latestClose.current();
  }, [save.isPending, changedElsewhere]);
  return {
    /** Handed to `useKeptDraft`, which needs the send's state as well. */
    keeper,
    enabled: anchor !== null,
    held,
    restored: restoredKey === keyText && held !== null,
    settled: savedDraftSettled({
      anchored: anchor !== null,
      read,
      held: held !== null,
      restored: restoredKey === keyText,
      typed,
    }),
    changedElsewhere,
    conflict,
    /** The held draft, only while its words are the ones on screen; a send names this one. */
    onScreenId: changedElsewhere ? undefined : held?.id,
    saving: save.isPending,
    // A conflict is not a failure line: the notice above names it and offers
    // the way out.
    error: keeping.failure
      ? t(keeping.failure)
      : remove.isError
        ? problemMessageOf(remove.error, t)
        : null,
    /** Tries the refused save again now, as it was asked: quiet, or closing. */
    retry: saveFailed ? () => write(failedQuietly) : undefined,
    // Closing with words the draft does not hold yet keeps them first; the
    // close lands when the save does. A refused close leaves the composer open
    // over the text once, and the line says a second close discards it.
    requestClose,
    remove: () => {
      if (anchor && held)
        remove.mutate({ anchor, id: held.id, fields: fieldsOfDraft(held) });
    },
    removing: remove.isPending,
    /** What is on screen is what the saved draft holds. */
    kept: held !== null && !dirty,
    /** Throws the draft away and shuts the window: the saved one goes too,
     *  unless it was never on screen. */
    discard: () => {
      if (save.isPending || remove.isPending) return;
      if (anchor && held && !changedElsewhere) {
        remove.mutate(
          { anchor, id: held.id, fields: fieldsOfDraft(held) },
          { onSuccess: onClose },
        );
        return;
      }
      onCleared();
      onClose();
    },
    loadSaved: () => {
      if (!held) return;
      onRestore(fieldsOfDraft(held));
      setConflict(null);
    },
    /** The reader's answer to a found draft: the words on screen replace it. */
    keepScreen: () => write(true),
    // The send discarded it server-side in the same transaction.
    sent: () => {
      sends.current += 1;
      if (anchor) queryClient.setQueryData(draftKey(anchor), null);
    },
  };
}
