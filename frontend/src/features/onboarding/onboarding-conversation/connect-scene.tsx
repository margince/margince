import { Check } from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../../../api/schema";
import { Badge, Button, Disclosure } from "../../../design-system/atoms";
import { Heading } from "../../../design-system/heading";
import { Row } from "../../../design-system/stack";
import { useT } from "../../../i18n";
import type { MessageKey } from "../../../i18n/en";
import { OvernightGrantChoice } from "../../automations/overnight-grant";
import { useConnectors } from "../../capture/connectors";
import { ConnectDialog } from "./connect-dialog";
import {
  ConnectorCard,
  LinkedinCard,
  MailRosterFailed,
  mailCardState,
  PROVIDER_COPY,
  ROSTER_PROVIDER,
} from "./connect-scene.cards";
import { WayOnward } from "./way-onward";
// The connect act's work surface: two sections of real-width cards — the
// required mailbox choice, and the network one beside it — each card opening
// its OWN dialog rather than growing an inline panel under itself. A card
// names the provider AND what it gives (its accessible name carries both), so
// a reader never has to open it to learn what it is for.
//
// LinkedIn is not a connection here. Nothing is authorized and nothing syncs:
// the member records which profile their imported network is attributed to,
// so a connection they bring in later reads "Anna knows them" rather than
// "the company knows them". The import itself lives in Settings.
//
// The four step-level consent guarantees render HERE, on the surface, in a
// real-width grid: they are substance about what this step does, and the
// rail narrates, it does not host a step's substance. Each mail provider's
// OWN disclosure (its OAuth hint) lives one level deeper, inside its dialog.

export type MailProvider = "google" | "microsoft" | "imap";

export type LinkedinStatus = "pending" | "saved" | "skipped";

// The mark key each provider card carries, straight from `ProviderMark`'s own
// vocabulary. `imap` has no brand of its own, so it takes the neutral mark the
// design system already renders for a provider it has no logo for.
const PROVIDER_MARKS: Readonly<Record<MailProvider, string>> = {
  google: "google",
  microsoft: "microsoft",
  imap: "imap",
};

export const MAIL_PROVIDERS: readonly MailProvider[] = [
  "google",
  "microsoft",
  "imap",
];

/** The connected roster and whether it has actually been verified. `verified`
 *  is false while the fetch is in flight (first load OR a later refetch, e.g.
 *  the invalidation IMAP's own successful connect fires) and after it fails —
 *  states the scene's "pick one" rule cannot tell apart from "nothing is
 *  connected" without this flag, and all of them must withhold provider
 *  actions rather than treat an unread or re-reading roster as an empty one.
 *  `failed` and `retry` exist so a genuine read failure can be said out loud
 *  and retried, rather than leaving every card silently and permanently
 *  disabled with nothing on the surface explaining why. */
export type ConnectedMailRoster = Readonly<{
  providers: ReadonlySet<string>;
  /** Why a provider could not be connected right now, keyed by the roster's
   *  own provider name. Absent for a provider the server called ready, and
   *  absent for the whole map on a build whose server does not report it yet,
   *  which reads the same as it always did: every card offered. */
  blockers: ReadonlyMap<string, ConnectBlocker>;
  verified: boolean;
  failed: boolean;
  retry: () => void;
}>;

/** One provider's answer from the roster read, decided by the same predicate
 *  the connect endpoint uses, so a card cannot offer what a click would be
 *  refused. */
export type ProviderAvailability =
  components["schemas"]["CaptureProviderAvailability"];

/** The reasons that refuse a connect, derived from the contract rather than
 *  respelled: `ready` is the fourth answer and never reaches the map below. */
export type ConnectBlocker = Exclude<ProviderAvailability["reason"], "ready">;

/**
 * Which mailboxes are already live, read fresh here rather than assumed from
 * the pre-consent `provider` selection: a reload can land on this step with a
 * mailbox already connected from an earlier session, and the cards have to
 * say so without the reader clicking anything. Shares the `["connectors"]`
 * query key with `OAuthReturnPanel` — react-query dedupes the two into one
 * request, so this costs nothing extra on the common path.
 */
function useConnectedMailProviders(): ConnectedMailRoster {
  const roster = useConnectors();
  const connected = roster.data?.data.filter((c) => c.status === "connected");
  const blockers = new Map<string, ConnectBlocker>();
  for (const entry of roster.data?.providers ?? []) {
    if (entry.reason !== "ready") {
      blockers.set(entry.provider, entry.reason);
    }
  }
  // A deployment that never wired mail capture answers 501, which the shared
  // read reports as `notConfigured` rather than as a failure. There is no
  // roster to draw a card from either way, so this scene says the same thing
  // for both: it could not read what this installation can connect.
  const unreadable = roster.isError || roster.data?.notConfigured === true;
  return {
    providers: new Set((connected ?? []).map((c) => c.provider)),
    blockers,
    verified: roster.isSuccess && !roster.isFetching && !unreadable,
    failed: unreadable,
    retry: () => void roster.refetch(),
  };
}

