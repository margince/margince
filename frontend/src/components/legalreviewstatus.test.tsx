// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import type { LegalReviewSummary } from "../lib/legalreviews.queries";
import { LegalReviewStatusBadge } from "./legalreviewstatus";

describe("LegalReviewStatusBadge", () => {
  it("names a status this build does not know with the neutral label", () => {
    render(
      <LocaleProvider initial="en">
        <LegalReviewStatusBadge
          status={"escalated_later" as LegalReviewSummary["status"]}
        />
      </LocaleProvider>,
    );
    expect(screen.getByText(en["common.unknownValue"])).toBeTruthy();
  });
});
