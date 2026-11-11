// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { HOME_DAY_FRESH_MS } from "../lib/workkeys";
import { AnswerHistory } from "./homeanswers.judge";
import type { Answer, HomeRead } from "./homejourney.judge";

const answer = (
  read: HomeRead,
  status = 200,
  body: unknown = { revision: 1 },
): Answer => ({ read, journey: "load-0", status, body });

function history() {
  let now = 10;
  const history = new AnswerHistory("reader", () => now);
  history.beginDocument();
  return {
    history,
    advance: (ms: number) => {
      now += ms;
    },
  };
}

it("retains actual fresh day answers and names their source journey", () => {
  const { history: observed, advance } = history();
  const received = answer("worklist");
  observed.received(received, observed.documentID());
  advance(20);
  expect(observed.fill([], [])).toEqual({
    answers: [received],
    retained: [{ read: "worklist", journey: "load-0", ageMs: 20 }],
  });
});

it("does not borrow an answer for a read that started in this journey", () => {
  const { history: observed } = history();
  observed.received(answer("worklist"), observed.documentID());
  expect(observed.fill([], ["worklist"]).answers).toEqual([]);
});

it("keeps a new failure visible instead of replacing it with cached success", () => {
  const { history: observed } = history();
  observed.received(answer("worklist"), observed.documentID());
  const refused = answer("worklist", 503);
  observed.received(refused, observed.documentID());
  expect(observed.fill([refused], ["worklist"]).answers).toEqual([refused]);
  expect(observed.fill([], []).answers).toEqual([]);
});

it("a transport failure invalidates its prior success", () => {
  const { history: observed } = history();
  observed.received(answer("worklist"), observed.documentID());
  observed.failed("worklist", observed.documentID());
  expect(observed.fill([], []).answers).toEqual([]);
});

it("expires at the product's own day freshness boundary", () => {
  const { history: observed, advance } = history();
  observed.received(answer("brief"), observed.documentID());
  advance(HOME_DAY_FRESH_MS);
  expect(observed.fill([], []).answers).toEqual([]);
});

it("never carries an old document's late reply into a new one", () => {
  const { history: observed } = history();
  const old = observed.documentID();
  observed.received(answer("magic"), old);
  observed.beginDocument();
  observed.received(answer("magic"), old);
  expect(observed.fill([], []).answers).toEqual([]);
});

it.each([
  answer("me", 200, { user: { id: "other" } }),
  answer("me", 200, {}),
  answer("me", 503),
])("a failed or different-reader probe disables retention: %j", (probe) => {
  const { history: observed } = history();
  observed.received(answer("worklist"), observed.documentID());
  observed.received(probe, observed.documentID());
  observed.received(
    answer("me", 200, { user: { id: "reader" } }),
    observed.documentID(),
  );
  observed.received(answer("worklist"), observed.documentID());
  expect(observed.fill([], []).answers).toEqual([]);
});

it("does not retain the handled lane outside the day-cache policy", () => {
  const { history: observed } = history();
  observed.received(answer("handled"), observed.documentID());
  expect(observed.fill([], []).answers).toEqual([]);
});

it("cannot retain answers without a verified reader", () => {
  const observed = new AnswerHistory(undefined, () => 10);
  observed.received(answer("worklist"), observed.documentID());
  expect(observed.fill([], []).answers).toEqual([]);
});
