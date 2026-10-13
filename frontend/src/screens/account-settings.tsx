// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import { THEME_LABEL_KEYS } from "../app/account";
import { useInstallState } from "../app/pwa";
import { setThemeChoice, THEME_CHOICES, useThemeChoice } from "../app/theme";
import { useUnsavedGuard } from "../app/unsaved";
import {
  Avatar,
  Button,
  Field,
  Modal,
  SegmentedControl,
  Textarea,
  TextInput,
} from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody } from "../design-system/panel";
import { RoleBadge } from "../design-system/rbac";
import { Select } from "../design-system/select";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { type Toast, useToast } from "../design-system/toast";
import { LOCALES, type Locale, localeNameKey, useLocale, useT } from "../i18n";
import { BriefDeliveryRows } from "./briefdelivery";
import {
  problemMessageOf,
  QueryGate,
  throwProblem,
  useLogout,
  useMe,
  WriteRefused,
} from "./common";
import { PasswordSettingRow } from "./passwordcard";
import { InstallSettingRow } from "./thisdevice";
import "./account-settings.css";

// The Account page as task cards, each one job a reader comes here to do: how
// colleagues see you, how you sign in, the language that follows the account,
// and what this browser alone remembers. Every row keeps the page's one row
// language, naming on the left and the control on the right.
export function AccountCard() {
  // The page owns where an announcement lands, not the row that produced it: a
  // toast region between two rows would take the hairline the list draws.
  const toast = useToast();
  return (
    <>
      <ProfilePanel toast={toast} />
      <SignInPanel />
      <LanguagePanel />
      <BrowserPanel />
      <ScheduledEmailsPanel />
    </>
  );
}

function ProfilePanel({ toast }: Readonly<{ toast: Toast }>) {
  const t = useT();
  const query = useMe();
  return (
    <Panel title={t("account.profileCard")}>
      <PanelBody className="form-stack">
        <QueryGate query={query} pendingLabel={t("account.profileCard")}>
          {(me) => (
            <div className="settings-identity account-identity">
              {/* Both halves are required on the wire, so `|| ""` is the promise
                  that a server answering with neither costs an unnamed chip, not
                  the app shell this block renders inside. */}
              <Avatar
                size="lg"
                identity={me.user.id}
                name={me.user.display_name || me.user.email || ""}
              />
              <div className="settings-identity-id">
                <div className="settings-identity-name">
                  <strong className="account-identity-name">
                    {me.user.display_name || me.user.email}
                  </strong>
                  {me.roles.map((role) => (
                    <RoleBadge key={role} roleKey={role} />
                  ))}
                </div>
                {/* The address the session is bound to and the workspace it is
                    bound to it in, on one line; each its own node so either can
                    be read or matched alone. */}
                <span className="t-caption account-identity-meta">
                  <span>{me.user.email}</span>
                  <span>{me.workspace_name}</span>
                </span>
              </div>
            </div>
          )}
        </QueryGate>
        <SettingList>
          <DisplayNameSettingRow toast={toast} />
          <SignatureSettingRow toast={toast} />
        </SettingList>
      </PanelBody>
    </Panel>
  );
}

// The credential and the session, together: the password row is the one that
// ends every other session, and Sign out is the one that ends this one.
function SignInPanel() {
  const t = useT();
  const me = useMe();
  const logout = useLogout();
  const email = me.data?.user.email;
  return (
    <Panel title={t("account.signInCard")}>
      <PanelBody>
        <SettingList>
          <PasswordSettingRow
            description={
              email ? t("account.passwordHelp", { email }) : undefined
            }
          />
          <SettingRow
            label={t("auth.signOut")}
            description={t("account.signOutHelp")}
            control={
              <Button
                variant="ghost"
                disabled={logout.isPending}
                onClick={() => logout.mutate()}
              >
                {t("auth.signOut")}
              </Button>
            }
          />
        </SettingList>
      </PanelBody>
    </Panel>
  );
}

function LanguagePanel() {
  const t = useT();
  return (
    <Panel title={t("account.languageCard")}>
      <PanelBody>
        <SettingList>
          <LanguageSettingRow />
        </SettingList>
      </PanelBody>
    </Panel>
  );
}

// What this browser keeps and the account does not. The card is always drawn
// because Appearance always is; only the install row is absent where the
// browser cannot install.
function BrowserPanel() {
  const t = useT();
  const install = useInstallState();
  return (
    <Panel title={t("account.browserCard")}>
      <PanelBody>
        <SettingList>
          <AppearanceSettingRow />
          <InstallSettingRow install={install} />
        </SettingList>
      </PanelBody>
    </Panel>
  );
}

