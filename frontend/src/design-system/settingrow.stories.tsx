// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, within } from "storybook/test";
import { Button, Card, Disclosure, SegmentedControl, TextInput } from "./atoms";
import { ChoiceList } from "./choicelist";
import { DataTable } from "./datatable";
import { Panel, PanelBody, PanelIntro } from "./panel";
import { Select } from "./select";
import { SettingList, SettingRow } from "./settingrow";
import { Switch } from "./switch";

type RefusedDomain = Readonly<{ domain: string; by: string }>;

const REFUSED: RefusedDomain[] = [
  { domain: "gmail.com", by: "Capture sink" },
  { domain: "t-online.de", by: "Marek Janetzke" },
];

const meta: Meta<typeof SettingRow> = {
  title: "Components/Layout and structure/Setting row",
  component: SettingRow,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof SettingRow>;

/**
 * The four shapes a settings card is built from, in one card, because the whole
 * point of the pair is that they line up: a reader auditing this page finds
 * every answer at the same x.
 */
function Catalog() {
  const [enrich, setEnrich] = useState(true);
  const [digest, setDigest] = useState(false);
  const [locale, setLocale] = useState("en");
  const [profile, setProfile] = useState("balanced");
  return (
    <Card title="Capture">
      <SettingList>
        <SettingRow
          label="Auto-enrich captured companies"
          description="Looks a company up the first time it is captured, and fills what the mail did not carry."
          control={
            <Switch
              label="Auto-enrich captured companies"
              labelHidden
              checked={enrich}
              onChange={setEnrich}
            />
          }
        />
        <SettingRow
          label="Weekly digest"
          description="One mail on Monday with what moved."
          control={
            <Switch
              label="Weekly digest"
              labelHidden
              checked={digest}
              onChange={setDigest}
            />
          }
        />
        <SettingRow
          label="Language"
          description="The language this installation speaks to you in."
          control={(control) => (
            <Select
              {...control}
              className="settingrow-measure"
              options={[
                { value: "en", label: "English" },
                { value: "de", label: "Deutsch" },
              ]}
              value={locale}
              onChange={setLocale}
            />
          )}
        />
        <SettingRow
          label="Reply-to address"
          value="marek@gradion.com"
          description="Where a reply to a captured thread is sent."
          control={<Button variant="ghost">Edit</Button>}
        />
        <SettingRow
          label="Extraction profile"
          description="How much the model may infer from a thread it has not seen before."
          layout="stack"
          control={
            <ChoiceList
              legend="Extraction profile"
              hideLegend
              value={profile}
              onChange={setProfile}
              choices={[
                {
                  value: "strict",
                  label: "Strict",
                  description: "Only what the thread states outright.",
                },
                {
                  value: "balanced",
                  label: "Balanced",
                  description: "Infers a role or a company from context.",
                },
              ]}
            />
          }
        />
        <SettingRow
          label="Refused domains"
          description="Domains this installation would not turn into a company, and who decided."
          layout="stack"
          control={
            <DataTable
              label={"Refused domains"}
              columns={[
                {
                  key: "domain",
                  header: "Domain",
                  render: (row: RefusedDomain) => row.domain,
                },
                {
                  key: "by",
                  header: "Decided by",
                  render: (row: RefusedDomain) => row.by,
                },
              ]}
              rows={REFUSED}
              rowKey={(row) => row.domain}
            />
          }
        />
        <Disclosure summary="Advanced">
          <SettingList>
            <SettingRow
              label="Retry a refused capture"
              description="Runs the sink again over mail it dropped in the last 24 hours."
              control={<Button variant="ghost">Run…</Button>}
            />
            <SettingRow
              label="Sink concurrency"
              description="How many mailboxes the sink reads at once."
              control={(control) => (
                <TextInput
                  {...control}
                  className="settingrow-measure"
                  type="number"
                  defaultValue={4}
                />
              )}
            />
          </SettingList>
        </Disclosure>
      </SettingList>
    </Card>
  );
}

export const Catalogue: Story = { render: () => <Catalog /> };

// At 390px a switch keeps its name's line while the description wraps beside
// it; a dropdown still drops under its name.
export const SwitchBesideNameAtPhoneWidth: Story = {
  render: () => <Catalog />,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const rect = (element: Element) => element.getBoundingClientRect();
    const name = rect(
      canvas.getByText("Auto-enrich captured companies", {
        selector: ".t-name",
      }),
    );
    const track = rect(
      await canvas.findByRole("switch", {
        name: "Auto-enrich captured companies",
      }),
    );
    await expect(track.top).toBeGreaterThanOrEqual(name.top);
    await expect(track.top).toBeLessThan(name.bottom);
    await expect(track.left).toBeGreaterThan(name.left);
    const language = rect(canvas.getByText("Language"));
    const select = rect(canvas.getByRole("combobox", { name: "Language" }));
    await expect(select.top).toBeGreaterThanOrEqual(language.bottom);
  },
};