export function ConnectScene({
  provider,
  onPick,
  onDialogClose,
  dialogShowsResult,
  dialogPanel,
  returnPanel,
  onFinish,
  finishing,
  wantsOvernight,
  onWantsOvernightChange,
  overnightFailed,
  linkedinStatus,
  onLinkedinSave,
  onLinkedinSkip,
  linkedinPending,
  linkedinError,
}: Readonly<{
  /** The provider whose dialog is open; null while none is chosen. */
  provider: MailProvider | null;
  onPick: (provider: MailProvider) => void;
  /** Closes the open dialog via its own chrome (the X, Escape, backdrop). */
  onDialogClose: () => void;
  /**
   * True when `provider`'s dialog is showing a proven attempt's RESULT
   * (`returnPanel`) rather than the pre-consent ask (`dialogPanel`) — the
   * caller is the one place that can tell a genuine return from this tab
   * apart from a stale or bookmarked outcome URL, so it decides which.
   */
  dialogShowsResult: boolean;
  /** The chosen provider's pre-consent panel, rendered INSIDE its dialog. */
  dialogPanel: ReactNode;
  /**
   * The post-consent result (`OAuthReturnPanel` plus the backfill window
   * control) once a redirect has returned. Shown INSIDE the dialog the
   * reader left from when `dialogShowsResult` says this tab genuinely made
   * that trip; otherwise rendered inline on the surface — still a real
   * finding, just not one this tab can vouch for having just requested.
   */
  returnPanel: ReactNode;
  /**
   * Leaves the step, saying how: `skipped` is true only when no mailbox is
   * connected and the reader chose to go on without one. The scene decides
   * which of the two it is asking for, because the roster it reads is the
   * one fact that tells them apart.
   */
  onFinish: (skipped: boolean) => void;
  finishing: boolean;
  /** The rep's preselected answer to the overnight question. Owned by the act,
   * because that is where it rides along with the connect the step performs —
   * this scene only asks it. */
  wantsOvernight: boolean;
  onWantsOvernightChange: (next: boolean) => void;
  /** The answer could not be recorded. The step completed anyway, so this says
   * so rather than blocking — the question is askable again in Settings. */
  overnightFailed: boolean;
  linkedinStatus: LinkedinStatus;
  onLinkedinSave: (profileUrl: string) => void;
  onLinkedinSkip: () => void;
  linkedinPending: boolean;
  linkedinError: string | null;
}>) {
  const t = useT();
  const mailRoster = useConnectedMailProviders();
  const anyMailConnected = MAIL_PROVIDERS.some((key) =>
    mailRoster.providers.has(ROSTER_PROVIDER[key]),
  );

  return (
    <div className="ob-scene ob-connect">
      <ConnectGuarantees />

      <div className="ob-connect-section-head">
        <Heading size="medium">{t("ob.conv.connect.mailboxTitle")}</Heading>
        <p className="t-sub">{t("ob.conv.connect.mailboxHint")}</p>
      </div>

      {/* A mailbox is the required gate on this whole step, so a roster read
          that failed outright cannot just leave every card silently disabled
          — that reads as an ordinary "still loading" moment forever, with no
          way out. Said out loud, with the one recovery that actually clears
          it: reading the roster again. */}
      {mailRoster.failed && <MailRosterFailed onRetry={mailRoster.retry} />}

      <div className="ob-connect-grid">
        {MAIL_PROVIDERS.map((key) => {
          const card = mailCardState(key, mailRoster, anyMailConnected);
          return (
            <ConnectorCard
              key={key}
              markKey={PROVIDER_MARKS[key]}
              name={t(PROVIDER_COPY[key].name)}
              brings={t(card.brings, { name: t(PROVIDER_COPY[key].name) })}
              auth={t(PROVIDER_COPY[key].auth)}
              settingsLink={card.settingsLink}
              state={card.state}
              // A card the roster hasn't verified yet is neither "connected"
              // nor "blocked" — it still reads and speaks as idle — but it
              // must not be clickable: opening it could connect a second
              // mailbox the still-loading (or failed) fetch just hasn't
              // reported yet.
              disabled={card.state !== "idle" || !mailRoster.verified}
              onOpen={() => onPick(key)}
            />
          );
        })}
      </div>

      {/* Under the mailboxes, because it is the same decision seen from the
          other side: the cards above say what Margince may READ, this says
          whether it may act on it overnight while nobody is watching. Asked
          here rather than inside a provider dialog because a real OAuth allow
          leaves the page entirely — a box ticked in that dialog would not
          survive the redirect back. */}
      <OvernightGrantChoice
        checked={wantsOvernight}
        onChange={onWantsOvernightChange}
        failed={overnightFailed}
      />

      {/* The inline fallback: a real finding either way, but shown here
          rather than inside a dialog because this tab has no proof it just
          requested it — see `dialogShowsResult` on why that distinction is
          the caller's to make. */}
      {!dialogShowsResult && returnPanel}

      <div className="ob-connect-section-head">
        {/* The default tone: LinkedIn never gates the act. */}
        <Row>
          <Heading size="medium">{t("ob.conv.connect.networkTitle")}</Heading>
          <Badge>{t("ob.conv.connect.recommended")}</Badge>
        </Row>
        <p className="t-sub">{t("ob.conv.connect.networkHint")}</p>
      </div>

      <LinkedinCard
        status={linkedinStatus}
        onSave={onLinkedinSave}
        onSkip={onLinkedinSkip}
        pending={linkedinPending}
        error={linkedinError}
      />

      {provider && (
        <ProviderDialog
          provider={provider}
          showsResult={dialogShowsResult}
          returnPanel={returnPanel}
          dialogPanel={dialogPanel}
          onClose={onDialogClose}
        />
      )}

      <ConnectWayOnward
        mailConnected={anyMailConnected}
        rosterVerified={mailRoster.verified}
        finishing={finishing}
        onFinish={onFinish}
      />
    </div>
  );
}

