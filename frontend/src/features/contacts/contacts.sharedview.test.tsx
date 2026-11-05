// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import {
  emptyPage,
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ContactsScreen } from "./contacts";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

it("asks for the contacts shared with the reader, beside Mine", async () => {
  const listed: string[] = [];
  installFetchStub({ "GET /me": meRoute({ contact: ["read"] }) }, () =>
    jsonResponse(emptyPage),
  );
  const stubbed = globalThis.fetch;
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = input instanceof Request ? input.url : String(input);
    if (new URL(url, "https://test.local").pathname.endsWith("/contacts")) {
      listed.push(url);
    }
    return stubbed(input, init);
  }) as typeof fetch;
  const user = userEvent.setup();
  render(
    <StoryProviders>
      <ContactsScreen />
    </StoryProviders>,
  );
  await user.click(
    await screen.findByRole("button", { name: "Shared with me" }),
  );
  await waitFor(() => expect(listed.at(-1)).toContain("shared_with_me=true"));
  expect(listed.at(-1)).not.toContain("owner_id=");
  expect(
    await screen.findByText(/No contacts shared with you\./),
  ).toBeVisible();
});