// A switch with a hint under it needs the width, so at 390px it drops under
// its name like any other control.
export const SwitchWithHintStacksAtPhoneWidth: Story = {
  render: () => (
    <Card title="Capture">
      <SettingList>
        <SettingRow
          label="Weekly digest"
          description="One mail on Monday with what moved."
          control={
            <Switch
              label="Weekly digest"
              labelHidden
              checked
              onChange={() => undefined}
              hint="Sent at 07:00 in your time zone."
            />
          }
        />
      </SettingList>
    </Card>
  ),
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const name = canvas
      .getByText("Weekly digest", { selector: ".t-name" })
      .getBoundingClientRect();
    const track = (
      await canvas.findByRole("switch", { name: "Weekly digest" })
    ).getBoundingClientRect();
    await expect(track.top).toBeGreaterThanOrEqual(name.bottom);
  },
};

// bleed="settings": a form standing straight in the Panel. The hairlines reach
// the pane's edges and the row text keeps the intro's x; no row lights up.
function AccountForm() {
  const [digest, setDigest] = useState(true);
  return (
    <Panel title="Account">
      <PanelBody>
        <PanelIntro>How this workspace reaches you.</PanelIntro>
      </PanelBody>
      <SettingList bleed="settings">
        <SettingRow
          label="Reply-to address"
          value="marek@gradion.com"
          control={<Button variant="ghost">Edit</Button>}
        />
        <SettingRow
          label="Weekly digest"
          description="One mail on Monday with what moved."
          control={
            <Switch
              label="Weekly digest"
              labelHidden
              checked={digest}
              onChange={setDigest}
            />
          }
        />
        <SettingRow
          label="Refused domains"
          layout="stack"
          control={
            <DataTable
              label="Refused domains"
              columns={[
                {
                  key: "domain",
                  header: "Domain",
                  render: (row: RefusedDomain) => row.domain,
                },
                {
                  key: "by",
                  header: "Decided by",
                  render: (row: RefusedDomain) => row.by,
                },
              ]}
              rows={REFUSED}
              rowKey={(row) => row.domain}
            />
          }
        />
      </SettingList>
      <PanelBody>
        <PanelIntro>Changes apply to the next mail sent.</PanelIntro>
      </PanelBody>
    </Panel>
  );
}

export const BleedSettings: Story = { render: () => <AccountForm /> };

// bleed="records": one row per thing, so every row takes a table row's hover,
// pressable or not.
function RolesList() {
  return (
    <Panel title="Roles">
      <PanelBody>
        <PanelIntro>What each role may read and change.</PanelIntro>
      </PanelBody>
      <SettingList bleed="records">
        <SettingRow
          label="Administrator"
          description="Every setting and every record."
          value="2 members"
          control={<Button variant="ghost">Edit</Button>}
        />
        <SettingRow
          label="Sales rep"
          description="Their own deals and the accounts on them."
          value="5 members"
          control={<Button variant="ghost">Edit</Button>}
        />
        <SettingRow
          label="Read only"
          description="Reads every record and changes none."
          value="1 member"
          control={null}
        />
      </SettingList>
    </Panel>
  );
}

