/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { DealStatusBadge } from "./dealstatusbadge";

afterEach(cleanup);

describe("the deal status badge", () => {
  it("names each status in the reader's language", () => {
    render(
      <LocaleProvider initial="vi">
        <DealStatusBadge status="open" />
        <DealStatusBadge status="won" />
      </LocaleProvider>,
    );
    expect(screen.getByText("Đang mở")).toBeTruthy();
    expect(screen.getByText("Thắng")).toBeTruthy();
  });

  it("reads a status this build has no word for as sent", () => {
    render(
      <LocaleProvider initial="vi">
        <DealStatusBadge status="paused" />
      </LocaleProvider>,
    );
    expect(screen.getByText("paused")).toBeTruthy();
  });
});
