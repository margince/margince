/** @vitest-environment happy-dom */
import { act, cleanup, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { en as englishCopy } from "../i18n/en";
import { navigateReplacing } from "../lib/router";
import { useUnsavedGuard } from "../lib/unsavedclaims";
import { replaceDial } from "../lib/urlstate";
import { Heading } from "./heading";
import { Modal } from "./modal";

// A dialog the shell holds (the approval and source drawers, the shortcuts
// dialog) sits outside every routed screen. No remount closes it. These cases
// draw it there. A dialog inside a routed screen is App.views.overlays.test.tsx's.

beforeEach(() => {
  globalThis.history.replaceState(null, "", "#/contacts/c1/overview");
});
afterEach(cleanup);

function Draft() {
  useUnsavedGuard(true);
  return null;
}

function ShellDialog({ draft = false }: Readonly<{ draft?: boolean }>) {
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

const dialog = () =>
  screen.queryByRole("dialog", {
    name: englishCopy["shell.shortcuts.logActivity"],
  });

describe("a dialog the shell holds, open while the screen changes", () => {
  it("closes when the reader moves to another screen", () => {
    render(<ShellDialog />);
    act(() => navigateReplacing({ screen: "deals", id: "d1" }));
    expect(dialog()).toBeNull();
  });

  it("closes when Back lands on another screen", async () => {
    globalThis.history.pushState(null, "", "#/leads");
    render(<ShellDialog />);
    const landed = new Promise<void>((done) =>
      globalThis.addEventListener("popstate", () => done(), { once: true }),
    );
    await act(async () => {
      globalThis.history.back();
      await landed;
    });
    expect(dialog()).toBeNull();
  });

  it("stays open while it turns a dial of the screen under it", () => {
    render(<ShellDialog />);
    act(() => replaceDial("stage", "won"));
    expect(dialog()).not.toBeNull();
  });

  it("stays open across a tab of the same record", () => {
    render(<ShellDialog />);
    act(() =>
      navigateReplacing({ screen: "contacts", id: "c1", id2: "deals" }),
    );
    expect(dialog()).not.toBeNull();
  });

  it("asks before it drops a draft no screen holds", () => {
    render(<ShellDialog draft />);
    act(() => navigateReplacing({ screen: "deals" }));
    expect(screen.getByText(englishCopy["unsaved.title"])).toBeTruthy();
    expect(dialog()).not.toBeNull();
  });
});
