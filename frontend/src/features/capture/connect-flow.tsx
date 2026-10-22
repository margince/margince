// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Mail } from "lucide-react";
import { type ReactNode, useEffect, useId, useRef, useState } from "react";
import type { components } from "../../api/schema";
import { ActionRow } from "../../design-system/actionrow";
import { Button, Field, Modal } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { Checklist, ChecklistStep } from "../../design-system/checklist";
import { type Choice, ChoiceList } from "../../design-system/choicelist";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { ProviderMark } from "../../design-system/provider-mark";
import { Select } from "../../design-system/select";
import { ServiceList, ServiceRow } from "../../design-system/servicerow";
import {
  ACCOUNT_NAME,
  type AccountVendor,
} from "../../format/connectorprovider";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  notConfiguredProvider,
  useConnectAccount,
} from "../../lib/connect-account";
import {
  type Availability,
  accountUnwired,
  connectInputFor,
  type Draft,
  draftFor,
  FLOW_ACCOUNTS,
  type FlowAccount,
  type ImportPick,
  openServices,
  type Service,
  type Step,
} from "../../lib/connect-flow-draft";
import { SERVICE_ICON, serviceChoices } from "../../lib/connect-services";
import {
  IMPORT_WINDOW_LABELS,
  isImportWindow,
} from "../../mail-history/window-picker";

type Provider = components["schemas"]["CaptureConnection"]["provider"];

const ACCOUNT_DESCRIPTION: Record<FlowAccount, MessageKey> = {
  google: "connectFlow.google",
  microsoft: "connectFlow.microsoft",
  imap: "connectFlow.otherHostDesc",
};

/**
 * Connecting mail and calendar, in three steps. First comes which account, then
 * what it brings in, then the hand-off to the vendor's own sign-in. The server
 * takes one consent per service, so a vendor account asks twice, chained
 * server-side in one round trip that lands back on Connections.
 *
 * `startFrom` opens on step 2 for a reader who already chose the vendor.
 * Another mail host has no step 3 here. Continue hands over to the IMAP form,
 * which the caller owns because Reconnect opens the same one.
 */
export function ConnectFlow({
  open,
  onClose,
  onImap,
  startFrom,
  connected = [],
  availability = [],
  returnFocusTo,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  /** Opens the IMAP credentials form once this dialog has closed. */
  onImap: () => void;
  startFrom?: FlowAccount;
  /** The providers on the roster, which this flow does not offer again. */
  connected?: readonly Provider[];
  /** Whether each mail provider would connect right now. */
  availability?: readonly Availability[];
  /** Where focus lands on close, when the verb that opened it has moved. */
  returnFocusTo?: () => HTMLElement | null;
}>) {
  const t = useT();
  const headingId = useId();
  const connect = useConnectAccount();
  const [draft, setDraft] = useState(() =>
    draftFor(startFrom ?? "", connected),
  );
  // Every opening starts from the caller's pick, never from where the last
  // visit stopped: a half-finished choice from earlier is not this reader's.
  const [wasOpen, setWasOpen] = useState(open);
  if (open !== wasOpen) {
    setWasOpen(open);
    if (open) {
      setDraft(draftFor(startFrom ?? "", connected));
    }
  }
  const close = () => {
    connect.reset();
    onClose();
  };
  const { step, account } = draft;
  // Each step swaps the controls out from under the press that moved it, so
  // focus goes to the new title. An opening is the dialog's own to place.
  const headingRef = useRef<HTMLHeadingElement>(null);
  const seen = useRef({ open, step });
  useEffect(() => {
    const before = seen.current;
    seen.current = { open, step };
    if (open && before.open && before.step !== step) {
      headingRef.current?.focus();
    }
  }, [open, step]);
  const update = (next: Partial<Draft>) => setDraft({ ...draft, ...next });
  const back = (to: Step) => {
    connect.reset();
    update({ step: to });
  };

  let body: ReactNode = null;
  if (step === 1) {
    body = (
      <AccountStep
        value={account}
        connected={connected}
        availability={availability}
        onChange={(picked) => update({ account: picked })}
        onCancel={close}
        onNext={(picked) =>
          setDraft({
            ...draftFor(picked, connected),
            importPick: draft.importPick,
          })
        }
      />
    );
  } else if (step === 2 && account !== "") {
    body = (
      <BringInStep
        account={account}
        draft={draft}
        connected={connected}
        onChange={update}
        onBack={() => back(1)}
        onNext={() => {
          if (account === "imap") {
            close();
            onImap();
          } else {
            update({ step: 3 });
          }
        }}
      />
    );
  } else if (account !== "" && account !== "imap") {
    body = (
      <SignInStep
        vendor={account}
        draft={draft}
        connect={connect}
        onBack={() => back(2)}
      />
    );
  }

  return (
    <Modal
      open={open}
      onClose={close}
      labelledBy={headingId}
      intent="form"
      returnFocusTo={returnFocusTo}
    >
      <Heading
        size="large"
        id={headingId}
        ref={headingRef}
        tabIndex={-1}
        className="t-h2 modal-title"
      >
        {flowTitle(draft, t)}
      </Heading>
      <div className="form-stack">
        <StepTrail step={step} />
        {body}
      </div>
    </Modal>
  );
}

