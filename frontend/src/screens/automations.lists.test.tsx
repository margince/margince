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
import type { components } from "../api/schema";
import { Field } from "../design-system/atoms";
import { pickOption } from "../design-system/select-testing";
import { en } from "../i18n/en";
import { DeleteAutomationAction } from "./automations.delete";
import { AutomationForm } from "./automations.form";
import { ListParamSelect, RulePausedReason } from "./automations.lists";
import { paramFields } from "./automations.params";
import { liveList, shortlist } from "./lists.fixtures";
import { jsonResponse, StoryProviders } from "./story-utils";

type Automation = components["schemas"]["Automation"];

const rule: Automation = {
  id: "01a0f000-0000-7000-8000-000000000031",
  key: "list_membership_notify",
  name: "Tell me about new buyers",
  status: "paused",
  params: { list_id: "01a0f000-0000-7000-8000-000000000001" },
  created_at: "2026-09-20T08:00:00Z",
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type List = components["schemas"]["List"];

/** Answers GET /lists by the list type asked for, and records each query. */
function listsBackend(byType: Record<string, List[] | "fail">) {
  const asked: URLSearchParams[] = [];
  vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
    const url = new URL(input instanceof Request ? input.url : String(input));
    asked.push(url.searchParams);
    const lists = byType[url.searchParams.get("list_type") ?? ""] ?? [];
    return lists === "fail"
      ? jsonResponse({ title: "unavailable" }, 500)
      : jsonResponse({ data: lists, page: { has_more: false } });
  });
  return asked;
}

function picker(
  kind: "live_list" | "shortlist",
  watchedList: string,
  onChange: (value: string) => void = () => undefined,
) {
  return render(
    <StoryProviders>
      <Field label={kind}>
        {(control) => (
          <ListParamSelect
            kind={kind}
            value=""
            watchedList={watchedList}
            onChange={onChange}
            control={control}
          />
        )}
      </Field>
    </StoryProviders>,
  );
}

describe("a rule that watches a list", () => {
  it("draws a list picker for a property that names a list", () => {
    expect(
      paramFields({
        type: "object",
        properties: {
          list_id: { type: "string", format: "live_list" },
          shortlist_id: { type: "string", format: "shortlist" },
          direction: {
            type: "string",
            enum: ["entered", "left", "either"],
            default: "entered",
          },
        },
      }).map((field) => [field.key, field.kind]),
    ).toEqual([
      ["list_id", "live_list"],
      ["shortlist_id", "shortlist"],
      ["direction", "enum"],
    ]);
  });

  it("says why it paused itself in short, the whole reason on a press, and nothing when paused by hand", async () => {
    const user = userEvent.setup();
    const { rerender } = render(
      <StoryProviders>
        <RulePausedReason automation={{ ...rule, paused_reason: "burst" }} />
      </StoryProviders>,
    );
    await user.click(
      screen.getByRole("button", { name: en["auto.pausedShort.burst"] }),
    );
    expect(screen.getByText(en["auto.pausedReason.burst"])).toBeInTheDocument();
    rerender(
      <StoryProviders>
        <RulePausedReason automation={rule} />
      </StoryProviders>,
    );
    expect(
      screen.queryByText(en["auto.pausedShort.burst"]),
    ).not.toBeInTheDocument();
  });
});

