// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */

import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import type { Receipt } from "../lib/handled";
import { OpenSourceProvider } from "../lib/opensource";
import { automaticDateReceipt } from "../testing/worklist.receiptreview.fixtures";
import { ReceiptDetail } from "./receiptdetail";

afterEach(cleanup);

describe("a receipt's source", () => {
  it("marks its name as data aligned to the start, so a long one wraps inside its link", () => {
    render(
      <LocaleProvider initial="en">
        <OpenSourceProvider openSource={vi.fn()}>
          <ReceiptDetail
            receipt={{
              ...automaticDateReceipt,
              source_record: {
                type: "activity",
                id: "01a05500-0000-7000-8000-00000000a301",
                label:
                  "Quarterly renewal call with the procurement team of Nordlicht Logistik",
              },
            }}
          />
        </OpenSourceProvider>
      </LocaleProvider>,
    );

    const source = screen.getByRole("button", {
      name: /Quarterly renewal call/,
    });

    expect(source.classList).toContain("btn-valuelabel");
    expect(source.dataset.align).toBe("start");
  });
});

function opened(receipt: Receipt) {
  render(
    <LocaleProvider initial="en">
      <OpenSourceProvider openSource={vi.fn()}>
        <ReceiptDetail receipt={receipt} />
      </OpenSourceProvider>
    </LocaleProvider>,
  );
  // The facts are a term list; each term's value is the next element.
  return (term: string) =>
    screen.queryByText(term, { selector: "dt" })?.nextElementSibling
      ?.textContent ?? null;
}

const call = {
  type: "activity" as const,
  id: "01a05500-0000-7000-8000-00000000a302",
  label: "Renewal call",
};

describe("when the source came", () => {
  it("dates the message Margince read apart from when it acted", () => {
    const factOf = opened({
      ...automaticDateReceipt,
      occurred_at: "2026-09-13T02:00:00Z",
      source_record: call,
      source_occurred_at: "2026-09-10T15:30:00Z",
    });
    const read = factOf(en["receipt.detail.sourceAt"]);
    const acted = factOf(en["receipt.detail.when"]);
    expect(read).toMatch(/10/);
    expect(acted).toMatch(/13/);
    expect(read).not.toBe(acted);
  });

  it("gives a source known only to the day its date alone", () => {
    const factOf = opened({
      ...automaticDateReceipt,
      source_record: call,
      source_occurred_at: "2026-09-10T12:00:00Z",
      source_occurred_precision: "day",
    });
    const read = factOf(en["receipt.detail.sourceAt"]);
    expect(read).toMatch(/10/);
    expect(read).not.toMatch(/\d:\d\d/);
  });

  it("dates nothing when the reader may not open the source", () => {
    const factOf = opened({
      ...automaticDateReceipt,
      source_record: { ...call, label: undefined },
      source_occurred_at: "2026-09-10T15:30:00Z",
    });
    expect(factOf(en["receipt.detail.sourceAt"])).toBeNull();
  });
});

describe("the kind of reason", () => {
  it.each([
    ["rule", en["receipt.detail.reasonKind.rule"]],
    ["inference", en["receipt.detail.reasonKind.inference"]],
  ] as const)("names a %s as such", (kind, words) => {
    const factOf = opened({ ...automaticDateReceipt, reason_kind: kind });
    expect(factOf(en["receipt.detail.reasonKind"])).toBe(words);
  });

  it("says nothing of a kind this build does not know", () => {
    const factOf = opened({
      ...automaticDateReceipt,
      reason_kind: "guess" as Receipt["reason_kind"],
    });
    expect(factOf(en["receipt.detail.reasonKind"])).toBeNull();
    expect(within(document.body).queryByText("guess")).toBeNull();
  });
});
