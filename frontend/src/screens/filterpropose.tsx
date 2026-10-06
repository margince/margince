// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Describing a filter in plain words, and getting it back as conditions.
//
// The model proposes and nothing more: its answer lands in the builder as
// ordinary rows marked as proposed, which the reader changes, removes or keeps,
// and Save is still the reader's press.

import { useMutation } from "@tanstack/react-query";
import { type Dispatch, useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { AiPending } from "../design-system/aipending";
import {
  Button,
  Card,
  Disclosure,
  Field,
  Textarea,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ErrorLine } from "../design-system/errorline";
import { formatNumber } from "../format/format";
import { type Locale, useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemCodeOf, throwProblem } from "./common";
import {
  type FilterResource,
  fieldLabel,
  type VocabularyField,
} from "./filterdata";
import type { FilterDraftAction } from "./filterdraft";
import { type Proposal, proposedCount } from "./filterproposal";
import { decode, type Group, rootGroup } from "./segmentpredicate";

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

/**
 * The ask, with its answer handled at the hook rather than per `mutate`: a
 * per-call callback runs only while the component that asked is mounted.
 */
export function useFilterProposal(
  handlers: Readonly<{
    onSuccess: (answer: FilterProposal, ask: ProposalAsk) => void;
    onError: () => void;
  }>,
) {
  return useMutation({
    mutationFn: async (ask: ProposalAsk) => {
      const { data, error } = await api.POST("/filters/propose", { body: ask });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: handlers.onSuccess,
    onError: handlers.onError,
  });
}

/** The page's description box: what is typed in it and how its ask stands. */
export type PlainWords = Readonly<{
  /** False on a record type no proposal can be asked for. */
  available: boolean;
  text: string;
  setText: (text: string) => void;
  submit: () => void;
  pending: boolean;
  /** The installation has no model, so the box gives way to a line saying so. */
  noModel: boolean;
  /** Any other refusal of the last ask. */
  failure: unknown;
  /** The last answer arrived in a shape no condition can be read from. */
  unreadable: boolean;
}>;

/**
 * Called by the PAGE rather than the box it is typed in: the box folds and
 * trades places with the calm start, and the answer must land through the
 * page's reducer, against the tree as it stands when it arrives.
 */
export function usePlainWords({
  resource,
  dispatch,
}: Readonly<{
  resource: FilterResource;
  dispatch: Dispatch<FilterDraftAction>;
}>): PlainWords {
  const { locale } = useLocale();
  const [text, setText] = useState("");
  const [unreadable, setUnreadable] = useState(false);
  const propose = useFilterProposal({
    onSuccess: (answer, ask) => {
      if (answer.filter === null || answer.filter === undefined) {
        dispatch({ type: "unused", unused: answer.unsupported });
        return;
      }
      const proposed = decode(answer.filter);
      if (proposed === null) {
        setUnreadable(true);
        dispatch({ type: "setWordsOpen", open: true });
        return;
      }
      dispatch({
        type: "answer",
        proposed: rootGroup(proposed),
        unused: answer.unsupported,
        text: ask.text,
      });
    },
    // The folded box opens on its own refusal, or the reason would sit
    // inside a closed disclosure nobody is looking at.
    onError: () => dispatch({ type: "setWordsOpen", open: true }),
  });
  const askable = proposalResource(resource);
  const noModel =
    propose.isError && problemCodeOf(propose.error) === "ai_not_configured";
  const submit = () => {
    const sentence = text.trim();
    if (askable === null || sentence === "" || propose.isPending) {
      return;
    }
    setUnreadable(false);
    dispatch({ type: "asking" });
    propose.mutate({ resource: askable, text: sentence, locale });
  };
  return {
    available: askable !== null,
    text,
    setText,
    submit,
    pending: propose.isPending,
    noModel,
    failure: propose.isError && !noModel ? propose.error : null,
    unreadable,
  };
}

export type PlainWordsFilterProps = Readonly<{
  words: PlainWords;
  /** The record type in the reader's words: "contacts". */
  records: string;
  /**
   * `start`: one of the two ways in, on a page with no conditions yet.
   * `folded`: behind a disclosure above the reader's rows.
   */
  layout: "start" | "folded";
  /** Whether the folded box is open; the start card is always open. */
  open: boolean;
  onOpen: (open: boolean) => void;
}>;

export function PlainWordsFilter({
  words,
  records,
  layout,
  open,
  onOpen,
}: PlainWordsFilterProps) {
  const t = useT();
  if (!words.available) {
    return null;
  }
  if (words.noModel) {
    return <ErrorLine>{t("filters.propose.noModel")}</ErrorLine>;
  }
  const body = (
    <div className="filters-propose">
      <Field
        label={t("filters.propose.label", { records })}
        hint={t("filters.propose.hint")}
      >
        {(control) => (
          <Textarea
            {...control}
            rows={2}
            maxLength={500}
            value={words.text}
            onChange={(event) => words.setText(event.target.value)}
            onKeyDown={(event) => {
              // Enter sends the description and Shift+Enter breaks the line;
              // Enter that confirms an input method's word is not a send.
              if (
                event.key === "Enter" &&
                !event.shiftKey &&
                !event.nativeEvent.isComposing
              ) {
                event.preventDefault();
                words.submit();
              }
            }}
            placeholder={t("filters.propose.placeholder")}
          />
        )}
      </Field>
      <div className="form-actions">
        <Button
          variant="ai"
          onClick={words.submit}
          pending={words.pending}
          busyLabel={t("filters.propose.busy")}
          disabled={words.text.trim() === ""}
        >
          {t("filters.propose.submit")}
        </Button>
      </div>
      {words.pending && <AiPending label={t("filters.propose.busy")} />}
      <ErrorLine error={words.failure} />
      {words.unreadable && (
        <ErrorLine>{t("filters.propose.unreadable")}</ErrorLine>
      )}
    </div>
  );
  if (layout === "start") {
    return (
      <Card as="div" inset>
        {body}
      </Card>
    );
  }
  return (
    <Disclosure
      summary={t("filters.describeChanges")}
      open={open}
      onToggle={onOpen}
    >
      {body}
    </Disclosure>
  );
}

/**
 * Above the rows while a proposal still has a marked row: what was proposed
 * from which words, and the three answers to it. Undo returns to the tree
 * before the FIRST proposal on screen, which is what `before` holds.
 */
export function ProposalBar({
  tree,
  proposal,
  dispatch,
}: Readonly<{
  tree: Group;
  proposal: Proposal | null;
  dispatch: Dispatch<FilterDraftAction>;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const keep = useRef<HTMLButtonElement>(null);
  const count = proposedCount(tree);
  if (proposal === null || count === 0) {
    return null;
  }
  return (
    <Callout
      tone="ai"
      live="status"
      title={plural("filters.proposal.title", count, {
        count: formatNumber(count, locale),
        text: proposal.text,
      })}
      actions={
        <>
          <Button
            ref={keep}
            variant="ai"
            onClick={() => dispatch({ type: "keepAll" })}
          >
            {t("filters.proposal.keepAll")}
          </Button>
          {proposal.hadOwn && (
            <Button
              onClick={() => {
                // This button leaves with the press, and Keep all is the
                // question still open about the rows left.
                keep.current?.focus();
                dispatch({ type: "replaceMine" });
              }}
            >
              {t("filters.proposal.replaceMine")}
            </Button>
          )}
          <Button variant="link" onClick={() => dispatch({ type: "undo" })}>
            {t("common.undo")}
          </Button>
        </>
      }
    >
      {t("filters.proposal.body")}
    </Callout>
  );
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
