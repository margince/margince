import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Building2 } from "lucide-react";
import { type ReactNode, useState } from "react";
import { api } from "../api/client";
import { Badge, Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import {
  Panel,
  PanelBody,
  PanelIntro,
  PanelPlate,
} from "../design-system/panel";
import { QueryGate } from "../design-system/querystates";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { Switch } from "../design-system/switch";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { useCanWrite } from "../lib/capability";
import { problemMessageOf, throwProblem, unwrap } from "../lib/problem";
import { useMe } from "../lib/session";

// The workspace mail-sharing posture: ON by default, captured mail is
// readable by every colleague who can see the contact — the thing that makes
// the pipeline shared. Switching it OFF holds every email captured from then
// on to its participants, which makes shared CRM work hard, so the change is
// a deliberate act: a switch plus a Save button plus a warning that says the
// cost out loud, never a silent instant toggle. Every role sees the posture;
// only admin/ops may change it (same gating as the auto-enrich card).

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

type BandState = Readonly<{
  notice: ReactNode;
  error: unknown;
  dirty: boolean;
}>;

function bandShows({ notice, error, dirty }: BandState): boolean {
  return notice !== null || error !== undefined || dirty;
}

/**
 * The band under the rows: what the posture costs, what the write refused, and
 * the verb that commits it.
 *
 * All three belong to the CARD rather than to a row — an unsaved flip is a
 * state of the whole card, and a notice squeezed into a row's right column
 * would read as that switch's own answer. One component because this card
 * carries two of them, and two inline copies of the shape had already begun to
 * disagree about which of their notices interrupts.
 */
function CommitBand({
  notice,
  error,
  dirty,
  pending,
  onSave,
}: BandState & Readonly<{ pending: boolean; onSave: () => void }>) {
  const t = useT();
  if (!bandShows({ notice, error, dirty })) return null;
  return (
    <div className="settings-panel-commit">
      {notice}
      {error !== undefined && (
        <Callout
          kind="outcome"
          tone="danger"
          title={t("mailSharing.saveFailed")}
        >
          {problemMessageOf(error, t)}
        </Callout>
      )}
      {dirty && (
        <Button variant="primary" disabled={pending} onClick={onSave}>
          {t("mailSharing.save")}
        </Button>
      )}
    </div>
  );
}

export function MailSharingCard() {
  const t = useT();
  const toast = useToast();
  const canManage = useCanWrite("capture_settings", "update");
  const query = useMailSharing();
  const queryClient = useQueryClient();
  // null = no unsaved change; the switch renders the stored posture.
  const [pending, setPending] = useState<boolean | null>(null);
  // The two settings on this card are saved apart. They point in OPPOSITE
  // directions — one withholds mail from colleagues, the other permits a seat
  // to hand it over on arrival — so a single Save button committing both at
  // once would let a reader flip one while meaning the other.
  const [pendingShared, setPendingShared] = useState<boolean | null>(null);
  const save = useMutation({
    mutationFn: async (mailSharing: boolean) => {
      return unwrap(
        await api.PATCH("/capture/settings", {
          body: { mail_sharing: mailSharing },
        }),
      );
    },
    onSuccess: (data) => {
      queryClient.setQueryData(["capture-settings"], data);
      setPending(null);
      toast.show(t("settings.saved"));
    },
  });

  const saveShared = useMutation({
    mutationFn: async (allowed: boolean) => {
      return unwrap(
        await api.PATCH("/capture/settings", {
          body: { shared_posture_allowed: allowed },
        }),
      );
    },
    onSuccess: (data) => {
      queryClient.setQueryData(["capture-settings"], data);
      setPendingShared(null);
      toast.show(t("settings.saved"));
    },
  });

  return (
    <Panel title={t("mailSharing.title")}>
      <PanelBody>
        <PanelIntro>{t("mailSharing.sub")}</PanelIntro>
      </PanelBody>
      <QueryGate query={query} pendingLabel={t("mailSharing.title")}>
        {(settings) => {
          const shown = pending ?? settings.mail_sharing;
          const dirty = pending !== null && pending !== settings.mail_sharing;
          const sharedShown = pendingShared ?? settings.shared_posture_allowed;
          const sharedDirty =
            pendingShared !== null &&
            pendingShared !== settings.shared_posture_allowed;
          const sharedBand: BandState = {
            notice: sharedShown ? (
              <Callout
                kind="standing"
                tone="warning"
                title={t("mailSharing.sharedPosture.warningTitle")}
              >
                {t("mailSharing.sharedPosture.warning")}
              </Callout>
            ) : null,
            error: saveShared.isError ? saveShared.error : undefined,
            dirty: sharedDirty,
          };
          const sharingBand: BandState = {
            notice: shown ? null : (
              <Callout
                kind="standing"
                tone="warning"
                title={t("mailSharing.dangerTitle")}
              >
                {t("mailSharing.danger")}
              </Callout>
            ),
            error: save.isError ? save.error : undefined,
            dirty,
          };
          return (
            <>
              <SettingList bleed="settings">
                {/* The row draws the naming, so the switch carries the same
                    words as its hidden label rather than a second heading and
                    a hint of its own beside them. */}
                <SettingRow
                  label={t("mailSharing.label")}
                  description={t("mailSharing.help")}
                  // Without the row's description, a screen reader never
                  // hears what sharing does.
                  control={(control) => (
                    <Switch
                      describedBy={control["aria-describedby"]}
                      testId="mail-sharing-toggle"
                      label={t("mailSharing.label")}
                      labelHidden
                      reason={
                        canManage ? undefined : t("captureSettings.adminOnly")
                      }
                      checked={shown}
                      disabled={!canManage || save.isPending}
                      onChange={(next) => setPending(next)}
                    />
                  )}
                />
                {/* The one capture setting whose default withholds, so switching
                    it on is the permissive answer and the warning fires on true. */}
                <SettingRow
                  label={t("mailSharing.sharedPosture.label")}
                  description={t("mailSharing.sharedPosture.help")}
                  control={(control) => (
                    <Switch
                      describedBy={control["aria-describedby"]}
                      testId="shared-posture-allowed-toggle"
                      label={t("mailSharing.sharedPosture.label")}
                      labelHidden
                      reason={
                        canManage ? undefined : t("captureSettings.adminOnly")
                      }
                      checked={sharedShown}
                      disabled={!canManage || saveShared.isPending}
                      onChange={(next) => setPendingShared(next)}
                    />
                  )}
                />
              </SettingList>
              {(bandShows(sharedBand) || bandShows(sharingBand)) && (
                <PanelBody>
                  <CommitBand
                    {...sharedBand}
                    pending={saveShared.isPending}
                    onSave={() => {
                      if (pendingShared !== null) {
                        saveShared.mutate(pendingShared);
                      }
                    }}
                  />
                  <CommitBand
                    {...sharingBand}
                    pending={save.isPending}
                    onSave={() => {
                      if (pending !== null) {
                        save.mutate(pending);
                      }
                    }}
                  />
                </PanelBody>
              )}
            </>
          );
        }}
      </QueryGate>
    </Panel>
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
