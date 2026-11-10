// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { ActivityGate } from "../../components/activitygate";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { type LogDrawer, ProjectLogVerbs } from "./projectlogverbs";

const meta: Meta = {
  title: "Records/Project 360/Log verbs",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const OPEN: ActivityGate = {
  refusedId: undefined,
  pending: false,
  sentenceId: "log-refused",
};

const REFUSED: ActivityGate = {
  refusedId: "log-refused",
  pending: false,
  sentenceId: "log-refused",
  refusalKey: "record.logActivityRefused",
};

function verbs(activity: ActivityGate, opened: LogDrawer = null) {
  function Host() {
    const [drawer, setDrawer] = useState<LogDrawer>(opened);
    return (
      <ProjectLogVerbs
        projectId="pr-1"
        activity={activity}
        drawer={drawer}
        onDrawer={setDrawer}
      />
    );
  }
  return () => {
    installFetchStub({ "GET /me": meRoute({ activity: ["create"] }) });
    return (
      <StoryProviders>
        <Host />
      </StoryProviders>
    );
  };
}

/** Log activity for a seat that may log. */
export const Live: Story = { render: verbs(OPEN) };

/** A seat that may not log: the verb stays, refused, over the one sentence. */
export const Refused: Story = { render: verbs(REFUSED) };

/** The drawer the menu's Add task opens, on the task form. */
export const AddTaskOpen: Story = { render: verbs(OPEN, "task") };
