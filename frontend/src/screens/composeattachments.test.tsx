/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { CarriageNotice } from "./composeattachments";

describe("CarriageNotice", () => {
  it.each([
    [1, "its 1 attachment cannot be sent there"],
    [3, "its 3 attachments cannot be sent there"],
  ])("counts %i attachment(s) on a channel without files", (count, phrase) => {
    render(
      <LocaleProvider initial="en">
        <CarriageNotice channel="SMS" blocks={[{ kind: "carries", count }]} />
      </LocaleProvider>,
    );
    expect(screen.getByRole("listitem")).toHaveTextContent(phrase);
  });
});
