// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { Panel, PanelBody } from "../design-system/panel";
import { useT } from "../i18n";
import { useFilterExport } from "./filterexport";
import { FilterFoot } from "./filterfoot";
import { SaveToListAction } from "./filterlistedit";
import { listsMe, liveList } from "./lists.fixtures";
import { newGroup, newLeaf } from "./segmentpredicate";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The filter Panel's footer band, drawn once a filter is complete: what state
// it is in on the leading edge, and the one emerald Save on the trailing edge
// with the exports folded into More beside it.
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

/** The band under the editor it belongs to, as the page draws it. */
function Footed({ listFilter = false }: Readonly<{ listFilter?: boolean }>) {
  const t = useT();
  const run = useFilterExport();
  return (
    <Panel
      title={t("filters.find", { records: t("unit.companies") })}
      footer={
        <FilterFoot
          resource="company"
          tree={COMPLETE}
          onSave={() => undefined}
          exportRun={run}
          saveTo={
            listFilter ? (
              <SaveToListAction
                edited={{ list: liveList, version: liveList.version }}
                tree={COMPLETE}
              />
            ) : undefined
          }
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
    return <Footed listFilter />;
  },
  play: async ({ canvasElement }) => {
    await openMore(canvasElement);
  },
};
