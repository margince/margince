// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { act, cleanup, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PageHeadAction } from "../components/pageheadaction";
import { navigate, useRoute } from "../lib/router";
import {
  ignoreSearch,
  newClient,
  render,
  renderWith,
} from "../testing/shellharness";
import { Shell } from "./shell";

// The chrome's own reads never answer: what is asked here is where the verb draws.
beforeEach(() => {
  vi.stubGlobal("fetch", () => new Promise<Response>(() => undefined));
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

// A screen whose verb belongs to one page only, as Members' invite does.
function Screen({ verbOn }: Readonly<{ verbOn: string }>) {
  const route = useRoute();
  return (
    <div data-testid="page">
      {route.screen === verbOn && (
        <PageHeadAction>
          <button type="button">Export</button>
        </PageHeadAction>
      )}
    </div>
  );
}

function pageHead(): HTMLElement {
  const head = document.querySelector(".pagetitle");
  if (!(head instanceof HTMLElement)) {
    throw new Error("the shell drew no page head");
  }
  return head;
}

describe("PageHeadAction under the shell", () => {
  it("draws the page's verb in the head, not in the page that declares it", async () => {
    window.location.hash = "#/analytics";
    render(
      <Shell onOpenSearch={ignoreSearch}>
        <Screen verbOn="analytics" />
      </Shell>,
    );
    expect(
      await within(pageHead()).findByRole("button", { name: "Export" }),
    ).toBeTruthy();
    expect(within(screen.getByTestId("page")).queryByRole("button")).toBeNull();
  });

  it("takes the verb out of the head when the reader leaves its page", async () => {
    window.location.hash = "#/analytics";
    render(
      <Shell onOpenSearch={ignoreSearch}>
        <Screen verbOn="analytics" />
      </Shell>,
    );
    await within(pageHead()).findByRole("button", { name: "Export" });

    act(() => navigate({ screen: "filters" }));

    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "Export" })).toBeNull(),
    );
    expect(
      pageHead().querySelector(".pagehead-action")?.childElementCount,
    ).toBe(0);
  });

  // A record names itself, so the shell draws no page head and no slot.
  it("draws the verb in the page where the shell has no head to give it", async () => {
    window.location.hash = "#/contacts/p-anna";
    const client = newClient();
    client.setQueryData(["contact", "ref", "p-anna"], "Anna Weber");
    renderWith(
      client,
      <Shell onOpenSearch={ignoreSearch}>
        <Screen verbOn="contacts" />
      </Shell>,
    );
    expect(document.querySelector(".pagetitle")).toBeNull();
    const verb = within(screen.getByTestId("page")).getByRole("button", {
      name: "Export",
    });
    expect(verb.parentElement?.className).toBe("pagehead-action");
  });
});
