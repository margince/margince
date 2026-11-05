// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
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
    expect(source.classList).toContain("btn-align-start");
  });
});
