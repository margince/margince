import { useEffect, useState } from "react";
import { OAuthAppSteps } from "../../components/oauth-app-steps";
import { RedirectUris } from "../../components/oauth-redirects";
import { Button, Field, TextInput } from "../../design-system/atoms";
import { Checklist, ChecklistStep } from "../../design-system/checklist";
import { ChoiceList } from "../../design-system/choicelist";
import { ErrorLine } from "../../design-system/errorline";
import { StageActions } from "../../design-system/onboarding-stage";
import { Panel, PanelBody } from "../../design-system/panel";
import { ProviderMark } from "../../design-system/provider-mark";
import { WriteRefused } from "../../design-system/querystates";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  useOAuthApp,
  useSetOAuthApp,
  type Vendor,
  vendorCopy,
} from "../../lib/oauth-app";
import { ImapMailboxForm } from "../capture/imap-connect-form";

/**
 * What the company runs on: one answer covering mail and sign-in.
 *
 * They are separate mechanisms in the server and the same fact about a company.
 * A company on Workspace reads mail through a Google app. It signs its users in
 * with Google accounts, through that same app and console entry. Two questions
 * would ask for one fact twice and then keep the two answers agreeing.
 */
const PLATFORMS = ["google", "microsoft", "imap"] as const;

type Platform = (typeof PLATFORMS)[number];

// Typed by `Platform` in both directions: an answer with no copy fails, and
// copy for an answer that does not exist fails too.
const PLATFORM_COPY: Readonly<
  Record<Platform, { label: MessageKey; what: MessageKey }>
> = {
  google: {
    label: "firstRun.platform.google",
    what: "firstRun.platform.googleWhat",
  },
  microsoft: {
    label: "firstRun.platform.microsoft",
    what: "firstRun.platform.microsoftWhat",
  },
  imap: {
    label: "firstRun.platform.imap",
    what: "firstRun.platform.imapWhat",
  },
};

/**
 * The vendor's app: the two values every OAuth client has, and Microsoft's
 * optional directory pin. Its own component so the app query is asked only
 * for the vendor on screen, and so the fields reset when the answer changes.
 */
function VendorAppFields({
  vendor,
  onBusy,
  onDecline,
}: Readonly<{
  vendor: Vendor;
  onBusy: (busy: boolean) => void;
  /** The way past for an admin without the console open: the step does not
   * block, and the same form waits in Settings. */
  onDecline: () => void;
}>) {
  const t = useT();
  const app = useOAuthApp(vendor);
  const save = useSetOAuthApp(vendor);
  const [clientId, setClientId] = useState("");
  const [clientSecret, setClientSecret] = useState("");
  const [tenant, setTenant] = useState("");
  // The directory is asked for here, unlike Settings, where it stays optional.
  // It decides whether Microsoft appears on the login page at all. In the
  // backend, compose/microsoftsignin.go names nobody and signs nobody in for an
  // app on no directory. An installation that registered Microsoft one step
  // earlier would find no Microsoft button and no way to tell why. The server
  // refuses anything but a directory id. So "not empty" is the whole client
  // rule, and the shape stays the server's.
  const missing = [
    [clientId.trim() === "", t("oauthApp.clientId")],
    [clientSecret.trim() === "", t("oauthApp.clientSecret")],
    [vendor === "microsoft" && tenant.trim() === "", t("oauthApp.tenant")],
  ]
    .filter((need): need is [true, string] => need[0] === true)
    .map(([, label]) => label);
  const ready = missing.length === 0;
  const [attempted, setAttempted] = useState(false);
  const needed = (absent: boolean) =>
    attempted && absent ? t("firstRun.needed") : undefined;
  useEffect(() => {
    onBusy(save.isPending);
    return () => onBusy(false);
  }, [save.isPending, onBusy]);
  return (
    <>
      <OAuthAppSteps
        provider={vendor}
        redirects={<RedirectUris uris={app.data?.redirect_uris} />}
      >
        <Field
          label={t("oauthApp.clientId")}
          error={needed(clientId.trim() === "")}
        >
          {(control) => (
            <TextInput
              {...control}
              value={clientId}
              disabled={save.isPending}
              autoComplete="off"
              placeholder={t(vendorCopy[vendor].clientIdPlaceholder)}
              onChange={(e) => setClientId(e.target.value)}
            />
          )}
        </Field>
        <Field
          label={t("oauthApp.clientSecret")}
          error={needed(clientSecret.trim() === "")}
        >
          {(control) => (
            <TextInput
              {...control}
              type="password"
              autoComplete="off"
              value={clientSecret}
              disabled={save.isPending}
              onChange={(e) => setClientSecret(e.target.value)}
            />
          )}
        </Field>
        {vendor === "microsoft" && (
          <Field
            label={t("oauthApp.tenant")}
            // Required here, unlike Settings, so the hint says why: without a
            // pin there is no sign-in, and nothing else on screen says so.
            hint={`${t("firstRun.microsoft.tenantHint")} ${t("firstRun.microsoft.helpSignIn")}`}
            error={needed(tenant.trim() === "")}
          >
            {(control) => (
              <TextInput
                {...control}
                value={tenant}
                autoComplete="off"
                disabled={save.isPending}
                placeholder={t("oauthApp.tenantPlaceholder")}
                onChange={(e) => setTenant(e.target.value)}
              />
            )}
          </Field>
        )}
      </OAuthAppSteps>
      <WriteRefused titleKey="oauthApp.saveFailed" error={save.error} />
      <StageActions>
        <StepNeeds attempted={attempted} missing={missing} />
        <Button variant="ghost" onClick={onDecline} disabled={save.isPending}>
          {t("firstRun.platform.skip")}
        </Button>
        <Button
          variant="primary"
          pending={save.isPending}
          onClick={() => {
            if (!ready) {
              setAttempted(true);
              return;
            }
            save.reset();
            save.mutate(
              {
                clientId: clientId.trim(),
                clientSecret: clientSecret.trim(),
                tenant: tenant.trim(),
              },
              {
                onSuccess: () => {
                  // Cleared on the way out rather than left in state: the
                  // field is the only copy this app holds, and it has done
                  // its job. The invalidated setup report is what moves the
                  // screen on.
                  setClientSecret("");
                  save.reset();
                },
              },
            );
          }}
        >
          {t("firstRun.continue")}
        </Button>
      </StageActions>
    </>
  );
}

