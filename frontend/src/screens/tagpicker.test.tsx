// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { en } from "../i18n/en";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
import { AddTagPicker } from "./tagpicker";
import type { RecordTag } from "./tags.queries";

const COMPANY = "01a06151-0000-7000-8000-000000000001";

const ON_RECORD: RecordTag = {
  tag_id: "t-2",
  name: "Renewal",
  archived: false,
  assigned_at: "2026-06-01T09:00:00Z",
};

function mount({
  truncated = false,
  apply = () => new Response(null, { status: 204 }),
}: Readonly<{
  truncated?: boolean;
  apply?: () => Response | Promise<Response>;
}> = {}) {
  const applied: unknown[] = [];
  installFetchStub({
    "GET /tags": () =>
      jsonResponse({
        data: [
          { id: "t-1", workspace_id: "w", name: "Key Account" },
          { id: "t-2", workspace_id: "w", name: "Renewal" },
        ],
        page: { has_more: truncated, next_cursor: null },
      }),
    "POST /tags/t-1/apply": (body) => {
      applied.push(body);
      return apply();
    },
  });
  render(
    <StoryProviders>
      <AddTagPicker
        entityType="company"
        entityID={COMPANY}
        current={[ON_RECORD]}
      />
    </StoryProviders>,
  );
  return applied;
}

async function openPicker(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: en["tags.add"] }));
  return within(await screen.findByRole("dialog"));
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the add-tag picker", () => {
  it("applies the picked word to the record and closes", async () => {
    const user = userEvent.setup();
    const applied = mount();
    const panel = await openPicker(user);

    await user.click(await panel.findByRole("option", { name: "Key Account" }));

    expect(applied).toEqual([{ entity_type: "company", entity_id: COMPANY }]);
    expect(await screen.findByRole("button", { name: en["tags.add"] })).toBe(
      document.activeElement,
    );
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("offers a word the record carries once, as already added", async () => {
    const user = userEvent.setup();
    mount();
    const panel = await openPicker(user);

    const renewal = await panel.findByRole("option", { name: /Renewal/ });
    expect(renewal).toHaveAttribute("aria-disabled", "true");
    expect(renewal).toHaveTextContent(en["tags.alreadyAdded"]);
  });

  it("says who can coin a word the search does not find", async () => {
    const user = userEvent.setup();
    mount();
    const panel = await openPicker(user);
    await panel.findByRole("option", { name: "Key Account" });

    await user.type(panel.getByRole("combobox"), "Partner");

    expect(panel.getByText(en["tags.noMatch"])).toBeInTheDocument();
    expect(panel.queryByRole("option")).toBeNull();
  });

  it("keeps the list open over a refused apply, saying why", async () => {
    const user = userEvent.setup();
    mount({
      apply: () =>
        jsonResponse(
          { title: "Forbidden", detail: "This record is locked." },
          403,
        ),
    });
    const panel = await openPicker(user);

    await user.click(await panel.findByRole("option", { name: "Key Account" }));

    expect(await panel.findByRole("alert")).toHaveTextContent(
      "This record is locked.",
    );
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("takes one apply from a double press on a word", async () => {
    const user = userEvent.setup();
    const applied = mount();
    const panel = await openPicker(user);

    const option = await panel.findByRole("option", { name: "Key Account" });
    // Two presses in one task: the second lands before any render says the
    // first is pending.
    fireEvent.click(option);
    fireEvent.click(option);

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(applied).toHaveLength(1);
  });

  it("stays open through Escape while the apply is out, so a late refusal is read", async () => {
    const user = userEvent.setup();
    let answer: (response: Response) => void = () => {};
    mount({
      apply: () =>
        new Promise<Response>((resolve) => {
          answer = resolve;
        }),
    });
    const panel = await openPicker(user);
    await user.click(await panel.findByRole("option", { name: "Key Account" }));

    await user.keyboard("{Escape}");
    expect(screen.getByRole("dialog")).toBeInTheDocument();

    answer(
      jsonResponse(
        { title: "Forbidden", detail: "This record is locked." },
        403,
      ),
    );
    expect(await panel.findByRole("alert")).toHaveTextContent(
      "This record is locked.",
    );
  });

  it("closes on Escape and hands focus back to the trigger", async () => {
    const user = userEvent.setup();
    mount();
    await openPicker(user);

    await user.keyboard("{Escape}");

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByRole("button", { name: en["tags.add"] })).toBe(
      document.activeElement,
    );
  });

  // The catalog is capped and carries no cursor, so a workspace past the cap
  // gets a CUT list, and a missing word reads as one the workspace lacks.
  it("says the list is cut when the catalog was truncated, and only then", async () => {
    const user = userEvent.setup();
    mount({ truncated: true });
    const panel = await openPicker(user);
    expect(
      await panel.findByText(en["tags.catalogTruncated"]),
    ).toBeInTheDocument();
    cleanup();

    mount();
    const whole = await openPicker(user);
    await whole.findByRole("option", { name: "Key Account" });
    expect(whole.queryByText(en["tags.catalogTruncated"])).toBeNull();
  });
});