function ScheduledEmailsPanel() {
  const t = useT();
  return (
    <Panel title={t("account.emailsCard")}>
      <PanelBody>
        <SettingList>
          <BriefDeliveryRows />
        </SettingList>
      </PanelBody>
    </Panel>
  );
}

/**
 * The name colleagues see you by, edited in place.
 *
 * The saved answer is read back from `/me` rather than kept here, so the shell's
 * account chip and the roster agree with this row the moment it lands.
 */
function DisplayNameSettingRow({ toast }: Readonly<{ toast: Toast }>) {
  const t = useT();
  const me = useMe();
  const queryClient = useQueryClient();
  const stored = me.data?.user.display_name ?? "";
  // null means "not editing": a `/me` refetch cannot overwrite what the reader
  // is in the middle of typing.
  const [draft, setDraft] = useState<string | null>(null);
  const shown = draft ?? stored;
  const save = useMutation({
    mutationFn: async (next: string) => {
      const { data, error } = await api.PUT("/me/display-name", {
        body: { display_name: next },
      });
      if (error) {
        throwProblem(error, t);
      }
      return data;
    },
    onSuccess: (saved) => {
      // Keep the saved name visible if the account refetch is delayed or fails.
      setDraft(saved?.display_name ?? null);
      void queryClient.invalidateQueries({ queryKey: ["scheduling-profile"] });
      toast.show(t("settings.saved"));
      void queryClient.invalidateQueries({ queryKey: ["me"] });
    },
  });
  // Trimmed for the comparison as well as for the send, or a trailing space
  // reads as a change the server answers is not one.
  const trimmed = shown.trim();
  const dirty = trimmed !== stored;
  // Counted in characters, as the contract's `maxLength` and the server's
  // `utf8.RuneCountInString` do; `String.length` counts UTF-16 units.
  const tooLong = [...trimmed].length > 255;
  const refusal = save.error ? problemMessageOf(save.error, t) : undefined;
  return (
    <SettingRow
      label={t("settings.displayName")}
      description={t("settings.displayNameHelp")}
      control={(row) => (
        <form
          className="account-inline settingrow-measure"
          onSubmit={(event) => {
            event.preventDefault();
            if (dirty && trimmed !== "" && !tooLong && !save.isPending) {
              save.mutate(trimmed);
            }
          }}
        >
          <Field label={t("settings.displayName")} labelHidden error={refusal}>
            {(field) => (
              <TextInput
                {...field}
                aria-labelledby={row["aria-labelledby"]}
                aria-describedby={[field, row]
                  .map((owner) => owner["aria-describedby"])
                  .filter(Boolean)
                  .join(" ")}
                autoComplete="name"
                value={shown}
                // No `maxLength`: UTF-16 units, not the runes `tooLong` counts.
                onChange={(event) => setDraft(event.target.value)}
              />
            )}
          </Field>
          <Button
            type="submit"
            variant="ghost"
            disabled={!dirty || trimmed === "" || tooLong || save.isPending}
          >
            {t("settings.displayNameSave")}
          </Button>
        </form>
      )}
    />
  );
}

