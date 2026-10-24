// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { ContactLink } from "../../design-system/contactlink";
import { en } from "../../i18n/en";
import { view } from "../../testing/contactpage.testkit";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ContactActions } from "./contactactions";

type Contact360 = components["schemas"]["Contact360"];
type ContactPhone = components["schemas"]["ContactPhone"];

afterEach(cleanup);

const OFFICE = "+49 30 1111 2222";
const MOBILE = "+49 170 333 4444";

function phone(id: string, number: string, isPrimary: boolean): ContactPhone {
  return {
    id,
    phone: number,
    phone_type: "work",
    is_primary: isPrimary,
    position: Number(id.slice(-1)),
    source: "manual",
    captured_by: "human:u-1",
  };
}

function withContact(contact: Partial<Contact360["contact"]>): Contact360 {
  return { ...view, contact: { ...view.contact, ...contact } };
}

function mount(page: Contact360, refusedReasonId?: string) {
  installFetchStub({
    "GET /me": meRoute(
      { contact: ["read", "update"], activity: ["create"] },
      { seat: "full" },
    ),
    "GET /channel-providers": () => jsonResponse({ data: [] }),
  });
  render(
    <StoryProviders>
      <div data-testid="header">
        <ContactActions
          view={page}
          contactId={page.contact.id}
          onWrite={() => undefined}
          onResearch={() => undefined}
          onLogActivity={() => undefined}
          onAddTask={() => undefined}
          refusedReasonId={refusedReasonId}
        />
      </div>
      {refusedReasonId && (
        <p id={refusedReasonId}>{en["contact.rail.archivedReadOnly"]}</p>
      )}
      <div data-testid="fact">
        <ContactLink
          kind="phone"
          value={MOBILE}
          record={{ entityType: "contact", entityId: page.contact.id }}
        />
        <ContactLink
          kind="phone"
          value={OFFICE}
          record={{ entityType: "contact", entityId: page.contact.id }}
        />
      </div>
    </StoryProviders>,
  );
}

function factHref(number: string): string | null {
  return within(screen.getByTestId("fact"))
    .getByRole("link", { name: number })
    .getAttribute("href");
}

function headerCall(role: "link" | "button"): HTMLElement {
  return within(screen.getByTestId("header")).getByRole(role, {
    name: en["contact.action.call"],
  });
}

function describedTextOf(control: HTMLElement): string {
  return (control.getAttribute("aria-describedby") ?? "")
    .split(/\s+/)
    .map((id) => document.getElementById(id)?.textContent ?? "")
    .join(" ")
    .trim();
}

describe("the contact header's Call", () => {
  it("dials the number the phone fact dials", () => {
    mount(withContact({ phones: [phone("ph-0", OFFICE, false)] }));
    expect(headerCall("link").getAttribute("href")).toBe(factHref(OFFICE));
  });

  it("dials the primary number, not the first", () => {
    mount(
      withContact({
        phones: [phone("ph-0", OFFICE, false), phone("ph-1", MOBILE, true)],
      }),
    );
    expect(headerCall("link").getAttribute("href")).toBe(factHref(MOBILE));
  });

  it("is refused with the reason when there is no number", () => {
    mount(withContact({ phones: [] }));
    const control = headerCall("button");
    expect(control.hasAttribute("disabled")).toBe(true);
    expect(describedTextOf(control)).toBe(en["contact.action.noPhone"]);
  });

  it("keeps the archived refusal on an archived contact with a number", () => {
    mount(
      withContact({
        archived_at: "2026-08-14T09:00:00Z",
        phones: [phone("ph-0", OFFICE, true)],
      }),
      "contact-archived",
    );
    const control = headerCall("button");
    expect(control.hasAttribute("disabled")).toBe(true);
    expect(describedTextOf(control)).toBe(en["contact.rail.archivedReadOnly"]);
  });
});
