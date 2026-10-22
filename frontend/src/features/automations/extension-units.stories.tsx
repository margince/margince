// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import type {
  ExtensionDescriptor,
  UnitSecretScope,
} from "../../lib/extensions";
import { StoryProviders } from "../../testing/story-utils";
import { ExtensionUnitsCard, ExtensionUnitsPanel } from "./extension-units";

// Where an installation's own units are offered: the manifest's declared secret
// scope decides which of two settings pages holds the unit.
//
// The card reads the composed registry, which is empty in the vanilla lane every
// story runs against. So the two "no unit" stories show the card withholding
// itself under the page's real last card. The unit rows are drawn through
// ExtensionUnitsPanel with descriptors of the story's own.

// The real last card of each page, by the scope that decides which page a unit
// lands on. Its own copy rather than an invented frame: a made-up neighbour
// would put this card on a page the product does not have.
const CARD_ABOVE: Record<
  UnitSecretScope,
  Readonly<{ title: MessageKey; sub: MessageKey }>
> = {
  user: { title: "linkedinReach.title", sub: "linkedinImport.sub" },
  workspace: { title: "webhooks.title", sub: "webhooks.sub" },
};

function Page({ scope }: Readonly<{ scope: UnitSecretScope }>) {
  const t = useT();
  const above = CARD_ABOVE[scope];
  return (
    // `.settings-stack` owns the gap between cards, so the interval below the
    // neighbour is the page's own rather than one this story invented.
    <div className="settings-stack">
      <Panel title={t(above.title)}>
        <PanelBody>
          <PanelIntro>{t(above.sub)}</PanelIntro>
        </PanelBody>
      </Panel>
      <ExtensionUnitsCard scope={scope} />
    </div>
  );
}

function story(scope: UnitSecretScope) {
  return () => (
    <StoryProviders>
      <Page scope={scope} />
    </StoryProviders>
  );
}

const meta: Meta<typeof ExtensionUnitsCard> = {
  title: "Settings/Across pages/Units offered in settings",
  component: ExtensionUnitsCard,
};
export default meta;

type Story = StoryObj<typeof ExtensionUnitsCard>;

/**
 * A member's own Connections page, where a `user`-scoped secret puts a unit:
 * the credential is theirs, at a provider they signed into.
 *
 * No unit is composed, so nothing follows the page's last card. A heading over
 * an empty list would promise something the installation never made.
 */
export const NoUnitOnConnections: Story = { render: story("user") };

/**
 * Integrations, where a `workspace`-scoped secret puts a unit: the credential
 * is the installation's, like the cards above it.
 *
 * The same absence on the other page. It earns its own frame because the two
 * pages are the whole of the placement rule: a unit lands on one or neither.
 */
export const NoUnitOnIntegrations: Story = { render: story("workspace") };

const DISPATCH_UNIT: ExtensionDescriptor = {
  name: "dispatch-connector",
  title: "",
  secretScope: "workspace",
  verbs: [],
  entries: [],
  panels: [],
};

function unitsStory(units: readonly ExtensionDescriptor[]) {
  return () => (
    <StoryProviders>
      <div className="settings-stack">
        <ExtensionUnitsPanel scope="workspace" units={units} />
      </div>
    </StoryProviders>
  );
}

/** One composed unit: its key as the name, a press on the row opens it. */
export const OneUnit: Story = { render: unitsStory([DISPATCH_UNIT]) };

/**
 * Two units, with the hairline between rows, and a long key that has to wrap
 * instead of pushing the chevron off the row.
 */
export const TwoUnits: Story = {
  render: unitsStory([
    DISPATCH_UNIT,
    {
      ...DISPATCH_UNIT,
      name: "warehouse-inventory-sync-for-regional-partners",
    },
  ]),
};
