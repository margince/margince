// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { UseMutationResult } from "@tanstack/react-query";
import { useId, useState } from "react";
import type { components } from "../api/schema";
import {
  Button,
  Checkbox,
  Field,
  Modal,
  TextInput,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Heading } from "../design-system/heading";
import { Select } from "../design-system/select";
import { type Translator, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { labelFor } from "../lib/labelfor";
import { problemMessageOf } from "../lib/problem";

type ChannelLinkState = components["schemas"]["ChannelLinkState"];
type LinkChannelRequest = components["schemas"]["LinkChannelRequest"];
export type Verbosity = components["schemas"]["ChannelVerbosity"];

const VERBOSITY_LABEL: Readonly<Record<Verbosity, MessageKey>> = {
  everything: "channelLink.verbosity.everything",
  important: "channelLink.verbosity.important",
  milestones: "channelLink.verbosity.milestones",
  silent: "channelLink.verbosity.silent",
};
const VERBOSITIES: readonly Verbosity[] = [
  "everything",
  "important",
  "milestones",
  "silent",
];
const DEFAULT_VERBOSITY: Verbosity = "important";

export function isVerbosity(value: string): value is Verbosity {
  return Object.hasOwn(VERBOSITY_LABEL, value);
}

export function verbosityOptions(t: Translator) {
  return VERBOSITIES.map((value) => ({
    value,
    label: t(labelFor(VERBOSITY_LABEL, value)),
  }));
}

// The form that links a channel the team already talks in. The history summary
// is asked for by default, as the talk so far is what a new reader of the
// record lacks.
export function LinkExistingDialog({
  open,
  onClose,
  link,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  link: UseMutationResult<ChannelLinkState, Error, LinkChannelRequest>;
}>) {
  const t = useT();
  const headingId = useId();
  const formId = useId();
  const [channel, setChannel] = useState("");
  const [summarize, setSummarize] = useState(true);
  const [verbosity, setVerbosity] = useState<Verbosity>(DEFAULT_VERBOSITY);
  const ready = channel.trim() !== "";
  const failed = link.isError && link.variables?.mode === "existing";
  return (
    <Modal open={open} onClose={onClose} labelledBy={headingId} intent="form">
      <Heading size="large" id={headingId} className="t-h2 modal-title">
        {t("channelLink.linkExisting")}
      </Heading>
      <form
        id={formId}
        className="form-stack"
        onSubmit={(event) => {
          event.preventDefault();
          if (!ready || link.isPending) return;
          link.mutate(
            {
              mode: "existing",
              channel: channel.trim(),
              verbosity,
              summarize_history: summarize,
            },
            { onSuccess: onClose },
          );
        }}
      >
        <Field
          label={t("channelLink.channelField")}
          required
          hint={t("channelLink.channelHint")}
        >
          {(control) => (
            <TextInput
              {...control}
              autoComplete="off"
              value={channel}
              onChange={(event) => setChannel(event.target.value)}
            />
          )}
        </Field>
        <Field label={t("channelLink.verbosity")}>
          {(control) => (
            <Select
              id={control.id}
              aria-describedby={control["aria-describedby"]}
              options={verbosityOptions(t)}
              value={verbosity}
              onChange={(value) => {
                if (isVerbosity(value)) setVerbosity(value);
              }}
            />
          )}
        </Field>
        <Checkbox
          label={t("channelLink.summarize")}
          checked={summarize}
          onChange={(event) => setSummarize(event.target.checked)}
        />
        {failed && (
          <Callout
            tone="danger"
            kind="outcome"
            title={t("channelLink.linkFailed")}
          >
            {problemMessageOf(link.error, t)}
          </Callout>
        )}
      </form>
      <div className="actions">
        <Button type="button" onClick={onClose} disabled={link.isPending}>
          {t("create.cancel")}
        </Button>
        <Button
          variant="primary"
          type="submit"
          form={formId}
          disabled={!link.isPending && !ready}
          pending={link.isPending}
          busyLabel={t("create.saving")}
        >
          {t("channelLink.linkCta")}
        </Button>
      </div>
    </Modal>
  );
}
