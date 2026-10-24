import { useId, useState } from "react";
import type { components } from "../../api/schema";
import { OAuthOutcomeNote } from "../../components/connectors.notices";
import { MailSharingPlate } from "../../components/mail-sharing";
import { Button, EmptyState } from "../../design-system/atoms";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { ProviderList } from "../../design-system/providerrow";
import { QueryStates } from "../../design-system/querystates";
import { StatusBadge } from "../../design-system/statusbadge";
import {
  type AccountVendor,
  accountsOf,
  vendorOf,
} from "../../format/connectorprovider";
import { useT } from "../../i18n";
import {
  type ImportWindow,
  useConnectAccount,
} from "../../lib/connect-account";
import { canAddAccount, type FlowAccount } from "../../lib/connect-flow-draft";
import { useConnectors } from "../../lib/connectors";
import { problemMessageOf } from "../../lib/problem";
import { type Route, useRoute } from "../../lib/router";
import { type UrlParams, useUrlParams } from "../../lib/urlstate";
import { isImportWindow } from "../../mail-history/window-picker";
import { ConnectFlow } from "./connect-flow";
import { ConnectionAccountRow, offersReconnect } from "./connection-account";
import { ConnectTiles } from "./connection-empty";
import { ImapConnectForm } from "./imap-connect-form";
import "./connectors.css";

// The Connections family has one entry for the Settings screen: the mailbox card
// and the workspace bot beside it on Integrations.
export { TelegramConnectorsPanel } from "./connectors.telegram";

// The connected-inboxes surface (RC-8), the Settings cards the onboarding copy
// promises ("disconnect in one click", "manage in Settings"). It lists the live
// capture connections and lets a stale one reconnect (re-mint the same consent
// URL). One confirmed click disconnects. Every field shown is a server fact from
// GET /connectors, never a claim.
//
// Mail only. The workspace's Telegram bot is company wiring and lives on
// Integrations. The address emailed links use is reported on System health.
//
// The roster is one ProviderRow per account (connection-account.tsx), with what
// each service brings in under it. Adding an account is the card's create verb,
// so it sits in the header and opens the connect dialog.

type CaptureConnection = components["schemas"]["CaptureConnection"];

// The roster as this card's list: one row per account.
function MailRoster({
  rows,
  landing,
  onImapReconnect,
}: Readonly<{
  rows: readonly CaptureConnection[];
  landing: Landing;
  onImapReconnect: () => void;
}>) {
  const accounts = accountsOf(rows);
  // One primary per panel: the first broken account keeps a primary Reconnect.
  const firstBroken = accounts.find(offersReconnect);
  return (
    <ProviderList>
      {accounts.map((account) => (
        <ConnectionAccountRow
          key={account.key}
          account={account}
          reconnectPrimary={
            firstBroken === undefined || account === firstBroken
          }
          heldWindow={
            landing.heldVendor !== null && landing.heldVendor === account.key
              ? landing.heldWindow
              : null
          }
          onImapReconnect={onImapReconnect}
        />
      ))}
    </ProviderList>
  );
}

type Landing = Readonly<{
  outcome: string | undefined;
  provider: string | undefined;
  /** A past-email window the connect asked for and the server held for the
   *  reader, and the vendor whose mailbox it belongs to. */
  heldWindow: ImportWindow | null;
  heldVendor: AccountVendor | null;
}>;

// The consent callback lands on #/settings/connections/{outcome}/{provider},
// with `?import={window}` when the import it asked for waits for the reader.
// A chained consent lands on the calendar, so the window is matched to its
// account by vendor rather than by provider.
function connectionsLanding(route: Route, params: UrlParams): Landing {
  if (route.screen !== "settings" || route.id !== "connections") {
    return {
      outcome: undefined,
      provider: undefined,
      heldWindow: null,
      heldVendor: null,
    };
  }
  const held = params.get("import");
  return {
    outcome: route.id2,
    provider: route.id3,
    heldWindow: isImportWindow(held) ? held : null,
    heldVendor: vendorOf(route.id3),
  };
}

