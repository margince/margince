/** @vitest-environment happy-dom */
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it } from "vitest";
import { useBookingTitle } from "./booking-common";

// The app shell's own head, as index.html ships it, with its scripts left out.
const shellHead = new DOMParser()
  .parseFromString(
    readFileSync(
      join(dirname(fileURLToPath(import.meta.url)), "../../../index.html"),
      "utf8",
    ),
    "text/html",
  )
  .head.innerHTML.replace(/<script[\s\S]*?<\/script>/g, "");

const LOGO = "https://crm.example.test/v1/public/booking/company-logo";
const acme = { paused: false, company_name: "Acme GmbH", logo_url: LOGO };

const content = (selector: string) =>
  document.head.querySelector<HTMLMetaElement>(selector)?.content;
const icons = () =>
  Array.from(
    document.head.querySelectorAll<HTMLLinkElement>('link[rel~="icon"]'),
    (link) => link.getAttribute("href"),
  );

beforeEach(() => {
  document.head.innerHTML = shellHead;
});
afterEach(cleanup);

it("starts from a shell head that names Margince", () => {
  expect(document.title).toBe("Margince");
  expect(document.head.innerHTML).toContain("Margince");
});

it("names the booking and the host's company in the tab and the preview tags", () => {
  renderHook(() => useBookingTitle("Book a meeting with Ada", acme));
  expect(document.title).toBe("Book a meeting with Ada");
  expect(icons()).toEqual([LOGO]);
  expect(content('meta[property="og:title"]')).toBe("Book a meeting with Ada");
  expect(content('meta[property="og:site_name"]')).toBe("Acme GmbH");
  expect(content('meta[property="og:image"]')).toBe(LOGO);
  expect(content('meta[name="twitter:image"]')).toBe(LOGO);
  expect(document.head.innerHTML).not.toMatch(/margince|og\.png/i);
  expect(document.head.querySelector('link[rel="manifest"]')).toBeNull();
});

it.each([
  ["no company", undefined],
  ["a company with no logo", { ...acme, logo_url: null }],
  ["a paused page", { ...acme, paused: true }],
])("with %s, keeps a neutral head", (_case, brand) => {
  renderHook(() => useBookingTitle("Book a meeting", brand));
  expect(document.title).toBe("Book a meeting");
  expect(icons()).toEqual(["data:,"]);
  expect(content('meta[property="og:image"]')).toBeUndefined();
  if (brand?.paused !== false)
    expect(document.head.innerHTML).not.toContain("Acme");
  expect(document.head.innerHTML).not.toMatch(/margince|og\.png/i);
});

it("follows the brand once the profile arrives", () => {
  const { rerender } = renderHook(
    ({ brand }) => useBookingTitle("Book a meeting with Ada", brand),
    { initialProps: { brand: undefined as typeof acme | undefined } },
  );
  expect(icons()).toEqual(["data:,"]);
  rerender({ brand: acme });
  expect(icons()).toEqual([LOGO]);
  expect(
    document.head.querySelectorAll('meta[property="og:title"]'),
  ).toHaveLength(1);
});

it("gives the shell its own head back when the screen leaves", () => {
  const before = document.head.innerHTML;
  const { unmount } = renderHook(() => useBookingTitle("Your meeting", acme));
  unmount();
  expect(document.head.innerHTML).toBe(before);
  expect(document.title).toBe("Margince");
});

it("leaves the head alone for the host's own signed-in view", () => {
  const before = document.head.innerHTML;
  renderHook(() => useBookingTitle(null, acme));
  expect(document.head.innerHTML).toBe(before);
});
