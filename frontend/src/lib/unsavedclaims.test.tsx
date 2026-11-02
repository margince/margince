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

// An editor whose save finishes outside any event, as a mutation's does.
function Editor({ finish }: Readonly<{ finish: { current: () => void } }>) {
  const [editing, setEditing] = useState(true);
  useUnsavedGuard(editing);
  finish.current = () => setEditing(false);
  return editing ? <button type="button">Save</button> : null;
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

describe("a claim released by the render that removes its control", () => {
  it("is already released when the control is no longer on screen", async () => {
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
