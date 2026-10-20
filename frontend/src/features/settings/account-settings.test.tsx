/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { AccountSettings } from "./account-settings";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("says so under Language when the account refuses the new language", async () => {
  const user = userEvent.setup();
  installFetchStub({
    "GET /me": () => jsonResponse(meFixture({})),
    "PUT /me/locale": () =>
      new Response(
        JSON.stringify({
          status: 503,
          code: "service_unavailable",
          detail: "The language could not be saved.",
        }),
        {
          status: 503,
          headers: { "content-type": "application/problem+json" },
        },
      ),
  });
  render(
    <StoryProviders>
      <AccountSettings />
    </StoryProviders>,
  );
  const language = await screen.findByRole("combobox", { name: "Language" });
  const before = language.getAttribute("aria-describedby");
  await pickOption(user, language, "Deutsch");
  const refusal = await screen.findByRole("alert");
  expect(before).not.toContain(refusal.id);
  expect(screen.getByRole("combobox", { name: "Sprache" })).toHaveAttribute(
    "aria-describedby",
    `${before} ${refusal.id}`,
  );
});
