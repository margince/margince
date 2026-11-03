import type { ReactNode } from "react";
import type { components } from "../../../api/schema";
import { Button } from "../../../design-system/atoms";
import { Callout } from "../../../design-system/callout";
import { StageActions } from "../../../design-system/onboarding-stage";
import { Panel, PanelBody, PanelIntro } from "../../../design-system/panel";
import { providerBrandName } from "../../../design-system/provider-mark";
import {
  type AccountVendor,
  accountsOf,
  type ConnectionAccount,
} from "../../../format/connectorprovider";
import { useT } from "../../../i18n";
import type { MessageKey } from "../../../i18n/en";
import { isConnectedAtAll } from "../../../lib/connector-status";
import { useConnectors } from "../../../lib/connectors";
import { type AdminWait, waitsOnAdmin } from "../../../lib/setup-checklist";
import { AlsoWorthDoing, type AlsoWorthDoingProps } from "./connect-also";
import { AppWaitingPanel } from "./connect-app-missing";
import { ConnectedStep } from "./connect-connected";
import {
  MAIL_PROVIDERS,
  type MailProvider,
  VendorAsk,
  VendorChoice,
} from "./connect-vendor";

// The connect act's work surface, inline and status first: where the work
// account lives and what to bring in from it. Once a mailbox is live, it shows
// what is connected and how far back to read it. Mail and calendar come from
// the same account, so the account is picked once.

// The roster row each account's mailbox carries. The calendar is a grant of
// the same account and never decides which account the reader picked.
const ROSTER_PROVIDER: Readonly<Record<MailProvider, string>> = {
  google: "gmail",
  microsoft: "graph",
  imap: "imap",
};

/** The connected roster and whether it has actually been verified. `verified`
 *  is false while the fetch is in flight (first load OR a later refetch, e.g.
 *  the invalidation IMAP's own successful connect fires) and after it fails:
 *  states the scene cannot tell apart from "nothing is connected" without this
 *  flag, and all of them must withhold the ask rather than treat an unread or
 *  re-reading roster as an empty one. `failed` and `retry` let a genuine read
 *  failure be said out loud and retried. */
type ConnectedMailRoster = Readonly<{
  providers: ReadonlySet<string>;
  /** The roster folded into accounts, disconnected rows left out. */
  accounts: readonly ConnectionAccount[];
  /** Why a provider could not be connected right now, keyed by the roster's
   *  own provider name. Absent for a provider the server called ready, and
   *  absent for the whole map on a build whose server does not report it yet,
   *  which reads the same as it always did: every account offered. */
  blockers: ReadonlyMap<string, ConnectBlocker>;
  /** An answer is in hand, though a refetch may be replacing it. */
  read: boolean;
  verified: boolean;
  failed: boolean;
  retry: () => void;
}>;

/** One provider's answer from the roster read, decided by the same predicate
 *  the connect endpoint uses, so the step cannot offer what a press would be
 *  refused. */
export type ProviderAvailability =
  components["schemas"]["CaptureProviderAvailability"];

/** The reasons that refuse a connect, derived from the contract rather than
 *  respelled: `ready` is the fourth answer and never reaches the map below. */
type ConnectBlocker = Exclude<ProviderAvailability["reason"], "ready">;

type CaptureConnection = components["schemas"]["CaptureConnection"];

/** An account whose company app waits on an administrator. */
type Waiting = Readonly<{ vendor: AccountVendor; wait: AdminWait }>;

/**
 * Which mailboxes are already live, read fresh rather than assumed from the
 * pre-consent selection. A reload can land on this step with a mailbox
 * connected in an earlier session, and the step has to say so without a click.
 * Shares the `["connectors"]` query key with `OAuthReturnPanel` and the
 * connected account panel, so react-query dedupes them into one request.
 */
