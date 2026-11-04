// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Panel, PanelBody } from "../../design-system/panel";
import { useT } from "../../i18n";
import { newGroup, newLeaf } from "../../lib/segmentpredicate";
import { listsMe, liveList } from "../../testing/lists.fixtures";
import { installFetchStub, StoryProviders } from "../../testing/story-utils";
import { FilterFoot, type FootMode } from "./filterfoot";
import { SaveToListAction } from "./filterlistedit";

// The filter Panel's footer band. The leading edge says what state the filter
// is in. The trailing edge holds the ways to keep it, the one accent last. A
// new filter, an opened view and a Live List's filter each keep it their own
// way.
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
          tree={COMPLETE}
          mode={modes[kind]}
          onSave={() => undefined}
        />
      }
    >
      <PanelBody>
        <p className="t-sub">{t("filters.start.buildBody")}</p>
      </PanelBody>
    </Panel>
  );
}

function routes() {
  installFetchStub({ "GET /me": listsMe(true) });
}

export const UnsavedFilter: Story = {
  render: () => {
    routes();
    return <Footed />;
  },
};

// A Live List's filter opened for editing: "Save to {name}" takes Save's
// place, and saving as a new view stands beside it.
export const SavingToAList: Story = {
  render: () => {
    routes();
    return <Footed kind="list" changed />;
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
