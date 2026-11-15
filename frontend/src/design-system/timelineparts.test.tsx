/** @vitest-environment happy-dom */
import {
  cleanup,
  render,
  renderHook,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { browserZone } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import { useRoute } from "../lib/router";
import { GroupedTimelineList } from "./timeline";
import type { TimelineEntry } from "./timelineentry";
import { groupChronology } from "./timelinegroups";

afterEach(() => {
  cleanup();
  history.replaceState(null, "", "#/home");
  vi.restoreAllMocks();
});

function entry(
  id: string,
  kind: TimelineEntry["kind"],
  title: string,
  day: string,
  threadKey?: string,
): TimelineEntry {
  return {
    id,
    kind,
    title,
    subject: title,
    atIso: `2026-07-${day}T10:00:00Z`,
    atPrecision: "instant",
    threadKey,
    provenance: { kind: "human", self: true },
  };
}

it("reveals either thread part without navigating away from the record", async () => {
  const user = userEvent.setup();
  history.replaceState(null, "", "#/contacts/contact-one?tab=history");
  const { result } = renderHook(useRoute);
  const address = location.hash;
  const groups = groupChronology([
    entry("new", "email", "Re: Pricing", "10", "thread-one"),
    entry("task", "task", "Send quote", "05"),
    entry("meeting", "meeting", "Review", "02"),
    entry("old", "email", "Pricing", "01", "thread-one"),
  ]);
  const { container } = render(
    <LocaleProvider initial="en">
      <GroupedTimelineList groups={groups} zone={browserZone()} />
    </LocaleProvider>,
  );
  const rows = container.querySelectorAll<HTMLLIElement>(".timeline > li");
  expect(rows).toHaveLength(4);
  expect(within(rows[1]).getByText("Send quote")).toBeTruthy();
  expect(within(rows[2]).getByText("Review")).toBeTruthy();
  const older = screen.getByRole("button", { name: "Earlier messages below" });
  const newer = screen.getByRole("button", { name: "Later messages above" });
  const olderScroll = vi.fn();
  const newerScroll = vi.fn();
  rows[3].scrollIntoView = olderScroll;
  rows[0].scrollIntoView = newerScroll;
  await user.click(older);
  expect(olderScroll).toHaveBeenCalledOnce();
  await user.click(newer);
  expect(newerScroll).toHaveBeenCalledOnce();
  expect(location.hash).toBe(address);
  expect(result.current).toEqual({
    screen: "contacts",
    id: "contact-one",
    id2: undefined,
    id3: undefined,
  });
  expect(within(rows[0]).getByText("2 messages")).toBeTruthy();
  expect(within(rows[3]).getByText("2 messages")).toBeTruthy();
  expect(within(rows[3]).getByText("Re: Pricing")).toBeTruthy();
});
