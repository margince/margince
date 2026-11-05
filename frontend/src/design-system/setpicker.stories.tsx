// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { SetPicker, type SetPickerOption } from "./setpicker";

const meta: Meta<typeof SetPicker> = {
  title: "Components/Forms and input/Set picker",
  component: SetPicker,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof SetPicker>;

const REGIONS = ["DACH", "Nordics", "Benelux", "UK & Ireland", "Iberia"];
const TEAMS: SetPickerOption[] = Array.from({ length: 30 }, (_, index) => {
  const region = REGIONS[index % REGIONS.length] ?? "";
  const kind = ["Sales", "Delivery", "Success"][index % 3] ?? "";
  return {
    value: `team-${index}`,
    label: `${region} ${kind}`,
    detail: `${(index % 7) + 2} members`,
  };
}).filter(
  (team, index, all) => all.findIndex((t) => t.label === team.label) === index,
);

function Live({
  initial,
  options,
}: Readonly<{ initial: string[]; options: SetPickerOption[] }>) {
  const [values, setValues] = useState(initial);
  const [known, setKnown] = useState(options);
  return (
    <div style={{ maxInlineSize: "34rem" }}>
      <SetPicker
        legend="Teams"
        options={known}
        values={values}
        onChange={setValues}
        none={known.length === 0 ? "No teams yet." : "Not on a team yet."}
        addLabel={known.length === 0 ? "New team" : "Add teams"}
        searchLabel={known.length === 0 ? "Team name" : "Search teams"}
        noMatch="No team matches."
        doneLabel="Done"
        empty="Type a name to create the first team."
        removeLabel={(item) => `Remove from ${item.label}`}
        create={{
          label: (name) => `Create team “${name}”`,
          onCreate: (name) => {
            const value = `new-${known.length}`;
            setKnown([...known, { value, label: name }]);
            setValues([...values, value]);
          },
        }}
      />
    </div>
  );
}

export const Chosen: Story = {
  render: () => (
    <Live initial={["team-0", "team-4", "team-8"]} options={TEAMS} />
  ),
};

export const NoneChosen: Story = {
  render: () => <Live initial={[]} options={TEAMS} />,
};

// The first-run state: nothing to pick yet, so the trigger creates.
export const NoTeamsYet: Story = {
  render: () => <Live initial={[]} options={[]} />,
};

export const ReadOnly: Story = {
  render: () => (
    <SetPicker
      legend="Teams"
      options={TEAMS}
      values={["team-0", "team-4"]}
      onChange={() => undefined}
      none="Not on a team yet."
      addLabel="Add teams"
      searchLabel="Search teams"
      noMatch="No team matches."
      doneLabel="Done"
      removeLabel={(item) => `Remove from ${item.label}`}
      disabled
    />
  ),
};
