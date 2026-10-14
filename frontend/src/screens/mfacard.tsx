// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import { Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { ErrorLine } from "../design-system/errorline";
import { SettingRow } from "../design-system/settingrow";
import { formatNumber } from "../format/format";
import { type Translator, useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { unwrap } from "../lib/problem";
import { type MfaStatus, MY_MFA_KEY, useMyMfa } from "./mfa.queries";
import { enteredCode, MfaCodeField } from "./mfacode";
import {
  codeRefusalOf,
  MfaSetupModal,
  mfaFailureMessage,
  refusalCopy,
} from "./mfasetup";

const LOW_RECOVERY_CODES = 2;

type Removal = Readonly<{ code: string; unfinished: boolean }>;

function useRowValue(status: ReturnType<typeof useMyMfa>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const mfa = status.data;
  if (status.isError) {
    return (
      <ErrorLine inline standing>
        {t("mfa.statusFailed")}
      </ErrorLine>
    );
  }
  if (!mfa) return undefined;
  if (mfa.confirmed) {
    return plural("mfa.on", mfa.recovery_codes_left, {
      count: formatNumber(mfa.recovery_codes_left, locale),
    });
  }
  return t(mfa.enrolled ? "mfa.pending" : "mfa.off");
}

function removalFailure(
  error: unknown,
  confirmedElsewhere: boolean,
  t: Translator,
): string | null {
  if (confirmedElsewhere) return t("mfa.onElsewhere");
  return error ? mfaFailureMessage(error, t) : null;
}

function RemoveModal({
  open,
  mfa,
  onClose,
  onRemoved,
  onStale,
}: Readonly<{
  open: boolean;
  mfa: MfaStatus | undefined;
  onClose: () => void;
  onRemoved: (removal: Removal) => void;
  onStale: () => void;
}>) {
  const t = useT();
  const [code, setCode] = useState("");
  const unfinished = mfa?.confirmed !== true;
  const entered = enteredCode(code);
  const remove = useMutation({
    gcTime: 0,
    mutationFn: async (removal: Removal) => {
      const result = await api.DELETE("/me/mfa", {
        body: { code: removal.code },
      });
      const refusal = codeRefusalOf(result.response.status);
      if (refusal === null) unwrap(result, t);
      return refusal;
    },
    onSuccess: (refusal, removal) => {
      if (refusal === "wrong-code") setCode("");
      // An unfinished setup needs no code, so a refusal means it was confirmed elsewhere.
      if (refusal && removal.unfinished) onStale();
      if (refusal) return;
      setCode("");
      onRemoved(removal);
    },
  });
  const confirmedElsewhere =
    remove.data === "wrong-code" && remove.variables?.unfinished === true;
  const refused = confirmedElsewhere ? null : refusalCopy(remove.data);
  const submit = () => {
    if (remove.isPending || (!unfinished && entered === "")) return;
    remove.mutate({ code: unfinished ? "" : entered, unfinished });
  };
  const close = () => {
    remove.reset();
    setCode("");
    onClose();
  };

  return (
    <ConfirmModal
      open={open}
      onClose={close}
      title={t(unfinished ? "mfa.discardTitle" : "mfa.removeTitle")}
      confirmLabel={t(unfinished ? "mfa.discard" : "mfa.removeConfirm")}
      confirmVariant="danger"
      confirmDisabled={!remove.isPending && !unfinished && entered === ""}
      pending={remove.isPending}
      error={removalFailure(remove.error, confirmedElsewhere, t)}
      onConfirm={submit}
    >
      <p>{t(unfinished ? "mfa.discardBody" : "mfa.removeBody")}</p>
      {!unfinished && (
        <form
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <MfaCodeField
            label={t("mfa.removeCode")}
            hint={t("mfa.removeCodeHint")}
            error={refused ? t(refused) : undefined}
            value={code}
            onChange={setCode}
          />
        </form>
      )}
    </ConfirmModal>
  );
}

/**
 * Two-step sign-in as one row of the account card. A pending enrolment is not
 * a factor yet, so it reads as unfinished and Set up starts it over.
 */
export function MfaSettingRow() {
  const t = useT();
  const queryClient = useQueryClient();
  const status = useMyMfa();
  const value = useRowValue(status);
  const [setupOpen, setSetupOpen] = useState(false);
  const [removeOpen, setRemoveOpen] = useState(false);
  const [outcome, setOutcome] = useState<MessageKey | null>(null);
  const start = useMutation({
    gcTime: 0,
    onMutate: () => setOutcome(null),
    mutationFn: async () => unwrap(await api.POST("/me/mfa/totp"), t),
  });
  const refresh = () => queryClient.invalidateQueries({ queryKey: MY_MFA_KEY });

  const mfa = status.data;
  const confirmed = mfa?.confirmed === true;
  const unfinished = mfa?.enrolled === true && !confirmed;
  const low = confirmed && mfa.recovery_codes_left <= LOW_RECOVERY_CODES;

  const openSetup = () => {
    setSetupOpen(true);
    start.mutate();
  };
  const closeSetup = () => {
    setSetupOpen(false);
    start.reset();
    void refresh();
  };
  const openRemove = () => {
    setOutcome(null);
    setRemoveOpen(true);
  };
  const removeButton = (
    <Button variant="ghost" onClick={openRemove}>
      {t("mfa.remove")}
    </Button>
  );

  return (
    <>
      <SettingRow
        label={t("mfa.title")}
        description={t("mfa.body")}
        value={value}
        testId="mfa-row"
        control={
          mfa && (
            <>
              {!confirmed && (
                <Button variant="ghost" onClick={openSetup}>
                  {t("mfa.setUp")}
                </Button>
              )}
              {(confirmed || unfinished) && removeButton}
            </>
          )
        }
      />
      {low && (
        <Callout kind="standing" tone="warning" title={t("mfa.lowTitle")}>
          {t("mfa.low")}
        </Callout>
      )}
      {outcome && <Callout kind="outcome" tone="success" title={t(outcome)} />}
      <MfaSetupModal
        open={setupOpen}
        enrolment={start.data}
        starting={start.isPending}
        startError={start.error}
        onRetry={() => start.mutate()}
        onClose={closeSetup}
      />
      <RemoveModal
        open={removeOpen}
        mfa={mfa}
        onClose={() => setRemoveOpen(false)}
        onStale={() => void refresh()}
        onRemoved={(removal) => {
          setRemoveOpen(false);
          setOutcome(removal.unfinished ? "mfa.discarded" : "mfa.removed");
          void refresh();
        }}
      />
    </>
  );
}
