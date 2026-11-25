/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { en } from "../../i18n/en";
import { RecordIntroductionDialog } from "./recordintroduction";

// The server decides who may record. The form only says on press which of
// the two colleagues, the day and the channel is missing.

function stubRoster(rosterAnswers: () => boolean = () => true) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const path = typeof input === "string" ? input : (input as Request).url;
      if (path.includes("/users") && !rosterAnswers()) {
        return new Response(JSON.stringify({ title: "unavailable" }), {
          status: 503,
          headers: { "content-type": "application/problem+json" },
        });
      }
      if (path.includes("/users")) {
        return new Response(
          JSON.stringify({
            data: [{ id: "u-1", display_name: "Mira Introducer" }],
            page: { has_more: false },
          }),
          { status: 200, headers: { "content-type": "application/json" } },
        );
      }
      return new Response("{}", {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }),
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function renderDialog() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <RecordIntroductionDialog
        contactId="p-1"
        contactName="Dana"
        open
        onClose={() => undefined}
      />
    </QueryClientProvider>,
  );
}

it("says the colleagues did not load, and Retry reads them again", async () => {
  let answers = false;
  stubRoster(() => answers);
  renderDialog();

  expect(await screen.findByText(en["roster.failedUsers"])).toBeTruthy();
  answers = true;
  fireEvent.click(screen.getByRole("button", { name: en["common.retry"] }));
  await waitFor(() => {
    expect(screen.queryByText(en["roster.failedUsers"])).toBeNull();
  });
});

it("says each empty field is required on Record, and sends nothing", async () => {
  stubRoster();
  renderDialog();

  await waitFor(() => {
    expect(screen.getByText(en["contact.intro.recordHint"])).toBeTruthy();
  });
  fireEvent.click(
    screen.getByRole("button", { name: en["contact.intro.recordSubmit"] }),
  );
  expect(screen.getByText("Introduced by is required.")).toBeTruthy();
  expect(screen.getByText("Date is required.")).toBeTruthy();
  expect(screen.getByText("How it happened is required.")).toBeTruthy();
  expect(document.activeElement?.getAttribute("aria-invalid")).toBe("true");
  expect(document.activeElement?.id).toBe(
    screen.getByText("Introduced by").closest("label")?.htmlFor,
  );
  const fetched = vi
    .mocked(fetch)
    .mock.calls.map(([input]) =>
      typeof input === "string" ? input : (input as Request).url,
    );
  expect(fetched.some((url) => url.includes("introduc"))).toBe(false);
});
