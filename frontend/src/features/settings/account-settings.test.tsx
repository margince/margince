/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { PageSaveBar, SavePageProvider } from "../../design-system/savepage";
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

it("says so on the save bar and under the field when the account refuses the new language", async () => {
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
      <SavePageProvider>
        <AccountSettings />
        <PageSaveBar />
      </SavePageProvider>
    </StoryProviders>,
  );
  const language = await screen.findByRole("combobox", { name: "Language" });
  await pickOption(user, language, "Deutsch");
  expect(language).toHaveTextContent("Deutsch");
  await user.click(await screen.findByRole("button", { name: "Save" }));
  const alerts = await screen.findAllByRole("alert");
  for (const alert of alerts) {
    expect(alert).toHaveTextContent("The language could not be saved.");
  }
  // The field names its own refusal beside the bar's summary.
  const named = (language.getAttribute("aria-describedby") ?? "").split(" ");
  expect(alerts.some((alert) => named.includes(alert.id))).toBe(true);
});
