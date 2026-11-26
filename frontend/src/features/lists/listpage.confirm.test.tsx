// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { en } from "../../i18n/en";
import type { List } from "../../lib/lists.queries";
import {
  listsMe,
  MEMBER_ID,
  members,
  SHORTLIST_ID,
  shortlist,
  visitAnswer,
} from "../../testing/lists.fixtures";
import {
  emptyPage,
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "../../testing/story-utils";
import { ListScreen } from "./listpage";

// Every change to a list from its own page says it landed. An archive that
// loses nothing on its way back runs at once, with an Undo.

afterEach(cleanup);

const ARCHIVED_AT = "2026-09-30T08:00:00Z";

const watched: List = {
  ...shortlist,
  dependencies: [
    {
      kind: "automation",
      occurred_at: "2026-09-20T08:00:00Z",
      blocking: false,
      role: "watches",
      automation_id: MEMBER_ID,
      automation_name: "Follow up on launch references",
    },
  ],
};

type Writes = string[];

// A Shortlist whose archive and restore change what its read answers.
function stubShortlist(start: List, routes: RouteMap = {}): Writes {
  const writes: Writes = [];
  let current = start;
  installFetchStub({
    "GET /me": listsMe(true),
    [`GET /lists/${SHORTLIST_ID}`]: () => jsonResponse(current),
    [`POST /lists/${SHORTLIST_ID}/visit`]: visitAnswer(SHORTLIST_ID),
    [`GET /lists/${SHORTLIST_ID}/history`]: () => jsonResponse(emptyPage),
    "GET /companies": () =>
      jsonResponse({ data: members, page: { has_more: false } }),
    [`DELETE /lists/${SHORTLIST_ID}`]: () => {
      writes.push("archive");
      current = { ...current, archived_at: ARCHIVED_AT };
      return jsonResponse(current);
    },
    [`POST /lists/${SHORTLIST_ID}/restore`]: () => {
      writes.push("restore");
      current = { ...current, archived_at: null };
      return jsonResponse(current);
    },
    ...routes,
  });
  return writes;
}

function page() {
  return render(
    <StoryProviders>
      <ToastProvider>
        <ListScreen listID={SHORTLIST_ID} />
        <ToastRegion />
      </ToastProvider>
    </StoryProviders>,
  );
}

// The pointer rests on the toast, which holds its clock (WCAG 2.2.1), so it
// cannot time out between a check and the next step on a slow machine.
async function heldToast(
  user: ReturnType<typeof userEvent.setup>,
  said: string,
) {
  const text = await screen.findByText(said);
  await user.hover(text);
  const toast = text.closest("output");
  if (toast === null) {
    throw new Error(`"${said}" is not inside a toast`);
  }
  return toast;
}

const more = () =>
  screen.findByRole("button", {
    name: en["filters.library.rowMore"].replace("{name}", shortlist.name),
  });

describe("archiving a list", () => {
  it("runs at once when no rule uses the list, and its Undo restores it", async () => {
    const writes = stubShortlist(shortlist);
    const user = userEvent.setup();
    page();

    await user.click(await more());
    await user.click(screen.getByRole("button", { name: en["lists.archive"] }));

    const toast = await heldToast(user, en["lists.archivedDone"]);
    expect(toast).toBeVisible();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(writes).toEqual(["archive"]);

    await user.click(
      within(toast).getByRole("button", { name: en["common.undo"] }),
    );
    await waitFor(() => expect(writes).toEqual(["archive", "restore"]));
  });

  it("asks first when a rule uses the list, and offers no Undo after", async () => {
    const writes = stubShortlist(watched);
    const user = userEvent.setup();
    page();

    await user.click(await more());
    await user.click(screen.getByRole("button", { name: en["lists.archive"] }));
    const dialog = await screen.findByRole("dialog");
    expect(writes).toEqual([]);
    await user.click(
      await within(dialog).findByRole("button", { name: en["lists.archive"] }),
    );

    const toast = await heldToast(user, en["lists.archivedDone"]);
    expect(toast).toBeVisible();
    expect(writes).toEqual(["archive"]);
    expect(
      within(toast).queryByRole("button", { name: en["common.undo"] }),
    ).toBeNull();
  });
});

describe("restoring a list", () => {
  it("says the list is back", async () => {
    const writes = stubShortlist({ ...shortlist, archived_at: ARCHIVED_AT });
    const user = userEvent.setup();
    page();

    await user.click(
      await screen.findByRole("button", { name: en["lists.restore"] }),
    );

    expect(await heldToast(user, en["lists.restored"])).toBeVisible();
    expect(writes).toEqual(["restore"]);
  });
});

describe("editing a list", () => {
  async function saveName(user: ReturnType<typeof userEvent.setup>) {
    await user.click(await more());
    await user.click(
      screen.getByRole("button", { name: en["lists.settings"] }),
    );
    const name = await screen.findByRole("textbox", { name: en["lists.name"] });
    await user.clear(name);
    await user.type(name, "Launch references, October");
    await user.click(screen.getByRole("button", { name: en["lists.save"] }));
  }

  it("says the list was saved", async () => {
    stubShortlist(shortlist, {
      [`PATCH /lists/${SHORTLIST_ID}`]: () =>
        jsonResponse({ ...shortlist, name: "Launch references, October" }),
    });
    const user = userEvent.setup();
    page();

    await saveName(user);

    expect(await heldToast(user, en["lists.settingsSaved"])).toBeVisible();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("names the field a length refusal is about", async () => {
    stubShortlist(shortlist, {
      [`PATCH /lists/${SHORTLIST_ID}`]: () =>
        jsonResponse(
          {
            status: 422,
            code: "validation_error",
            details: {
              errors: [
                {
                  field: "name",
                  code: "too_long",
                  message: "name is too long",
                  max: 120,
                },
              ],
            },
          },
          422,
        ),
    });
    const user = userEvent.setup();
    page();

    await saveName(user);

    expect(
      await screen.findByText("Name: at most 120 characters."),
    ).toBeVisible();
    expect(screen.queryByText(en["lists.settingsSaved"])).toBeNull();
  });
});
