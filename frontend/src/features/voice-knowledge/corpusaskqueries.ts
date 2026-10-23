// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { grants } from "../../lib/accessgrants";
import { throwProblem } from "../../lib/problem";
import { useMe } from "../../lib/session";

// The two requests behind asking a document set. The ask drawer and the search
// box share them, so both read one list of sets and ask through one call.

export type Answer = components["schemas"]["KnowledgeAnswer"];
export type Corpus = components["schemas"]["KnowledgeCorpus"];

const askableSets = queryOptions({
  queryKey: ["knowledge-corpora"],
  queryFn: async () => {
    const { data, error, response } = await api.GET("/knowledge/corpora");
    if (error || !response.ok) {
      throwProblem(error);
    }
    return data;
  },
});

export function useAskableSets(enabled: boolean) {
  return useQuery({ ...askableSets, enabled });
}

export async function askCorpus(
  corpusId: string,
  question: string,
): Promise<Answer> {
  const { data, error } = await api.POST("/knowledge/corpora/{id}/ask", {
    params: { path: { id: corpusId } },
    body: { question },
  });
  if (error || !data) {
    throwProblem(error);
  }
  return data;
}

export function useAsk() {
  return useMutation({
    mutationFn: ({
      corpusId,
      question,
    }: {
      corpusId: string;
      question: string;
    }): Promise<Answer> => askCorpus(corpusId, question),
  });
}

/** The sets the search box asks, in order: the shipped handbook, then the default. */
export function handbookAskOrder(sets: readonly Corpus[]): readonly string[] {
  const order = [
    sets.find((set) => set.shipped)?.id,
    sets.find((set) => set.default_ask)?.id,
  ].filter((id): id is string => id !== undefined);
  return [...new Set(order)];
}

export type HandbookAnswer = Readonly<{
  question: string;
  answer: Answer;
  // The sets that were asked, by name, in order. Two when the handbook did not
  // cover the question and the workspace's default set was asked after it.
  asked: readonly string[];
}>;

/** The ask found no document set this reader may ask. */
export class NoDocumentSetsError extends Error {
  constructor() {
    super("no document set to ask");
    this.name = "NoDocumentSetsError";
  }
}

/**
 * Asks each set in turn until one answers something other than `not_covered`.
 * A failed first ask moves on to the next set. A failed ask after a
 * `not_covered` is thrown, because showing that answer alone would claim the
 * later set was searched too.
 */
async function askInOrder(
  order: readonly string[],
  question: string,
): Promise<HandbookAnswer> {
  const asked: string[] = [];
  let answer: Answer | undefined;
  let failure: unknown;
  for (const id of order) {
    try {
      answer = await askCorpus(id, question);
    } catch (error) {
      if (answer !== undefined) {
        throw error;
      }
      failure ??= error;
      continue;
    }
    asked.push(answer.corpus.name);
    if (answer.outcome !== "not_covered") {
      break;
    }
  }
  if (answer === undefined) {
    throw failure;
  }
  return { question, answer, asked };
}

/**
 * Asks the shipped handbook, and on `not_covered` or a failed ask the
 * workspace's default set. Every other outcome of the first set stands: a set
 * still being read or a missing search lane says nothing about the question.
 *
 * The sets are read only once `wanted`, so a box used to look something up
 * costs no request for them. The ask reads them itself, so an Enter pressed
 * before the list arrived still asks rather than falling through to search.
 */
export function useHandbookAsk(wanted: boolean) {
  const client = useQueryClient();
  const canAsk = grants(useMe(wanted).data, "knowledge_corpus", "read");
  const sets = useAskableSets(canAsk && wanted);
  // Unknown counts as askable until the list says there is nothing to ask.
  const askable =
    sets.data === undefined || handbookAskOrder(sets.data.items).length > 0;
  const mutation = useMutation({
    mutationFn: async ({
      question,
    }: {
      question: string;
    }): Promise<HandbookAnswer> => {
      const listed = await client.ensureQueryData(askableSets);
      const order = handbookAskOrder(listed.items);
      if (order.length === 0) {
        throw new NoDocumentSetsError();
      }
      return askInOrder(order, question);
    },
  });
  return { available: canAsk && askable, mutation };
}
