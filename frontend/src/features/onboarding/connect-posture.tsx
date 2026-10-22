import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { useCaptureSettings } from "../../components/capture-settings";
import { type Choice, ChoiceList } from "../../design-system/choicelist";
import { ErrorLine } from "../../design-system/errorline";
import { useT } from "../../i18n/index";
import { problemMessageOf, unwrap } from "../../lib/problem";

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
      unwrap(
        await api.PUT("/connectors/{provider}/mail-posture", {
          params: { path: { provider } },
          body: {
            posture: vars.posture,
            apply_to_history: vars.applyToHistory,
          },
        }),
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["connectors"] });
    },
  });
}

/**
 * Who may read this mailbox's mail, chosen before its history is read, and
 * saved on the choice: there is no form to submit around it.
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
  // The server's answer, always, never a local echo of the last save. The one
  // option that can be refused (shared, 422 without the workspace opt-in) must
  // show its refusal, so optimism is wrong here.
  const current = posture ?? "classified";

  useEffect(() => {
    onPendingChange?.(save.isPending);
  }, [save.isPending, onPendingChange]);
  const sharedAllowed = settings.data?.shared_posture_allowed ?? false;
  const choices: Choice<MailPosture>[] = [
    {
      value: "classified",
      label: t("connectors.mailPosture.classified"),
      description: t("connectors.mailPosture.help.classified"),
    },
    {
      value: "held",
      label: t("connectors.mailPosture.held"),
      description: t("connectors.mailPosture.help.held"),
    },
    {
      value: "shared",
      label: t("connectors.mailPosture.shared"),
      description: sharedAllowed
        ? t("connectors.mailPosture.help.shared")
        : `${t("connectors.mailPosture.help.shared")} ${t("connectors.mailPosture.sharedNeedsAdmin")}`,
      // Present and refused rather than absent, for the reason the Settings
      // row gives: a missing option tells a reader their product has two.
      disabled: !sharedAllowed,
    },
  ];

  return (
    <div className="form-stack">
      <ChoiceList
        legend={t("connectors.mailPosture.label")}
        hideLegend
        choices={choices}
        value={current}
        disabled={save.isPending}
        onChange={(chosen) =>
          save.mutate({
            posture: chosen,
            applyToHistory: postureRank[chosen] > postureRank[current],
          })
        }
      />
      {save.isError && <ErrorLine>{problemMessageOf(save.error, t)}</ErrorLine>}
    </div>
  );
}
