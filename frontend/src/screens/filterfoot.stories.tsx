// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { Panel, PanelBody } from "../design-system/panel";
import { useT } from "../i18n";
import { useFilterExport } from "./filterexport";
import { FilterFoot, type FootMode } from "./filterfoot";
import { SaveToListAction } from "./filterlistedit";
import { listsMe, liveList } from "./lists.fixtures";
import { newGroup, newLeaf } from "./segmentpredicate";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The filter Panel's footer band: what state the filter is in on the leading
// edge, and the one emerald way to keep it on the trailing edge, with the rarer
// verbs folded into More beside it. A new filter, an opened view and a Live
// List's filter each keep it their own way.
const meta: Meta = {
  title: "Patterns/Filters and views/Filter footer",
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj;

const COMPLETE = newGroup("and", [newLeaf("industry", "eq", "Manufacturing")]);

type Kind = FootMode["kind"];

/** The band under the editor it belongs to, as the page draws it. */
function Footed({
  kind = "new",
  changed = false,
}: Readonly<{ kind?: Kind; changed?: boolean }>) {
  const t = useT();
  const run = useFilterExport();
  const modes: Record<Kind, FootMode> = {
    new: { kind: "new" },
    view: {
      kind: "view",
      changed,
      onDone: () => undefined,
      onDiscard: () => undefined,
      onSaveChanges: () => undefined,
      saving: false,
    },
    list: {
      kind: "list",
      changed,
      onDiscard: () => undefined,
      saveTo: (
        <SaveToListAction
          list={liveList}
          tree={COMPLETE}
          disabled={!changed}
          onSaved={() => undefined}
        />
      ),
    },
  };
  return (
    <Panel
      title={t("filters.builderTitle")}
      footer={
        <FilterFoot
          resource="company"
          tree={COMPLETE}
          mode={modes[kind]}
          onSave={() => undefined}
          exportRun={run}
        />
      }
    >
      <PanelBody>
        <p className="t-sub">{t("filters.start.buildBody")}</p>
      </PanelBody>
    </Panel>
  );
}

type ExportAnswer = "file" | "refused" | "held";

function routes(answer: ExportAnswer = "file") {
  installFetchStub({
    "GET /me": listsMe(true),
    "POST /exports": () => {
      if (answer === "held") {
        return new Promise<Response>(() => undefined);
      }
      return answer === "refused"
        ? jsonResponse(
            {
              title: "Export refused",
              status: 403,
              detail: "Bulk record read is human-only.",
            },
            403,
          )
        : new Response("id\n", { status: 200 });
    },
  });
}

/** More's items are portalled to the body, outside the story's root. */
async function openMore(canvasElement: HTMLElement) {
  await userEvent.click(
    within(canvasElement).getByRole("button", {
      name: "More for this filter",
    }),
  );
  return within(canvasElement.ownerDocument.body);
}

export const UnsavedFilter: Story = {
  render: () => {
    routes();
    return <Footed />;
  },
};

export const MoreOpen: Story = {
  render: () => {
    routes();
    return <Footed />;
  },
  play: async ({ canvasElement }) => {
    await openMore(canvasElement);
  },
};

// The menu that asked has closed, so the server's reason lands in the band.
export const ExportRefused: Story = {
  render: () => {
    routes("refused");
    return <Footed />;
  },
  play: async ({ canvasElement }) => {
    const page = await openMore(canvasElement);
    await userEvent.click(page.getByRole("button", { name: "Export CSV" }));
    await within(canvasElement).findByRole("alert");
  },
};

// The menu closed on the press, so the band says a file is coming.
export const Exporting: Story = {
  render: () => {
    routes("held");
    return <Footed />;
  },
  play: async ({ canvasElement }) => {
    const page = await openMore(canvasElement);
    await userEvent.click(page.getByRole("button", { name: "Export CSV" }));
    await within(canvasElement).findByText("Exporting…");
  },
};

// A Live List's filter opened for editing: "Save to {name}" takes Save's
// place, and saving as a new view moves into More beside the exports.
export const SavingToAList: Story = {
  render: () => {
    routes();
    return <Footed kind="list" changed />;
  },
  play: async ({ canvasElement }) => {
    await openMore(canvasElement);
  },
};

// The list's filter as it opened: nothing to save to it yet.
export const ListUnchanged: Story = {
  render: () => {
    routes();
    return <Footed kind="list" />;
  },
};

// An opened view's rows, unchanged: Done folds them back to the sentence.
export const ViewUnchanged: Story = {
  render: () => {
    routes();
    return <Footed kind="view" />;
  },
};

// An opened view, changed: discard, keep as a new view, or save it back.
export const ViewChanged: Story = {
  render: () => {
    routes();
    return <Footed kind="view" changed />;
  },
};