function useConnectedMailProviders(): ConnectedMailRoster {
  const roster = useConnectors();
  const connected = roster.data?.data.filter(isConnectedAtAll);
  const blockers = new Map<string, ConnectBlocker>();
  for (const entry of roster.data?.providers ?? []) {
    if (entry.reason !== "ready") {
      blockers.set(entry.provider, entry.reason);
    }
  }
  // A deployment that never wired mail capture answers 501, which the shared
  // read reports as `notConfigured` rather than as a failure. There is no
  // roster to offer an account from either way, so both say the same thing.
  const unreadable = roster.isError || roster.data?.notConfigured === true;
  const shown = roster.data?.data.filter((c) => c.status !== "disconnected");
  return {
    providers: new Set((connected ?? []).map((c) => c.provider)),
    accounts: accountsOf(shown ?? []),
    blockers,
    read: roster.isSuccess && !unreadable,
    verified: roster.isSuccess && !roster.isFetching && !unreadable,
    failed: unreadable,
    retry: () => void roster.refetch(),
  };
}

export function ConnectScene({
  vendor,
  onVendorChange,
  askBusy,
  onAskBusyChange,
  onImapDone,
  returnPanel,
  onFinish,
  finishing,
  ...also
}: Readonly<
  {
    /** The account whose ask is open; null while none is picked. */
    vendor: MailProvider | null;
    onVendorChange: (next: MailProvider | null) => void;
    /** A connect POST is in flight: every other way off the ask waits for it,
     *  because a success landing after the reader moved on contradicts them. */
    askBusy: boolean;
    onAskBusyChange: (busy: boolean) => void;
    /** The IMAP mailbox is live and its own result has been closed. */
    onImapDone: () => void;
    /** What a consent round trip came back with, once one has. */
    returnPanel: ReactNode;
    /**
     * Leaves the step, saying how: `skipped` is true only when no mailbox is
     * connected and the reader chose to go on without one. The scene decides
     * which of the two it is asking for, because the roster it reads is the
     * one fact that tells them apart.
     */
    onFinish: (skipped: boolean) => void;
    finishing: boolean;
  } & AlsoWorthDoingProps
>) {
  const t = useT();
  const mailRoster = useConnectedMailProviders();
  const connected =
    MAIL_PROVIDERS.find((key) =>
      mailRoster.providers.has(ROSTER_PROVIDER[key]),
    ) ?? null;
  const live =
    connected === null ? undefined : liveAccount(mailRoster, connected);
  const { notes, refused, waiting } = accountNotes(mailRoster, t);
  // A picked account whose company app an administrator has yet to register
  // or repair: the step says who fixes it instead of asking.
  const waitingOn = vendor === null ? undefined : waiting.get(vendor);
  // An ask is open for a picked account this installation can connect while
  // no mailbox is live. A live one answers the ask on its own.
  const asking =
    vendor !== null &&
    mailRoster.read &&
    !refused.has(vendor) &&
    waitingOn === undefined &&
    connected === null;

  return (
    <div className="ob-scene ob-connect">
      {/* A mailbox is what this step is for, so a roster read that failed
          outright cannot just leave every account silently disabled: that
          reads as "still loading" forever. Said out loud, with the one
          recovery that clears it. */}
      {mailRoster.failed && <MailRosterFailed onRetry={mailRoster.retry} />}

      {returnPanel}

      {live ? (
        <ConnectedStep
          account={live.account}
          mailbox={live.mailbox}
          also={<AlsoWorthDoing {...also} />}
          finishing={finishing}
          onFinish={() => onFinish(false)}
        />
      ) : (
        <>
          <Panel title={t("ob.conv.connect.whereTitle")}>
            <PanelBody>
              <PanelIntro>{t("ob.conv.connect.whereHint")}</PanelIntro>
              <VendorChoice
                value={vendor}
                onChange={onVendorChange}
                // Held while the roster has not answered: an ask opened against
                // it could connect a second mailbox the read has not reported.
                disabled={!mailRoster.verified || connected !== null || askBusy}
                notes={notes}
                refused={refused}
                waiting={new Set<MailProvider>(waiting.keys())}
              />
            </PanelBody>
          </Panel>

          {waitingOn && (
            <AppWaitingPanel
              vendor={waitingOn.vendor}
              wait={waitingOn.wait}
              onUseImap={
                refused.has("imap") ? null : () => onVendorChange("imap")
              }
            />
          )}

          {asking && (
            <VendorAsk
              vendor={vendor}
              onPendingChange={onAskBusyChange}
              onImapDone={onImapDone}
              onImapDismiss={() => onVendorChange(null)}
            />
          )}

          <AlsoWorthDoing {...also} />

          <SkipWayOnward
            caption={
              waitingOn
                ? t("ob.conv.connect.skipCaptionWaiting", {
                    vendor:
                      providerBrandName(waitingOn.vendor) ?? waitingOn.vendor,
                  })
                : t("ob.conv.connect.skipCaption")
            }
            // Recording "no mailbox" before the roster answers, or while a
            // connect is in flight, could persist a fact that is not so.
            held={!mailRoster.verified || askBusy || connected !== null}
            finishing={finishing}
            onSkip={() => onFinish(true)}
          />
        </>
      )}
    </div>
  );
}