describe("the list pickers", () => {
  it("offers the Live Lists that are not archived, and picks one by id", async () => {
    listsBackend({
      dynamic: [
        liveList,
        {
          ...liveList,
          id: "archived",
          name: "Old list",
          archived_at: "2026-09-01T00:00:00Z",
        },
      ],
    });
    const picked: string[] = [];
    const user = userEvent.setup();
    picker("live_list", "", (value) => picked.push(value));
    const control = await screen.findByRole("combobox", { name: "live_list" });
    await waitFor(() => expect(control).toBeEnabled());
    await pickOption(user, control, liveList.name);
    expect(picked).toEqual([liveList.id]);
    await user.click(control);
    expect(
      within(screen.getByRole("listbox")).queryByRole("option", {
        name: "Old list",
      }),
    ).not.toBeInTheDocument();
  });

  it("asks for the watched list before it offers a Shortlist", async () => {
    listsBackend({ dynamic: [liveList] });
    picker("shortlist", "");
    expect(
      await screen.findByText(en["auto.lists.needsWatched"]),
    ).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "shortlist" })).toBeDisabled();
  });

  it("offers only Shortlists of the watched list's record type that the reader may change", async () => {
    const asked = listsBackend({
      dynamic: [liveList],
      static: [
        { ...shortlist, can_edit: true },
        { ...shortlist, id: "locked", name: "Locked list", can_edit: false },
      ],
    });
    const user = userEvent.setup();
    picker("shortlist", liveList.id);
    const control = await screen.findByRole("combobox", { name: "shortlist" });
    await waitFor(() => expect(control).toBeEnabled());
    await user.click(control);
    const options = within(screen.getByRole("listbox")).getAllByRole("option");
    expect(options.map((o) => o.textContent)).toEqual([shortlist.name]);
    expect(
      asked.some(
        (q) =>
          q.get("list_type") === "static" &&
          q.get("entity_type") === liveList.entity_type,
      ),
    ).toBe(true);
  });

  it("says when there is no Live List yet, and when lists could not be read", async () => {
    listsBackend({ dynamic: [] });
    picker("live_list", "");
    expect(
      await screen.findByText(en["auto.lists.noLive"]),
    ).toBeInTheDocument();
    cleanup();
    listsBackend({ dynamic: "fail" });
    picker("live_list", "");
    expect(
      await screen.findByText(en["auto.lists.loadError"]),
    ).toBeInTheDocument();
  });

  it("says when no Shortlist of that record type can be changed", async () => {
    listsBackend({ dynamic: [liveList], static: [] });
    picker("shortlist", liveList.id);
    expect(
      await screen.findByText(en["auto.lists.noShortlist"]),
    ).toBeInTheDocument();
  });
});

describe("deleting an automation", () => {
  it("asks first, and only the confirm deletes", async () => {
    const deleted: string[] = [];
    vi.stubGlobal(
      "fetch",
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const request = input instanceof Request ? input : null;
        const method = request?.method ?? init?.method ?? "GET";
        if (method === "DELETE") {
          deleted.push(String(request?.url ?? input));
          return new Response(null, { status: 204 });
        }
        return jsonResponse({ data: [], page: { has_more: false } });
      },
    );
    const user = userEvent.setup();
    render(
      <StoryProviders>
        <DeleteAutomationAction automation={rule} />
      </StoryProviders>,
    );
    await user.click(screen.getByRole("button", { name: en["auto.delete"] }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText(new RegExp(rule.name))).toBeInTheDocument();
    expect(deleted).toEqual([]);
    await user.click(
      within(dialog).getByRole("button", { name: en["auto.delete"] }),
    );
    await waitFor(() => expect(deleted).toHaveLength(1));
    expect(deleted[0]).toContain(`/automations/${rule.id}`);
  });
});

describe("a list rule's form", () => {
  it("draws the list pickers its schema names, and the Shortlist waits for the Live List", async () => {
    listsBackend({
      dynamic: [liveList],
      static: [{ ...shortlist, can_edit: true }],
    });
    const user = userEvent.setup();
    render(
      <StoryProviders>
        <AutomationForm
          entry={{
            key: "list_membership_shortlist",
            name: "Add to a Shortlist",
            trigger: "list.evaluated",
            action: "add_to_shortlist",
            params_schema: {
              type: "object",
              properties: {
                list_id: { type: "string", format: "live_list" },
                shortlist_id: { type: "string", format: "shortlist" },
              },
            },
          }}
          formId="automation"
          initialName="Add to a Shortlist"
          onSubmit={() => undefined}
        />
      </StoryProviders>,
    );
    expect(
      await screen.findByText(en["auto.lists.needsWatched"]),
    ).toBeInTheDocument();
    const watched = screen.getByRole("combobox", { name: "list_id" });
    await waitFor(() => expect(watched).toBeEnabled());
    await pickOption(user, watched, liveList.name);
    const target = screen.getByRole("combobox", { name: "shortlist_id" });
    await waitFor(() => expect(target).toBeEnabled());
  });
});
