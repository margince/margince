/** @vitest-environment happy-dom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import {
  UnsavedContext,
  useUnsavedGuard,
  useUnsavedRegistry,
} from "./unsavedclaims";

afterEach(cleanup);

// The claim's owner, mounted for as long as the reader is editing.
function Claimant() {
  useUnsavedGuard(true);
  return <button type="button">Save</button>;
}

// An editor whose save finishes outside any event, as a mutation's does.
function Editor({ finish }: Readonly<{ finish: { current: () => void } }>) {
  const [editing, setEditing] = useState(true);
  finish.current = () => setEditing(false);
  return editing ? <Claimant /> : null;
}

function Host({
  finish,
  seen,
}: Readonly<{
  finish: { current: () => void };
  seen: { dirty: boolean | null };
}>) {
  const registry = useUnsavedRegistry(null);
  // What an Escape in the frame this render paints would read.
  seen.dirty = registry.dirty;
  return (
    <UnsavedContext.Provider value={registry}>
      <Editor finish={finish} />
    </UnsavedContext.Provider>
  );
}

describe("a claim released when its owner unmounts", () => {
  it("is released before the control is gone from the screen", async () => {
    const finish = { current: () => {} };
    const seen: { dirty: boolean | null } = { dirty: null };
    render(<Host finish={finish} seen={seen} />);
    await waitFor(() => expect(seen.dirty).toBe(true));

    let dirtyWhenGone: boolean | null = null;
    const watcher = new MutationObserver(() => {
      if (!screen.queryByRole("button", { name: "Save" })) {
        dirtyWhenGone = seen.dirty;
        watcher.disconnect();
      }
    });
    watcher.observe(document.body, { childList: true, subtree: true });

    finish.current();
    await waitFor(() => expect(dirtyWhenGone).not.toBeNull());

    expect(dirtyWhenGone).toBe(false);
  });
});
