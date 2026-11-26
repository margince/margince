import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import { Button, Field, Modal, TextInput } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { Heading } from "../../design-system/heading";
import { Panel } from "../../design-system/panel";
import { usePasswordReveal } from "../../design-system/passwordreveal";
import { useFieldFaults } from "../../design-system/savepage";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  blankRequired,
  ProblemError,
  problemCode,
  problemFieldErrors,
  problemMessageOf,
  unwrap,
} from "../../lib/problem";
import { passwordFault } from "./passwordrule";

// Changing your own password, from the account settings page.
//
// The product could set a password three ways before this — a reset token
// mailed to the account, an admin minting a set-password link for someone else,
// and an operator running the CLI against the database — and none of them was
// "I am signed in and I would like a different password". On an installation
// with no outbound email that left a member with no way to rotate their own
// credential at all.
//
// The current password is what authorizes the change, not the session. So this
// card asks for it, and the server verifies it: a session is what a stolen
// laptop already has.
//
// Three fields committed together is the settings page's modal case, not its row
// case: a row states one setting and its answer, and a credential rotation is a
// short form with a precondition, a rule and a confirm. So the row says what the
// setting IS and carries the verb, and the form is what the verb opens.

type ChangeFields = { current: string; next: string; confirm: string };

const EMPTY: ChangeFields = { current: "", next: "", confirm: "" };

// The server's details are English for API readers; this route's refusals,
// top-level or on the new password, get the reader's language.
const REFUSAL_COPY = new Map<string, MessageKey>([
  ["current_password_invalid", "password.currentWrong"],
  ["account_locked", "password.locked"],
  ["unchanged", "password.unchanged"],
  ["length", "password.length"],
  ["blank", "password.blank"],
  ["repeated", "password.repeated"],
  ["email", "password.email"],
]);

function refusalKey(error: unknown): MessageKey | undefined {
  if (!(error instanceof ProblemError)) return undefined;
  const codes = [
    problemCode(error.problem),
    ...problemFieldErrors(error.problem).map((fault) => fault.code),
  ];
  for (const code of codes) {
    const key = code === null ? undefined : REFUSAL_COPY.get(code);
    if (key) return key;
  }
  return undefined;
}

/**
 * The password setting as one row: what it is, and the verb that changes it.
 *
 * Exported beside the card so the account page can place this among its other
 * identity rows instead of giving a one-row card its own header band — the row
 * is the unit, and which card holds it is the page's business.
 */