// The head says nothing is connected until something is. From then on it
// carries the create verb, while there is still an account left to add.
function MailHeadAction({
  empty,
  offerAdd,
  onAdd,
  addId,
}: Readonly<{
  empty: boolean;
  offerAdd: boolean;
  onAdd: () => void;
  addId: string;
}>) {
  const t = useT();
  if (empty) {
    return (
      <StatusBadge kind="neutral" label={t("connectors.health.notConnected")} />
    );
  }
  if (!offerAdd) {
    return null;
  }
  return (
    <Button id={addId} onClick={onAdd}>
      {t("connectors.addOpen")}
    </Button>
  );
}

// The OAuth return's note, with its own "Connect calendar"; each account row
// carries its own connect.
function LandingOutcome({
  landing,
  connections,
}: Readonly<{
  landing: Landing;
  connections: readonly CaptureConnection[] | undefined;
}>) {
  const t = useT();
  const connect = useConnectAccount();
  return (
    <OAuthOutcomeNote
      outcome={landing.outcome}
      provider={landing.provider}
      connections={connections}
      connectCalendar={{
        onConnect: (calendar) =>
          connect.mutate({ provider: calendar, returnTo: "settings" }),
        pending: connect.isPending,
        error: connect.isError ? problemMessageOf(connect.error, t) : null,
      }}
    />
  );
}

// The card: the roster of mailboxes this member captures from, the verb
// that adds another, and the company's sharing rule those mailboxes work under.
export function ConnectorsCard() {
  const t = useT();
  const [imapConnectOpen, setImapConnectOpen] = useState(false);
  // The connect dialog is closed, open on the account step (""), or open on what
  // a vendor or another host brings in. The last applies when the page chose.
  const [adding, setAdding] = useState<FlowAccount | "" | null>(null);
  const [params] = useUrlParams();
  const landing = connectionsLanding(useRoute(), params);
  const connectors = useConnectors();

  const notConfigured = connectors.data?.notConfigured ?? false;
  const rows = (connectors.data?.data ?? []).filter(
    (c) => c.status !== "disconnected",
  );
  const present = rows.map((r) => r.provider);

  // Gated on the list having answered, not merely on it not having failed.
  // What is left to add is derived from what is connected, so before the read
  // lands every account looks free.
  const rosterKnown = connectors.isSuccess && !notConfigured;
  const empty = rosterKnown && rows.length === 0;
  const offerAdd = rosterKnown && canAddAccount(present);
  const titleId = useId();
  const addId = useId();
  // Adding a mailbox can take the verb away (every account connected, or the
  // roster's first row replaces the empty tiles). Focus then falls back to the
  // card's title when the head verb is gone.
  const backToAdd = () =>
    document.getElementById(addId) ?? document.getElementById(titleId);

  return (
    <Panel
      title={
        <span id={titleId} tabIndex={-1}>
          {t("connectors.title")}
        </span>
      }
      titleAction={
        <MailHeadAction
          empty={empty}
          offerAdd={offerAdd}
          onAdd={() => setAdding("")}
          addId={addId}
        />
      }
    >
      <PanelBody>
        {/* The ask, while it may still be the answer: a roster has no intro. */}
        {(empty || connectors.isPending) && (
          <PanelIntro>{t("connectors.sub")}</PanelIntro>
        )}
        <LandingOutcome landing={landing} connections={connectors.data?.data} />
        <QueryStates query={connectors} pendingLabel={t("connectors.loading")}>
          {/* A deployment that never wired mail capture: a calm state. */}
          {notConfigured && (
            <EmptyState>{t("connectors.notConfigured")}</EmptyState>
          )}
          {empty && (
            <ConnectTiles
              availability={connectors.data?.providers ?? []}
              onPick={setAdding}
              onImap={() => setAdding("imap")}
            />
          )}
          {rosterKnown && !empty && (
            <MailRoster
              rows={rows}
              landing={landing}
              onImapReconnect={() => setImapConnectOpen(true)}
            />
          )}
        </QueryStates>
      </PanelBody>
      <MailSharingPlate />
      <ConnectFlow
        open={adding !== null && offerAdd}
        onClose={() => setAdding(null)}
        startFrom={adding || undefined}
        connected={present}
        availability={connectors.data?.providers}
        onImap={() => setImapConnectOpen(true)}
        returnFocusTo={backToAdd}
      />
      <ImapConnectForm
        open={imapConnectOpen}
        onClose={() => setImapConnectOpen(false)}
        onConnected={() => setImapConnectOpen(false)}
        returnFocusTo={backToAdd}
      />
    </Panel>
  );
}
