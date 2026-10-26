// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CalendarDays, LogIn, Mail } from "lucide-react";
import { useId, useRef, useState } from "react";
import { Button, Field, TextInput } from "../design-system/atoms";
import { Checklist, ChecklistStep } from "../design-system/checklist";
import { ConfirmModal } from "../design-system/confirmmodal";
import {
  ConfirmPhraseBox,
  phraseMatches,
} from "../design-system/confirmphrase";
import { describedByIds } from "../design-system/describedby";
import {
  DrawerBody,
  DrawerFoot,
  DrawerHead,
} from "../design-system/drawerbands";
import { Heading } from "../design-system/heading";
import { Modal } from "../design-system/modal";
import { OffsiteLink } from "../design-system/offsitelink";
import { providerBrandName } from "../design-system/provider-mark";
import { QueryGate, WriteRefused } from "../design-system/querystates";
import { Chip } from "../design-system/readings";
import { useT } from "../i18n";
import { useCanWrite } from "../lib/capability";
import { useCompanyPhrase } from "../lib/company-phrase";
import {
  useOAuthApp,
  useRemoveOAuthApp,
  useSetOAuthApp,
  VENDOR_CONSOLE,
  type Vendor,
  vendorAppState,
  vendorCopy,
} from "../lib/oauth-app";
import { problemMessageOf } from "../lib/problem";
import "./oauth-app.css";
import { RedirectUris } from "./oauth-redirects";

// The drawer's per-vendor wording, as literal keys for the reason vendorCopy
// spells its own that way.
const drawerCopy = {
  google: {
    title: "oauthApp.drawer.google.title",
    intro: "oauthApp.drawer.google.intro",
    value: "oauthApp.drawer.google.value",
    step1Title: "oauthApp.drawer.google.step1Title",
    step1Body: "oauthApp.drawer.google.step1Body",
    step1Link: "oauthApp.drawer.google.step1Link",
    step2Body: "oauthApp.drawer.google.step2Body",
    unlockMail: "oauthApp.unlock.gmail",
    save: "oauthApp.google.save",
    removeTitle: "oauthApp.google.removeConfirmTitle",
    removeSecret: "oauthApp.google.removeSecret",
    removeUsedBy: "oauthApp.google.removeUsedBy",
    removeOthers: "oauthApp.google.removeOthers",
    removeSignIn: "oauthApp.google.removeSignIn",
  },
  microsoft: {
    title: "oauthApp.drawer.microsoft.title",
    intro: "oauthApp.drawer.microsoft.intro",
    value: "oauthApp.drawer.microsoft.value",
    step1Title: "oauthApp.drawer.microsoft.step1Title",
    step1Body: "oauthApp.drawer.microsoft.step1Body",
    step1Link: "oauthApp.drawer.microsoft.step1Link",
    step2Body: "oauthApp.drawer.microsoft.step2Body",
    unlockMail: "oauthApp.unlock.outlook",
    save: "oauthApp.microsoft.save",
    removeTitle: "oauthApp.microsoft.removeConfirmTitle",
    removeSecret: "oauthApp.microsoft.removeSecret",
    removeUsedBy: "oauthApp.microsoft.removeUsedBy",
    removeOthers: "oauthApp.microsoft.removeOthers",
    removeSignIn: "oauthApp.microsoft.removeSignIn",
  },
} as const;

/**
 * Setting up one vendor's app in three steps: create the client in the
 * vendor's console, register the redirect addresses, paste the pair. Every
 * step stays open, because the product cannot see the vendor's console.
 *
 * One form and one mount serve first setup and replacement. Settings opens it
 * from the vendor's row; onboarding opens it for an admin whose company has
 * no app yet.
 */
export function OAuthAppDrawer({
  provider,
  open,
  onClose,
}: Readonly<{ provider: Vendor; open: boolean; onClose: () => void }>) {
  const t = useT();
  const titleId = useId();
  const copy = drawerCopy[provider];
  const save = useSetOAuthApp(provider);
  // Resetting drops the mutation's `variables`, which hold the secret.
  function finish() {
    save.reset();
    onClose();
  }
  // Held while the PUT is out: a drawer that closed mid-save would land the app
  // with nobody left to see whether it was accepted.
  function dismiss() {
    if (!save.isPending) {
      finish();
    }
  }
  return (
    <Modal
      open={open}
      onClose={dismiss}
      closeReason={save.isPending ? t("common.saving") : undefined}
      labelledBy={titleId}
      intent="drawer-reading"
    >
      <DrawerHead>
        <Heading size="large" id={titleId} className="t-h2 modal-title">
          {t(copy.title)}
        </Heading>
        <p className="t-caption">{t(copy.intro)}</p>
      </DrawerHead>
      {/* Mounted only while the drawer is, so a typed secret never outlives
          it: closing without saving drops the draft. */}
      <SetupForm
        provider={provider}
        save={save}
        onCancel={dismiss}
        onSaved={finish}
      />
    </Modal>
  );
}

