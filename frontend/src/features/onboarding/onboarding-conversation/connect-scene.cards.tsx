import { Check, Circle } from "lucide-react";
import { useState } from "react";
import { Button, Field } from "../../../design-system/atoms";
import { ErrorLine } from "../../../design-system/errorline";
import { ProviderMark } from "../../../design-system/provider-mark";
import { useT } from "../../../i18n";
import type { MessageKey } from "../../../i18n/en";
import { ConnectDialog } from "./connect-dialog";
import type {
  ConnectBlocker,
  ConnectedMailRoster,
  LinkedinStatus,
  MailProvider,
} from "./connect-scene";

export const PROVIDER_COPY: Readonly<
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

// The provider name this build's roster row carries for each card. `gcal` is
// a paired connector some OAuth grants also create and never gets its own
// card, so it is absent from this map.
export const ROSTER_PROVIDER: Readonly<Record<MailProvider, string>> = {
  google: "gmail",
  microsoft: "graph",
  imap: "imap",
};

// What a blocked card says, and whether Settings is where it gets fixed. A
// deployment that does not serve a provider at all is nobody's setting, so it
// offers no link that would lead to an empty form.
//
// Registering the app is not offered here, on any of them. The first-run
// platform step asks for the installation's OAuth app, so that question is
// answered before this screen. A vendor still missing one was not set up by
// this installation, which makes it an admin's errand in Settings.
const BLOCKER_COPY: Readonly<
  Record<ConnectBlocker, { body: MessageKey; settings: boolean }>
> = {
  app_missing: { body: "ob.conv.connect.appMissingCard", settings: true },
  app_unusable: { body: "ob.conv.connect.appUnusableCard", settings: true },
  unsupported: { body: "ob.conv.connect.unsupportedCard", settings: false },
};

/** The failure the mailbox section shows when the roster read itself fails.
 * It matches the read-failure card of the OAuth/IMAP panels (`ConnectWarn` in
 * onboarding-connect-panels.imap.tsx), with the one recovery this failure has:
 * reading the roster again. */
export function MailRosterFailed({
  onRetry,
}: Readonly<{ onRetry: () => void }>) {
  const t = useT();
  return (
    <div className="readfail warning ob-conv-rosterfail" role="alert">
      <span className="rfi">
        <Circle aria-hidden />
      </span>
      <div>
        <div className="rft">{t("ob.conv.connect.rosterFailedTitle")}</div>
        <p className="rfp">{t("ob.conv.connect.rosterFailedBody")}</p>
        <Button
          variant="ghost"
          className="ob-conv-rosterfail-retry"
          onClick={onRetry}
        >
          {t("common.retry")}
        </Button>
      </div>
    </div>
  );
}

/**
 * What one mailbox card is, given the roster. Three refusals in a fixed order,
 * because they are not equally useful to the reader. An already-connected
 * mailbox is the whole answer, and a mailbox chosen elsewhere makes every other
 * card moot. Only then does a provider this installation cannot run explain
 * itself. Asking for a second vendor's OAuth app after mail is connected would
 * be true and useless.
 */
export function mailCardState(
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
 * One provider tile: a mark, the name and one line of what it gives. Its
 * footer names the auth mechanism and the affordance for the tile's state.
 * The whole tile is the button. Its accessible name carries the
 * provider and what connecting it grants, so a reader never has to open it.
 *
 * `disabled` is the caller's call, separate from `state`. A "connected" or
 * "blocked" tile is always disabled: disconnecting is Settings' job, and a
 * blocked tile names the mailbox already chosen. An "idle" tile can also be
 * disabled while its status is unverified, without being mislabelled blocked.
 *
 * An "unavailable" tile is not a button at all. Nothing here works until
 * somebody registers the company's app. The one thing a reader can do is
 * follow a link, which HTML does not allow inside a button.
 */
export function ConnectorCard({
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
              <a className="ob-connect-card-setup" href="#/settings/general">
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
 * The LinkedIn card. While pending it says what saving the profile is for,
 * and opens its own dialog once clicked. Once `linkedinStatus` settles it
 * shows the resolved state (saved / skipped). A component of its own so ConnectScene
 * stays about composition.
 */
export function LinkedinCard({
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

      {status === "pending" && (
        <ConnectDialog
          open={open}
          // X, Escape and backdrop all resolve to this one handler, so this
          // guard keeps every dismissal route from racing the save. A PUT
          // landing after the reader backed out would save the profile against
          // a dialog that closed on a different decision.
          onClose={() => {
            if (!pending) {
              setOpen(false);
            }
          }}
          providerMarkKey="linkedin"
          headline={t("ob.conv.linkedin.dialogHeadline")}
        >
          <LinkedinPanel
            // No `setOpen(false)` here: a failed save stays on screen so
            // `error` (below) is seen and retried. A successful one unmounts
            // this dialog: `status` flips to "saved" and the guard above stops
            // rendering it.
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
// nothing is granted. The address is written to the member's own account and
// read only by the import that attributes their network.
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
    <>
      <div className="form-stack ob-connect-linkedin-panel">
        <Field label={t("ob.conv.linkedin.profileLabel")}>
          {(control) => (
            <input
              {...control}
              type="url"
              inputMode="url"
              placeholder={t("ob.conv.linkedin.profilePlaceholder")}
              value={profile}
              onChange={(event) => setProfile(event.target.value)}
            />
          )}
        </Field>
        <p className="t-sub">{t("ob.conv.linkedin.profileWhy")}</p>
        {error !== null && <ErrorLine>{error}</ErrorLine>}
        <p className="t-sub">{t("ob.conv.linkedin.importLater")}</p>
      </div>
      <div className="actions">
        <Button
          // Skipping and saving answer the same question, so they cannot both
          // be in flight. A skip during a pending save PUT would mark the
          // account skipped locally while the save lands right after. Nothing
          // would reconcile the two.
          disabled={pending}
          onClick={onSkip}
        >
          {t("ob.conv.linkedin.skip")}
        </Button>
        <Button
          variant="primary"
          disabled={trimmed === "" || pending}
          onClick={() => onSave(trimmed)}
        >
          {t("ob.conv.linkedin.save")}
        </Button>
      </div>
    </>
  );
}
