/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { meFixture } from "../testing/mefixture";
import { lockScreen } from "./confinement";

function mount(node: ReturnType<typeof lockScreen>) {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = input instanceof Request ? input.url : String(input);
    if (url.includes("/installation/license")) {
      return new Response("{}", { status: 403 });
    }
    return new Response(
      JSON.stringify({ ...meFixture({ roles: ["rep"] }), trial_ended: true }),
      { headers: { "Content-Type": "application/json" } },
    );
  });
  vi.stubGlobal("fetch", fetchMock);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{node}</LocaleProvider>
    </QueryClientProvider>,
  );
  return fetchMock;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("holds a seat of an installation whose trial ended to the one screen that says so", async () => {
  const user = userEvent.setup();
  const fetchMock = mount(lockScreen(null, null, { trial_ended: true }));

  expect(await screen.findByText(en["trial.locked.title"])).toBeTruthy();
  // A seat that may not read the licence is pointed at its admin, and never
  // asks for the licence it may not read.
  expect(await screen.findByText(en["trial.locked.member"])).toBeTruthy();
  expect(
    fetchMock.mock.calls.some(([input]) =>
      String(input instanceof Request ? input.url : input).includes(
        "/installation/license",
      ),
    ),
  ).toBe(false);

  await user.click(
    screen.getByRole("button", { name: en["trial.contact.title"] }),
  );
  const dialog = await screen.findByRole("dialog");
  expect(within(dialog).getByText(en["trial.notice.endedTitle"])).toBeTruthy();
  const href =
    within(dialog)
      .getByRole("link", { name: en["trial.contact.open"] })
      .getAttribute("href") ?? "";
  // With no day to name, the email names none rather than an empty date.
  expect(new URL(href).searchParams.get("body")).not.toContain("Trial ended:");
});

it("leaves an installation still on its trial, and a refused session, to the rest of the app", () => {
  expect(lockScreen(null, null, { trial_ended: false })).toBeNull();
  expect(lockScreen(null, null, {})).toBeNull();
  expect(
    lockScreen(null, new Error("signed out"), { trial_ended: true }),
  ).toBeNull();
});

it("lets an admin who may change the installation check the licence again from the lock", async () => {
  const user = userEvent.setup();
  const reads: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = input instanceof Request ? input.url : String(input);
      reads.push(url);
      const body = url.includes("/installation/license")
        ? { state: "valid", seats_used: 3, over_limit: false }
        : {
            ...meFixture({
              allow: {
                license: ["read"],
                installation_settings: ["update"],
              },
            }),
            trial_ended: true,
          };
      return new Response(JSON.stringify(body), {
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        {lockScreen(null, null, { trial_ended: true })}
      </LocaleProvider>
    </QueryClientProvider>,
  );
  const meReads = () => reads.filter((url) => url.endsWith("/me")).length;

  const check = await screen.findByRole("button", {
    name: en["license.check.button"],
  });
  const before = meReads();
  await user.click(check);

  expect(await screen.findByText(en["license.check.done"])).toBeTruthy();
  // /me is what holds the seat on this screen, so the check reads it again.
  await vi.waitFor(() => expect(meReads()).toBeGreaterThan(before));
  expect(reads.some((url) => url.includes("/installation/license/check"))).toBe(
    true,
  );
});
