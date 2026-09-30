// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { UnitSecretScope } from "../app/extensions";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { ExtensionUnitsCard } from "./extension-units";
import { StoryProviders } from "./story-utils";

// Where an installation's own units are offered: the manifest's declared secret
// scope decides which of two settings pages holds the unit, because each page
// already means exactly the kind of credential that scope names.
//
// WHAT THIS CANNOT SHOW, said plainly so the next reader does not go looking:
// the rows. The card reads the composed registry, which is empty by
// construction in the vanilla lane every story runs against
// (src/composition/extensions.gen.ts), so the only answer reachable here is the
// one a core checkout gives — the card withholds itself. Narrowing a composed
// build is the way to watch a unit row land, the same limit
// extension-access.stories.tsx records for the screen registry beside it.
//
// So both stories draw the card ABOVE this one as well: an absence is legible
// only against the thing it follows, and the page ending cleanly — no empty
// heading, no gap where a panel failed to draw — is the whole of what there is
// to check.

// The real last card of each page, by the scope that decides which page a unit
// lands on. Its own copy rather than an invented frame: a made-up neighbour
// would put this card on a page the product does not have.
const CARD_ABOVE: Record<
  UnitSecretScope,
  Readonly<{ title: MessageKey; sub: MessageKey }>
> = {
  user: { title: "linkedinReach.title", sub: "linkedinReach.sub" },
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
 * an empty list would be a promise the installation never made — the same
 * reason the navigation group this replaced was absent rather than empty.
 */
export const NoUnitOnConnections: Story = { render: story("user") };

/**
 * Integrations, where a `workspace`-scoped secret puts a unit: the credential
 * is the installation's, like the cards above it.
 *
 * The same absence on the other page, and it is worth its own frame because the
 * two pages are the whole of the placement rule — a unit lands on one of them
 * or on neither, and never on both.
 */
export const NoUnitOnIntegrations: Story = { render: story("workspace") };
