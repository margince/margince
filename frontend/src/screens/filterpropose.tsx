// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Describing a list in plain words, and getting it back as clauses.
//
// The model proposes and nothing more: what it answers lands in the builder as
// ordinary clauses the reader edits, the preview counts them as it counts any
// edit, and Save is still the reader's press. A proposal never lands over a
// filter the reader already built without asking first, because replacing four
// clauses somebody chose with one a model guessed is not an undoable glance.

import { useMutation } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, Field, Textarea } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ErrorLine } from "../design-system/errorline";
import { type Locale, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemCodeOf, problemMessageOf, throwProblem } from "./common";
import {
  type FilterResource,
  fieldLabel,
  type VocabularyField,
} from "./filterdata";
import { addProposal } from "./filterproposal";
import { decode, isGroup, type Node } from "./segmentpredicate";

export type FilterProposal = components["schemas"]["FilterProposal"];
export type UnusedPhrase = components["schemas"]["FilterProposalUnsupported"];
type ProposalResource =
  components["schemas"]["FilterProposalRequest"]["resource"];

/** The record types a proposal may be asked for: the builder's, minus project. */
function proposalResource(resource: FilterResource): ProposalResource | null {
  return resource === "project" ? null : resource;
}

type ProposalAsk = Readonly<{
  resource: ProposalResource;
  text: string;
  locale: Locale;
}>;

export function useFilterProposal() {
  return useMutation({
    mutationFn: async (ask: ProposalAsk) => {
      const { data, error } = await api.POST("/filters/propose", { body: ask });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

function isEmptyTree(tree: Node): boolean {
  return isGroup(tree) && tree.children.length === 0;
}

type Pending = Readonly<{ tree: Node; unused: readonly UnusedPhrase[] }>;

export type PlainWordsFilterProps = Readonly<{
  resource: FilterResource;
  tree: Node;
  /** Puts a tree in the builder and the phrases that did not make it beside it. */
  onApply: (tree: Node | null, unused: readonly UnusedPhrase[]) => void;
}>;

export function PlainWordsFilter({
  resource,
  tree,
  onApply,
}: PlainWordsFilterProps) {
  const t = useT();
  const { locale } = useLocale();
  const [text, setText] = useState("");
  const [pending, setPending] = useState<Pending | null>(null);
  const [unreadable, setUnreadable] = useState(false);
  // The tree as it stands when the answer ARRIVES. The reader may have added a
  // clause or loaded a view while the model was reading, and replace-or-ask is
  // decided against what is on screen then, not what was there at submit.
  const current = useRef(tree);
  useEffect(() => {
    current.current = tree;
  }, [tree]);
  const propose = useFilterProposal();
  const askable = proposalResource(resource);
  if (askable === null) {
    return null;
  }

  const received = (proposal: FilterProposal) => {
    setUnreadable(false);
    if (proposal.filter === null || proposal.filter === undefined) {
      onApply(null, proposal.unsupported);
      return;
    }
    const proposed = decode(proposal.filter);
    if (proposed === null) {
      setUnreadable(true);
      return;
    }
    if (isEmptyTree(current.current)) {
      onApply(proposed, proposal.unsupported);
      return;
    }
    setPending({ tree: proposed, unused: proposal.unsupported });
  };

  const submit = () => {
    const sentence = text.trim();
    if (sentence === "") {
      return;
    }
    setPending(null);
    propose.mutate(
      { resource: askable, text: sentence, locale },
      { onSuccess: received },
    );
  };

  return (
    <div className="filters-propose">
      <Field
        label={t("filters.propose.label")}
        hint={t("filters.propose.hint")}
      >
        {(control) => (
          <Textarea
            {...control}
            rows={2}
            maxLength={500}
            value={text}
            onChange={(event) => setText(event.target.value)}
            placeholder={t("filters.propose.placeholder")}
          />
        )}
      </Field>
      <div className="form-actions">
        <Button
          variant="ai"
          onClick={submit}
          pending={propose.isPending}
          busyLabel={t("filters.propose.busy")}
          disabled={text.trim() === ""}
        >
          {t("filters.propose.submit")}
        </Button>
      </div>
      {propose.isError && <ProposalFailure error={propose.error} />}
      {unreadable && <ErrorLine>{t("filters.propose.unreadable")}</ErrorLine>}
      {pending && (
        <Callout
          tone="ai"
          live="status"
          title={t("filters.propose.readyTitle")}
          actions={
            <>
              <Button
                variant="ai"
                onClick={() => {
                  onApply(pending.tree, pending.unused);
                  setPending(null);
                }}
              >
                {t("filters.propose.replace")}
              </Button>
              <Button
                onClick={() => {
                  onApply(addProposal(tree, pending.tree), pending.unused);
                  setPending(null);
                }}
              >
                {t("filters.propose.add")}
              </Button>
              <Button variant="link" onClick={() => setPending(null)}>
                {t("filters.propose.discard")}
              </Button>
            </>
          }
        >
          {t("filters.propose.readyBody")}
        </Callout>
      )}
    </div>
  );
}

function ProposalFailure({ error }: Readonly<{ error: unknown }>) {
  const t = useT();
  if (problemCodeOf(error) === "ai_not_configured") {
    return <ErrorLine>{t("filters.propose.noModel")}</ErrorLine>;
  }
  return <ErrorLine>{problemMessageOf(error, t)}</ErrorLine>;
}

/** What a dropped clause's code says, in the reader's words. */
const UNUSED_REASON: Record<
  Exclude<UnusedPhrase["code"], "not_expressible">,
  MessageKey
> = {
  unknown_field: "filters.propose.reason.unknownField",
  operator_not_allowed: "filters.propose.reason.operator",
  value_not_allowed: "filters.propose.reason.value",
  value_not_verifiable: "filters.propose.reason.notVerifiable",
  too_many_conditions: "filters.propose.reason.tooMany",
};

/**
 * The phrases a proposal could not use, and why. The model's own reason is
 * already in the reader's language; a clause the server dropped is explained
 * from its code, naming the field as the builder names it.
 */
export function UnusedPhrases({
  unused,
  fields,
  onDismiss,
}: Readonly<{
  unused: readonly UnusedPhrase[];
  fields: readonly VocabularyField[];
  onDismiss: () => void;
}>) {
  const t = useT();
  if (unused.length === 0) {
    return null;
  }
  const named = (field: string | undefined) => {
    const known = fields.find((candidate) => candidate.name === field);
    return known ? fieldLabel(known, t) : (field ?? "");
  };
  return (
    <Callout
      tone="warning"
      title={t("filters.propose.unusedTitle")}
      dismiss={{ label: t("filters.propose.unusedDismiss"), onDismiss }}
    >
      <ul className="filters-propose-unused">
        {unused.map((item, index) => (
          // The phrase alone is not unique: two clauses can come from one phrase.
          // biome-ignore lint/suspicious/noArrayIndexKey: the list is replaced whole, never reordered
          <li key={`${index}-${item.phrase}`}>
            {t("filters.propose.unusedItem", {
              phrase: item.phrase,
              reason:
                item.code === "not_expressible"
                  ? item.reason
                  : t(UNUSED_REASON[item.code], { field: named(item.field) }),
            })}
          </li>
        ))}
      </ul>
    </Callout>
  );
}
