// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Field } from "../../design-system/atoms";
import { Select } from "../../design-system/select";
import { providerName } from "../../format/connectorprovider";
import { useT } from "../../i18n";
import { sendingMailboxes, useConnectors } from "../../lib/connectors";

/** Which of the sender's mailboxes the mail goes out from. Shown only when
 *  more than one can send. The empty choice leaves it to the server: a reply
 *  goes out from the mailbox the conversation arrived in, a new mail from
 *  the default. */
export function FromMailboxRow({
  value,
  onChange,
}: Readonly<{ value: string; onChange: (connectionId: string) => void }>) {
  const t = useT();
  const connectors = useConnectors();
  const mailboxes = sendingMailboxes(connectors.data?.data ?? []);
  if (mailboxes.length < 2) {
    return null;
  }
  const options = [
    { value: "", label: t("compose.from.automatic") },
    ...mailboxes.map((mailbox) => ({
      value: mailbox.id,
      label: mailbox.account_label ?? providerName(mailbox.provider, t),
    })),
  ];
  return (
    <div className="compose-strip">
      <Field className="compose-strip-field" label={t("compose.from.label")}>
        {(control) => (
          <Select
            {...control}
            options={options}
            value={value}
            onChange={onChange}
            testId="compose-from-mailbox"
          />
        )}
      </Field>
    </div>
  );
}
