// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { en } from "../i18n/en";
import { AddToShortlistAction } from "./addtoshortlist";
import { listsMe, MEMBER_ID, SHORTLIST_ID, shortlist } from "./lists.fixtures";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function action() {
  return render(
    <StoryProviders>
      <AddToShortlistAction entityType="company" entityId={MEMBER_ID} />
    </StoryProviders>,
  );
}

describe("Add to Shortlist on a record page", () => {
  it("is not offered while lists are switched off", async () => {
    let probed = 0;
    const me = listsMe(false);
    installFetchStub({
      "GET /me": () => {
        probed++;
        return me();
      },
    });
    action();
    await vi.waitFor(() => expect(probed).toBeGreaterThan(0));
    expect(
      screen.queryByRole("button", { name: en["lists.addToShortlist"] }),
    ).toBeNull();
  });

  it("adds the record to the chosen Shortlist with its note", async () => {
    const posted: unknown[] = [];
    installFetchStub({
      "GET /me": listsMe(true),
      "GET /lists": () =>
        jsonResponse({ data: [shortlist], page: { has_more: false } }),
      [`POST /lists/${SHORTLIST_ID}/members`]: (body) => {
        posted.push(body);
        return jsonResponse(
          {
            id: "m",
            list_id: SHORTLIST_ID,
            entity_type: "company",
            entity_id: MEMBER_ID,
          },
          201,
        );
      },
    });
    const user = userEvent.setup();
    action();
    await user.click(
      await screen.findByRole("button", { name: en["lists.addToShortlist"] }),
    );
    await user.click(
      await screen.findByRole("combobox", { name: en["lists.shortlist"] }),
    );
    await user.click(
      await screen.findByRole("option", { name: shortlist.name }),
    );
    await user.type(
      screen.getByRole("textbox", { name: en["lists.note"] }),
      "reference",
    );
    await user.click(screen.getByRole("button", { name: en["lists.add"] }));
    await vi.waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toEqual({
      entity_type: "company",
      entity_id: MEMBER_ID,
      note: "reference",
    });
  });
});