function SetupForm({
  provider,
  save,
  onCancel,
  onSaved,
}: Readonly<{
  provider: Vendor;
  save: ReturnType<typeof useSetOAuthApp>;
  onCancel: () => void;
  onSaved: () => void;
}>) {
  const t = useT();
  const copy = drawerCopy[provider];
  const company = useCompanyPhrase();
  const canManage = useCanWrite("capture_settings", "update");
  const app = useOAuthApp(provider);
  // Both `undefined` until edited. A stored app opens with its own client id
  // and directory in place, so rotating a secret never widens a pinned app.
  // An app supplied by this installation's configuration is not ours to carry
  // over, so replacing it starts from blank.
  const [clientId, setClientId] = useState<string | undefined>(undefined);
  const [tenant, setTenant] = useState<string | undefined>(undefined);
  const [clientSecret, setClientSecret] = useState("");
  const status = app.data;
  // The installation's own app refuses every value, so the form only explains.
  const state = status ? vendorAppState(status, provider, t) : undefined;
  const lockReason = state?.lock.disabled ? state.lock.reason : undefined;
  const lockId = useId();
  const id =
    clientId ?? (state?.origin === "stored" ? (status?.client_id ?? "") : "");
  const directory = tenant ?? status?.tenant ?? "";
  const ready =
    status !== undefined &&
    lockReason === undefined &&
    id.trim() !== "" &&
    clientSecret.trim() !== "";
  const vendor = providerBrandName(provider) ?? provider;
  const locked = !canManage || save.isPending || lockReason !== undefined;

  function submit() {
    save.mutate(
      {
        clientId: id.trim(),
        clientSecret: clientSecret.trim(),
        tenant: directory.trim(),
      },
      {
        onSuccess: () => {
          // The field was the only copy of the secret, and it has done its job.
          setClientSecret("");
          onSaved();
        },
      },
    );
  }

  return (
    <>
      <DrawerBody className="oauthapp-drawer-body">
        <p className="t-body">{t(copy.value, { company })}</p>
        <div className="oauthapp-unlocks">
          <Chip icon={Mail}>{t(copy.unlockMail)}</Chip>
          <Chip icon={CalendarDays}>{t("oauthApp.unlock.calendar")}</Chip>
          <Chip icon={LogIn}>{t("oauthApp.unlock.signIn")}</Chip>
        </div>
        <Checklist>
          <ChecklistStep
            state="open"
            title={t(copy.step1Title)}
            caption={t(copy.step1Body)}
          >
            <OffsiteLink href={VENDOR_CONSOLE[provider]}>
              {t(copy.step1Link)}
            </OffsiteLink>
          </ChecklistStep>
          <ChecklistStep
            state="open"
            title={t("oauthApp.drawer.step2Title")}
            caption={t(copy.step2Body)}
          >
            <QueryGate query={app} pendingLabel={t("oauthApp.redirectTable")}>
              {(loaded) => <RedirectUris uris={loaded.redirect_uris} />}
            </QueryGate>
          </ChecklistStep>
          <ChecklistStep
            state="open"
            title={t("oauthApp.drawer.step3Title")}
            caption={t("oauthApp.drawer.step3Body", { vendor })}
          >
            <div className="form-stack">
              {lockReason !== undefined && (
                <p id={lockId} className="t-caption">
                  {lockReason}
                </p>
              )}
              <Field label={t("oauthApp.clientId")}>
                {(control) => (
                  <TextInput
                    {...control}
                    aria-describedby={describedByIds(
                      control["aria-describedby"],
                      lockReason === undefined ? undefined : lockId,
                    )}
                    value={id}
                    autoComplete="off"
                    disabled={locked}
                    placeholder={t(vendorCopy[provider].clientIdPlaceholder)}
                    onChange={(e) => setClientId(e.target.value)}
                  />
                )}
              </Field>
              <Field
                label={t("oauthApp.clientSecret")}
                hint={t("oauthApp.drawer.secretHint")}
              >
                {(control) => (
                  <TextInput
                    {...control}
                    aria-describedby={describedByIds(
                      control["aria-describedby"],
                      lockReason === undefined ? undefined : lockId,
                    )}
                    type="password"
                    autoComplete="off"
                    value={clientSecret}
                    disabled={locked}
                    onChange={(e) => setClientSecret(e.target.value)}
                  />
                )}
              </Field>
              {/* Google has no directories, and a field that silently does
                  nothing is worse than none. */}
              {provider === "microsoft" && (
                <Field
                  label={t("oauthApp.tenant")}
                  hint={t("oauthApp.tenantHint")}
                >
                  {(control) => (
                    <TextInput
                      {...control}
                      aria-describedby={describedByIds(
                        control["aria-describedby"],
                        lockReason === undefined ? undefined : lockId,
                      )}
                      value={directory}
                      autoComplete="off"
                      disabled={locked}
                      placeholder={t("oauthApp.tenantPlaceholder")}
                      onChange={(e) => setTenant(e.target.value)}
                    />
                  )}
                </Field>
              )}
            </div>
          </ChecklistStep>
        </Checklist>
        {/* Last in the body, so a refusal lands beside the Save that drew it. */}
        <WriteRefused titleKey="oauthApp.saveFailed" error={save.error} />
      </DrawerBody>
      <DrawerFoot>
        <span className="t-caption">{t("oauthApp.drawer.foot")}</span>
        <div className="form-actions">
          <Button onClick={onCancel} disabled={save.isPending}>
            {t("create.cancel")}
          </Button>
          <Button
            variant="primary"
            pending={save.isPending}
            reason={canManage ? undefined : t("oauthApp.noWriteReason")}
            disabled={!ready}
            onClick={submit}
          >
            {t(copy.save)}
          </Button>
        </div>
      </DrawerFoot>
    </>
  );
}

