/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { AppLink } from "./applink";

const ID = "01a00000-0000-7000-8000-000000000001";
afterEach(() => {
  cleanup();
  history.replaceState(null, "", "/");
});
it("keeps new tabs and copied links independent of the current record", () => {
  history.replaceState(null, "", `/r/companies/${ID}#/companies/${ID}`);
  render(
    <>
      <AppLink href={`#/contacts/${ID}`} target="_blank">
        Buyer
      </AppLink>
      <AppLink href="#/home">Home</AppLink>
      <AppLink href="#/settings/account" target="_blank">
        Settings
      </AppLink>
    </>,
  );
  expect(screen.getByText("Buyer").getAttribute("href")).toBe(
    `/r/contacts/${ID}#/contacts/${ID}`,
  );
  expect(screen.getByText("Home").getAttribute("href")).toBe("/#/home");
  expect(screen.getByText("Settings").getAttribute("href")).toBe(
    "/#/settings/account",
  );
  fireEvent.click(screen.getByText("Home"));
  expect(location.hash).toBe("#/home");
});
it("respects a caller that cancels navigation and preserves external destinations", () => {
  render(
    <>
      <AppLink href="#/home" onClick={(event) => event.preventDefault()}>
        Cancel
      </AppLink>
      <AppLink href="https://example.test/">External</AppLink>
    </>,
  );
  fireEvent.click(screen.getByText("Cancel"));
  expect(location.hash).toBe("");
  expect(screen.getByText("External").getAttribute("href")).toBe(
    "https://example.test/",
  );
});

it("allows a native anchor with no destination without trying to navigate", () => {
  render(<AppLink>Unavailable record</AppLink>);
  expect(() =>
    fireEvent.click(screen.getByText("Unavailable record")),
  ).not.toThrow();
  expect(location.hash).toBe("");
});
