// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import { Button, Field, Modal, TextInput } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { Heading } from "../../design-system/heading";
import { OffsiteLink } from "../../design-system/offsitelink";
import { QueryStates } from "../../design-system/querystates";
import { useT } from "../../i18n";
import { useCanMutate } from "../../lib/capability";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { useMessengerSetup } from "./messenger.queries";

export function DispactParticipantConnect() {
  const t = useT();
  const setup = useMessengerSetup("dispact");
  const canMutate = useCanMutate();
  const [open, Open] = useState(false);
  return (
    <QueryStates query={setup} pendingLabel={t("messenger.loading")}>
      {setup.data?.configured ? (
        <>
          <p>{t("messenger.dispactConsent")}</p>
          <Button disabled={!canMutate} onClick={() => Open(true)}>
            {t("messenger.connect")}
          </Button>
          {open && (
            <DispactCredentialForm
              accountUrl={setup.data.account_url}
              onClose={() => Open(false)}
            />
          )}
        </>
      ) : (
        <p>{t("messenger.appMissing")}</p>
      )}
      {!canMutate && <p>{t("settings.seatRefused")}</p>}
    </QueryStates>
  );
}

function DispactCredentialForm({
  accountUrl,
  onClose,
}: Readonly<{ accountUrl: string | undefined; onClose: () => void }>) {
  const t = useT();
  const heading = useId();
  const [token, Token] = useState("");
  const [failure, Failure] = useState<unknown>(null);
  const client = useQueryClient();
  const connect = useMutation({
    gcTime: 0,
    mutationFn: async ({
      personalToken,
    }: Readonly<{ personalToken: string }>) =>
      unwrap(
        await api.POST("/messenger/dispact/credential", {
          body: { personal_token: personalToken },
        }),
      ),
    onSuccess: async () => {
      Token("");
      await client.invalidateQueries({ queryKey: ["messenger-identity"] });
      onClose();
    },
    onError: Failure,
    onSettled: (): void => connect.reset(),
  });
  return (
    <Modal
      open
      intent="form"
      labelledBy={heading}
      onClose={onClose}
      closeDisabled={connect.isPending}
    >
      <Heading id={heading} size="large" className="modal-title">
        {t("messenger.dispactTokenTitle")}
      </Heading>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          Failure(null);
          connect.mutate({ personalToken: token });
        }}
      >
        <div className="form-stack">
          <p>{t("messenger.dispactTokenHelp")}</p>
          {accountUrl && (
            <OffsiteLink href={accountUrl}>
              {t("messenger.dispactOpen")}
            </OffsiteLink>
          )}
          <Field label={t("messenger.dispactToken")}>
            {(control) => (
              <TextInput
                {...control}
                type="password"
                autoComplete="off"
                required
                pattern="pat_.+"
                maxLength={8192}
                value={token}
                onChange={(event) => Token(event.target.value)}
                disabled={connect.isPending}
              />
            )}
          </Field>
          {failure !== null && (
            <Callout tone="warning" title={t("messenger.connectProblem")}>
              {problemMessageOf(failure, t)}
            </Callout>
          )}
        </div>
        <div className="actions">
          <Button onClick={onClose} disabled={connect.isPending}>
            {t("common.close")}
          </Button>
          <Button type="submit" variant="primary" pending={connect.isPending}>
            {t("messenger.connect")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
