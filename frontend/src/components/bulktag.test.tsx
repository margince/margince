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
import { afterEach, describe, expect, it, vi } from "vitest";

import { en } from "../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "../testing/story-utils";
import { type PickedTag, TagVerbs } from "./bulktag";

function mount(routes: RouteMap) {
  const picked: Array<{ verb: string; tag: PickedTag }> = [];
  installFetchStub({
    "GET /tags": () =>
      jsonResponse({
        data: [{ id: "t-1", workspace_id: "w", name: "Key Account" }],
        page: { has_more: false, next_cursor: null },
      }),
    ...routes,
  });
  render(
    <StoryProviders>
      <TagVerbs
        disabled={false}
        onPick={(verb, tag) => picked.push({ verb, tag })}
      />
    </StoryProviders>,
  );
  return picked;
}

async function openPicker(user: ReturnType<typeof userEvent.setup>) {
  await user.click(
    await screen.findByRole("button", { name: en["bulk.tagPick"] }),
  );
  const panel = within(await screen.findByRole("dialog"));
  await panel.findByRole("option", { name: "Key Account" });
  return panel;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the bulk tag picker", () => {
  it("creates a missing word and makes it the tag the change carries", async () => {
    const user = userEvent.setup();
    const created: unknown[] = [];
    const picked = mount({
      "GET /me": meRoute({ tag: ["read", "create"] }, { roles: ["admin"] }),
      "POST /tags": (body) => {
        created.push(body);
        return jsonResponse(
          { id: "t-new", workspace_id: "w", name: "Spring campaign" },
          201,
        );
      },
    });
    const panel = await openPicker(user);

    await user.type(panel.getByRole("combobox"), "Spring campaign");
    await user.click(
      panel.getByRole("button", {
        name: en["tags.createNamed"].replace("{name}", "Spring campaign"),
      }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await user.click(screen.getByRole("button", { name: en["bulk.addTag"] }));

    expect(created).toEqual([{ name: "Spring campaign" }]);
    expect(picked).toEqual([
      { verb: "add_tag", tag: { id: "t-new", name: "Spring campaign" } },
    ]);
  });

  it("tells a seat that may not create tags who can, and offers no create", async () => {
    const user = userEvent.setup();
    mount({ "GET /me": meRoute({ tag: ["read"] }, { roles: ["rep"] }) });
    const panel = await openPicker(user);
    expect(panel.getByText(en["tags.whoAdds"])).toBeInTheDocument();

    await user.type(panel.getByRole("combobox"), "Spring campaign");

    expect(panel.getByText(en["tags.noMatch"])).toBeInTheDocument();
    expect(
      panel.queryByRole("button", {
        name: en["tags.createNamed"].replace("{name}", "Spring campaign"),
      }),
    ).toBeNull();
  });
});
