// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Check } from "lucide-react";
import { useEffect, useState } from "react";
import { CaptureNotice } from "../../../components/capture-notice";
import { Button, Disclosure } from "../../../design-system/atoms";
import { Callout } from "../../../design-system/callout";
import { type Choice, ChoiceList } from "../../../design-system/choicelist";
import { Panel, PanelBody } from "../../../design-system/panel";
import { ProviderMark } from "../../../design-system/provider-mark";
import {
  type AccountVendor,
  VENDOR_SERVICES,
} from "../../../format/connectorprovider";
import { useT } from "../../../i18n";
import type { MessageKey } from "../../../i18n/en";
import { useCanWrite } from "../../../lib/capability";
import {
  type ConnectAccountInput,
  notConfiguredProvider,
  useConnectAccount,
} from "../../../lib/connect-account";
import type { Service } from "../../../lib/connect-flow-draft";
import { serviceChoices } from "../../../lib/connect-services";
import { problemMessageOf } from "../../../lib/problem";
import {
  ImapConnectPanel,
  markOAuthAttempt,
} from "../onboarding-connect-panels";

// The connect step's ask, inline on the step: the work account picked once,
// then what to bring in from it. Mail and calendar are two grants of one
// account, so one press asks for both. The server chains the second consent
// onto the first's return.

export type MailProvider = AccountVendor | "imap";

export const MAIL_PROVIDERS: readonly MailProvider[] = [
  "google",
  "microsoft",
  "imap",
];

const VENDOR_COPY: Readonly<
  Record<MailProvider, { label: MessageKey; desc: MessageKey }>
> = {
  google: { label: "ob.s4.provGoogle", desc: "ob.conv.connect.tileGoogleDesc" },
  microsoft: {
    label: "ob.s4.provMicrosoft",
    desc: "ob.conv.connect.tileMicrosoftDesc",
  },
  imap: {
    label: "ob.conv.connect.tileImap",
    desc: "ob.conv.connect.tileImapDesc",
  },
};

/**
 * Where the reader's work account lives. `notes` replaces a choice's own line
 * with what the roster says about it (connected, refused). A choice with a
 * note in `refused` cannot be picked. While an account waits on an
 * administrator, each description says which wait and which are open now. The
 * way round it is visible before anything is picked.
 */
export function VendorChoice({
  value,
  onChange,
  disabled,
  notes,
  refused,
  waiting,
}: Readonly<{
  value: MailProvider | null;
  onChange: (next: MailProvider) => void;
  disabled: boolean;
  notes: ReadonlyMap<MailProvider, string>;
  refused: ReadonlySet<MailProvider>;
  waiting: ReadonlySet<MailProvider>;
}>) {
  const t = useT();
  // The reader who can register the app is the administrator being waited on.
  const holder = useCanWrite("capture_settings", "update");
  function standing(key: MailProvider): string | null {
    if (waiting.has(key)) {
      return t(
        holder ? "oauthApps.statusNeedsSetup" : "ob.conv.connect.waitingAdmin",
      );
    }
    if (waiting.size > 0 && !refused.has(key)) {
      return t("ob.conv.connect.availableNow");
    }
    return null;
  }
  const choices: Choice<MailProvider>[] = MAIL_PROVIDERS.map((key) => {
    const words = standing(key);
    return {
      value: key,
      label: t(VENDOR_COPY[key].label),
      description: (
        <>
          {notes.get(key) ?? t(VENDOR_COPY[key].desc)}
          {words !== null && (
            <span className="ob-connect-standing">{words}</span>
          )}
        </>
      ),
      mark: <ProviderMark providerKey={key} />,
      disabled: refused.has(key),
    };
  });
  return (
    <ChoiceList
      legend={t("ob.conv.connect.whereTitle")}
      hideLegend
      layout="cards"
      choices={choices}
      value={value ?? ""}
      onChange={onChange}
      disabled={disabled}
    />
  );
}

/** The ask for the picked account: the OAuth panel for a vendor, the IMAP form
 *  for any other host. */
export function VendorAsk({
  vendor,
  onPendingChange,
  onImapDone,
  onImapDismiss,
}: Readonly<{
  vendor: MailProvider;
  /** Told while a connect POST is in flight, so the surface holds every other
   *  way off this ask until it settles. */
  onPendingChange: (pending: boolean) => void;
  onImapDone: () => void;
  onImapDismiss: () => void;
}>) {
  const t = useT();
  if (vendor === "imap") {
    return (
      <Panel title={t("ob.conv.connect.dialogHeadlineImap")}>
        <PanelBody>
          <ImapConnectPanel
            onDone={onImapDone}
            onDismiss={onImapDismiss}
            onPendingChange={onPendingChange}
          />
        </PanelBody>
      </Panel>
    );
  }
  return <OAuthAsk vendor={vendor} onPendingChange={onPendingChange} />;
}

const OAUTH_COPY: Readonly<
  Record<AccountVendor, { unverified: MessageKey; failed: MessageKey }>
> = {
  google: {
    unverified: "ob.s4.googleUnverified",
    failed: "ob.s4.googleFailed",
  },
  microsoft: {
    unverified: "ob.s4.microsoftUnverified",
    failed: "ob.s4.microsoftFailed",
  },
};

