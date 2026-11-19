// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { en } from "../i18n/en";
import { recordListsKey } from "../lib/activitykeys";
import {
  listsMe,
  MEMBER_ID,
  SHORTLIST_ID,
  shortlist,
} from "../testing/lists.fixtures";
import { ME_ANSWERED, MeAnswered } from "../testing/sessionprobe";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
  storyQueryClient,
} from "../testing/story-utils";
import { AddToShortlistAction } from "./addtoshortlist";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function action(client = storyQueryClient()) {
  return render(
    <StoryProviders client={client}>
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

  it("is not offered to a seat that may not change a Shortlist", async () => {
    installFetchStub({
      "GET /me": listsMe(true, [], {
        allow: { list: ["read"], company: ["read"] },
      }),
    });
    render(
      <StoryProviders>
        <MeAnswered />
        <AddToShortlistAction entityType="company" entityId={MEMBER_ID} />
      </StoryProviders>,
    );
    await screen.findByText(ME_ANSWERED);
    expect(
      screen.queryByRole("button", { name: en["lists.addToShortlist"] }),
    ).toBeNull();
  });

  async function offeredShortlists(me: () => Response) {
    installFetchStub({
      "GET /me": me,
      "GET /lists": () =>
        jsonResponse({
          data: [
            shortlist,
            { ...shortlist, id: "s2", name: "Trade fair", can_edit: false },
            {
              ...shortlist,
              id: "s3",
              name: "Last year",
              archived_at: "2026-01-12T09:00:00Z",
            },
          ],
          page: { has_more: false },
        }),
    });
    const user = userEvent.setup();
    action();
    await user.click(
      await screen.findByRole("button", { name: en["lists.addToShortlist"] }),
    );
    const dialog = await screen.findByRole("dialog");
    await user.click(
      within(dialog).getByRole("combobox", { name: en["lists.shortlist"] }),
    );
    return (await screen.findAllByRole("option")).map(
      (option) => option.textContent,
    );
  }

  it("offers only the live Shortlists the reader may change, and a new one", async () => {
    expect(await offeredShortlists(listsMe(true))).toEqual([
      shortlist.name,
      en["lists.newShortlistOption"],
    ]);
  });

  it("offers no new Shortlist to a seat that may not create one", async () => {
    const mayOnlyAdd = listsMe(true, [], {
      allow: { list: ["read", "update"], company: ["read"] },
    });
    expect(await offeredShortlists(mayOnlyAdd)).toEqual([shortlist.name]);
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

describe("a refused or unfinished add", () => {
  async function openDialog(client = storyQueryClient()) {
    const user = userEvent.setup();
    action(client);
    await user.click(
      await screen.findByRole("button", { name: en["lists.addToShortlist"] }),
    );
    const dialog = await screen.findByRole("dialog");
    const picker = within(dialog).getByRole("combobox", {
      name: en["lists.shortlist"],
    });
    return { user, dialog, picker };
  }

  it("asks for a Shortlist on its field instead of a grey button", async () => {
    let posted = 0;
    installFetchStub({
      "GET /me": listsMe(true),
      "GET /lists": () =>
        jsonResponse({ data: [shortlist], page: { has_more: false } }),
      [`POST /lists/${SHORTLIST_ID}/members`]: () => {
        posted++;
        return jsonResponse({}, 201);
      },
    });
    const { user, dialog, picker } = await openDialog();
    await user.click(
      within(dialog).getByRole("button", { name: en["lists.add"] }),
    );

    expect(
      await within(dialog).findByText(`${en["lists.shortlist"]} is required.`),
    ).toBeVisible();
    expect(picker).toHaveAttribute("aria-invalid", "true");
    expect(posted).toBe(0);
  });

  it("says once, on the Shortlist field, that the record is already on it", async () => {
    installFetchStub({
      "GET /me": listsMe(true),
      "GET /lists": () =>
        jsonResponse({ data: [shortlist], page: { has_more: false } }),
      [`POST /lists/${SHORTLIST_ID}/members`]: () =>
        jsonResponse(
          {
            title: "Conflict",
            status: 409,
            code: "conflict",
            detail: "the record is already on this list: conflict",
          },
          409,
        ),
    });
    const { user, dialog, picker } = await openDialog();
    await user.click(picker);
    await user.click(
      await screen.findByRole("option", { name: shortlist.name }),
    );
    await user.click(
      within(dialog).getByRole("button", { name: en["lists.add"] }),
    );

    expect(
      await screen.findAllByText(en["lists.alreadyOnShortlist"]),
    ).toHaveLength(1);
    expect(picker).toHaveAttribute("aria-invalid", "true");
    expect(screen.queryByText(/: conflict/)).toBeNull();
  });

  it("marks a Shortlist the record page already shows it on", async () => {
    installFetchStub({
      "GET /me": listsMe(true),
      "GET /lists": () =>
        jsonResponse({ data: [shortlist], page: { has_more: false } }),
    });
    const client = storyQueryClient();
    client.setQueryData(recordListsKey("company", MEMBER_ID), {
      data: [shortlist],
      page: { has_more: false },
    });
    const { user, picker } = await openDialog(client);
    await user.click(picker);

    expect(
      await screen.findByRole("option", {
        name: en["lists.alreadyOnOption"].replace("{name}", shortlist.name),
      }),
    ).toBeInTheDocument();
  });
});
