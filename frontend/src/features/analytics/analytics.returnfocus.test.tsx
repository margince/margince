/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useRef } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { forgetScrollMemory } from "../../lib/scrollmemory";
import { forgetReturnFocus, useReturnFocus } from "./analytics.returnfocus";

afterEach(() => {
  cleanup();
  forgetReturnFocus();
  forgetScrollMemory();
  history.replaceState(null, "");
});

// A surface whose controls arrive after it mounts, as figures do.
function Surface({ ready }: Readonly<{ ready: boolean }>) {
  const surface = useRef<HTMLDivElement>(null);
  useReturnFocus(surface);
  return (
    <div ref={surface}>
      {ready && (
        <>
          <span id="won-label">Sales won</span>
          <button type="button" aria-describedby="won-label">
            Open
          </button>
          <span id="finish-label">Expected finish</span>
          <button type="button" aria-describedby="finish-label">
            Open
          </button>
        </>
      )}
    </div>
  );
}

const finishDoor = () =>
  screen.getByRole("button", { description: "Expected finish" });

// Leave the surface through Expected finish, as a door out of the page does.
async function leaveThroughFinish(): Promise<unknown> {
  const user = userEvent.setup({ delay: null });
  const view = render(<Surface ready />);
  await user.click(finishDoor());
  const entry = history.state;
  view.unmount();
  return entry;
}

describe("the control a reader left a page through", () => {
  it("has focus again on coming back to that history entry, once it renders", async () => {
    const entry = await leaveThroughFinish();
    history.replaceState(entry, "");

    const back = render(<Surface ready={false} />);
    back.rerender(<Surface ready />);

    await waitFor(() => expect(finishDoor()).toHaveFocus());
  });

  it("is not focused on another entry, such as a new visit", async () => {
    await leaveThroughFinish();
    history.replaceState(null, "");

    render(<Surface ready />);

    expect(finishDoor()).not.toHaveFocus();
  });

  it("leaves focus the reader already moved elsewhere", async () => {
    const entry = await leaveThroughFinish();
    history.replaceState(entry, "");
    const elsewhere = document.createElement("input");
    document.body.append(elsewhere);
    elsewhere.focus();

    render(<Surface ready />);

    expect(elsewhere).toHaveFocus();
    elsewhere.remove();
  });
});