/** The live mailbox's account, found in the same read that said it is live.
 *  Undefined only in the render where the two disagree, which draws the ask's
 *  side with the skip held rather than a panel for a row nobody can see. */
function liveAccount(
  roster: ConnectedMailRoster,
  connected: MailProvider,
): { account: ConnectionAccount; mailbox: CaptureConnection } | undefined {
  for (const account of roster.accounts) {
    const mailbox = account.mail;
    if (mailbox?.provider === ROSTER_PROVIDER[connected]) {
      return { account, mailbox };
    }
  }
  return undefined;
}

/** What the roster says about each account, in place of its own line; which
 *  accounts this installation cannot connect at all; and which wait on an
 *  administrator. A waiting account stays pickable, because picking it is how
 *  the reader learns who fixes it. */
function accountNotes(
  roster: ConnectedMailRoster,
  t: ReturnType<typeof useT>,
): {
  notes: Map<MailProvider, string>;
  refused: Set<MailProvider>;
  waiting: Map<MailProvider, Waiting>;
} {
  const notes = new Map<MailProvider, string>();
  const refused = new Set<MailProvider>();
  const waiting = new Map<MailProvider, Waiting>();
  for (const key of MAIL_PROVIDERS) {
    const blocker = roster.blockers.get(ROSTER_PROVIDER[key]);
    if (key !== "imap" && waitsOnAdmin(blocker)) {
      waiting.set(key, { vendor: key, wait: blocker });
    } else if (blocker) {
      notes.set(
        key,
        t("ob.conv.connect.unsupportedCard", { name: t(nameOf(key)) }),
      );
      refused.add(key);
    }
  }
  return { notes, refused, waiting };
}

function nameOf(key: MailProvider): MessageKey {
  if (key === "google") {
    return "ob.s4.provGoogle";
  }
  return key === "microsoft"
    ? "ob.s4.provMicrosoft"
    : "ob.conv.connect.tileImap";
}

/** The way on before a mailbox: the page's one primary is the ask's own
 *  Connect, so the rail offers only the skip and says what it costs. */
function SkipWayOnward({
  caption,
  held,
  finishing,
  onSkip,
}: Readonly<{
  caption: string;
  held: boolean;
  finishing: boolean;
  onSkip: () => void;
}>) {
  const t = useT();
  return (
    <StageActions>
      <span className="t-caption">{caption}</span>
      <Button
        variant="ghost"
        pending={finishing}
        disabled={held}
        onClick={onSkip}
      >
        {t("ob.conv.connect.skip")}
      </Button>
    </StageActions>
  );
}

/** The failure the step shows when the roster read itself fails, with the one
 *  recovery it has. */
function MailRosterFailed({ onRetry }: Readonly<{ onRetry: () => void }>) {
  const t = useT();
  return (
    <Callout
      tone="danger"
      kind="event"
      live="alert"
      title={t("ob.conv.connect.rosterFailedTitle")}
      actions={<Button onClick={onRetry}>{t("common.retry")}</Button>}
    >
      <p>{t("ob.conv.connect.rosterFailedBody")}</p>
    </Callout>
  );
}
