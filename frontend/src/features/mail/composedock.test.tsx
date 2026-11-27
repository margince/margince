// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useContext } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { UnsavedExits, useExitAsking } from "../../components/unsaved";
import { en as englishCopy } from "../../i18n/en";
import { installComposeClock } from "./compose.testkit";
import { render, stubRoutes } from "./compose-saveddraft.testkit";
import { ComposeDockProvider } from "./composedock";
import { ComposeDockContext } from "./composedockcontext";

beforeEach(installComposeClock);

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

// The account menu's Sign out, beside a page that holds no draft.
function Beside({ signOut }: Readonly<{ signOut: () => void }>) {
  const dock = useContext(ComposeDockContext);
  const askFirst = useExitAsking();
  return (
    <>
      <button
        type="button"
        onClick={() =>
          dock?.show({
            id: "mail-1",
            props: {
              open: true,
              entityType: "contact",
              entityId: "p-1",
              contactId: "p-1",
              onClose: () => {},
            },
          })
        }
      >
        compose
      </button>
      <button type="button" onClick={() => askFirst(signOut)}>
        sign out
      </button>
    </>
  );
}

function renderDock(signOut: () => void) {
  // The save never answers, so the words on screen stay unkept.
  stubRoutes({ "PUT /mail-drafts": () => new Promise(() => {}) });
  render(
    <UnsavedExits>
      <ComposeDockProvider>
        <Beside signOut={signOut} />
      </ComposeDockProvider>
    </UnsavedExits>,
  );
}

it("asks before Sign out drops words in the docked composer", async () => {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  const signOut = vi.fn();
  renderDock(signOut);
  await user.click(screen.getByRole("button", { name: "compose" }));
  await user.type(await screen.findByLabelText("Subject"), "Pricing");
  await user.click(screen.getByRole("button", { name: "sign out" }));

  expect(
    await screen.findByRole("dialog", { name: englishCopy["unsaved.title"] }),
  ).toBeTruthy();
  expect(signOut).not.toHaveBeenCalled();
});

it("signs out at once over an empty docked composer", async () => {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  const signOut = vi.fn();
  renderDock(signOut);
  await user.click(screen.getByRole("button", { name: "compose" }));
  await screen.findByLabelText("Subject");
  await user.click(screen.getByRole("button", { name: "sign out" }));

  expect(signOut).toHaveBeenCalledTimes(1);
});
