// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
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

it("shows a day-only source date apart from the time Margince acted", () => {
  render(
    <LocaleProvider initial="en">
      <ReceiptDetail
        receipt={{
          ...automaticDateReceipt,
          source_record: {
            type: "activity",
            id: "source",
            label: "Renewal call",
          },
          source_occurred_at: "2026-07-15T12:00:00Z",
          source_occurred_precision: "day",
        }}
      />
    </LocaleProvider>,
  );
  const sourceDate =
    screen.getByText("Source date").nextElementSibling?.textContent;
  expect(sourceDate).toContain("2026");
  expect(sourceDate).toContain("15");
  expect(sourceDate).not.toContain("12:00");
  expect(screen.getByText("When")).toBeTruthy();
});

it("withholds a source date when the source has no readable label", () => {
  render(
    <LocaleProvider initial="en">
      <ReceiptDetail
        receipt={{
          ...automaticDateReceipt,
          source_record: { type: "activity", id: "source" },
          source_occurred_at: "2026-07-15T12:00:00Z",
          source_occurred_precision: "day",
        }}
      />
    </LocaleProvider>,
  );
  expect(screen.queryByText("Source date")).toBeNull();
});

it.each<{ kind?: Receipt["reason_kind"]; label: string }>([
  { kind: "system_rule", label: "System rule" },
  { kind: "inference", label: "AI assessment" },
])(
  "shows the recorded reason kind $kind without guessing from its words",
  ({ kind, label }) => {
    render(
      <LocaleProvider initial="en">
        <ReceiptDetail
          receipt={{
            ...automaticDateReceipt,
            reason: "The next date follows the stage history.",
            reason_kind: kind,
          }}
        />
      </LocaleProvider>,
    );
    expect(screen.getByText("Reason type")).toBeTruthy();
    expect(screen.getByText(label)).toBeTruthy();
  },
);

it("keeps ordinary receipt reasons free of a missing correction classification", () => {
  render(
    <LocaleProvider initial="en">
      <ReceiptDetail
        receipt={{ ...automaticDateReceipt, reason: "Task completed." }}
      />
    </LocaleProvider>,
  );
  expect(screen.queryByText("Reason type")).toBeNull();
  expect(screen.queryByText("Not recorded")).toBeNull();
});