/**
 * The open provider's dialog: the ask — or, once a proven trip has returned,
 * its result. The result dialog names the provider plainly: `returnPanel`
 * (OAuthReturnPanel) carries its own heading for what happened, so a second
 * "access needed" headline above it would be both redundant and wrong,
 * nothing being asked for any more.
 */
function ProviderDialog({
  provider,
  showsResult,
  returnPanel,
  dialogPanel,
  onClose,
}: Readonly<{
  provider: MailProvider;
  showsResult: boolean;
  returnPanel: ReactNode;
  dialogPanel: ReactNode;
  onClose: () => void;
}>) {
  const t = useT();
  const copy = PROVIDER_COPY[provider];
  return (
    <ConnectDialog
      open
      onClose={onClose}
      providerMarkKey={PROVIDER_MARKS[provider]}
      headline={showsResult ? t(copy.name) : copy.dialogHeadline(t)}
      intro={
        showsResult
          ? undefined
          : t("ob.conv.connect.dialogIntro", { brings: t(copy.brings) })
      }
    >
      {showsResult ? returnPanel : dialogPanel}
    </ConnectDialog>
  );
}

/**
 * The way on, pinned to the surface's own foot rather than a chip in the
 * thread below: the reader is done choosing on THIS panel, so the action that
 * leaves it belongs here too. It always presses; without a mailbox it names
 * the gap, and the honest way past it stands beside it — worded for what it
 * is, because LinkedIn may well be connected by now and "skip connecting"
 * would then be false.
 */
function ConnectWayOnward({
  mailConnected,
  rosterVerified,
  finishing,
  onFinish,
}: Readonly<{
  mailConnected: boolean;
  rosterVerified: boolean;
  finishing: boolean;
  onFinish: (skipped: boolean) => void;
}>) {
  const t = useT();
  return (
    <WayOnward
      label={t("ob.conv.connect.continue")}
      pending={finishing}
      blockers={mailConnected ? [] : [t("ob.conv.connect.mailboxNeeded")]}
      stillNeeded={(why) => why.join(" ")}
      onGo={() => onFinish(false)}
    >
      {!mailConnected && (
        <Button
          variant="ghost"
          // Held until the roster is read: recording "no mailbox" against a
          // roster that has not answered yet could persist a fact that is
          // not so.
          disabled={finishing || !rosterVerified}
          onClick={() => onFinish(true)}
        >
          {t("ob.conv.connect.skip")}
        </Button>
      )}
    </WayOnward>
  );
}

/**
 * The four step-level promises, as a real two-column grid rather than a
 * flowing paragraph forced into a rail's ~250px column — the wrapping that
 * made every cell there read as broken text. Every word survives from the
 * rail turn it replaces (`ob.s4.scope*`); only its column changed.
 */
function ConnectGuarantees() {
  const t = useT();
  const items: { lead: MessageKey; rest: MessageKey }[] = [
    { lead: "ob.s4.scope1Lead", rest: "ob.s4.scope1Rest" },
    { lead: "ob.s4.scope2Lead", rest: "ob.s4.scope2Rest" },
    { lead: "ob.s4.scope3Lead", rest: "ob.s4.scope3Rest" },
    { lead: "ob.s4.scope4Lead", rest: "ob.s4.scope4Rest" },
  ];
  // Shut, like every other disclosure in the product. The four promises are
  // what the summary line names, so nothing is hidden that the reader is not
  // told about — and a scene that arrives with a block of reassurance already
  // unfolded buries the providers it exists to offer.
  return (
    <Disclosure summary={t("ob.conv.connect.guaranteesToggle")}>
      <ul className="ob-connect-guarantees-grid">
        {items.map((item) => (
          <li key={item.lead}>
            <Check aria-hidden className="ob-connect-guarantee-check" />
            <span>
              <b>{t(item.lead)}</b> {t(item.rest)}
            </span>
          </li>
        ))}
      </ul>
    </Disclosure>
  );
}
