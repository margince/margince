/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { en } from "../../i18n/en";
import { parseHash } from "../../lib/router";
import { settingsHref } from "../../lib/settingsrouting";
import { MessengerAccountActions } from "./messenger.account";
import type { MessengerProvider } from "./messenger.queries";
import { MessengerStory } from "./messenger.stories.testkit";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  window.location.hash = "";
});

function account(provider: MessengerProvider, manageProviders: boolean) {
  render(
    <MessengerStory manageProviders={manageProviders} dispactConfigured={false}>
      <MessengerAccountActions provider={provider} state="unavailable" />
    </MessengerStory>,
  );
}

it.each<MessengerProvider>(["dispact", "slack", "teams"])(
  "takes an administrator to provider setup from an unavailable %s account",
  async (provider) => {
    const user = userEvent.setup();
    account(provider, true);
    await user.click(
      await screen.findByRole("button", { name: en["messenger.settings"] }),
    );
    expect(parseHash(window.location.hash)).toEqual(
      settingsHref("integrations", "telegram"),
    );
  },
);

it("explains missing setup without offering installation controls to a rep", async () => {
  account("dispact", false);
  expect(await screen.findByText(en["messenger.appMissing"])).toBeVisible();
  expect(
    screen.queryByRole("button", { name: en["messenger.settings"] }),
  ).toBeNull();
});
