import { useQuery } from "@tanstack/react-query";
import { Check, Circle } from "lucide-react";
import type { ReactNode } from "react";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button, Disclosure } from "../../design-system/atoms";
import { ProviderMark } from "../../design-system/provider-mark";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { throwProblem } from "../common";
import { OvernightGrantChoice } from "../overnight-grant";
import { ConnectDialog } from "./connect-dialog";
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

const PROVIDER_COPY: Readonly<
  Record<
    MailProvider,
    {
      name: MessageKey;
      brings: MessageKey;
      auth: MessageKey;
      dialogHeadline: (t: ReturnType<typeof useT>) => string;
    }
  >
> = {
  google: {
    name: "ob.s4.provGoogle",
    brings: "ob.conv.connect.gmailBrings",
    auth: "ob.conv.connect.scopeGoogle",
    dialogHeadline: (t) =>
      t("ob.conv.connect.dialogHeadlineAccess", {
        name: t("ob.s4.provGoogle"),
      }),
  },
  microsoft: {
    name: "ob.s4.provMicrosoft",
    brings: "ob.conv.connect.microsoftBrings",
    auth: "ob.conv.connect.scopeMicrosoft",
    dialogHeadline: (t) =>
      t("ob.conv.connect.dialogHeadlineAccess", {
        name: t("ob.s4.provMicrosoft"),
      }),
  },
  imap: {
    name: "ob.s4.provImap",
    brings: "ob.conv.connect.imapBrings",
    auth: "ob.conv.connect.scopeImap",
    dialogHeadline: (t) => t("ob.conv.connect.dialogHeadlineImap"),
  },
};

export const MAIL_PROVIDERS: readonly MailProvider[] = [
  "google",
  "microsoft",
  "imap",
];

// The provider name this build's roster row carries for each card. `gcal` is
// a paired connector some OAuth grants also create and never gets its own
// card, so it is deliberately absent from this map.
const ROSTER_PROVIDER: Readonly<Record<MailProvider, string>> = {
  google: "gmail",
  microsoft: "graph",
  imap: "imap",
};

/** The connected roster and whether it has actually been verified. `verified`
 *  is false while the fetch is in flight (first load OR a later refetch, e.g.
 *  the invalidation IMAP's own successful connect fires) and after it fails —
 *  states the scene's "pick one" rule cannot tell apart from "nothing is
 *  connected" without this flag, and all of them must withhold provider
 *  actions rather than treat an unread or re-reading roster as an empty one.
 *  `failed` and `retry` exist so a genuine read failure can be said out loud
 *  and retried, rather than leaving every card silently and permanently
 *  disabled with nothing on the surface explaining why. */
