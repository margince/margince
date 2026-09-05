import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId } from "react";
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

// How strict each posture is. Only a NARROWING has anything to offer history:
// opening what was captured under a stricter answer is a separate decision the
// server refuses to make as a side effect of this one.
const postureRank: Record<MailPosture, number> = {
  shared: 0,
  classified: 1,
  held: 2,
};

function useSetConnectPosture(provider: Provider) {
  const queryClient = useQueryClient();
  return useMutation({
    // The posture rides as a variable rather than closed over, for the reason
    // frontend/AGENTS.md gives: a mutationFn reading rendered state answers
    // with the previous render's.
    mutationFn: async (vars: {
      posture: MailPosture;
      applyToHistory: boolean;
    }) => {
      const { error } = await api.PUT("/connectors/{provider}/mail-posture", {
        params: { path: { provider } },
        body: {
          posture: vars.posture,
          apply_to_history: vars.applyToHistory,
        },
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
  onPendingChange,
}: Readonly<{
  provider: Provider;
  posture: MailPosture | undefined;
  /** Raised while a posture write is in flight, so a caller can hold back the
   *  controls that read history or leave the flow: a backread that starts
   *  mid-write imports under whichever answer wins the race. */
  onPendingChange?: (pending: boolean) => void;
}>) {
  const t = useT();
  const settings = useCaptureSettings();
  const save = useSetConnectPosture(provider);
  const labelId = useId();
  const helpId = useId();
  // The server's answer, always — never a local echo of the last successful
  // save. A save invalidates the connectors query, so the prop is what comes
  // back; holding a local copy beside it would keep showing this session's
  // choice after another session, a reconnect or a later refusal changed the
  // real value. Optimism is wrong here for the same reason: the one option
  // that can be refused (shared, 422 without the workspace opt-in) is the one
  // whose refusal a reader must see.
  const current = posture ?? "classified";

  useEffect(() => {
    onPendingChange?.(save.isPending);
  }, [save.isPending, onPendingChange]);
  const sharedAllowed = settings.data?.shared_posture_allowed ?? false;

  return (
    <div className="form-stack">
      {/* A visible label rather than an aria-label: the question is one a
          reader has to READ to answer, and Settings gives it the same words
          through SettingRow. */}
      <p className="t-caption" id={labelId}>
        {t("connectors.mailPosture.label")}
      </p>
      <Select
        aria-labelledby={labelId}
        aria-describedby={helpId}
        value={current}
        disabled={save.isPending}
        onChange={(next) => {
          const chosen = next as MailPosture;
          save.mutate({
            posture: chosen,
            applyToHistory: postureRank[chosen] > postureRank[current],
          });
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
      <p className="t-caption" id={helpId}>
        {t(`connectors.mailPosture.help.${current}` as MessageKey)}
        {sharedAllowed
          ? ""
          : ` ${t("connectors.mailPosture.sharedNeedsAdmin")}`}
      </p>
      {save.isError && (
        <p className="t-caption readfail warn" role="alert">
          {problemMessageOf(save.error, t)}
        </p>
      )}
    </div>
  );
}
