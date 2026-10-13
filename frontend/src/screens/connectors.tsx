import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { connectorsPollInterval } from "../app/capture-progress";
import { type Route, useRoute } from "../app/router";
import { type UrlParams, useUrlParams } from "../app/urlstate";
import { Badge, Button, EmptyState } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { ProviderList } from "../design-system/providerrow";
import { useT } from "../i18n";
import { isImportWindow } from "../mail-history/window-picker";
import { problemCode, problemMessageOf, throwProblem } from "./common";
import { type ImportWindow, useConnectAccount } from "./connect-account";
import { ConnectFlow } from "./connect-flow";
import { canAddAccount, type FlowAccount } from "./connect-flow-draft";
import { ConnectionAccountRow } from "./connection-account";
import { ConnectTiles } from "./connection-empty";
import { type AccountVendor, accountsOf, vendorOf } from "./connectorproviders";
import { OAuthOutcomeNote } from "./connectors.notices";
import { ImapConnectForm } from "./imap-connect-form";
import { MailSharingPlate } from "./mail-sharing";
import "./connectors.css";

// The connected-inboxes surface (RC-8): the Settings cards the onboarding copy
// has always promised ("disconnect in one click", "manage in Settings"). It
// lists the live capture connections, lets a stale one reconnect (re-mint the
// same consent URL), and disconnects one in a single confirmed click.
// Every field shown is a server fact from GET /connectors — never a claim.
//
// Mail only. The workspace's Telegram bot is company wiring and lives on
// Integrations; the address emailed links use is reported on System health.
//
// The roster is one ProviderRow per account (connection-account.tsx), with
// what each service brings in under it. Adding an account is the card's create
// verb, so it sits in the header and opens the connect dialog.

type CaptureConnection = components["schemas"]["CaptureConnection"];

export type ConnectorsResult = {
  // GET /connectors answers 501 code:not_implemented when this deployment
  // never wired mail capture (httperr.NotImplemented) — a calm, documented
  // feature-off state, never an error card (mirrors webhooks.tsx's
  // webhooks_not_configured treatment).
  notConfigured: boolean;
  data: CaptureConnection[];
  // Which providers this installation could connect at all, and why not where
  // it could not. The connect surfaces draw their blockers from it; a body that
  // carried none reads as an empty roster, which is what a deployment with no
  // capture wired actually has.
  providers: ProviderReadiness[];
  // The address this installation puts in outgoing links, and whether it
  // answered when last asked. Absent when none is configured — which is
  // itself the answer, not a failure.
  publicOrigin?: PublicOriginStatus;
};

type PublicOriginStatus =
  components["schemas"]["CaptureConnectionListResponse"]["public_origin"];

type ProviderReadiness = NonNullable<
  components["schemas"]["CaptureConnectionListResponse"]["providers"]
>[number];