// The sign-off appended below every message this member sends. It sits with
// identity rather than under the composer because it is who the sender IS; a
// signature written per message would differ every time. Plain text, because
// the transport sends text/plain.
function SignatureSettingRow({ toast }: Readonly<{ toast: Toast }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const titleId = useId();
  const [open, setOpen] = useState(false);
  const [body, setBody] = useState<string | null>(null);
  const signature = useQuery({
    queryKey: ["me-email-signature"],
    queryFn: async () => {
      const { data, error } = await api.GET("/me/email-signature");
      if (error) {
        throwProblem(error);
      }
      return data ?? { body: "" };
    },
  });
  const save = useMutation({
    mutationFn: async (next: string) => {
      const { data, error } = await api.PUT("/me/email-signature", {
        body: { body: next },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (saved) => {
      // The server trims what it stores; handing the edit back to its answer
      // keeps Save from offering a difference that exists only in the browser.
      setBody(saved?.body ?? "");
      queryClient.invalidateQueries({ queryKey: ["me-email-signature"] });
      setOpen(false);
      toast.show(t("settings.saved"));
    },
  });

  // The saved value until the member types; theirs from then on, so a refetch
  // landing underneath them discards no keystroke.
  const stored = signature.data?.body ?? "";
  const shown = body ?? stored;
  const dirty = shown !== stored;
  // Only while the dialog is showing: it outlives its own close, and a draft the
  // reader walked away from must not go on blocking navigation.
  useUnsavedGuard(open && dirty);
  // The first line is the part a reader recognises their sign-off by. Nothing
  // is claimed about the signature until the read has answered.
  const firstLine = stored.split("\n")[0].trim();
  const empty = firstLine === "";
  const answer = signature.isPending
    ? undefined
    : empty
      ? t("settings.signatureNone")
      : firstLine;

  // Closing only closes, so the dialog animates out over the draft it showed;
  // the draft is discarded on the next open, when the reader asks for a form.
  const close = () => setOpen(false);
  const edit = () => {
    setBody(null);
    save.reset();
    setOpen(true);
  };

  return (
    <>
      <SettingRow
        label={t("settings.signature")}
        description={t("account.signatureHelp")}
        value={answer}
        control={
          <Button variant="ghost" onClick={edit}>
            {signature.isSuccess && empty
              ? t("account.signatureAdd")
              : t("settings.signatureEdit")}
          </Button>
        }
      />
      <Modal open={open} onClose={close} labelledBy={titleId}>
        <form
          className="form-stack"
          onSubmit={(event) => {
            event.preventDefault();
            if (dirty && !save.isPending) save.mutate(shown);
          }}
        >
          <Heading size="large" className="t-h3" id={titleId}>
            {t("settings.signature")}
          </Heading>
          <WriteRefused titleKey="settings.saveFailed" error={save.error} />
          <Field label={t("settings.signatureLabel")}>
            {(control) => (
              <Textarea
                {...control}
                rows={5}
                value={shown}
                placeholder={t("settings.signaturePlaceholder")}
                onChange={(event) => setBody(event.target.value)}
              />
            )}
          </Field>
          <p className="t-caption">{t("settings.signatureHint")}</p>
          <div className="form-actions">
            <Button variant="ghost" onClick={close}>
              {t("settings.signatureCancel")}
            </Button>
            <Button
              type="submit"
              variant="primary"
              disabled={!save.isPending && !dirty}
              pending={save.isPending}
              busyLabel={t("settings.signatureSaving")}
            >
              {t("record.save")}
            </Button>
          </div>
        </form>
      </Modal>
    </>
  );
}

function LanguageSettingRow() {
  const t = useT();
  const { locale, setLocale } = useLocale();
  const queryClient = useQueryClient();
  // Written to the seat so it follows this colleague to their next browser;
  // `setLocale` keeps the local copy that renders before the request lands.
  // A failed write does not revert: the page is already in the language asked
  // for, and the next sign-in re-asks the server.
  const remember = useMutation({
    mutationFn: async (next: Locale) => {
      const { error } = await api.PUT("/me/locale", { body: { locale: next } });
      if (error) {
        throwProblem(error, t);
      }
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["me"] });
    },
  });
  return (
    <SettingRow
      label={t("locale.switchLabel")}
      description={t("account.languageHelp")}
      control={(control) => (
        <Select
          {...control}
          className="settingrow-measure"
          value={locale}
          // Narrowed through the list the options were built from, so the
          // answer is a Locale with no assertion.
          onChange={(next) => {
            const picked = LOCALES.find((option) => option === next);
            if (picked) {
              setLocale(picked);
              remember.mutate(picked);
            }
          }}
          // Each language name is in its own language, so each carries `lang`
          // (WCAG 2.2 AA 3.1.2); a locale code is already a BCP 47 subtag.
          options={LOCALES.map((option) => ({
            value: option,
            label: t(localeNameKey(option)),
            lang: option,
          }))}
        />
      )}
    />
  );
}

/**
 * The appearance choice, here as well as in the account menu.
 *
 * One state, two faces: both read `useThemeChoice` and write `setThemeChoice`,
 * so changing it in either place moves the other.
 */
function AppearanceSettingRow() {
  const t = useT();
  const choice = useThemeChoice();
  const labels = {
    light: t(THEME_LABEL_KEYS.light),
    dark: t(THEME_LABEL_KEYS.dark),
    system: t(THEME_LABEL_KEYS.system),
  };
  return (
    <SettingRow
      label={t("settings.appearance")}
      description={t("account.appearanceHelp")}
      control={
        <SegmentedControl
          label={t("settings.appearance")}
          options={THEME_CHOICES}
          value={choice}
          labels={labels}
          onChange={setThemeChoice}
        />
      }
    />
  );
}
