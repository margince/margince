import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Callout } from "../design-system/callout";
import { Panel, PanelBody } from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { Switch } from "../design-system/switch";
import { useToast } from "../design-system/toast";
import { type Translator, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, QueryGate, throwProblem } from "./common";

// Which kinds of proposal answer themselves, for the reader and nobody else.
//
// It sits on the account tab rather than beside the admin cards because the
// answer is one person's: an admin does not decide how much of a rep's queue
// applies without asking, so there is no role gating here and no refusal copy.
// The only thing that can stop a row is a write already in flight.
//
// The contract calls this `autonomy`; the copy calls it what a rep would. The
// hook and the types keep the contract's word so a reader of this file can find
// the endpoint, and only the visible strings change register.

type KindAutonomy = components["schemas"]["KindAutonomy"];

function useAutonomy() {
  return useQuery({
    queryKey: ["autonomy"],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/autonomy");
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

function useUpdateAutonomy() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const t = useT();
  return useMutation({
    // Kind and mode travel together as one variable, never read off a render:
    // the row that carries the switch is the row that names the kind, and a
    // handler reaching back for either could act on the previous render's.
    mutationFn: async (choice: { kind: string; auto: boolean }) => {
      const { data, error } = await api.PATCH("/autonomy", { body: choice });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    // The server answers with the whole set, so the cache takes its word rather
    // than patching one row locally: the decision counts beside the switch move
    // as decisions are made elsewhere, and a hand-patched row would show a
    // stale record beside a fresh switch.
    onSuccess: (data) => {
      queryClient.setQueryData(["autonomy"], data);
      toast.show(t("settings.saved"));
    },
  });
}

// decidedSoFar is the track record under one switch, or nothing when the reader
// has never met this kind.
//
// Absent rather than a row of zeroes: "you have approved 0 of these unchanged"
// invites the reader to weigh evidence that does not exist, where saying nothing
// leaves the switch to be judged on the description above it.
function decidedSoFar(row: KindAutonomy, t: Translator): string {
  const decided = row.approved_clean + row.approved_edited + row.rejected;
  if (decided === 0) {
    return t("autonomy.noRecord");
  }
  return t("autonomy.record", {
    clean: String(row.approved_clean),
    edited: String(row.approved_edited),
    rejected: String(row.rejected),
  });
}

// KIND_COPY names the label and the description for each kind this catalog has
// words for. Spelled out rather than composed from the kind at the call site,
// because `t` takes a key from a closed union — a computed key does not compile,
// and the compiler refusing one is the reason this map is honest about which
// kinds it covers.
//
// A kind absent here still renders. The eligible set is a property of what the
// product can put back and moves as reversibility does, so a server offering a
// fourth kind before its copy lands should cost the reader an unpolished row,
// never a choice they have but cannot see.
const KIND_COPY: Readonly<
  Record<string, { label: MessageKey; help: MessageKey }>
> = {
  close_date_correction: {
    label: "autonomy.kind.close_date_correction.label",
    help: "autonomy.kind.close_date_correction.help",
  },
  org_name_promotion: {
    label: "autonomy.kind.org_name_promotion.label",
    help: "autonomy.kind.org_name_promotion.help",
  },
  lifecycle_change: {
    label: "autonomy.kind.lifecycle_change.label",
    help: "autonomy.kind.lifecycle_change.help",
  },
};

// kindLabel is what the row calls this kind — its own words where the catalog
// has them, and the contract's spelling where it does not.
function kindLabel(kind: string, t: Translator): string {
  const copy = KIND_COPY[kind];
  return copy ? t(copy.label) : kind;
}

// kindHelp is the sentence under the label, or nothing for a kind this catalog
// does not know. Empty rather than a placeholder: a row whose description is a
// raw key reads as broken, where a row with only a name reads as terse.
function kindHelp(kind: string, t: Translator): string {
  const copy = KIND_COPY[kind];
  return copy ? t(copy.help) : "";
}

export function AutonomySettingsCard() {
  const t = useT();
  const query = useAutonomy();
  const update = useUpdateAutonomy();

  return (
    <Panel title={t("autonomy.title")}>
      <PanelBody className="form-stack">
        <p className="settings-panel-sub">{t("autonomy.sub")}</p>
        <QueryGate query={query}>
          {(settings) => (
            <SettingList>
              {settings.data.map((row) => (
                <SettingRow
                  key={row.kind}
                  label={kindLabel(row.kind, t)}
                  description={[kindHelp(row.kind, t), decidedSoFar(row, t)]
                    .filter(Boolean)
                    .join(" ")}
                  control={
                    <Switch
                      testId={`autonomy-toggle-${row.kind}`}
                      label={kindLabel(row.kind, t)}
                      labelHidden
                      checked={row.mode === "auto"}
                      disabled={update.isPending}
                      onChange={(next) =>
                        update.mutate({ kind: row.kind, auto: next })
                      }
                    />
                  }
                />
              ))}
            </SettingList>
          )}
        </QueryGate>
        {update.isError && (
          <Callout tone="danger" live="alert">
            {problemMessageOf(update.error, t)}
          </Callout>
        )}
      </PanelBody>
    </Panel>
  );
}
