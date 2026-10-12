/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { translate } from "../i18n";
import { en } from "../i18n/en";
import type { FilterVocabulary } from "./filterdata";
import { FiltersScreen } from "./filters";
import { type FiltersServer, mountFilters, type Sent } from "./filters.testkit";
import { TEAM_ID } from "./lists.fixtures";

// "Save this filter", driven from the page that offers it: what each answer
// sends, where the reader lands, and what the dialog keeps between openings.

const CONTACT_VOCAB: FilterVocabulary = {
  resource: "contact",
  fields: [
    {
      name: "full_name",
      type: "text",
      operators: ["eq", "contains"],
      custom: false,
    },
  ],
};

const FILTER = { and: [{ field: "full_name", op: "eq", value: "ann" }] };
const NAME = en["views.name"];
const SAVE_VIEW = en["views.save"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

function mount(server: FiltersServer = {}) {
  const mounted = mountFilters({
    preview: { match_count: 2 },
    vocabularies: { contact: CONTACT_VOCAB },
    ...server,
  });
  render(withToasts(<FiltersScreen id="contacts" />), {
    wrapper: mounted.wrapper,
  });
  return mounted;
}

/** The confirmations a save shows, drawn where a test can read them. */
const withToasts = (page: ReactNode) => (
  <ToastProvider>
    {page}
    <ToastRegion />
  </ToastProvider>
);

/** One complete condition, then Save: the dialog open on the page. */
async function openSave(user: ReturnType<typeof userEvent.setup>) {
  await user.click(
    await screen.findByRole("button", { name: en["filters.addClause"] }),
  );
  await user.type(screen.getByLabelText("Value"), "ann");
  await user.click(
    await screen.findByRole("button", { name: en["filters.save"] }),
  );
  return screen.findByRole("dialog");
}

const posted = (written: readonly Sent[], path: string) =>
  written.filter((sent) => sent.url.endsWith(path)).map((sent) => sent.body);

describe("saving with lists switched off", () => {
  it("asks for a name only, and refuses a blank one", async () => {
    mount();
    const user = userEvent.setup();
    const dialog = await openSave(user);

    expect(dialog).toHaveTextContent(en["filters.saveTitle"]);
    expect(screen.queryByRole("radio")).toBeNull();
    expect(screen.getByRole("button", { name: SAVE_VIEW })).toBeDisabled();
    await user.type(screen.getByRole("textbox", { name: NAME }), "   ");
    expect(screen.getByRole("button", { name: SAVE_VIEW })).toBeDisabled();
  });

  it("saves the tree as a view and opens it", async () => {
    const { written } = mount();
    const user = userEvent.setup();
    await openSave(user);
    expect(screen.getByRole("textbox", { name: NAME })).toHaveAttribute(
      "placeholder",
      "German contacts, quiet for 45 days",
    );
    await user.type(screen.getByRole("textbox", { name: NAME }), "Anns");
    await user.click(screen.getByRole("button", { name: SAVE_VIEW }));

    await waitFor(() =>
      expect(window.location.hash).toBe("#/filters/contacts/v-new"),
    );
    // `contacts`, not `contact`: `/views` spells the object its own way, and
    // the tree goes under `filter` carrying no editor ids.
    expect(posted(written, "/views")).toEqual([
      { resource: "contacts", name: "Anns", query: { filter: FILTER } },
    ]);
    expect(screen.getByText(en["filters.viewSaved"])).toBeInTheDocument();
  });

  it("keeps the dialog and the name when the server refuses, and starts empty when opened again", async () => {
    mount({
      createRefused: { status: 422, detail: "That name is already taken." },
    });
    const user = userEvent.setup();
    await openSave(user);
    await user.type(screen.getByRole("textbox", { name: NAME }), "Anns");
    await user.click(screen.getByRole("button", { name: SAVE_VIEW }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "That name is already taken.",
    );
    expect(screen.getByRole("textbox", { name: NAME })).toHaveValue("Anns");
    expect(window.location.hash).toBe("");

    await user.click(screen.getByRole("button", { name: en["create.cancel"] }));
    await user.click(screen.getByRole("button", { name: en["filters.save"] }));
    // A reopened create offering the last name is how a duplicate gets made.
    expect(await screen.findByRole("textbox", { name: NAME })).toHaveValue("");
    expect(screen.queryByRole("alert")).toBeNull();
  });
});

describe("saving with lists switched on", () => {
  const listsOn = { listsOn: true, teams: { [TEAM_ID]: "Team Germany" } };

  it("offers a saved view first, and asks who can find a Live List", async () => {
    mount(listsOn);
    const user = userEvent.setup();
    await openSave(user);

    expect(
      screen.getByRole("group", { name: en["filters.keepAs"] }),
    ).toBeInTheDocument();
    expect(screen.getByRole("radio", { name: /^Saved view/ })).toBeChecked();
    expect(
      screen.queryByRole("combobox", { name: en["lists.sharingLabel"] }),
    ).toBeNull();

    await user.click(screen.getByRole("radio", { name: /^Live List/ }));
    expect(
      screen.getByRole("combobox", { name: en["lists.sharingLabel"] }),
    ).toHaveTextContent(en["lists.sharing.team"]);
    expect(
      screen.getByRole("combobox", { name: en["lists.teamLabel"] }),
    ).toHaveTextContent(en["lists.team.allMine"]);
    expect(
      screen.getByRole("textbox", { name: en["filters.purpose"] }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: en["filters.saveListConfirm"] }),
    ).toBeInTheDocument();
  });

  it("creates the Live List the reader named, for all of their teams, and opens it", async () => {
    const { written } = mount(listsOn);
    const user = userEvent.setup();
    await openSave(user);
    await user.click(screen.getByRole("radio", { name: /^Live List/ }));
    await user.type(screen.getByRole("textbox", { name: NAME }), "Anns");
    await user.click(
      screen.getByRole("button", { name: en["filters.saveListConfirm"] }),
    );

    await waitFor(() => expect(window.location.hash).toBe("#/lists/new-list"));
    // No purpose given, so the body is the one a create has always sent.
    expect(posted(written, "/lists")).toEqual([
      {
        name: "Anns",
        entity_type: "contact",
        list_type: "dynamic",
        definition: FILTER,
        sharing: "team",
      },
    ]);
    expect(
      screen.getByText(
        translate("en", "filters.listCreated", { name: "Anns" }),
      ),
    ).toBeInTheDocument();
  });

  it("sends the purpose and the team the reader picked", async () => {
    const { written } = mount(listsOn);
    const user = userEvent.setup();
    await openSave(user);
    await user.click(screen.getByRole("radio", { name: /^Live List/ }));
    await user.type(screen.getByRole("textbox", { name: NAME }), "Anns");
    await user.type(
      screen.getByRole("textbox", { name: en["filters.purpose"] }),
      "Callbacks for the fair ",
    );
    await user.click(
      screen.getByRole("combobox", { name: en["lists.teamLabel"] }),
    );
    await user.click(
      await screen.findByRole("option", { name: "Team Germany" }),
    );
    await user.click(
      screen.getByRole("button", { name: en["filters.saveListConfirm"] }),
    );

    await waitFor(() => expect(posted(written, "/lists")).toHaveLength(1));
    expect(posted(written, "/lists")[0]).toMatchObject({
      purpose: "Callbacks for the fair",
      sharing: "team",
      team_id: TEAM_ID,
    });
  });

  it("holds the dialog and its answer while the save is on its way, then opens what it made", async () => {
    let answer = () => {};
    mount({
      ...listsOn,
      createAnswered: new Promise<void>((resolve) => {
        answer = resolve;
      }),
    });
    const user = userEvent.setup();
    await openSave(user);
    await user.type(screen.getByRole("textbox", { name: NAME }), "Anns");
    await user.click(screen.getByRole("button", { name: SAVE_VIEW }));

    // Escape reaches past the disabled Cancel; closing now would hide the
    // dialog while the view is still being made, and drop the arrival.
    await user.keyboard("{Escape}");
    expect(screen.getByRole("dialog")).toHaveTextContent(
      en["filters.saveTitle"],
    );
    expect(screen.getByRole("radio", { name: /^Live List/ })).toBeDisabled();

    answer();
    await waitFor(() =>
      expect(window.location.hash).toBe("#/filters/contacts/v-new"),
    );
    expect(screen.getByText(en["filters.viewSaved"])).toBeInTheDocument();
  });

  it("sends only-me sharing with no team", async () => {
    const { written } = mount(listsOn);
    const user = userEvent.setup();
    await openSave(user);
    await user.click(screen.getByRole("radio", { name: /^Live List/ }));
    await user.type(screen.getByRole("textbox", { name: NAME }), "Mine");
    await user.click(
      screen.getByRole("combobox", { name: en["lists.sharingLabel"] }),
    );
    await user.click(
      await screen.findByRole("option", { name: en["lists.sharing.private"] }),
    );
    expect(
      screen.queryByRole("combobox", { name: en["lists.teamLabel"] }),
    ).toBeNull();
    await user.click(
      screen.getByRole("button", { name: en["filters.saveListConfirm"] }),
    );

    await waitFor(() => expect(posted(written, "/lists")).toHaveLength(1));
    expect(posted(written, "/lists")[0]).toMatchObject({ sharing: "private" });
    expect(posted(written, "/lists")[0]).not.toHaveProperty("team_id");
  });
});