/**
 * The redirect addresses of an app that is already set up, on their own.
 * An operator copies from this table when a connection fails with a mismatch.
 */
export function RedirectAddressesDialog({
  provider,
  open,
  onClose,
}: Readonly<{ provider: Vendor; open: boolean; onClose: () => void }>) {
  const t = useT();
  const titleId = useId();
  const app = useOAuthApp(provider);
  return (
    <Modal
      open={open}
      onClose={onClose}
      labelledBy={titleId}
      intent="drawer-reading"
    >
      <Heading size="large" id={titleId} className="t-h2 modal-title">
        {t("oauthApp.redirectTable")}
      </Heading>
      <p className="t-body">{t(drawerCopy[provider].step2Body)}</p>
      <QueryGate query={app} pendingLabel={t("oauthApp.redirectTable")}>
        {(loaded) => <RedirectUris uris={loaded.redirect_uris} />}
      </QueryGate>
      <div className="actions">
        <Button onClick={onClose}>{t("common.close")}</Button>
      </div>
    </Modal>
  );
}

/**
 * Removing a stored app. Typed, because the secret cannot be read back and
 * every mailbox on this vendor connects through it.
 */
export function RemoveOAuthAppDialog({
  provider,
  open,
  onClose,
  returnFocusTo,
}: Readonly<{
  provider: Vendor;
  open: boolean;
  onClose: () => void;
  /** The trigger lives in a row that changes once the app is gone. */
  returnFocusTo?: () => HTMLElement | null;
}>) {
  const t = useT();
  const copy = drawerCopy[provider];
  const remove = useRemoveOAuthApp(provider);
  const [typed, setTyped] = useState("");
  const phraseInput = useRef<HTMLInputElement>(null);
  const phrase = t("confirmPhrase.risk");
  function close() {
    setTyped("");
    remove.reset();
    onClose();
  }
  return (
    <ConfirmModal
      open={open}
      onClose={close}
      title={t(copy.removeTitle)}
      confirmLabel={t("oauthApp.remove")}
      confirmVariant="danger"
      confirmDisabled={!phraseMatches(typed, phrase)}
      pending={remove.isPending}
      // A refused DELETE leaves the dialog open, so its reason is said inside it.
      error={remove.error ? problemMessageOf(remove.error, t) : null}
      initialFocusTo={() => phraseInput.current}
      returnFocusTo={returnFocusTo}
      onConfirm={() => remove.mutate(undefined, { onSuccess: close })}
    >
      <ConfirmPhraseBox
        title={t("oauthApp.removeImpactTitle")}
        impact={[
          t(copy.removeSecret),
          t(copy.removeUsedBy),
          t(copy.removeOthers),
          t("oauthApp.removeFirstRun"),
          t(copy.removeSignIn),
        ]}
        prompt={t("confirmPhrase.riskPrompt")}
        phrase={phrase}
        value={typed}
        onChange={setTyped}
        inputRef={phraseInput}
        disabled={remove.isPending}
      />
    </ConfirmModal>
  );
}
