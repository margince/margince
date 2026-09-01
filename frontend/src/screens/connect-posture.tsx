import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useCaptureSettings } from "./capture-settings";
import { problemMessageOf, throwProblem } from "./common";

// The posture question, asked while a mailbox is being connected.
//
// Settings already carries this decision (connectors.tsx MailPostureRow), and
// this is deliberately NOT that control moved: the two ask at different moments
// and owe the reader different things. Settings changes a posture a mailbox has
// been running under, so it must offer to narrow the mail already captured.
// Here there is no history — the connection was granted seconds ago and the
// backread has not run — so there is nothing to apply to, and a control that
// asked would be offering to narrow an empty set.
//
// What they DO share is the vocabulary: the same three postures, the same
// labels, the same help sentences, the same admin refusal on `shared`. Those
// live in i18n under connectors.mailPosture.* and are read from there by both,
// because two spellings of "held until classified" would drift into two
// different promises about the same column.
//
// It changes nothing on its own. A connection is born `classified` from the
// column default, so a reader who ignores this control entirely gets the safe
// answer — the same one they would get if this component did not exist. Its
// whole job is to let somebody say `held` before the backread reads a year of
// mail, rather than after.

type MailPosture = NonNullable<
  components["schemas"]["CaptureConnection"]["mail_posture"]
>;

type Provider = components["schemas"]["CaptureConnection"]["provider"];

function useSetConnectPosture(provider: Provider) {
  const queryClient = useQueryClient();
  return useMutation({
    // The posture rides as a variable rather than closed over, for the reason
    // frontend/AGENTS.md gives: a mutationFn reading rendered state answers
    // with the previous render's.
    mutationFn: async (vars: { posture: MailPosture }) => {
      const { error } = await api.PUT("/connectors/{provider}/mail-posture", {
        params: { path: { provider } },
        // Never true here, and not a variable: there is no captured mail to
        // narrow at connect time. Sending the flag would ask the server to walk
        // a set that is empty by construction.
        body: { posture: vars.posture, apply_to_history: false },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["connectors"] });
    },
  });
}

/**
 * Who may read this mailbox's mail, chosen before its history is read.
 *
 * `classified` is preselected because it is what the server already wrote; the
 * control reports the connection's own value rather than a local default, so a
 * failed save leaves the screen agreeing with the database instead of showing
 * an answer nobody stored.
 */
export function ConnectPostureStep({
  provider,
  posture,
}: Readonly<{ provider: Provider; posture: MailPosture | undefined }>) {
  const t = useT();
  const settings = useCaptureSettings();
  const save = useSetConnectPosture(provider);
  // What the server says this connection is, until a save of our own succeeds.
  // Optimism here would be a lie a reader acts on: the one option that can be
  // refused (shared, 422 when the workspace has not allowed it) is exactly the
  // one whose refusal matters.
  const [saved, setSaved] = useState<MailPosture | null>(null);
  const labelId = useId();
  const helpId = useId();
  const current = saved ?? posture ?? "classified";
  const sharedAllowed = settings.data?.shared_posture_allowed ?? false;

  return (
    <div className="form-stack">
      {/* A visible label rather than an aria-label: the question is one a
          reader has to READ to answer, and Settings gives it the same words
          through SettingRow. */}
      <p className="t-small" id={labelId}>
        {t("connectors.mailPosture.label")}
      </p>
      <Select
        aria-labelledby={labelId}
        aria-describedby={helpId}
        value={current}
        disabled={save.isPending}
        onChange={(next) => {
          const chosen = next as MailPosture;
          save.mutate(
            { posture: chosen },
            { onSuccess: () => setSaved(chosen) },
          );
        }}
        options={[
          {
            value: "classified",
            label: t("connectors.mailPosture.classified"),
          },
          { value: "held", label: t("connectors.mailPosture.held") },
          {
            value: "shared",
            label: t("connectors.mailPosture.shared"),
            // Present and refused rather than absent, for the reason the
            // Settings row gives: a missing option tells a reader their
            // product has two postures.
            disabled: !sharedAllowed,
          },
        ]}
      />
      <p className="t-small" id={helpId}>
        {t(`connectors.mailPosture.help.${current}` as MessageKey)}
        {sharedAllowed
          ? ""
          : ` ${t("connectors.mailPosture.sharedNeedsAdmin")}`}
      </p>
      {save.isError && (
        <p className="t-small readfail warn" role="alert">
          {problemMessageOf(save.error, t)}
        </p>
      )}
    </div>
  );
}
