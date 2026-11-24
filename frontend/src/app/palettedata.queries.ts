// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { api } from "../api/client";
import type { components } from "../api/schema";
import type { HandbookAnswer } from "../features/voice-knowledge/corpusaskqueries";
import { throwProblem } from "../lib/problem";
import { looksLikeDataQuestion } from "../lib/question";

export type DataAnswer = components["schemas"]["SearchDataAnswer"];
export type HandbookFallback = HandbookAnswer & {
  dataError?: unknown;
  dataAttempted?: boolean;
};
export type PaletteAskAnswer =
  | HandbookFallback
  | { question: string; dataAnswer: DataAnswer }
  | { question: string; unsupportedData: true; dataError?: unknown };

export async function askPaletteQuestion({
  question,
  readData,
  askHandbook,
  handbookAvailable = true,
}: {
  question: string;
  readData: boolean;
  handbookAvailable?: boolean;
  askHandbook: (input: { question: string }) => Promise<HandbookAnswer>;
}): Promise<PaletteAskAnswer> {
  if (!readData || !looksLikeDataQuestion(question))
    return askHandbook({ question });
  let dataError: unknown;
  try {
    const { data, error, response } = await api.POST("/search/data-answer", {
      body: { question },
    });
    if (error || !response.ok) throwProblem(error);
    if (data?.outcome === "answered") return { question, dataAnswer: data };
  } catch (error) {
    dataError = error;
  }
  return handbookAvailable
    ? { ...(await askHandbook({ question })), dataError, dataAttempted: true }
    : { question, unsupportedData: true, dataError };
}
