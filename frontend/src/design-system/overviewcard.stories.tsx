// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Badge, Button } from "./atoms";
import { CallAction } from "./callaction";
import { Checkbox } from "./field";
import {
  OverviewCard,
  OverviewList,
  OverviewRow,
  OverviewText,
} from "./overviewcard";

const meta: Meta<typeof OverviewCard> = {
  title: "Components/Layout and structure/Overview card",
  component: OverviewCard,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: "26rem" }}>
        <Story />
      </div>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof OverviewCard>;

const link = (label: string) => <Button variant="link">{label}</Button>;

/** Tasks with a tick box and a tone on the right. */
export const Tasks: Story = {
  render: () => (
    <OverviewCard title="Also on your list" action={link("All tasks")}>
      <OverviewList>
        <OverviewRow
          lead={<Checkbox label={<span className="sr-only">Done</span>} />}
          title="Prepare the walkthrough brief"
          sub="Due 19 Aug · for the 20 Aug walkthrough"
          right={<Badge tone="warning">In 6 days</Badge>}
        />
        <OverviewRow
          lead={<Checkbox label={<span className="sr-only">Done</span>} />}
          title="Find a second contact at Brandt"
          sub="Only Dana is in touch · single-threaded"
          right="No date"
        />
      </OverviewList>
    </OverviewCard>
  ),
};

/** Amounts on the right, and a closing line in warning ink. */
export const Deals: Story = {
  render: () => (
    <OverviewCard
      title="Deals and projects"
      action={link("All deals")}
      foot="No economic buyer named yet"
      footTone="warn"
    >
      <OverviewList>
        <OverviewRow
          title="Fleet retrofit"
          sub="Proposal · stage 3 of 7 · close 30 Sep"
          right="€48,000"
          figure
        />
        <OverviewRow
          title="Depot rollout"
          sub="Project · delivering · ends 30 Nov"
          right="€16,500"
          figure
        />
      </OverviewList>
    </OverviewCard>
  ),
};

/** One paragraph for a body. */
export const Text: Story = {
  render: () => (
    <OverviewCard title="What this deal is" action={link("Edit")}>
      <OverviewText>
        Retrofitting 40 depot vans with telematics in two waves, keeping each
        depot offline for under four hours.
      </OverviewText>
    </OverviewCard>
  ),
};

/** Nothing to list: one sentence and its verb. */
export const Empty: Story = {
  render: () => (
    <OverviewCard
      title="Offers"
      action={link("New offer")}
      empty="No offer sent yet."
      emptyAction={link("Write the first offer")}
    />
  ),
};

/** German is longer: the title and its link share the line while they fit and
 *  wrap together when they do not, and a long row wraps inside the card. */
export const LongGermanRows: Story = {
  decorators: [
    (Story) => (
      <div lang="de" style={{ maxWidth: "20rem" }}>
        <Story />
      </div>
    ),
  ],
  render: () => (
    <OverviewCard
      title="Offene Deals und Projekte dieses Unternehmens"
      action={link("Alle Deals und Projekte anzeigen")}
    >
      <OverviewList>
        <OverviewRow
          title="Erweiterung Phase 2 für die Standorte Nord und Süd"
          sub="Angebot · Abschluss am 30. November · ins Stocken geraten"
          right="95.000 €"
          figure
        />
      </OverviewList>
    </OverviewCard>
  ),
};

/** A pill and a verb share the right slot, and wrap rather than overflow on
 *  a phone. */
export const PillAndVerbPhone: Story = {
  name: "pill and verb — on a phone",
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => (
    <OverviewCard title="What was said" action={link("Full history")}>
      <OverviewList>
        <OverviewRow
          title="Call about the second depot and the revised rollout dates"
          sub="Anna Weber · 12 Aug"
          right={
            <>
              <Badge>Call</Badge>
              <CallAction
                appearance="button"
                number="+49 30 1234 5678"
                label="Call"
                record={{ entityType: "contact", entityId: "p-1" }}
              />
            </>
          }
        />
        <OverviewRow
          title="Note"
          sub="Dana Buyer · 10 Aug"
          right={<Badge>Note</Badge>}
        />
      </OverviewList>
    </OverviewCard>
  ),
};