/** What the press asks for, by name, or null when nothing is ticked. */
function connectLabel(services: readonly Service[]): MessageKey | null {
  const mail = services.includes("mail");
  const calendar = services.includes("calendar");
  if (mail && calendar) {
    return "ob.conv.connect.connectBoth";
  }
  if (mail) {
    return "ob.conv.connect.connectMailOnly";
  }
  return calendar ? "ob.conv.connect.connectCalendar" : null;
}

/** The consent the ticks ask for. Mail leads and the calendar rides as its
 *  `also_connect`; a calendar alone is its own single consent. No import
 *  window: onboarding asks how far back once the mailbox is live. */
export function connectInput(
  vendor: AccountVendor,
  services: readonly Service[],
): ConnectAccountInput {
  const pair = VENDOR_SERVICES[vendor];
  if (!services.includes("mail")) {
    return { provider: pair.calendar, returnTo: "onboarding" };
  }
  return {
    provider: pair.mail,
    returnTo: "onboarding",
    ...(services.includes("calendar") && { alsoConnect: pair.calendar }),
  };
}

function OAuthAsk({
  vendor,
  onPendingChange,
}: Readonly<{
  vendor: AccountVendor;
  onPendingChange: (pending: boolean) => void;
}>) {
  const t = useT();
  const [services, setServices] = useState<Service[]>(["mail", "calendar"]);
  // The mark lets the return tell this tab's own trip from a stale link. So it is
  // written only once the vendor's address is in hand.
  const connect = useConnectAccount({
    onRedirect: (input) => markOAuthAttempt(input.provider),
  });
  useEffect(() => {
    onPendingChange(connect.isPending);
  }, [connect.isPending, onPendingChange]);
  const name = t(
    vendor === "google" ? "ob.s4.provGoogle" : "ob.s4.provMicrosoft",
  );
  const both = services.length === 2;
  const label = connectLabel(services);
  const unwired = notConfiguredProvider(connect.data);
  const choices = serviceChoices(t, { account: vendor });
  return (
    <Panel
      title={t("ob.conv.connect.panelTitle", { vendor: name })}
      actions={
        <Button
          variant="primary"
          pending={connect.isPending}
          reason={label === null ? t("ob.conv.connect.pickService") : undefined}
          onClick={() => connect.mutate(connectInput(vendor, services))}
        >
          {t(label ?? "ob.conv.connect.connectBoth")}
        </Button>
      }
    >
      <PanelBody>
        <div className="ob-connect-ask">
          <ChoiceList
            multiple
            legend={t("ob.conv.connect.servicesLegend")}
            hideLegend
            layout="cards"
            choices={choices}
            value={services}
            onChange={setServices}
            disabled={connect.isPending}
          />
          {/* Beside the button because it describes the NEXT screens: a
              reader who meets the vendor's warning untold concludes the press
              went wrong. */}
          <Callout
            tone="info"
            kind="standing"
            title={t(
              both
                ? "ob.conv.connect.asksTwiceTitle"
                : "ob.conv.connect.asksOnceTitle",
              { vendor: name },
            )}
          >
            <p>
              {both && `${t("ob.conv.connect.asksTwiceBody")} `}
              {t(OAUTH_COPY[vendor].unverified)}
            </p>
          </Callout>
          {connect.isError && (
            <Callout
              tone="danger"
              kind="outcome"
              title={t(OAUTH_COPY[vendor].failed)}
            >
              {problemMessageOf(connect.error, t)}
            </Callout>
          )}
          {unwired && (
            <Callout
              tone="warning"
              kind="outcome"
              title={t("connectors.providerNotConfigured", { provider: name })}
            />
          )}
          {/* In the ask's own body: a second body would draw its seam right
              above the fold's own rule. */}
          <ConnectingMeans />
        </div>
      </PanelBody>
    </Panel>
  );
}

const PROMISES: readonly { lead: MessageKey; rest: MessageKey }[] = [
  { lead: "ob.s4.scope1Lead", rest: "ob.s4.scope1Rest" },
  { lead: "ob.s4.scope2Lead", rest: "ob.s4.scope2Rest" },
  { lead: "ob.s4.scope3Lead", rest: "ob.s4.scope3Rest" },
  { lead: "ob.s4.scope4Lead", rest: "ob.s4.scope4Rest" },
];

/**
 * The four step-level promises and what happens to the mail, behind one fold
 * the summary names, so nothing is hidden unannounced. Shut like every other
 * disclosure: a panel that opens on a block of reassurance buries the press it
 * exists to offer.
 */
function ConnectingMeans() {
  const t = useT();
  return (
    <Disclosure
      summary={
        <>
          {t("ob.conv.connect.meansTitle")}{" "}
          <span className="t-caption">{t("ob.conv.connect.meansSub")}</span>
        </>
      }
    >
      <div className="ob-connect-ask">
        <ul className="ob-connect-guarantees-grid">
          {PROMISES.map((item) => (
            <li key={item.lead}>
              <Check aria-hidden className="ob-connect-guarantee-check" />
              <span>
                <b>{t(item.lead)}</b> {t(item.rest)}
              </span>
            </li>
          ))}
        </ul>
        <CaptureNotice />
      </div>
    </Disclosure>
  );
}
