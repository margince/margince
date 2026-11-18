// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ScreenView } from "./App.views";
import { Heading } from "./design-system/heading";
import { Modal } from "./design-system/modal";
import { LocaleProvider } from "./i18n";
import { useRoute } from "./lib/router";
import { useUnsavedGuard } from "./lib/unsavedclaims";
import { meFixture } from "./testing/mefixture";

// A dialog a routed screen holds leaves with the screen: the screen is keyed by
// what it shows. Its own page check must not close it a second time.

const closed = vi.fn();

vi.mock("./App.screens", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./App.screens")>()),
  ContactsScreen: () => <ScreenWithDialog />,
  CompaniesScreen: () => <p>Companies</p>,
}));

function Draft() {
  const [typed, setTyped] = useState("");
  useUnsavedGuard(typed !== "");
  return (
    <label>
      Note
      <input value={typed} onChange={(event) => setTyped(event.target.value)} />
    </label>
  );
}

function ScreenWithDialog() {
  const [open, setOpen] = useState(true);
  return (
    <Modal
      open={open}
      onClose={() => {
        closed();
        setOpen(false);
      }}
      labelledBy="t"
      intent="form"
    >
      <Heading size="large" id="t">
        Log activity
      </Heading>
      <Draft />
    </Modal>
  );
}

// The route as App hands it to ScreenView.
function Routed() {
  const route = useRoute();
  return <ScreenView screen={route.screen} id={route.id} id2={route.id2} />;
}

function renderRouted(at: string) {
  globalThis.history.replaceState(null, "", at);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(["me"], meFixture({}));
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <Routed />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

async function moveTo(hash: string) {
  await act(async () => {
    globalThis.history.pushState(null, "", hash);
    globalThis.dispatchEvent(new PopStateEvent("popstate"));
  });
}

const dialog = () => screen.queryByRole("dialog", { name: "Log activity" });

afterEach(() => {
  cleanup();
  closed.mockReset();
  globalThis.history.replaceState(null, "", "#/");
});

describe("a dialog a routed screen holds, open while the reader moves", () => {
  it("goes with the screen, closed once", async () => {
    renderRouted("#/contacts");
    expect(await screen.findByRole("dialog")).toBeInTheDocument();

    await moveTo("#/companies");

    expect(await screen.findByText("Companies")).toBeInTheDocument();
    expect(dialog()).toBeNull();
    expect(closed).not.toHaveBeenCalled();
  });

  it("stays under the one leave question while a draft holds the screen", async () => {
    const user = userEvent.setup();
    renderRouted("#/contacts");
    await user.type(await screen.findByLabelText("Note"), "Called back");

    await moveTo("#/companies");

    expect(
      await screen.findByRole("dialog", { name: "Discard unsaved changes?" }),
    ).toBeInTheDocument();
    expect(
      screen.getAllByRole("dialog", { name: "Discard unsaved changes?" }),
    ).toHaveLength(1);
    expect(dialog()).toBeInTheDocument();
    expect(closed).not.toHaveBeenCalled();
  });

  it("keeps the leave question open when the reader moves again", async () => {
    const user = userEvent.setup();
    renderRouted("#/contacts");
    await user.type(await screen.findByLabelText("Note"), "Called back");
    await moveTo("#/companies");
    await screen.findByRole("dialog", { name: "Discard unsaved changes?" });

    await moveTo("#/deals");

    expect(
      screen.getByRole("dialog", { name: "Discard unsaved changes?" }),
    ).toBeInTheDocument();
    expect(globalThis.location.hash).toBe("#/deals");
  });
});