// Readers share the connector query shape because its cache entry is shared.
export function useConnectors(
  options?: Readonly<{
    /** False holds the request without leaving the key: a reader that only
     *  wants the answer when it already exists still shares the one entry. */
    enabled?: boolean;
    refetchOnWindowFocus?: boolean | "always";
  }>,
) {
  return useQuery({
    ...options,
    queryKey: ["connectors"],
    enabled: options?.enabled ?? true,
    queryFn: async (): Promise<ConnectorsResult> => {
      const { data, error, response } = await api.GET("/connectors");
      if (response.status === 501 && problemCode(error) === "not_implemented") {
        return { notConfigured: true, data: [], providers: [] };
      }
      if (error) {
        throwProblem(error);
      }
      return {
        notConfigured: false,
        data: data.data,
        providers: data.providers ?? [],
        publicOrigin: data.public_origin,
      };
    },
    // Live while a mailbox is importing: the orb's ring, the capture chip and
    // this card's own run row all draw the import from THIS read, and at the
    // client's default staleness an import would be reported thirty seconds
    // behind the counts the server holds.
    refetchInterval: (query) => connectorsPollInterval(query.state.data),
  });
}

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
  return (
    <ProviderList>
      {accountsOf(rows).map((account) => (
        <ConnectionAccountRow
          key={account.key}
          account={account}
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

// Everything the mail card shows INSTEAD of a roster: the read in flight, the
// read refused, and a deployment that never wired mail capture.
function RosterNotice({
  query,
}: Readonly<{ query: ReturnType<typeof useConnectors> }>) {
  const t = useT();
  if (query.isPending) {
    return <p>{t("connectors.loading")}</p>;
  }
  if (query.isError) {
    return (
      <Callout tone="danger" kind="outcome" title={t("connectors.loadFailed")}>
        {problemMessageOf(query.error, t)}
      </Callout>
    );
  }
  if (query.data.notConfigured) {
    return (
      <EmptyState>
        <p>{t("connectors.notConfigured")}</p>
      </EmptyState>
    );
  }
  return null;
}

// The head says nothing is connected until something is; from then on it
// carries the create verb, while there is still an account left to add.
function MailHeadAction({
  empty,
  offerAdd,
  onAdd,
}: Readonly<{ empty: boolean; offerAdd: boolean; onAdd: () => void }>) {
  const t = useT();
  if (empty) {
    return <Badge>{t("connectors.health.notConnected")}</Badge>;
  }
  if (!offerAdd) {
    return null;
  }
  return <Button onClick={onAdd}>{t("connectors.addOpen")}</Button>;
}

// The card: the roster of mailboxes this member captures from, the verb
// that adds another, and the company's sharing rule those mailboxes work under.
export function ConnectorsCard() {
  const t = useT();
  const [imapConnectOpen, setImapConnectOpen] = useState(false);
  // The connect dialog: closed, open on the account step (""), or open on what
  // a vendor or another host brings in, when the page already chose it.
  const [adding, setAdding] = useState<FlowAccount | "" | null>(null);
  const [params] = useUrlParams();
  const landing = connectionsLanding(useRoute(), params);
  const connectors = useConnectors();
  // The outcome note's own "Connect calendar"; each account row carries its own.
  const connect = useConnectAccount();

  const notConfigured = connectors.data?.notConfigured ?? false;
  const rows = (connectors.data?.data ?? []).filter(
    (c) => c.status !== "disconnected",
  );
  const present = rows.map((r) => r.provider);

  // Gated on the list having ANSWERED, not merely on it not having failed:
  // what is left to add is derived from what is already connected, so before
  // the read lands every account looks free.
  const rosterKnown = connectors.isSuccess && !notConfigured;
  const empty = rosterKnown && rows.length === 0;
  const offerAdd = rosterKnown && canAddAccount(present);

  return (
    <Panel
      title={t("connectors.title")}
      // Nothing connected is the one move this page asks for, so the empty card
      // leads; once an account exists the create verb takes the header.
      tone={empty ? "accent" : undefined}
      titleAction={
        <MailHeadAction
          empty={empty}
          offerAdd={offerAdd}
          onAdd={() => setAdding("")}
        />
      }
    >
      <PanelBody>
        {/* The ask, while it may still be the answer: a roster has no intro. */}
        {(empty || connectors.isPending) && (
          <PanelIntro>{t("connectors.sub")}</PanelIntro>
        )}
        <OAuthOutcomeNote
          outcome={landing.outcome}
          provider={landing.provider}
          connections={connectors.data?.data}
          connectCalendar={{
            onConnect: (calendar) =>
              connect.mutate({ provider: calendar, returnTo: "settings" }),
            pending: connect.isPending,
            error: connect.isError ? problemMessageOf(connect.error, t) : null,
          }}
        />
        <RosterNotice query={connectors} />
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
      </PanelBody>
      <MailSharingPlate />
      <ConnectFlow
        open={adding !== null && offerAdd}
        onClose={() => setAdding(null)}
        startFrom={adding || undefined}
        connected={present}
        availability={connectors.data?.providers}
        onImap={() => setImapConnectOpen(true)}
      />
      <ImapConnectForm
        open={imapConnectOpen}
        onClose={() => setImapConnectOpen(false)}
        onConnected={() => setImapConnectOpen(false)}
      />
    </Panel>
  );
}
