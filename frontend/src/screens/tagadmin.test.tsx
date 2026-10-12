// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import {
  cleanup,
  render,
  renderHook,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { TAG_TONES } from "../design-system/tagpill";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { translate } from "../i18n";
import { en } from "../i18n/en";
import { useSentenceWords } from "./filtersentence";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";
import { TagVocabularyCard } from "./tagadmin";

// Settings › Data model: the one door that coins a word. Every verb but merge
// is reversible, and merge says so.

const KEY_ACCOUNT = {
  id: "t-1",
  workspace_id: "w",
  name: "Key Account",
  color: "amber",
  version: 3,
  carried_by: 7,
};
const RETIRED = {
  id: "t-2",
  workspace_id: "w",
  name: "Trade Fair 2025",
  version: 1,
  archived_at: "2026-01-01T00:00:00Z",
  carried_by: 0,
};

const ADMIN = { tag: ["read", "create", "update", "delete"] };

function routeTags(
  words: readonly unknown[],
  grants: Record<string, string[]>,
  extra: RouteMap,
  asked: URL[],
) {
  installFetchStub({
    "GET /me": meRoute(grants as never),
    "GET /tags": () =>
      jsonResponse({
        data: words,
        page: { has_more: false, next_cursor: null },
      }),
    ...extra,
  });
  const routed = globalThis.fetch;
  globalThis.fetch = (input, init) => {
    const url = input instanceof Request ? input.url : String(input);
    asked.push(new URL(url, "http://localhost"));
    return routed(input, init);
  };
}

function mount(
  words: readonly unknown[],
  grants: Record<string, string[]> = ADMIN,
  extra: RouteMap = {},
  asked: URL[] = [],
) {
  routeTags(words, grants, extra, asked);
  render(
    <StoryProviders>
      <ToastProvider>
        <TagVocabularyCard />
        <ToastRegion />
      </ToastProvider>
    </StoryProviders>,
  );
}