/**
 * What still stands between the reader and the next step, beside the button
 * they pressed. Nothing until Continue has been pressed with something missing.
 * A form that lists its gaps before anyone has typed scolds a reader for arriving.
 */
export function StepNeeds({
  attempted,
  missing,
}: Readonly<{ attempted: boolean; missing: readonly string[] }>) {
  const t = useT();
  if (!attempted || missing.length === 0) {
    return null;
  }
  return (
    <ErrorLine inline>
      {t("firstRun.stillNeeded", { fields: missing.join(", ") })}
    </ErrorLine>
  );
}

/**
 * IMAP has no company app. Its steps are what each mailbox owner does in
 * their own mail service before the mailbox form can sign in.
 */
function ImapSteps({
  onBusy,
  onDecline,
}: Readonly<{ onBusy: (busy: boolean) => void; onDecline: () => void }>) {
  const t = useT();
  return (
    <Checklist>
      <ChecklistStep
        state="open"
        title={t("firstRun.imap.passwordTitle")}
        caption={t("firstRun.imap.passwordBody")}
      />
      <ChecklistStep
        state="open"
        title={t("firstRun.imap.serverTitle")}
        caption={t("firstRun.imap.serverBody")}
      />
      <ChecklistStep
        state="open"
        title={t("firstRun.imap.connectTitle")}
        caption={t("firstRun.platform.imapNote")}
      >
        {/* The same standing connect Settings makes, for the admin on
            screen. There is no installation-wide IMAP app to store, so a
            confirmed mailbox and "not now" both answer the question. */}
        <ImapMailboxForm
          dismissLabel={t("firstRun.platform.skip")}
          onDismiss={onDecline}
          onConnected={onDecline}
          onPendingChange={onBusy}
          renderActions={(actions) => <StageActions>{actions}</StageActions>}
        />
      </ChecklistStep>
    </Checklist>
  );
}

/**
 * The platform step: which vendor's app mailboxes and calendars connect
 * through, asked once, of the admin running the cold start. IMAP is an answer
 * too, since each mailbox carries its own credentials. So is "not now": the step
 * does not block, and Settings keeps the same form.
 */
export function PlatformStep({
  onBusy,
  onDecline,
}: Readonly<{
  onBusy: (busy: boolean) => void;
  /** The reader passed on the question; the gate lets them through. */
  onDecline: () => void;
}>) {
  const t = useT();
  const [platform, setPlatform] = useState<Platform>("google");
  return (
    <div className="ob-fr-form">
      <Panel>
        <PanelBody>
          {/* Plates rather than a radio column: the answers are two vendors
              and a protocol, and an admin picks the platform their company
              already runs by recognising it. `imap` is a protocol, not a vendor,
              so it gets the component's own fallback mark rather than an
              invented logo. */}
          <ChoiceList<Platform>
            legend={t("firstRun.platform.legend")}
            hideLegend
            layout="cards"
            value={platform}
            onChange={setPlatform}
            choices={PLATFORMS.map((id) => ({
              value: id,
              label: t(PLATFORM_COPY[id].label),
              description: t(PLATFORM_COPY[id].what),
              mark: <ProviderMark providerKey={id} />,
            }))}
          />
          {platform === "imap" ? (
            <ImapSteps onBusy={onBusy} onDecline={onDecline} />
          ) : (
            <VendorAppFields
              key={platform}
              vendor={platform}
              onBusy={onBusy}
              onDecline={onDecline}
            />
          )}
        </PanelBody>
      </Panel>
    </div>
  );
}
