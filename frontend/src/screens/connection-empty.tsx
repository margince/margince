// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CalendarDays, Mail, Send } from "lucide-react";
import { useCanWrite } from "../app/capability";
import { routeHash } from "../app/router";
import { Button, Card } from "../design-system/atoms";
import { ProviderMark } from "../design-system/provider-mark";
import { ProviderRow } from "../design-system/providerrow";
import { Chip } from "../design-system/readings";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { type Availability, accountUnwired } from "./connect-flow-draft";
import { ConnectStrip } from "./connect-flow-trail";
import { ACCOUNT_NAME, type AccountVendor } from "./connectorproviders";
import { settingsHref } from "./settingsrouting";
import "./connection-empty.css";

// The Mail and calendar card before anything is connected: the steps a Connect
// walks, one tile per vendor with its own Connect, and a row for any other mail
// host. Each tile opens the connect dialog with its vendor already chosen.

const VENDORS: readonly AccountVendor[] = ["google", "microsoft"];

const VENDOR_SERVICES_LINE: Record<AccountVendor, MessageKey> = {
  google: "connectors.tile.google",
  microsoft: "connectors.tile.microsoft",
};

/** One vendor as a card of what it brings in. A vendor this installation has
 *  not wired keeps its tile, says so in its head, and names who turns it on. */
function VendorTile({
  vendor,
  unwired,
  canSetUp,
  onConnect,
}: Readonly<{
  vendor: AccountVendor;
  unwired: boolean;
  /** Whether this reader can register the company app that wires it. */
  canSetUp: boolean;
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
        <Button className="connection-tile-action" onClick={onConnect}>
          {t("connectors.connectVendor", { vendor: name })}
        </Button>
      )}
    </Card>
  );
}

/** Where a Connect would stand on a vendor nobody can connect yet: who turns
 *  it on, and for that person the way to the page where they do. */
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
    <>
      <p className="connection-tile-note">
        {t("connectors.tile.registerApp", { vendor })}
      </p>
      <a
        className="btn btn-ghost connection-tile-action"
        href={routeHash(settingsHref("authentication"))}
      >
        {t("connectors.tile.openPage", { page })}
      </a>
    </>
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
  return (
    <>
      <div className="connection-steps">
        <ConnectStrip />
      </div>
      <ul className="connection-tiles">
        {VENDORS.map((vendor) => (
          <VendorTile
            key={vendor}
            vendor={vendor}
            unwired={accountUnwired(vendor, availability)}
            canSetUp={canSetUp}
            onConnect={() => onPick(vendor)}
          />
        ))}
      </ul>
      {/* Secondary on purpose: another host brings mail alone, so it is the
          way in for the reader the two tiles do not fit. */}
      {!imapUnwired && (
        <div className="connection-imap-row">
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
        </div>
      )}
    </>
  );
}
