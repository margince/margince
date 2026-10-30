// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CalendarDays, Mail, Send } from "lucide-react";
import { AppLink } from "../../design-system/applink";
import { Button, Card } from "../../design-system/atoms";
import { Checklist, ChecklistStep } from "../../design-system/checklist";
import { ProviderMark } from "../../design-system/provider-mark";
import { ProviderList, ProviderRow } from "../../design-system/providerrow";
import { Chip } from "../../design-system/readings";
import {
  ACCOUNT_NAME,
  type AccountVendor,
} from "../../format/connectorprovider";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useCanWrite } from "../../lib/capability";
import {
  type Availability,
  accountUnwired,
} from "../../lib/connect-flow-draft";
import { routeHash } from "../../lib/router";
import { settingsHref } from "../../lib/settingsrouting";
import "./connection-empty.css";

// The Mail and calendar card before anything is connected. It shows the steps a
// Connect walks, one tile per vendor with its own Connect, and a row for any
// other mail host. Each tile opens the connect dialog with its vendor chosen.

const VENDORS: readonly AccountVendor[] = ["google", "microsoft"];

const VENDOR_SERVICES_LINE: Record<AccountVendor, MessageKey> = {
  google: "connectors.tile.google",
  microsoft: "connectors.tile.microsoft",
};

const STRIP_LABELS: readonly MessageKey[] = [
  "connectors.strip.pick",
  "connectors.strip.tick",
  "connectors.strip.signIn",
];

/** What connecting will ask, before anything is asked: the dialog's steps,
 *  none started, and under them what happens without the reader. */
function ConnectStrip() {
  const t = useT();
  return (
    <div className="connection-steps">
      <Checklist variant="inline" label={t("connectors.strip.label")}>
        {STRIP_LABELS.map((key) => (
          <ChecklistStep key={key} state="open" title={t(key)} />
        ))}
      </Checklist>
      <p className="t-caption">{t("connectors.strip.syncs")}</p>
    </div>
  );
}

/** One vendor as a card of what it brings in. A vendor this installation has
 *  not wired keeps its tile, says so in its head, and names who turns it on. */
function VendorTile({
  vendor,
  unwired,
  canSetUp,
  lead,
  onConnect,
}: Readonly<{
  vendor: AccountVendor;
  unwired: boolean;
  /** Whether this reader can register the company app that wires it. */
  canSetUp: boolean;
  /** The panel's one primary: the first tile a reader can connect. */
  lead: boolean;
  onConnect: () => void;
}>) {
  const t = useT();
  const name = t(ACCOUNT_NAME[vendor]);
  return (
    <Card as="li" className="connection-tile" testId={`connect-tile-${vendor}`}>
      {/* The connected roster's row, not connected yet, so the tile and the
          row it becomes read as one thing. */}
      <ProviderRow
        mark={<ProviderMark providerKey={vendor} />}
        name={name}
        headingLevel="h3"
        status={
          unwired
            ? { tone: "default", label: t("connectors.tile.notSetUp") }
            : undefined
        }
        facts={t(VENDOR_SERVICES_LINE[vendor])}
      />
      <div className="connection-tile-chips">
        <Chip icon={Mail} dense>
          {t("connectors.bringsMail")}
        </Chip>
        <Chip icon={CalendarDays} dense>
          {t("connectors.bringsCalendar")}
        </Chip>
        <Chip icon={Send} dense>
          {t("connectors.bringsSend")}
        </Chip>
      </div>
      {unwired ? (
        <UnwiredFoot vendor={name} canSetUp={canSetUp} />
      ) : (
        <Button
          variant={lead ? "primary" : "ghost"}
          className="connection-tile-action"
          onClick={onConnect}
        >
          {t("connectors.connectVendor", { vendor: name })}
        </Button>
      )}
    </Card>
  );
}

/** Where a Connect would stand on a vendor nobody can connect yet: who turns
 *  it on, and for whoever can, the way to the page where they do. */
function UnwiredFoot({
  vendor,
  canSetUp,
}: Readonly<{ vendor: string; canSetUp: boolean }>) {
  const t = useT();
  const page = t("settings.tab.authentication");
  if (!canSetUp) {
    return (
      <p className="connection-tile-note">
        {t("connectors.tile.askAdmin", { vendor, page })}
      </p>
    );
  }
  return (
    <p className="connection-tile-note">
      {t("connectors.tile.registerApp", { vendor })}{" "}
      <AppLink
        className="link-button"
        href={routeHash(settingsHref("authentication"))}
      >
        {t("connectors.tile.openPage", { page })}
      </AppLink>
    </p>
  );
}

export function ConnectTiles({
  availability,
  onPick,
  onImap,
}: Readonly<{
  availability: readonly Availability[];
  /** Opens the connect dialog on what this vendor brings in. */
  onPick: (vendor: AccountVendor) => void;
  onImap: () => void;
}>) {
  const t = useT();
  // The grant the company-app endpoints enforce, as Sign-in and apps asks it.
  const canSetUp = useCanWrite("capture_settings", "update");
  const imapUnwired = accountUnwired("imap", availability);
  const lead = VENDORS.find((vendor) => !accountUnwired(vendor, availability));
  return (
    <>
      <ConnectStrip />
      <ul className="connection-tiles">
        {VENDORS.map((vendor) => (
          <VendorTile
            key={vendor}
            vendor={vendor}
            unwired={accountUnwired(vendor, availability)}
            canSetUp={canSetUp}
            lead={vendor === lead}
            onConnect={() => onPick(vendor)}
          />
        ))}
      </ul>
      {/* Secondary by design: another host brings mail alone, so it is the
          way in for the reader the two tiles do not fit. */}
      {!imapUnwired && (
        <div className="connection-imap">
          <ProviderList>
            <ProviderRow
              mark={<Mail />}
              name={t("connectFlow.otherHost")}
              facts={t("connectors.imapRow.note")}
              action={
                <Button onClick={onImap}>
                  {t("connectors.imapRow.connect")}
                </Button>
              }
            />
          </ProviderList>
        </div>
      )}
    </>
  );
}