type ConnectedMailRoster = Readonly<{
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
type ConnectBlocker = Exclude<ProviderAvailability["reason"], "ready">;

// What a blocked card says, and whether Settings is where it gets fixed. A
// deployment that does not serve a provider at all is nobody's setting, so it
// offers no link that would lead to an empty form.
//
// Registering the app is NOT offered here, on any of them. The installation's
// OAuth app is asked for once, on the first-run platform step, so by the time
// anybody reaches this screen that question is answered: a vendor still
// missing one is a vendor this installation did not set up, which is an
// admin's errand in Settings rather than a form to fill in mid-connect.
const BLOCKER_COPY: Readonly<
  Record<ConnectBlocker, { body: MessageKey; settings: boolean }>
> = {
  app_missing: { body: "ob.conv.connect.appMissingCard", settings: true },
  app_unusable: { body: "ob.conv.connect.appUnusableCard", settings: true },
  unsupported: { body: "ob.conv.connect.unsupportedCard", settings: false },
};

/**
 * Which mailboxes are already live, read fresh here rather than assumed from
 * the pre-consent `provider` selection: a reload can land on this step with a
 * mailbox already connected from an earlier session, and the cards have to
 * say so without the reader clicking anything. Shares the `["connectors"]`
 * query key with `OAuthReturnPanel` — react-query dedupes the two into one
 * request, so this costs nothing extra on the common path.
 */
function useConnectedMailProviders(): ConnectedMailRoster {
  const roster = useQuery({
    queryKey: ["connectors"],
    queryFn: async () => {
      const { data, error } = await api.GET("/connectors");
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
  const connected = roster.data?.data.filter((c) => c.status === "connected");
  const blockers = new Map<string, ConnectBlocker>();
  for (const entry of roster.data?.providers ?? []) {
    if (entry.reason !== "ready") {
      blockers.set(entry.provider, entry.reason);
    }
  }
  return {
    providers: new Set((connected ?? []).map((c) => c.provider)),
    blockers,
    verified: roster.isSuccess && !roster.isFetching,
    failed: roster.isError,
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
        <h3>
          {t("ob.conv.connect.mailboxTitle")}
          <span className="ob-connect-pill ob-connect-pill-required">
            {t("ob.conv.connect.required")}
          </span>
        </h3>
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
        <h3>
          {t("ob.conv.connect.networkTitle")}
          <span className="ob-connect-pill ob-connect-pill-recommended">
            {t("ob.conv.connect.recommended")}
          </span>
        </h3>
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

/** The honest failure the mailbox section shows when the roster read itself
 * fails, matching the read-failure card the OAuth/IMAP panels already use
 * (`ConnectWarn` in onboarding-connect-panels.tsx) — same visual language,
 * with the one recovery this failure actually has: reading the roster again. */
function MailRosterFailed({ onRetry }: Readonly<{ onRetry: () => void }>) {
  const t = useT();
  return (
    <div
      className="readfail warn"
      role="alert"
      style={{ maxWidth: 460, margin: "0 auto" }}
    >
      <span className="rfi">
        <Circle aria-hidden />
      </span>
      <div>
        <div className="rft">{t("ob.conv.connect.rosterFailedTitle")}</div>
        <p className="rfp">{t("ob.conv.connect.rosterFailedBody")}</p>
        <Button
          small
          variant="ghost"
          onClick={onRetry}
          style={{ marginTop: "var(--space-3)" }}
        >
          {t("common.retry")}
        </Button>
      </div>
    </div>
  );
}

/**
 * What one mailbox card is, given the roster. Three refusals in a fixed order,
 * because they are not equally useful to the reader: an already-connected
 * mailbox is the whole answer, a mailbox chosen elsewhere makes every other
 * card moot, and only then does a provider this installation cannot run get to
 * explain itself. Telling somebody to register a second vendor's OAuth app
 * when they have already connected their mail would be true and useless.
 *
 */
function mailCardState(
  key: MailProvider,
  roster: ConnectedMailRoster,
  anyMailConnected: boolean,
): { state: CardState; brings: MessageKey; settingsLink: boolean } {
  if (roster.providers.has(ROSTER_PROVIDER[key])) {
    return {
      state: "connected",
      brings: PROVIDER_COPY[key].brings,
      settingsLink: false,
    };
  }
  if (anyMailConnected) {
    return {
      state: "blocked",
      brings: "ob.conv.connect.blockedCard",
      settingsLink: false,
    };
  }
  const blocker = roster.blockers.get(ROSTER_PROVIDER[key]);
  if (blocker) {
    return {
      state: "unavailable",
      brings: BLOCKER_COPY[blocker].body,
      settingsLink: BLOCKER_COPY[blocker].settings,
    };
  }
  return {
    state: "idle",
    brings: PROVIDER_COPY[key].brings,
    settingsLink: false,
  };
}

type CardState = "idle" | "connected" | "blocked" | "unavailable";

/**
 * One provider tile: a mark, the name, one line of what it gives, and a
 * footer naming the auth mechanism plus the affordance for the tile's own
 * state. The whole tile is the button — its accessible name carries both the
 * provider and what connecting it grants, so a reader never has to open it to
 * find out.
 *
 * `disabled` is the caller's own call, separate from `state`: a "connected"
 * or "blocked" tile is always disabled (there is no further action here,
 * disconnecting a mailbox is Settings' job, and a blocked tile names the
 * mailbox already chosen instead of inviting a second one), but an "idle"
 * tile can ALSO be disabled while its own connected/blocked status is not yet
 * verified, without that unverified moment being mislabelled as blocked.
 *
 * An "unavailable" tile is not a button at all. Nothing here can be operated
 * until somebody registers the organization's app, and the one thing a reader
 * can do about it is a link, which HTML does not allow inside a button.
 */
function ConnectorCard({
  markKey,
  name,
  brings,
  auth,
  state,
  disabled,
  onOpen,
  settingsLink = false,
  idleCta,
  settledCta,
}: Readonly<{
  markKey: string;
  name: string;
  brings: string;
  auth: string;
  state: CardState;
  disabled: boolean;
  onOpen: () => void;
  /** Whether an "unavailable" tile offers the way out. A provider this
   *  deployment does not serve at all has no setting to reach, and a link to
   *  an empty form is worse than no link. */
  settingsLink?: boolean;
  /** What the idle tile's affordance says. Mail cards connect; a tile whose
   *  verb is something else names it, so the surface never invites a reader
   *  to "connect" a thing that is only being written down. */
  idleCta?: string;
  /** What the settled tile's affordance says, for the same reason: a saved
   *  address is not a connected integration and must not read as one. */
  settledCta?: string;
}>) {
  const t = useT();
  const face = (
    <>
      <span className="ob-connect-card-head">
        <span className="ob-connect-mark">
          <ProviderMark providerKey={markKey} />
        </span>
        {state === "connected" && (
          <span className="ob-connect-card-done" aria-hidden="true">
            <Check />
          </span>
        )}
      </span>
      <b>{name}</b>
      <small className="t-sub">{brings}</small>
      <span className="ob-connect-card-foot">
        <span className="ob-connect-card-auth t-caption">{auth}</span>
        {state === "unavailable"
          ? settingsLink && (
              <a
                className="ob-connect-card-setup"
                href="#/settings/admin/general"
              >
                {t("ob.conv.connect.appSetupLink")}
              </a>
            )
          : state !== "blocked" && (
              <span className="ob-connect-card-cta">
                {state === "connected"
                  ? (settledCta ?? t("ob.conv.connect.connectedCta"))
                  : (idleCta ?? t("ob.conv.connect.connectCta"))}
              </span>
            )}
      </span>
    </>
  );

  if (state === "unavailable") {
    return (
      <div className="ob-connect-card" data-state={state}>
        {face}
      </div>
    );
  }

  return (
    <button
      type="button"
      className="ob-connect-card"
      data-state={state}
      disabled={disabled}
      onClick={onOpen}
    >
      {face}
    </button>
  );
}

/**
 * The LinkedIn card: what saving the profile is for while pending, its own
 * dialog once clicked, or a resolved state (saved / skipped) once
 * `linkedinStatus` says it is settled. Split out of ConnectScene so the scene
 * itself stays about composition.
 */
function LinkedinCard({
  status,
  onSave,
  onSkip,
  pending,
  error,
}: Readonly<{
  status: LinkedinStatus;
  onSave: (profileUrl: string) => void;
  onSkip: () => void;
  pending: boolean;
  error: string | null;
}>) {
  const t = useT();
  const [open, setOpen] = useState(false);

  return (
    <div className="ob-connect-grid ob-connect-grid-single">
      <ConnectorCard
        markKey="linkedin"
        name={t("ob.conv.connect.linkedinName")}
        brings={
          status === "saved"
            ? t("ob.conv.connect.linkedinSaved")
            : status === "skipped"
              ? t("ob.conv.connect.linkedinSkippedNote")
              : t("ob.conv.linkedin.cardBody")
        }
        auth={t("ob.conv.connect.linkedinAuth")}
        idleCta={t("ob.conv.connect.saveCta")}
        settledCta={t("ob.conv.connect.savedCta")}
        state={
          status === "saved"
            ? "connected"
            : status === "skipped"
              ? "blocked"
              : "idle"
        }
        disabled={status !== "pending"}
        onOpen={() => setOpen(true)}
      />

      {status === "pending" && open && (
        <ConnectDialog
          open
          // X, Escape, and backdrop all resolve to this ONE handler, so
          // guarding it here is the one place that keeps every dismissal
          // route from racing the save: a successful PUT landing after the
          // reader already backed out would leave the profile saved against
          // a dialog that already closed on a different decision.
          onClose={() => {
            if (!pending) {
              setOpen(false);
            }
          }}
          providerMarkKey="linkedin"
          headline={t("ob.conv.linkedin.dialogHeadline")}
        >
          <LinkedinPanel
            // No `setOpen(false)` here: a failed save has to stay on screen
            // so `error` (below) is actually seen and retried, and a
            // successful one already unmounts this dialog on its own —
            // `status` flips to "saved" and the guard above stops rendering
            // it.
            onSave={onSave}
            onSkip={() => {
              onSkip();
              setOpen(false);
            }}
            pending={pending}
            error={error}
          />
        </ConnectDialog>
      )}
    </div>
  );
}

// The profile ask: one URL and why it is wanted. No scope list, because
// nothing is being granted — the address is written to the member's own
// account and read by nobody but the import that attributes their network.
function LinkedinPanel({
  onSave,
  onSkip,
  pending,
  error,
}: Readonly<{
  onSave: (profileUrl: string) => void;
  onSkip: () => void;
  pending: boolean;
  error: string | null;
}>) {
  const t = useT();
  const [profile, setProfile] = useState("");
  const trimmed = profile.trim();

  return (
    <div className="ob-connect-linkedin-panel">
      <label className="ob-conv-field" htmlFor="linkedin-profile">
        {t("ob.conv.linkedin.profileLabel")}
        <input
          id="linkedin-profile"
          type="url"
          inputMode="url"
          placeholder={t("ob.conv.linkedin.profilePlaceholder")}
          value={profile}
          onChange={(event) => setProfile(event.target.value)}
        />
      </label>
      <p className="t-sub">{t("ob.conv.linkedin.profileWhy")}</p>
      <div className="ob-connect-dialog-actions">
        <Button
          variant="primary"
          disabled={trimmed === "" || pending}
          onClick={() => onSave(trimmed)}
        >
          {t("ob.conv.linkedin.save")}
        </Button>
        <button
          type="button"
          className="ob-connect-dialog-notnow"
          // Skipping and saving are the two answers to the same question, so
          // they cannot both be in flight at once: a skip that lands while
          // the save PUT is still pending would leave the account skipped
          // locally against a save that lands right after, with nothing to
          // reconcile the two.
          disabled={pending}
          onClick={onSkip}
        >
          {t("ob.conv.linkedin.skip")}
        </button>
      </div>
      {error !== null && (
        <p role="alert" className="t-sub t-danger">
          {error}
        </p>
      )}
      <p className="t-sub">{t("ob.conv.linkedin.importLater")}</p>
    </div>
  );
}
