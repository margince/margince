/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { StandingExceptions } from "./standingexceptions";

const row = {
  override_id: "exception-1",
  category: "requested_followup",
  reason: "Asked for next steps.",
  authority: "user",
  saved_at: "2026-06-12T10:00:00Z",
  recorded_at: "2026-06-12T10:00:00Z",
  recorded_by_name: "Alex",
  sending_entity_name: "Example Company",
  may_revoke: true,
};
function mount() {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({
          defaultOptions: {
            queries: { retry: false },
            mutations: { retry: false },
          },
        })
      }
    >
      <LocaleProvider initial="en">
        <StandingExceptions contactId="p-1" />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

it("shows company, purpose and author, and requires a reason to revoke exactly that exception", async () => {
  const user = userEvent.setup();
  let revoked = false;
  const revoke = vi.fn((_body: unknown) => {
    revoked = true;
    return new Response(null, { status: 204 });
  });
  installFetchStub({
    "GET /contacts/p-1/consent/allow": () =>
      jsonResponse({ data: revoked ? [] : [row], page: { has_more: false } }),
    "POST /contacts/p-1/consent/allow/exception-1/revoke": revoke,
  });
  mount();
  expect(
    await screen.findByText("Sending company: Example Company"),
  ).toBeTruthy();
  expect(screen.getByText(/Recorded by Alex/)).toBeTruthy();
  expect(screen.getByText(/It grants no marketing consent/)).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Revoke exception" }));
  const dialog = await screen.findByRole("dialog");
  const confirm = within(dialog).getByRole("button", {
    name: "Revoke exception",
  });
  expect(confirm.hasAttribute("disabled")).toBe(true);
  await user.type(within(dialog).getByRole("textbox"), "Wrong context.");
  await user.click(confirm);
  await waitFor(() => expect(revoke).toHaveBeenCalledTimes(1));
  expect(revoke.mock.calls[0]?.[0]).toEqual({ reason: "Wrong context." });
  await waitFor(() =>
    expect(screen.queryByText("Asked for next steps.")).toBeNull(),
  );
});

it("keeps historical company scope unknown and offers no action without authority", async () => {
  installFetchStub({
    "GET /contacts/p-1/consent/allow": () =>
      jsonResponse({
        data: [{ ...row, sending_entity_name: null, may_revoke: false }],
        page: { has_more: false },
      }),
  });
  mount();
  expect(await screen.findByText(/Sending company not recorded/)).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Revoke exception" })).toBeNull();
});

it("retains the reason when revocation fails", async () => {
  const user = userEvent.setup();
  installFetchStub({
    "GET /contacts/p-1/consent/allow": () =>
      jsonResponse({ data: [row], page: { has_more: false } }),
    "POST /contacts/p-1/consent/allow/exception-1/revoke": () =>
      jsonResponse(
        { title: "Permission changed.", detail: "Permission changed." },
        403,
      ),
  });
  mount();
  await user.click(
    await screen.findByRole("button", { name: "Revoke exception" }),
  );
  const dialog = await screen.findByRole("dialog");
  await user.type(within(dialog).getByRole("textbox"), "Keep this reason.");
  await user.click(
    within(dialog).getByRole("button", { name: "Revoke exception" }),
  );
  await screen.findByText("Permission changed.");
  expect(
    within(dialog).getByRole("textbox").getAttribute("value") ??
      within(dialog).getByRole("textbox").textContent,
  ).toBe("Keep this reason.");
});

it("reads the next page instead of hiding older exceptions", async () => {
  let calls = 0;
  installFetchStub({
    "GET /contacts/p-1/consent/allow": () =>
      jsonResponse(
        ++calls === 1
          ? { data: [row], page: { has_more: true, next_cursor: "older" } }
          : {
              data: [
                { ...row, override_id: "older", reason: "Older request." },
              ],
              page: { has_more: false },
            },
      ),
  });
  const user = userEvent.setup();
  mount();
  await user.click(await screen.findByRole("button", { name: "Load more" }));
  expect(await screen.findByText("Older request.")).toBeTruthy();
  expect(screen.getByText("Asked for next steps.")).toBeTruthy();
});

it("does not attribute an erased origin to the person who saved a merged copy", async () => {
  installFetchStub({
    "GET /contacts/p-1/consent/allow": () =>
      jsonResponse({
        data: [{ ...row, recorded_at: null, recorded_by_name: null }],
        page: { has_more: false },
      }),
  });
  mount();
  expect(
    await screen.findByText("Original author and date unavailable."),
  ).toBeTruthy();
  expect(screen.queryByText(/Recorded by Alex/)).toBeNull();
});

it("distinguishes a known company with no name from a missing company scope", async () => {
  installFetchStub({
    "GET /contacts/p-1/consent/allow": () =>
      jsonResponse({
        data: [
          { ...row, sending_entity_id: "company-1", sending_entity_name: null },
        ],
        page: { has_more: false },
      }),
  });
  mount();
  expect(await screen.findByText("Sending company name not set.")).toBeTruthy();
  expect(screen.queryByText("Sending company not recorded.")).toBeNull();
});
