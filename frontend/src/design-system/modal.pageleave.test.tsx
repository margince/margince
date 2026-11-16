/** @vitest-environment happy-dom */
import { act, cleanup, render, screen } from "@testing-library/react";
import { type ReactNode, useState } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { navigateReplacing } from "../lib/router";
import { ScreenAddress } from "../lib/screenaddress";
import {
  UnsavedContext,
  useUnsavedGuard,
  useUnsavedRegistry,
} from "../lib/unsavedclaims";
import { replaceDial } from "../lib/urlstate";
import { Heading } from "./heading";
import { Modal } from "./modal";

// A dialog held in page state has no address of its own. The page moving under
// it is the only sign it now covers a different view.

beforeEach(() => {
  globalThis.history.replaceState(null, "", "#/contacts/c1/overview");
});
afterEach(cleanup);

function Draft() {
  useUnsavedGuard(true);
  return null;
}

function StateHeldDialog({ draft = false }: Readonly<{ draft?: boolean }>) {
  const [open, setOpen] = useState(true);
  return (
    <Modal
      open={open}
      onClose={() => setOpen(false)}
      labelledBy="t"
      intent="form"
    >
      <Heading size="large" id="t">
        Log activity
      </Heading>
      {draft && <Draft />}
    </Modal>
  );
}

function Page({ children }: Readonly<{ children: ReactNode }>) {
  const registry = useUnsavedRegistry(null);
  return (
    <UnsavedContext.Provider value={registry}>
      {children}
    </UnsavedContext.Provider>
  );
}

const dialog = () => screen.queryByRole("dialog", { name: "Log activity" });

describe("a dialog open while the page changes", () => {
  it("closes when the reader moves to another page", () => {
    render(<StateHeldDialog />);
    act(() => navigateReplacing({ screen: "deals", id: "d1" }));
    expect(dialog()).toBeNull();
  });

  it("closes when Back lands on another page", async () => {
    globalThis.history.pushState(null, "", "#/leads");
    render(<StateHeldDialog />);
    const landed = new Promise<void>((done) =>
      globalThis.addEventListener("popstate", () => done(), { once: true }),
    );
    await act(async () => {
      globalThis.history.back();
      await landed;
    });
    expect(dialog()).toBeNull();
  });

  it("stays open while it turns a dial of the page it stands on", () => {
    render(<StateHeldDialog />);
    act(() => replaceDial("stage", "won"));
    expect(dialog()).not.toBeNull();
  });

  it("stays open across a tab of the same record", () => {
    render(<StateHeldDialog />);
    act(() =>
      navigateReplacing({ screen: "contacts", id: "c1", id2: "deals" }),
    );
    expect(dialog()).not.toBeNull();
  });

  it("stays over a screen still showing the page it opened on", () => {
    render(
      <ScreenAddress.Provider value="#/contacts/c1/overview">
        <StateHeldDialog />
      </ScreenAddress.Provider>,
    );
    act(() => navigateReplacing({ screen: "deals" }));
    expect(dialog()).not.toBeNull();
  });

  it("leaves a draft the page holds to the page's own question", () => {
    render(
      <Page>
        <StateHeldDialog draft />
      </Page>,
    );
    act(() => navigateReplacing({ screen: "deals" }));
    expect(dialog()).not.toBeNull();
    expect(screen.queryByText("Discard unsaved changes?")).toBeNull();
  });

  it("asks before it drops a draft no page holds", () => {
    render(<StateHeldDialog draft />);
    act(() => navigateReplacing({ screen: "deals" }));
    expect(screen.getByText("Discard unsaved changes?")).toBeTruthy();
    expect(dialog()).not.toBeNull();
  });
});
