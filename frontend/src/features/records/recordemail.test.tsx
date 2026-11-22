// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { armHoverIntent } from "../../design-system/hoverintent-testing";
import { LocaleProvider } from "../../i18n";
import { RecordEmailVerb } from "./recordemail";

afterEach(() => {
  cleanup();
});

describe("the header's Email verb refused by the page", () => {
  const WHY = "You do not have permission to log activities on this record.";

  it("shows the page's sentence in its tooltip and is described by that sentence alone", async () => {
    const user = userEvent.setup();
    armHoverIntent();
    render(
      <QueryClientProvider client={new QueryClient()}>
        <LocaleProvider initial="en">
          <RecordEmailVerb
            entityType="deal"
            entityId="deal-1"
            disabledReasonId="page-why"
            reasonTip={WHY}
          />
          <p id="page-why">{WHY}</p>
        </LocaleProvider>
      </QueryClientProvider>,
    );
    const email = screen.getByRole("button", { name: "Email" });
    expect(email).toHaveProperty("disabled", true);
    expect(email.getAttribute("aria-describedby")).toBe("page-why");
    expect(screen.getAllByText(WHY)).toHaveLength(1);

    const host = email.closest(".icon-action");
    if (!(host instanceof HTMLElement)) {
      throw new Error("the Email verb lost its tooltip host");
    }
    await user.hover(host);
    expect((await screen.findByRole("tooltip")).textContent).toBe(
      `Email. ${WHY}`,
    );
  });
});
