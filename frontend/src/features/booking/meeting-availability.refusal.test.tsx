// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PageSaveBar, SavePageProvider } from "../../design-system/savepage";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { AvailabilityCard } from "./meeting-availability";

// A refused save of the hours is shown at the field the server named. The save
// bar is pinned over the foot of the page and would cover the sentence.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const HOURS = {
  chosen: true,
  working_hours: {
    start_time: "09:00",
    end_time: "17:00",
    days: [1, 2, 3, 4, 5],
    timezone: "Europe/Berlin",
  },
};

function refusing(field: string, message: string) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const req =
      input instanceof Request ? input : new Request(String(input), init);
    if (req.url.endsWith("/v1/me")) return jsonResponse(meFixture({}));
    if (req.method === "PUT") {
      return jsonResponse(
        {
          title: "Unprocessable Entity",
          status: 422,
          code: "validation_error",
          detail: message,
          details: { errors: [{ field, code: "invalid", message }] },
        },
        422,
      );
    }
    return jsonResponse(HOURS);
  });
}

async function saveRefused(field: string, message: string) {
  vi.stubGlobal("fetch", refusing(field, message));
  const user = userEvent.setup();
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <SavePageProvider>
          <AvailabilityCard hoursOnly />
          <PageSaveBar />
        </SavePageProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  await user.click(await screen.findByLabelText("Saturday"));
  await user.click(screen.getByRole("button", { name: "Save" }));
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("a refused save of the working hours", () => {
  it("marks the field the server named and says why beside it", async () => {
    await saveRefused("end_time", "the working day must end after it starts");

    const sentence = await screen.findByText(
      "The working day must end after it starts.",
    );
    const until = screen.getByLabelText("Until");
    expect(until).toHaveAttribute("aria-invalid", "true");
    expect(until.getAttribute("aria-describedby")).toContain(sentence.id);
    // Brought out from under the bar, with focus on it.
    await waitFor(() => expect(until).toHaveFocus());
    // The bar keeps to its short line rather than repeating the sentence.
    expect(screen.getByText("Fix the marked fields to save.")).not.toBeNull();
    expect(
      screen.getAllByText(/working day must end after it starts/i),
    ).toHaveLength(1);
  });

  it("keeps a refusal no field here can hold on the bar", async () => {
    await saveRefused("days", "choose at least one day you work");

    expect(
      await screen.findByText("choose at least one day you work"),
    ).not.toBeNull();
    expect(screen.queryByText("Fix the marked fields to save.")).toBeNull();
    expect(screen.getByLabelText("Until")).not.toHaveAttribute("aria-invalid");
  });
});
