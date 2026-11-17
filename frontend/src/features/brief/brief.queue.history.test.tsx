/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { readingsDay, taskRow } from "../../testing/brief.fixtures";
import { jsonResponse, render, stubApi } from "../../testing/brief.testkit";
import { BriefScreen } from "./brief";
import { openQueue } from "./brief.queue";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

function stubQueue() {
  stubApi({
    "GET /worklist": () =>
      jsonResponse(readingsDay({}, [taskRow("t", "Call Weber")])),
  });
}

// The browser accepting a traversal and not yet carrying it out.
const goPending = () =>
  vi.spyOn(globalThis.history, "go").mockImplementation(() => undefined);

const drawer = () =>
  screen.findByRole("dialog", { name: en["brief.queue.title"] });

it("closes a drawer opened from Home by going back to the entry under it", async () => {
  const user = userEvent.setup();
  window.history.replaceState(null, "", "#/home");
  stubQueue();
  render(<BriefScreen />);
  const entries = window.history.length;
  await user.click(
    await screen.findByRole("button", {
      name: new RegExp(en["brief.queue.show"]),
    }),
  );
  await drawer();
  expect(window.history.length).toBe(entries + 1);

  const go = goPending();
  await user.click(screen.getByRole("button", { name: /close/i }));
  expect(go.mock.calls).toEqual([[-1]]);
  expect(window.history.length).toBe(entries + 1);
});

it("closes a drawer reached on an entry of its own in place", async () => {
  const user = userEvent.setup();
  window.history.replaceState(
    null,
    "",
    "#/home?queue=1&queue_scope=team&view=weekly",
  );
  stubQueue();
  render(<BriefScreen />);
  const entries = window.history.length;
  await drawer();

  const go = goPending();
  await user.click(screen.getByRole("button", { name: /close/i }));
  expect(go).not.toHaveBeenCalled();
  expect(window.location.hash).toBe("#/home?view=weekly");
  expect(window.history.length).toBe(entries);
});

it("leaves an open drawer as it stands when asked to open again", () => {
  window.history.replaceState(null, "", "#/home?owner=u-2&queue=1");
  const entries = window.history.length;
  openQueue();
  expect(window.location.hash).toBe("#/home?owner=u-2&queue=1");
  expect(window.history.length).toBe(entries);
});