export const BleedRecords: Story = { render: () => <RolesList /> };

// The phone gate's browser runs at 390px only under `uat-phone`. The frame is
// the page's own gutter, so the pane meets it the way a settings page does.
export const BleedAtPhoneWidth: Story = {
  render: () => (
    <div
      style={{
        display: "grid",
        gap: "var(--space-4)",
        padding: "var(--padCard)",
      }}
    >
      <AccountForm />
      <RolesList />
    </div>
  ),
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

const DENSITIES = ["compact", "comfortable"] as const;
type Density = (typeof DENSITIES)[number];

// A short pick among two or three answers sits in the control column and,
// like a Switch, writes on the pick.
function DensityRow() {
  const [density, setDensity] = useState<Density>("comfortable");
  return (
    <Panel title="Display">
      <SettingList bleed="settings">
        <SettingRow
          label="Row density"
          description="How tightly lists pack their rows."
          control={
            <SegmentedControl
              label="Row density"
              options={DENSITIES}
              value={density}
              onChange={setDensity}
              labels={{ compact: "Compact", comfortable: "Comfortable" }}
            />
          }
        />
      </SettingList>
    </Panel>
  );
}

export const SegmentedControlControl: Story = {
  render: () => <DensityRow />,
};

function rowBoxes(row: Element) {
  const naming = row.querySelector(".settingrow-naming");
  const control = row.querySelector(".settingrow-control");
  if (naming === null || control === null) {
    throw new Error("a setting row drew no naming or no control");
  }
  return {
    naming: naming.getBoundingClientRect(),
    control: control.getBoundingClientRect(),
  };
}

// A card as narrow as a tablet's settings column. The long answer goes under
// its naming instead of squeezing it; the short one beside it keeps its line.
export const LongAnswerInNarrowCard: Story = {
  render: () => (
    <div style={{ maxWidth: "32rem" }}>
      <Panel title="Account">
        <SettingList bleed="settings">
          <SettingRow
            label="Email signature"
            description="Sits under every message you send, above the unsubscribe footer."
            value="Kind regards, Alice Müller, Head of Sales"
            control={<Button variant="ghost">Edit signature</Button>}
          />
          <SettingRow
            label="Reply-to address"
            value="alice@demo.test"
            control={<Button variant="ghost">Edit</Button>}
          />
        </SettingList>
      </Panel>
    </div>
  ),
  play: async ({ canvasElement }) => {
    const [long, short] = canvasElement.querySelectorAll(".settingrow");
    if (long === undefined || short === undefined) {
      throw new Error("the card drew fewer than two setting rows");
    }
    const wrapped = rowBoxes(long);
    await expect(wrapped.control.top).toBeGreaterThanOrEqual(
      wrapped.naming.bottom,
    );
    await expect(wrapped.control.right).toBeLessThanOrEqual(
      long.getBoundingClientRect().right,
    );
    const beside = rowBoxes(short);
    await expect(beside.control.top).toBeLessThan(beside.naming.bottom);
  },
};

// 32rem is the pane beside the rail at 768px. A story cannot set that viewport,
// and the collapse depends only on the row's own width.
export const LongValueInNarrowPane: Story = {
  render: () => (
    <div style={{ width: "32rem" }}>
      <SettingList>
        <SettingRow
          label="Projection basis for the quarterly forecast"
          value="Weighted pipeline by expected close date across every open opportunity in the workspace"
          control={<Button>Edit</Button>}
          testId="long-row"
        />
      </SettingList>
    </div>
  ),
  play: async ({ canvasElement }) => {
    const label = await within(canvasElement).findByText(
      "Projection basis for the quarterly forecast",
    );
    const naming = label.closest(".settingrow-naming");
    expect(naming?.getBoundingClientRect().width).toBeGreaterThanOrEqual(120);
  },
};
