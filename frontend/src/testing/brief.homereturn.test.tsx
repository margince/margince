/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { BriefFeed } from "../features/brief/brief.feed";
import type { WorklistItem } from "../features/worklist/worklist.queries";
import { WorklistReturnLink } from "../features/worklist/worklist.return";
import { en } from "../i18n/en";
import { readingsDay, taskRow } from "./brief.fixtures";
import { render, stubApi } from "./brief.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

const aboutWeber: WorklistItem = {
  ...taskRow("t-1", "Call Weber"),
  subject: { type: "contact", id: "c-1", label: "Weber" },
};

it("opens a record from Home's own list with the way back to the Worklist", async () => {
  window.location.hash = "#/home";
  stubApi({});
  render(<BriefFeed day={readingsDay({}, [aboutWeber])} state="ready" />);
  const href =
    (await screen.findByRole("link", { name: "Weber" })).getAttribute("href") ??
    "";
  expect(href).toBe("#/contacts/c-1?from=worklist");
  cleanup();

  window.location.hash = href;
  render(<WorklistReturnLink />);

  expect(
    screen
      .getByRole("link", { name: en["brief.queue.back"] })
      .getAttribute("href"),
  ).toBe("#/home?queue=1");
});