export function PasswordSettingRow({
  onChanged,
  description,
}: Readonly<{
  // Called after a successful change. The settings page needs nothing — the
  // confirmation on the row is the whole outcome there. A caller that sent the
  // reader here BECAUSE of a refused credential uses it to re-probe, since the
  // change is what resolves the refusal.
  onChanged?: () => void;
  // What the row says the password is for; the account page names the address
  // it signs in.
  description?: string;
}> = {}) {
  const t = useT();
  const titleId = useId();
  const formId = useId();
  const [open, setOpen] = useState(false);
  const [fields, setFields] = useState<ChangeFields>(EMPTY);
  const { locale } = useLocale();
  const [faults, faultsRoot] = useFieldFaults();
  const [done, setDone] = useState(false);

  const queryClient = useQueryClient();
  const change = useMutation({
    // Cleared before the attempt, not after it: without this a second attempt
    // that fails renders the success line and the error line together, telling
    // the reader both that the password changed and that it did not.
    onMutate: () => setDone(false),
    // Takes what it needs as a variable rather than closing over render state:
    // the click belongs to the committed render, so a variable it passes cannot
    // be older than the control that carried it.
    mutationFn: async (values: ChangeFields) => {
      // The server's problem says whether the current password was wrong or the
      // new one was refused; a generic message would throw that away.
      unwrap(
        await api.POST("/auth/change-password", {
          body: {
            current_password: values.current,
            new_password: values.next,
          },
        }),
        t,
      );
    },
    onSuccess: async () => {
      setFields(EMPTY);
      setDone(true);
      // The dialog closes on success and the confirmation is left on the row
      // behind it: the outcome is a sentence, not a form with nothing left to
      // type into.
      setOpen(false);
      // The server revoked every credential that existed before the change and
      // set the cookie for the fresh session it minted, so the reader stays
      // signed in HERE and is signed out everywhere else. The identity probe
      // is re-asked rather than the cache dropped: what it answers may have
      // changed — a forced rotation is lifted by this very call — and nothing
      // else cached belongs to a session that ended.
      await queryClient.invalidateQueries({ queryKey: ["me"] });
      onChanged?.();
    },
  });

  const refusalCopy = (error: unknown) => {
    const key = refusalKey(error);
    return key
      ? t(key)
      : problemMessageOf(error, t, t("password.errorGeneric"));
  };

  const set = (key: keyof ChangeFields) => (value: string) => {
    setFields((current) => ({ ...current, [key]: value }));
    faults.edited(key);
  };

  // One per field, because they toggle independently: a reader checking what
  // they typed in the confirm box has no reason to expose the new password
  // above it at the same time.
  const revealLabels = {
    show: t("auth.showPassword"),
    hide: t("auth.hidePassword"),
  };
  const current = usePasswordReveal(revealLabels);
  const next = usePasswordReveal(revealLabels);
  const confirm = usePasswordReveal(revealLabels);

  const fault = passwordFault(fields.next);
  const mismatch = fields.confirm.length > 0 && fields.confirm !== fields.next;
  // Each empty field says it is required. A new password the rule refuses, or
  // a confirm that differs, already says so; the press takes focus there.
  const faultsAtSubmit = () => {
    const found = blankRequired(locale, {
      current: [t("password.current"), fields.current],
      next: [t("password.next"), fields.next],
      confirm: [t("password.confirm"), fields.confirm],
    });
    if (fault && !found.has("next")) found.set("next", t(fault));
    if (fields.confirm !== fields.next && !found.has("confirm")) {
      found.set("confirm", t("password.mismatch"));
    }
    return found;
  };

  // Leaving the dialog discards what was typed, which is the honest outcome for
  // a credential: nothing here is a draft worth restoring, and holding a typed
  // password in state after the reader closed the form keeps it around for no
  // one's benefit.
  const close = () => {
    setFields(EMPTY);
    faults.show(new Map());
    setOpen(false);
  };

  return (
    <>
      <SettingRow
        label={t("password.title")}
        description={description ?? t("password.body")}
        control={
          <Button variant="ghost" onClick={() => setOpen(true)}>
            {t("password.open")}
          </Button>
        }
      />
      {/* The outcome lands on the ROW, under it, because by the time it is true
          the dialog is gone. A Callout in its own tone, not a grey line: a
          refused change and a successful one were the same colour once, one
          element apart, and the only thing telling them apart was reading the
          sentence. */}
      {done && (
        <Callout kind="outcome" tone="success" title={t("password.doneTitle")}>
          {t("account.passwordDone")}
        </Callout>
      )}
      <Modal open={open} onClose={close} labelledBy={titleId} intent="form">
        <Heading size="large" className="t-h2 modal-title" id={titleId}>
          {t("password.title")}
        </Heading>
        {/* A real form, so Enter in any of the three fields submits it. */}
        <form
          id={formId}
          ref={faultsRoot}
          className="form-stack"
          noValidate
          onSubmit={(event) => {
            event.preventDefault();
            if (!faults.show(faultsAtSubmit()) && !change.isPending) {
              change.mutate(fields);
            }
          }}
        >
          {/* Said before the fields, because the change ends more than the
              reader's other sessions: the server revokes every grant and
              passport issued for them too. */}
          <Callout
            kind="standing"
            tone="warning"
            title={t("account.passwordWarnTitle")}
          >
            {t("account.passwordWarnBody")}
          </Callout>
          <DialogRefusal title={t("password.changeFailedTitle")} message={change.isError ? refusalCopy(change.error) : null} />
          {/* Every password field carries a reveal, this one included. A
                mistyped CURRENT password is the cheapest of the three to
                diagnose — the server refuses it in one round trip — but being
                refused without being able to see what you typed is how a reader
                concludes they have forgotten a password they know. */}
          <Field
            label={t("password.current")}
            required
            error={faults.of("current")}
            trailing={current.trailing}
          >
            {(control) => (
              <TextInput
                {...control}
                type={current.type}
                name="current-password"
                autoComplete="current-password"
                value={fields.current}
                onChange={(event) => set("current")(event.target.value)}
              />
            )}
          </Field>
          {/* The new pair has the strongest claim on it: a mistyped current
                password is refused, while a mistyped NEW one simply becomes the
                password — with a twelve-character floor and a confirm field that
                agreed with it. */}
          <Field
            label={t("password.next")}
            required
            error={fault ? t(fault) : faults.of("next")}
            // The rule, until the rule is being broken — at which point the
            // refusal restates it in the danger tone and a second grey copy of
            // the same sentence underneath is noise.
            hint={fault ? undefined : t("password.hint")}
            trailing={next.trailing}
          >
            {(control) => (
              <TextInput
                {...control}
                type={next.type}
                name="new-password"
                autoComplete="new-password"
                value={fields.next}
                onChange={(event) => set("next")(event.target.value)}
              />
            )}
          </Field>
          <Field
            label={t("password.confirm")}
            required
            error={mismatch ? t("password.mismatch") : faults.of("confirm")}
            trailing={confirm.trailing}
          >
            {(control) => (
              <TextInput
                {...control}
                type={confirm.type}
                name="confirm-password"
                autoComplete="new-password"
                value={fields.confirm}
                onChange={(event) => set("confirm")(event.target.value)}
              />
            )}
          </Field>
        </form>
        <div className="actions">
          <Button variant="ghost" onClick={close}>
            {t("password.cancel")}
          </Button>
          {/* Never disabled: a press on an unfinished form says what is
                missing at the field, and a write on its way shows as pending. */}
          <Button
            type="submit"
            form={formId}
            variant="primary"
            pending={change.isPending}
            busyLabel={t("password.changing")}
          >
            {t("password.submit")}
          </Button>
        </div>
      </Modal>
    </>
  );
}

/**
 * The same setting as a card of its own, for a page that has nothing else to
 * put beside it.
 */
export function ChangePasswordCard({
  onChanged,
}: Readonly<{ onChanged?: () => void }> = {}) {
  const t = useT();
  return (
    <Panel title={t("password.title")}>
      <SettingList bleed="settings">
        <PasswordSettingRow onChanged={onChanged} />
      </SettingList>
    </Panel>
  );
}
