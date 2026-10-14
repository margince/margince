import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Building2 } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Badge } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import {
  Panel,
  PanelBody,
  PanelIntro,
  PanelPlate,
} from "../design-system/panel";
import { PanelNotices } from "../design-system/panelnotices";
import { QueryGate } from "../design-system/querystates";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { Switch } from "../design-system/switch";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { useCanWrite } from "../lib/capability";
import { throwProblem, unwrap } from "../lib/problem";
import { useMe } from "../lib/session";

// The workspace mail-sharing posture: ON by default, so captured mail is shared
// with every colleague who can see the contact. Every role reads it; only
// admin/ops may change it, and each switch writes on flip and toasts.

type CaptureSettingsPatch =
  components["schemas"]["UpdateCaptureSettingsRequest"];

function useMailSharing() {
  return useQuery({
    queryKey: ["capture-settings"],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/capture/settings");
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

/**
 * One switch's write. Each PATCH carries only its own field: the two settings
 * point in opposite directions, so one write must never also rewrite the other.
 */
function usePostureWrite(bodyOf: (next: boolean) => CaptureSettingsPatch) {
  const t = useT();
  const toast = useToast();
  const queryClient = useQueryClient();
  const [optimistic, setOptimistic] = useState<boolean | null>(null);
  const write = useMutation({
    mutationFn: async (next: boolean) =>
      unwrap(await api.PATCH("/capture/settings", { body: bodyOf(next) })),
    onSuccess: (data) => {
      queryClient.setQueryData(["capture-settings"], data);
      toast.show(t("settings.saved"));
    },
    onSettled: () => setOptimistic(null),
  });
  const flip = (next: boolean) => {
    setOptimistic(next);
    write.mutate(next);
  };
  return { write, optimistic, flip };
}

function PostureSwitch({
  testId,
  label,
  description,
  checked,
  canManage,
  pending,
  onChange,
}: Readonly<{
  testId: string;
  label: string;
  description: string;
  checked: boolean;
  canManage: boolean;
  pending: boolean;
  onChange: (next: boolean) => void;
}>) {
  const t = useT();
  return (
    <SettingRow
      label={label}
      description={description}
      // Without the row's description, a screen reader never hears what the
      // switch does.
      control={(control) => (
        <Switch
          describedBy={control["aria-describedby"]}
          testId={testId}
          label={label}
          labelHidden
          reason={canManage ? undefined : t("captureSettings.adminOnly")}
          checked={checked}
          disabled={!canManage}
          pending={pending}
          onChange={onChange}
        />
      )}
    />
  );
}

export function MailSharingCard() {
  const t = useT();
  const canManage = useCanWrite("capture_settings", "update");
  const query = useMailSharing();
  const sharing = usePostureWrite((next) => ({ mail_sharing: next }));
  const shared = usePostureWrite((next) => ({ shared_posture_allowed: next }));
  // One card, one refusal: the switch flipped last is the one the reader means.
  const refused = [sharing.write, shared.write]
    .filter((write) => write.isError)
    .sort((a, b) => b.submittedAt - a.submittedAt)[0];

  return (
    <Panel title={t("mailSharing.title")}>
      <PanelBody>
        <PanelIntro>{t("mailSharing.sub")}</PanelIntro>
      </PanelBody>
      <QueryGate query={query} pendingLabel={t("mailSharing.title")}>
        {(settings) => {
          const sharingOn = sharing.optimistic ?? settings.mail_sharing;
          const sharedOn = shared.optimistic ?? settings.shared_posture_allowed;
          return (
            <>
              <SettingList bleed="settings">
                <PostureSwitch
                  testId="mail-sharing-toggle"
                  label={t("mailSharing.label")}
                  description={t("mailSharing.help")}
                  checked={sharingOn}
                  canManage={canManage}
                  pending={sharing.write.isPending}
                  onChange={sharing.flip}
                />
                {/* The one capture setting whose default withholds, so on is the
                    permissive answer and its warning fires on true. */}
                <PostureSwitch
                  testId="shared-posture-allowed-toggle"
                  label={t("mailSharing.sharedPosture.label")}
                  description={t("mailSharing.sharedPosture.help")}
                  checked={sharedOn}
                  canManage={canManage}
                  pending={shared.write.isPending}
                  onChange={shared.flip}
                />
              </SettingList>
              <PostureCosts sharingOn={sharingOn} sharedOn={sharedOn} />
            </>
          );
        }}
      </QueryGate>
      <PanelNotices
        refused={
          refused === undefined
            ? undefined
            : { title: t("mailSharing.saveFailed"), error: refused.error }
        }
      />
    </Panel>
  );
}

/** What the current posture costs, said for as long as it stands. */
function PostureCosts({
  sharingOn,
  sharedOn,
}: Readonly<{ sharingOn: boolean; sharedOn: boolean }>) {
  const t = useT();
  if (sharingOn && !sharedOn) {
    return null;
  }
  return (
    <PanelBody className="panel-notices">
      {sharedOn && (
        <Callout
          kind="standing"
          tone="warning"
          title={t("mailSharing.sharedPosture.warningTitle")}
        >
          {t("mailSharing.sharedPosture.warning")}
        </Callout>
      )}
      {!sharingOn && (
        <Callout
          kind="standing"
          tone="warning"
          title={t("mailSharing.dangerTitle")}
        >
          {t("mailSharing.danger")}
        </Callout>
      )}
    </PanelBody>
  );
}

/**
 * What the sharing rule currently SAYS, as the plate under the reader's own
 * Mail and calendar card.
 *
 * A value, not a control. The switch lives on Capture rules, because it writes
 * the WORKSPACE's posture, and the badge says so: this one plate on a page of
 * the reader's own accounts binds the whole company. It sits under the
 * mailboxes because it governs the mail they bring in, which is where a reader
 * wonders who else can read it.
 *
 * The link appears only for a reader who may CHANGE the rule. Every seeded role
 * may read the posture, so a visibility test sent a rep to a disabled switch.
 */
export function MailSharingPlate() {
  const t = useT();
  const query = useMailSharing();
  const canChangeIt = useCanWrite("capture_settings", "update");
  const company = useMe().data?.workspace_name ?? "";
  // Nothing until the read lands: a wrong statement about who can read
  // somebody's mail is worse than a moment with no statement.
  if (!query.isSuccess) {
    return null;
  }
  return (
    <PanelPlate>
      <Badge icon={Building2}>
        <span className="sr-only">
          {t("settings.scopeAria", { scope: t("mailSharing.plate.scope") })}
        </span>
        <span aria-hidden="true">{t("mailSharing.plate.scope")}</span>
      </Badge>
      <p>
        {t(
          query.data.mail_sharing
            ? "mailSharing.plate.shared"
            : "mailSharing.plate.private",
          { company },
        )}{" "}
        {canChangeIt && (
          <a href="#/settings/capture">{t("mailSharing.posture.where")}</a>
        )}
      </p>
    </PanelPlate>
  );
}