const STEP_LABELS: readonly MessageKey[] = [
  "connectFlow.stepAccount",
  "connectFlow.stepBringIn",
  "connectFlow.stepSignIn",
];

/** Where the reader is in the three steps. A trail rather than a control: the
 *  steps are walked with Back and Continue, so none of it is pressable. */
function StepTrail({ step }: Readonly<{ step: Step }>) {
  const t = useT();
  return (
    <Checklist variant="inline" label={t("connectFlow.progress")}>
      {STEP_LABELS.map((key, index) => {
        const number = index + 1;
        let state: "done" | "current" | "open" = "open";
        if (number < step) state = "done";
        else if (number === step) state = "current";
        return <ChecklistStep key={key} state={state} title={t(key)} />;
      })}
    </Checklist>
  );
}

/** The dialog names the account once it is chosen, and the vendor whose
 *  sign-in comes next on the last step. */
function flowTitle(draft: Draft, t: ReturnType<typeof useT>): string {
  const { step, account } = draft;
  if (step === 1 || account === "") {
    return t("connectFlow.title");
  }
  if (account === "imap") {
    return t("connectFlow.titleImap");
  }
  const vendor = t(ACCOUNT_NAME[account]);
  return step === 2
    ? t("connectFlow.titleVendor", { vendor })
    : t("connectFlow.signInTitle", { vendor });
}

function AccountStep({
  value,
  connected,
  availability,
  onChange,
  onCancel,
  onNext,
}: Readonly<{
  value: FlowAccount | "";
  connected: readonly Provider[];
  availability: readonly Availability[];
  onChange: (next: FlowAccount) => void;
  onCancel: () => void;
  onNext: (picked: FlowAccount) => void;
}>) {
  const t = useT();
  const choices = FLOW_ACCOUNTS.map((account) =>
    accountChoice(account, connected, availability, t),
  );
  return (
    <>
      <ChoiceList
        legend={t("connectFlow.where")}
        layout="cards"
        choices={choices}
        value={value}
        onChange={onChange}
      />
      <ActionRow
        primary={
          <Button
            variant="primary"
            reason={value === "" ? t("connectFlow.pickAccount") : undefined}
            onClick={() => value !== "" && onNext(value)}
          >
            {t("connectFlow.next")}
          </Button>
        }
      >
        <Button onClick={onCancel}>{t("create.cancel")}</Button>
      </ActionRow>
    </>
  );
}

/** One account as a plate, refused with its reason when this installation
 *  cannot connect it or the roster already holds all it brings. */
function accountChoice(
  account: FlowAccount,
  connected: readonly Provider[],
  availability: readonly Availability[],
  t: ReturnType<typeof useT>,
): Choice<FlowAccount> {
  const label =
    account === "imap" ? t("connectFlow.otherHost") : t(ACCOUNT_NAME[account]);
  const unwired = accountUnwired(account, availability);
  const full = openServices(account, connected).length === 0;
  let description = t(ACCOUNT_DESCRIPTION[account]);
  if (unwired) {
    description = t("connectors.providerNotConfigured", { provider: label });
  } else if (full) {
    description = t("connectFlow.alreadyConnected");
  }
  return {
    value: account,
    label,
    description,
    disabled: unwired || full,
    mark:
      account === "imap" ? <Mail /> : <ProviderMark providerKey={account} />,
  };
}

