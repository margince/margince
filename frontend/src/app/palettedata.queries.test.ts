// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, expect, it, vi } from "vitest";
import type { HandbookAnswer } from "../features/voice-knowledge/corpusaskqueries";
import { askPaletteQuestion } from "./palettedata.queries";

afterEach(() => vi.unstubAllGlobals());

const handbook: HandbookAnswer = {
  question: "How many deals did I win?",
  asked: ["Handbook"],
  answer: {
    outcome: "not_covered",
    generated_by: "deterministic",
    summary: "Not covered",
    claims: [],
    corpus: {
      id: "handbook",
      name: "Handbook",
      topic_statement: "How Margince works",
      shipped: true,
    },
    coverage: {
      documents_total: 1,
      chunks_total: 1,
      chunks_embedded: 1,
      embed_state: "complete",
    },
  },
};

it("returns scoped CRM records without asking the handbook", async () => {
  const asked = vi.fn(async () => handbook);
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            outcome: "answered",
            source: '{"plan":{"target":"deal"}}',
            records: {
              coverage: "complete_exact",
              executed_plan: "Won deals",
              limit: 50,
              notes: [],
              rows: [],
            },
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
    ),
  );
  const result = await askPaletteQuestion({
    question: "Which deals close this month?",
    readData: true,
    askHandbook: asked,
  });
  expect(
    "dataAnswer" in result && result.dataAnswer.records?.executed_plan,
  ).toBe("Won deals");
  expect(asked).not.toHaveBeenCalled();
});

it("keeps a failed data read visible while falling back to the handbook", async () => {
  const failure = new Error("data reader unavailable");
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => {
      throw failure;
    }),
  );
  const asked = vi.fn(async () => handbook);
  const result = await askPaletteQuestion({
    question: handbook.question,
    readData: true,
    askHandbook: asked,
  });
  expect("answer" in result && result.answer.outcome).toBe("not_covered");
  expect("dataError" in result && result.dataError).toBe(failure);
  expect(asked).toHaveBeenCalledWith({ question: handbook.question });
});

it("falls back without calling a handbook the member cannot read", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify({ outcome: "unsupported" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
    ),
  );
  const asked = vi.fn(async () => handbook);
  const result = await askPaletteQuestion({
    question: handbook.question,
    readData: true,
    handbookAvailable: false,
    askHandbook: asked,
  });
  expect("unsupportedData" in result && result.unsupportedData).toBe(true);
  expect(asked).not.toHaveBeenCalled();
});
