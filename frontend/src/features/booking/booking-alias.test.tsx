/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { bookingProfile } from "../../testing/book.testkit";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "../../testing/story-utils";
import { BookingAliasField } from "./booking-alias";
import { ReplaceAddressConfirm } from "./meeting-link-drawer";

type Profile = components["schemas"]["SchedulingProfile"];

function mount(profile: Profile, routes: RouteMap = {}) {
  const writes: unknown[] = [];
  installFetchStub({
    "PUT /scheduling/profile/alias": (body) => {
      writes.push(body);
      const alias = (body as { alias: string }).alias;
      return jsonResponse({
        ...profile,
        alias: alias || undefined,
        public_url: `https://crm.example.test/book/${alias || profile.slug}`,
      });
    },
    ...routes,
  });
  render(
    <StoryProviders>
      <BookingAliasField profile={profile} />
    </StoryProviders>,
  );
  return writes;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The server answers an aliased profile with the alias as its public address.
const withAlias: Profile = {
  ...bookingProfile,
  alias: "ada",
  public_url: "https://crm.example.test/book/ada",
};

const field = () => screen.getByRole("textbox", { name: "Short address" });

it("explains that a short address is easy to guess before anything is saved", () => {
  mount(bookingProfile);
  expect(
    screen.getByText(
      /easy to guess. Anyone who guesses it sees your booking page/,
    ),
  ).toBeTruthy();
});

it("saves a first short address as the server will store it", async () => {
  const user = userEvent.setup();
  const writes = mount(bookingProfile);
  await user.type(field(), " Ada-Sales ");
  // Only the path: the whole address is the server's to give.
  expect(
    screen.getByText("Your link will end in /book/ada-sales"),
  ).toBeTruthy();
  expect(screen.queryByText(/crm\.example\.test\/book\/ada-sales/)).toBeNull();
  await user.click(screen.getByRole("button", { name: "Save address" }));
  await waitFor(() => expect(writes).toEqual([{ alias: "ada-sales" }]));
  expect(screen.queryByRole("dialog")).toBeNull();
});

it("asks before a rename, because the old address stops working at once", async () => {
  const user = userEvent.setup();
  const writes = mount(withAlias);
  await user.clear(field());
  await user.type(field(), "ada-lovelace");
  await user.click(screen.getByRole("button", { name: "Save address" }));
  const dialog = await screen.findByRole("dialog");
  expect(dialog.textContent).toContain(
    "https://crm.example.test/book/ada stops working at once",
  );
  expect(dialog.textContent).toContain("Nobody else can ever take it");
  expect(writes).toEqual([]);
  await user.click(
    screen
      .getAllByRole("button", { name: "Save address" })
      .at(-1) as HTMLElement,
  );
  await waitFor(() => expect(writes).toEqual([{ alias: "ada-lovelace" }]));
});

it("asks before removing a short address and then clears it", async () => {
  const user = userEvent.setup();
  const writes = mount(withAlias);
  await user.click(
    screen.getByRole("button", { name: "Remove short address" }),
  );
  await screen.findByRole("dialog", { name: "Remove your short address?" });
  await user.click(
    screen
      .getAllByRole("button", { name: "Remove short address" })
      .at(-1) as HTMLElement,
  );
  await waitFor(() => expect(writes).toEqual([{ alias: "" }]));
});

it("shows the server's refusal on the field in the reader's words", async () => {
  const user = userEvent.setup();
  mount(bookingProfile, {
    "PUT /scheduling/profile/alias": () =>
      jsonResponse(
        {
          status: 422,
          title: "Validation failed",
          code: "validation_error",
          details: {
            errors: [
              {
                field: "alias",
                code: "taken",
                message: "Another booking page uses this address",
              },
            ],
          },
        },
        422,
      ),
  });
  await user.type(field(), "sales");
  await user.click(screen.getByRole("button", { name: "Save address" }));
  expect(
    await screen.findByText(
      "Another booking page uses or has used this address. Choose another one.",
    ),
  ).toBeTruthy();
});

it("says before Replace address that the short address stops working too", async () => {
  installFetchStub({
    "GET /scheduling/profile": () => jsonResponse(withAlias),
  });
  render(
    <StoryProviders>
      <ReplaceAddressConfirm open onClose={vi.fn()} />
    </StoryProviders>,
  );
  expect(
    await screen.findByText(
      "Your short address https://crm.example.test/book/ada stops working too. You can choose it again afterwards.",
    ),
  ).toBeTruthy();
});

it("names the current address by its path when no public address is configured", async () => {
  const user = userEvent.setup();
  mount({ ...withAlias, public_url: undefined });
  await user.click(
    screen.getByRole("button", { name: "Remove short address" }),
  );
  const dialog = await screen.findByRole("dialog");
  expect(dialog.textContent).toContain("/book/ada stops working at once");
  expect(dialog.textContent).not.toContain("https://");
});
