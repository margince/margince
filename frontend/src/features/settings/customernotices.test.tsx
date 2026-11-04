// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { en } from "../../i18n/en";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { CustomerNoticesCard } from "./customernotices";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const ADMIN = { consent_config: ["read", "create", "update"] } as const;
const DAY_MS = 24 * 60 * 60 * 1000;

const entities = () =>
  jsonResponse({
    data: [
      { id: "le-default", is_default: true, display_name: "Brandt" },
      { id: "le-vienna", is_default: false, display_name: "Brandt Wien" },
    ].map((entity) => ({
      ...entity,
      address: {},
      member_count: 0,
      version: 1,
    })),
  });

const purposes = () =>
  jsonResponse({
    data: [
      {
        id: "p-news",
        key: "newsletter",
        label: "Newsletter",
        class: "marketing",
      },
      {
        id: "p-inv",
        key: "invoices",
        label: "Invoices",
        class: "transactional",
      },
    ].map((p) => ({ ...p, created_at: "2024-01-01T00:00:00Z" })),
    page: { has_more: false },
  });

function notice(over: Record<string, unknown> = {}) {
  return {
    id: "n-1",
    legal_entity_id: "le-vienna",
    customer_group: "Web shop buyers",
    collection_channel: "web shop checkout",
    notice_text: "You may object at any time.",
    effective_from: "2023-03-01T12:00:00Z",
    effective_until: null,
    covers_from: "2023-03-01T12:00:00Z",
    purpose_ids: ["p-news"],
    created_at: "2024-01-01T00:00:00Z",
    withdrawn_at: null,
    withdrawn_reason: null,
    ...over,
  };
}

function mount() {
  return render(
    <StoryProviders>
      <CustomerNoticesCard />
    </StoryProviders>,
  );
}

describe("the existing-customer notices card", () => {
  it("lists each declaration with its entity and purposes, and offers no withdrawal of a withdrawn one", async () => {
    stubWithSession(
      {
        "GET /existing-customer-notices": () =>
          jsonResponse({
            data: [
              notice(),
              notice({
                id: "n-2",
                customer_group: "Trade fair buyers",
                withdrawn_at: "2024-05-01T00:00:00Z",
                withdrawn_reason: "wrong period",
              }),
            ],
          }),
        "GET /legal-entities": entities,
        "GET /consent-purposes": purposes,
      },
      ADMIN,
    );
    mount();
    const live = (await screen.findByText("Web shop buyers")).closest("tr");
    if (!live) throw new Error("the live declaration has no row");
    expect(await within(live).findByText("Brandt Wien")).toBeInTheDocument();
    expect(within(live).getByText("Newsletter")).toBeInTheDocument();
    expect(
      within(live).getByRole("button", {
        name: en["privacy.customerNoticeWithdraw"],
      }),
    ).toBeInTheDocument();

    const withdrawn = screen.getByText("Trade fair buyers").closest("tr");
    if (!withdrawn) throw new Error("the withdrawn declaration has no row");
    expect(
      within(withdrawn).getByText(en["privacy.customerNoticeWithdrawn"]),
    ).toBeInTheDocument();
    expect(within(withdrawn).queryByRole("button")).not.toBeInTheDocument();
  });

  it("declares a notice for the default entity with a whole-day period and marketing purposes only", async () => {
    const posted: Record<string, unknown>[] = [];
    stubWithSession(
      {
        "GET /existing-customer-notices": () => jsonResponse({ data: [] }),
        "GET /legal-entities": entities,
        "GET /consent-purposes": purposes,
        "POST /existing-customer-notices": (body) => {
          posted.push(body as Record<string, unknown>);
          return jsonResponse(notice(), 201);
        },
      },
      ADMIN,
    );
    const user = userEvent.setup();
    mount();
    expect(
      await screen.findByText(en["privacy.customerNoticesEmpty"]),
    ).toBeInTheDocument();
    await user.click(
      screen.getByRole("button", { name: en["privacy.customerNoticeDeclare"] }),
    );
    const dialog = await screen.findByRole("dialog");
    await user.type(
      within(dialog).getByLabelText(en["privacy.customerNoticeGroup"]),
      "Web shop buyers",
    );
    await user.type(
      within(dialog).getByLabelText(en["privacy.customerNoticeChannel"]),
      "web shop checkout",
    );
    await user.type(
      within(dialog).getByLabelText(en["privacy.customerNoticeText"]),
      "You may object at any time.",
    );
    await user.type(
      within(dialog).getByRole("textbox", {
        name: en["privacy.customerNoticeFrom"],
      }),
      "2024-01-10{Enter}",
    );
    await user.type(
      within(dialog).getByRole("textbox", {
        name: en["privacy.customerNoticeUntil"],
      }),
      "2024-01-20{Enter}",
    );
    await user.type(
      within(dialog).getByLabelText(en["privacy.customerNoticeProof"]),
      "archived checkout page",
    );
    await user.type(
      within(dialog).getByRole("textbox", {
        name: en["privacy.customerNoticeProofDay"],
      }),
      "2024-01-05{Enter}",
    );
    expect(within(dialog).queryByLabelText("Invoices")).not.toBeInTheDocument();
    await user.click(await within(dialog).findByLabelText("Newsletter"));
    await user.click(
      within(dialog).getByRole("button", {
        name: en["privacy.customerNoticeSave"],
      }),
    );

    await vi.waitFor(() => expect(posted).toHaveLength(1));
    const body = posted[0];
    expect(body).toMatchObject({
      legal_entity_id: "le-default",
      customer_group: "Web shop buyers",
      collection_channel: "web shop checkout",
      purpose_ids: ["p-news"],
    });
    const from = Date.parse(String(body.effective_from));
    const until = Date.parse(String(body.effective_until));
    expect(until - from).toBe(11 * DAY_MS);
    expect(body.evidence_reference).toBe("archived checkout page");
    expect(from - Date.parse(String(body.evidence_dated_at))).toBe(5 * DAY_MS);
  });

  it("withdraws a declaration with the reason typed", async () => {
    const posted: unknown[] = [];
    stubWithSession(
      {
        "GET /existing-customer-notices": () =>
          jsonResponse({ data: [notice()] }),
        "GET /legal-entities": entities,
        "GET /consent-purposes": purposes,
        "POST /existing-customer-notices/n-1/withdraw": (body) => {
          posted.push(body);
          return jsonResponse(notice({ withdrawn_at: "2024-05-01T00:00:00Z" }));
        },
      },
      ADMIN,
    );
    const user = userEvent.setup();
    mount();
    await user.click(
      await screen.findByRole("button", {
        name: en["privacy.customerNoticeWithdraw"],
      }),
    );
    const dialog = await screen.findByRole("dialog");
    await user.type(
      within(dialog).getByLabelText(en["privacy.customerNoticeWithdrawReason"]),
      "the checkout never showed it",
    );
    await user.click(
      within(dialog).getByRole("button", {
        name: en["privacy.customerNoticeWithdraw"],
      }),
    );
    await vi.waitFor(() =>
      expect(posted).toEqual([{ reason: "the checkout never showed it" }]),
    );
  });

  it("is not shown to someone who may not read consent settings", async () => {
    stubWithSession(
      {
        "GET /existing-customer-notices": () =>
          jsonResponse({ data: [notice()] }),
      },
      { contact: ["read"] },
    );
    mount();
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(
      screen.queryByText(en["privacy.customerNotices"]),
    ).not.toBeInTheDocument();
  });
});