function BringInStep({
  account,
  draft,
  connected,
  onChange,
  onBack,
  onNext,
}: Readonly<{
  account: FlowAccount;
  draft: Draft;
  connected: readonly Provider[];
  onChange: (next: Partial<Draft>) => void;
  onBack: () => void;
  onNext: () => void;
}>) {
  const t = useT();
  const imap = account === "imap";
  const choices = serviceChoices(t, { account, connected });
  const mail = draft.services.includes("mail");
  return (
    <>
      <ChoiceList
        multiple
        legend={t("connectFlow.stepBringIn")}
        hideLegend
        layout="cards"
        choices={choices}
        value={draft.services}
        onChange={(services) => onChange({ services })}
      />
      {mail && imap && (
        <p className="t-caption">{t("backfill.unsupportedNote")}</p>
      )}
      {mail && !imap && (
        <ImportPicker
          value={draft.importPick}
          onChange={(importPick) => onChange({ importPick })}
        />
      )}
      <ActionRow
        primary={
          <Button
            variant="primary"
            reason={
              draft.services.length === 0
                ? t("connectFlow.pickService")
                : undefined
            }
            onClick={onNext}
          >
            {t("connectFlow.next")}
          </Button>
        }
      >
        <Button onClick={onBack}>{t("connectFlow.back")}</Button>
      </ActionRow>
    </>
  );
}

/**
 * How far back to import, or not at all. Not ImportWindowPicker: that one picks
 * a window for a mailbox that exists and shows its preview. Here no mailbox
 * exists yet, and None is an answer.
 */
function ImportPicker({
  value,
  onChange,
}: Readonly<{
  value: ImportPick;
  onChange: (next: ImportPick) => void;
}>) {
  const t = useT();
  const windows = Object.entries(IMPORT_WINDOW_LABELS).map(([window, key]) => ({
    value: window,
    label: t(key),
  }));
  return (
    <Field
      label={t("connectFlow.importLabel")}
      hint={t("connectFlow.importAfter")}
    >
      {(control) => (
        <Select
          {...control}
          value={value}
          options={[
            { value: "none", label: t("connectFlow.importNone") },
            ...windows,
          ]}
          onChange={(next) => {
            if (next === "none" || isImportWindow(next)) onChange(next);
          }}
        />
      )}
    </Field>
  );
}

const PERMISSIONS = [
  {
    service: "mail",
    name: "connectFlow.permMail",
    purpose: "connectFlow.permMailFor",
  },
  {
    service: "calendar",
    name: "connectFlow.permCalendar",
    purpose: "connectFlow.permCalendarFor",
  },
] as const satisfies readonly Readonly<{
  service: Service;
  name: MessageKey;
  purpose: MessageKey;
}>[];

/** The hand-off: what the vendor is about to ask, so its consent screens
 *  read as expected rather than as a surprise, then the one press that
 *  leaves for them. */
function SignInStep({
  vendor,
  draft,
  connect,
  onBack,
}: Readonly<{
  vendor: AccountVendor;
  draft: Draft;
  connect: ReturnType<typeof useConnectAccount>;
  onBack: () => void;
}>) {
  const t = useT();
  const vendorName = t(ACCOUNT_NAME[vendor]);
  const { services } = draft;
  const both = services.includes("mail") && services.includes("calendar");
  let after: MessageKey = "connectFlow.afterCalendar";
  if (services.includes("mail")) {
    after = both ? "connectFlow.afterBoth" : "connectFlow.afterMail";
  }
  const input = connectInputFor(vendor, services, draft.importPick);
  // A redirect leaves the page, so the press stays busy until it does.
  const leaving = connect.isPending || connect.data?.kind === "redirecting";
  return (
    <>
      <p>{t("connectFlow.handoff", { vendor: vendorName })}</p>
      <ServiceList label={t("connectFlow.permsLabel", { vendor: vendorName })}>
        {PERMISSIONS.filter((perm) => services.includes(perm.service)).map(
          ({ service, name, purpose }) => (
            <ServiceRow
              key={service}
              icon={SERVICE_ICON[service]}
              name={t(name)}
              caption={t(purpose)}
            />
          ),
        )}
      </ServiceList>
      {both && (
        <Callout title={t("connectFlow.asksTwice", { vendor: vendorName })} />
      )}
      <p className="t-caption">{t(after)}</p>
      {notConfiguredProvider(connect.data) && (
        <Callout
          tone="warning"
          kind="outcome"
          title={t("connectors.providerNotConfigured", {
            provider: vendorName,
          })}
        />
      )}
      <ErrorLine error={connect.error} />
      <ActionRow
        primary={
          <Button
            variant="primary"
            pending={leaving}
            onClick={() => input && connect.mutate(input)}
          >
            {t("connectFlow.continueTo", { vendor: vendorName })}
          </Button>
        }
      >
        <Button onClick={onBack} disabled={leaving}>
          {t("connectFlow.back")}
        </Button>
      </ActionRow>
    </>
  );
}
