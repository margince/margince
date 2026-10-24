/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { en } from "../../i18n/en";
import { RecordIntroductionDialog } from "./recordintroduction";

// The server decides who may record, so the form's own job is to hold the
// submit back until the two colleagues, the day and the channel are named.

function stubRoster() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const path = typeof input === "string" ? input : (input as Request).url;
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

it("holds the record button back until the form is complete", async () => {
  stubRoster();
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

  await waitFor(() => {
    expect(screen.getByText(en["contact.intro.recordHint"])).toBeTruthy();
  });
  const submit = screen.getByRole("button", {
    name: en["contact.intro.recordSubmit"],
  }) as HTMLButtonElement;
  expect(submit.disabled).toBe(true);
});
