/** @vitest-environment happy-dom */
import { en as englishCopy } from "../../i18n/en";
import "@testing-library/jest-dom/vitest";
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
import { meFixture } from "../../testing/mefixture";
import { DispactParticipantConnect } from "./messenger.dispact";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function mount(configured = true) {
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname;
      const json = (body: unknown) =>
        new Response(JSON.stringify(body), {
          headers: { "Content-Type": "application/json" },
        });
      if (path === "/v1/me")
        return json(meFixture({ allow: {}, seat: "full" }));
      if (path.endsWith("/setup"))
        return json({
          provider: "dispact",
          configured,
          callback_url: "",
          account_url: "https://chat.example.test",
          participant_scopes: ["read", "write"],
          max_group_recipients: 8,
        });
      if (
        path === "/v1/messenger/dispact/credential" &&
        request.method === "POST"
      ) {
        writes.push(await request.json());
        return new Response(null, { status: 204 });
      }
      throw new Error(`Unexpected route ${path}`);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <DispactParticipantConnect />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { writes, client };
}

it("connects a personal token through its own route and removes it from the active form and mutation", async () => {
  const { writes, client } = mount();
  const user = userEvent.setup();
  await user.click(
    await screen.findByRole("button", {
      name: englishCopy["messenger.connect"],
    }),
  );
  const dialog = await screen.findByRole("dialog");
  const token = within(dialog).getByLabelText(
    englishCopy["messenger.dispactToken"],
  );
  expect(token).toHaveAttribute("type", "password");
  await user.type(token, "pat_synthetic");
  await user.click(
    within(dialog).getByRole("button", {
      name: englishCopy["messenger.connect"],
    }),
  );
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(writes).toEqual([{ personal_token: "pat_synthetic" }]);
  await waitFor(() =>
    expect(
      client
        .getMutationCache()
        .getAll()
        .every((mutation) => mutation.state.variables === undefined),
    ).toBe(true),
  );
});

it("explains missing installation setup before asking for a personal secret", async () => {
  mount(false);
  expect(
    await screen.findByText(/An administrator needs to complete app setup/),
  ).toBeVisible();
  expect(
    screen.queryByRole("button", { name: englishCopy["messenger.connect"] }),
  ).toBeNull();
  expect(
    screen.queryByLabelText(englishCopy["messenger.dispactToken"]),
  ).toBeNull();
});