// The row's own menu: every row offers one, and "the first menu" would pass
// whichever row it opened.
async function openRowMenu(
  user: ReturnType<typeof userEvent.setup>,
  name: string,
) {
  const trigger = await screen.findByRole("button", {
    name: translate("en", "table.rowActions", { name }),
  });
  await user.click(trigger);
  const items = document.getElementById(
    trigger.getAttribute("aria-controls") ?? "",
  );
  if (!items) {
    throw new Error(`the menu for ${name} drew no items`);
  }
  return within(items);
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the tag vocabulary card", () => {
  it("draws the workspace's words", async () => {
    mount([KEY_ACCOUNT]);
    expect(await screen.findByText("Key Account")).toBeInTheDocument();
  });

  // Counting reads every tagging, so this card asks and the filter sentence
  // on every list page does not.
  it("asks the list for each word's count and draws it", async () => {
    const asked: URL[] = [];
    mount([KEY_ACCOUNT], ADMIN, {}, asked);
    expect(
      await screen.findByText(
        translate("en", "tagAdmin.usedBy_other", { count: "7" }),
      ),
    ).toBeInTheDocument();
    const list = asked.find((url) => url.pathname.endsWith("/tags"));
    expect(list?.searchParams.get("with_carried_by")).toBe("true");
  });

  it("leaves the count out of the catalog a filter sentence reads", async () => {
    const asked: URL[] = [];
    routeTags([], ADMIN, {}, asked);
    renderHook(() => useSentenceWords(), { wrapper: StoryProviders });
    await waitFor(() =>
      expect(asked.some((url) => url.pathname.endsWith("/tags"))).toBe(true),
    );
    const list = asked.find((url) => url.pathname.endsWith("/tags"));
    expect(list?.searchParams.get("include_archived")).toBe("true");
    expect(list?.searchParams.has("with_carried_by")).toBe(false);
  });

  // A retired word is restored HERE, so a list that hid it would leave the
  // verb with nothing to act on and an admin concluding the word was deleted.
  it("lists a retired word as retired, offering to restore rather than retire it", async () => {
    const user = userEvent.setup();
    mount([RETIRED]);
    const row = await screen.findByTestId("tag-t-2");
    expect(within(row).getByText(en["tagAdmin.retired"])).toBeInTheDocument();
    const menu = await openRowMenu(user, "Trade Fair 2025");
    expect(
      menu.getByRole("button", { name: en["tagAdmin.restore"] }),
    ).toBeInTheDocument();
    expect(
      menu.queryByRole("button", { name: en["tagAdmin.archive"] }),
    ).toBeNull();
    expect(
      menu.queryByRole("button", { name: en["tagAdmin.edit"] }),
    ).toBeNull();
  });

  // Applying a tag is every seat's; coining one is not. The card is drawn for
  // a reader who may see the vocabulary, and the verbs answer to the grants.
  it("offers no verbs to a seat that may only read the vocabulary", async () => {
    mount([KEY_ACCOUNT], { tag: ["read"] });
    expect(await screen.findByText("Key Account")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: en["tagAdmin.add"] }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", {
        name: translate("en", "table.rowActions", { name: "Key Account" }),
      }),
    ).toBeNull();
  });

  // The accident this prevents: an admin looks for a word, does not see it,
  // and is one press from a second spelling of one that already exists.
  it("warns when a new name is close to a word the workspace has", async () => {
    const user = userEvent.setup();
    mount([KEY_ACCOUNT]);
    await user.click(
      await screen.findByRole("button", { name: en["tagAdmin.add"] }),
    );
    await user.type(
      screen.getByLabelText(en["tagAdmin.nameLabel"]),
      "key-account",
    );
    // The heading makes the claim; the body names the word it is close to, so
    // both halves are asserted — a heading alone would pass on a notice that
    // never says which word.
    expect(
      await screen.findByText(en["tagAdmin.nearMatchTitle"]),
    ).toBeInTheDocument();
    expect(screen.getByText(/apply the existing tag/i)).toBeInTheDocument();
  });

  // An admin picks a tag's colour BY the colour. A list of tone words is what
  // the picker used to be, and it made choosing a hue a matter of knowing what
  // "slate" looks like — so the swatch is the feature and its absence is the
  // defect. Asserted through the dot class the pill itself draws, which is what
  // ties the choice made here to the mark that appears on the record.
  it("offers every tone, each drawn as its own dot", async () => {
    const user = userEvent.setup();
    mount([KEY_ACCOUNT]);
    await user.click(
      await screen.findByRole("button", { name: en["tagAdmin.add"] }),
    );
    await user.click(
      screen.getByRole("combobox", { name: en["tagAdmin.colorLabel"] }),
    );

    const listbox = await screen.findByRole("listbox");
    for (const tone of TAG_TONES) {
      const option = within(listbox).getByRole("option", {
        name: en[`tagAdmin.color.${tone}`],
      });
      expect(
        option.querySelector(`.tagpill-dot-${tone}`),
        `${tone} offers no swatch`,
      ).toBeInTheDocument();
    }
    // Clearing a colour is an option too, and the one that carries no dot.
    const none = within(listbox).getByRole("option", {
      name: en["tagAdmin.colorNone"],
    });
    expect(none.querySelector("[class*='tagpill-dot']")).toBeNull();
  });

  // The count rides the list read, so the card asks nothing per word.
  it("says how many records carry each word, from the list alone", async () => {
    const detail = vi.fn(() => jsonResponse(KEY_ACCOUNT));
    mount([KEY_ACCOUNT, RETIRED], ADMIN, { "GET /tags/t-1": detail });
    const row = await screen.findByTestId("tag-t-1");
    expect(within(row).getByText("7 records")).toBeInTheDocument();
    expect(
      within(screen.getByTestId("tag-t-2")).getByText("0 records"),
    ).toBeInTheDocument();
    expect(detail).not.toHaveBeenCalled();
  });

  it("retires a word at once and puts it back through Undo", async () => {
    const user = userEvent.setup();
    const retire = vi.fn(() => new Response(null, { status: 204 }));
    const restore = vi.fn(() => new Response(null, { status: 204 }));
    mount([KEY_ACCOUNT], ADMIN, {
      "DELETE /tags/t-1": retire,
      "POST /tags/t-1/restore": restore,
    });
    const menu = await openRowMenu(user, "Key Account");
    await user.click(
      menu.getByRole("button", { name: en["tagAdmin.archive"] }),
    );

    expect(screen.queryByRole("dialog")).toBeNull();
    const said = await screen.findByRole("status");
    await waitFor(() =>
      expect(said).toHaveTextContent(
        translate("en", "tagAdmin.retiredToast", { name: "Key Account" }),
      ),
    );
    expect(retire).toHaveBeenCalledTimes(1);

    await user.click(
      await within(said).findByRole("button", { name: en["common.undo"] }),
    );
    await waitFor(() => expect(restore).toHaveBeenCalledTimes(1));
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent(
        translate("en", "tagAdmin.restoredToast", { name: "Key Account" }),
      ),
    );
  });

  it("keeps a refused retire on screen as a danger toast", async () => {
    const user = userEvent.setup();
    mount([KEY_ACCOUNT], ADMIN, {
      "DELETE /tags/t-1": () =>
        jsonResponse({ detail: "The tag changed since you opened it." }, 409),
    });
    const menu = await openRowMenu(user, "Key Account");
    await user.click(
      menu.getByRole("button", { name: en["tagAdmin.archive"] }),
    );
    expect(
      await screen.findByText("The tag changed since you opened it."),
    ).toBeInTheDocument();
  });

  // A second create would come back a conflict over the word the first one
  // coined, so a double press must send one.
  it("coins one word for a double press on Add, and closes", async () => {
    const user = userEvent.setup();
    // Held open, so the second press lands while the first is still in flight.
    let answer: (response: Response) => void = () => {};
    const coin = vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          answer = resolve;
        }),
    );
    mount([KEY_ACCOUNT], ADMIN, { "POST /tags": coin });
    await user.click(
      await screen.findByRole("button", { name: en["tagAdmin.add"] }),
    );
    const dialog = within(await screen.findByRole("dialog"));
    await user.type(dialog.getByLabelText(en["tagAdmin.nameLabel"]), "Fresh");
    const add = dialog.getByRole("button", { name: en["tagAdmin.create"] });
    await user.dblClick(add);
    await user.click(add);
    expect(coin).toHaveBeenCalledTimes(1);

    answer(jsonResponse({ id: "t-9", name: "Fresh", version: 1 }, 201));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(coin).toHaveBeenCalledTimes(1);
  });

  // A row read back without a version takes no write: an unpinned PATCH is
  // last-write-wins, landing on top of an edit it never saw and reporting
  // success to both editors. The refusal has to reach the DIALOG — the same
  // throw from a click handler escapes into React and takes the page down.
  it("refuses to save a word that came back with no version, in the dialog", async () => {
    const user = userEvent.setup();
    const { version: _dropped, ...unversioned } = KEY_ACCOUNT;
    mount([unversioned]);
    const menu = await openRowMenu(user, "Key Account");
    await user.click(menu.getByRole("button", { name: en["tagAdmin.edit"] }));
    // Said before the press, not after it: there is nothing the reader can
    // retype to fix this, so offering the verb would be offering a refusal.
    expect(
      await screen.findByText(en["tagAdmin.noVersion"]),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: en["tagAdmin.save"] }),
    ).toBeDisabled();
  });

  // A suggested word needs a description to match mail against, so the
  // dialog holds Save until one is there, then sends both.
  it("sends a suggestible word only with a description of what interest looks like", async () => {
    const user = userEvent.setup();
    const sent: unknown[] = [];
    mount([KEY_ACCOUNT], ADMIN, {
      "PATCH /tags/t-1": (body) => {
        sent.push(body);
        return jsonResponse(KEY_ACCOUNT);
      },
    });
    const menu = await openRowMenu(user, "Key Account");
    await user.click(menu.getByRole("button", { name: en["tagAdmin.edit"] }));
    const dialog = within(await screen.findByRole("dialog"));
    await user.click(
      dialog.getByRole("checkbox", { name: en["tagAdmin.suggestibleLabel"] }),
    );
    const save = dialog.getByRole("button", { name: en["tagAdmin.save"] });
    expect(save).toBeDisabled();

    await user.type(
      dialog.getByRole("textbox", { name: en["tagAdmin.descriptionLabel"] }),
      "Product X demo, pricing",
    );
    await user.click(save);

    await waitFor(() =>
      expect(sent).toEqual([
        expect.objectContaining({
          description: "Product X demo, pricing",
          suggestible: true,
        }),
      ]),
    );
  });

  // The settings entry unions five data-model reads, so this card is mounted
  // for a seat holding one of the OTHER four. Asking anyway is a request whose
  // only answer is 403, and an empty list would read as "no tags here".
  it("says the vocabulary is withheld rather than asking for it", async () => {
    const tags = vi.fn(() =>
      jsonResponse({
        data: [KEY_ACCOUNT],
        page: { has_more: false, next_cursor: null },
      }),
    );
    mount([KEY_ACCOUNT], { custom_field: ["read"] }, { "GET /tags": tags });
    expect(
      await screen.findByText(en["tagAdmin.withheld"]),
    ).toBeInTheDocument();
    expect(screen.queryByText(en["tagAdmin.empty"])).toBeNull();
    expect(tags).not.toHaveBeenCalled();
  });

  // Merge is the one verb that cannot be undone, and the released name is the
  // part an admin does not expect.
  it("says merge cannot be undone and releases the name", async () => {
    const user = userEvent.setup();
    mount([KEY_ACCOUNT, { ...RETIRED, archived_at: null }]);
    const menu = await openRowMenu(user, "Key Account");
    await user.click(menu.getByRole("button", { name: en["tagAdmin.merge"] }));
    expect(
      await screen.findByRole("dialog", {
        name: translate("en", "tagAdmin.mergeTitle", { name: "Key Account" }),
      }),
    ).toBeInTheDocument();
    // The heading is the claim and the body is what it costs, so the released
    // name is asserted on the notice as a whole rather than on its heading.
    const warning = await screen.findByText(en["tagAdmin.mergeWarningTitle"]);
    expect(warning.closest(".callout")?.textContent).toContain(
      "becomes available again",
    );
  });
});
