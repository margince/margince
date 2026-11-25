// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { Field, Textarea, TextInput } from "../../design-system/field";
import { useT } from "../../i18n";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { TEXT_LIMITS } from "../../lib/textlimits_gen";
import { SETS_KEY } from "./knowledge.documents";

// Changing a document set after it was made: its words, and bringing an
// archived set back. Both refresh every list of sets, the ask picker's too.

type Corpus = components["schemas"]["KnowledgeCorpus"];

/** The sets the screen lists while archived sets are shown. The ask picker
 * reads SETS_KEY itself, so it never lists an archived set. */
export const SETS_WITH_ARCHIVED_KEY = [...SETS_KEY, "with-archived"] as const;

export function useRestoreDocumentSet() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { id: string }) =>
      unwrap(
        await api.POST("/knowledge/corpora/{id}/restore", {
          params: { path: { id: vars.id } },
        }),
      ),
    onSuccess: () => client.invalidateQueries({ queryKey: SETS_KEY }),
  });
}

type Draft = { name: string; description: string; topic: string };

function draftOf(set: Corpus): Draft {
  return {
    name: set.name,
    description: set.description ?? "",
    topic: set.topic_statement,
  };
}

// Sends only what changed, so an unchanged covers sentence is not written again.
function changesOf(set: Corpus, draft: Draft) {
  const before = draftOf(set);
  const name = draft.name.trim();
  const description = draft.description.trim();
  const topic = draft.topic.trim();
  return {
    ...(name !== before.name && { name }),
    ...(description !== before.description && { description }),
    ...(topic !== before.topic && { topic_statement: topic }),
  };
}

/** The set's name, description and covers sentence, edited in one form. */
export function EditDocumentSet({
  set,
  onClose,
}: Readonly<{ set: Corpus; onClose: () => void }>) {
  const t = useT();
  const client = useQueryClient();
  const [draft, setDraft] = useState(() => draftOf(set));
  const changes = changesOf(set, draft);
  const save = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.PATCH("/knowledge/corpora/{id}", {
          params: { path: { id: set.id } },
          body: changes,
        }),
      ),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: SETS_KEY });
      onClose();
    },
  });
  const blank = !draft.name.trim() || !draft.topic.trim();
  return (
    <ConfirmModal
      open
      intent="form"
      onClose={onClose}
      title={t("knowledge.edit.title", { name: set.name })}
      confirmLabel={t("knowledge.edit.save")}
      confirmDisabled={blank || Object.keys(changes).length === 0}
      pending={save.isPending}
      error={save.isError ? problemMessageOf(save.error, t) : null}
      onConfirm={() => save.mutate()}
    >
      <div className="form-stack">
        <Field label={t("knowledge.new.name")} required>
          {(control) => (
            <TextInput
              {...control}
              value={draft.name}
              maxLength={TEXT_LIMITS.label}
              onChange={(event) =>
                setDraft({ ...draft, name: event.target.value })
              }
            />
          )}
        </Field>
        <Field label={t("knowledge.edit.description")}>
          {(control) => (
            <Textarea
              {...control}
              value={draft.description}
              onChange={(event) =>
                setDraft({ ...draft, description: event.target.value })
              }
            />
          )}
        </Field>
        <Field
          label={t("knowledge.new.topic")}
          hint={t("knowledge.new.topicHint")}
          required
        >
          {(control) => (
            <Textarea
              {...control}
              value={draft.topic}
              onChange={(event) =>
                setDraft({ ...draft, topic: event.target.value })
              }
            />
          )}
        </Field>
      </div>
    </ConfirmModal>
  );
}
