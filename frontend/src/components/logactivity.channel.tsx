// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Field } from "../design-system/atoms";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import type { ActivityDraft } from "../lib/activitybody";
import { useChannelProviderOptions } from "../lib/channelproviders";

type ChannelOption = Readonly<{ value: string; label: string }>;

/**
 * The registered transports a draft may name, the one it names, and whether a
 * message still lacks one. With one registered transport it is the answer, so
 * the member is not asked to pick it.
 */
export function useMessageChannel(
  draft: Pick<ActivityDraft, "kind" | "channelProvider">,
) {
  const options = useChannelProviderOptions();
  const channel = pickedChannel(draft.channelProvider, options);
  return {
    options,
    channel,
    missing: draft.kind === "message" && channel === "",
  };
}

function pickedChannel(
  picked: string,
  options: readonly ChannelOption[],
): string {
  if (picked !== "" || options.length !== 1) return picked;
  return options[0]?.value ?? "";
}

/** Which registered chat app carried the message. */
export function MessageChannelField({
  active,
  options,
  value,
  onChange,
}: Readonly<{
  // Drawn for a message only; every other kind names no transport.
  active: boolean;
  options: readonly ChannelOption[];
  value: string;
  onChange: (provider: string) => void;
}>) {
  const t = useT();
  if (!active) return null;
  return (
    <Field label={t("log.channel")} required>
      {(control) => (
        <Select
          {...control}
          options={options}
          value={value}
          placeholder={t("log.channelPlaceholder")}
          onChange={onChange}
        />
      )}
    </Field>
  );
}
