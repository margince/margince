// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { KeyedName } from "../design-system/keyedname";
import { Panel, PanelBody, PanelIntro, PanelRow } from "../design-system/panel";
import { RowOpen } from "../design-system/rowopen";
import { useLocale, useT } from "../i18n";
import {
  EXTENSION_SCREEN,
  type ExtensionDescriptor,
  type UnitSecretScope,
  unitsForSecretScope,
  unitTitle,
} from "../lib/extensions";
import { navigate } from "../lib/router";
import "./extension-units.css";

// Where an installation's own units are offered: on the settings page that
// already holds the kind of credential each one is configured with.
//
// A unit used to have a row in the navigation rail, which put an
// installation's surface beside Pipeline and Reports as though the product had
// grown a destination. It has not — enabling a unit adds a thing to CONFIGURE,
// and this is the page that configures things.
//
// WHICH page is the manifest's decision and not this component's. A unit
// declaring a `user` secret holds one member's own credential at a provider,
// so it is offered on their personal Connections page; a `workspace` secret is
// the installation's, so its unit is offered under Integrations. Both pages
// already mean exactly that, which is why the declared scope is enough and no
// unit names a destination. See lib/extensions.ts's unitsForSecretScope.
//
// The permission story is the page, and it was already written: Connections is
// per-contact and ungated, Integrations is the company's and opens on the
// grants its cards ask for OR on a workspace-scoped unit being composed at all,
// because this page is the only place such a unit is offered. The rows here add
// no gate of their own — the rail rows they replace had none either, and a
// unit's screen refuses independently on the object it declares.
//
// A unit is one row whose only verb is to go there, so a press on the row opens
// it and the row's chevron is the keyboard path. Its own screen configures it.

/**
 * The units whose credential lives in `scope`, or nothing at all.
 *
 * Nothing at all is the vanilla tree, where no unit is composed, and it is also
 * the honest answer for a page whose scope no composed unit declares. A heading
 * over an empty list is a promise the installation did not make — the same
 * reason the rail group this replaces was absent rather than empty.
 */
export function ExtensionUnitsCard({
  scope,
}: Readonly<{ scope: UnitSecretScope }>) {
  const units = unitsForSecretScope(scope);
  if (units.length === 0) {
    return null;
  }
  return <ExtensionUnitsPanel scope={scope} units={units} />;
}

/** The card itself, for the units the registry offered; stories pass their own. */
export function ExtensionUnitsPanel({
  scope,
  units,
}: Readonly<{
  scope: UnitSecretScope;
  units: readonly ExtensionDescriptor[];
}>) {
  const t = useT();
  return (
    // Panel, not Card: every other card on the two pages this lands on is one,
    // and two card primitives on one tab grow header bands that disagree.
    <Panel title={t(`extUnits.${scope}.title`)}>
      <PanelBody>
        <PanelIntro>{t(`extUnits.${scope}.sub`)}</PanelIntro>
      </PanelBody>
      {units.map((unit) => (
        <UnitRow key={unit.name} unit={unit} />
      ))}
    </Panel>
  );
}

/** One unit, as a row that is its own way in. */
function UnitRow({ unit }: Readonly<{ unit: ExtensionDescriptor }>) {
  const t = useT();
  const { locale } = useLocale();
  const { name } = unit;
  const title = unitTitle(unit, locale);
  const open = () => navigate({ screen: EXTENSION_SCREEN, id: name });
  return (
    <PanelRow record onOpen={open} className="extunits-row">
      <span className="extunits-name" data-testid={`ext-unit-${name}`}>
        <KeyedName name={title} code={name} />
      </span>
      <RowOpen label={t("extUnits.openNamed", { name: title })} onOpen={open} />
    </PanelRow>
  );
}
