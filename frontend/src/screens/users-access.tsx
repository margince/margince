import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Trash2 } from "lucide-react";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import {
  Button,
  EmptyState,
  Field,
  Modal,
  TextInput,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Panel, PanelBody } from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { useToast } from "../design-system/toast";
import { stable } from "../format/collate";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, QueryGate, throwProblem } from "./common";
import { RosterPartialNote, useRoster, useRosterPartial } from "./entityref";
import "./users-access.css";

// What a seat will see, said by the server. The invite form asks before the
// invite goes out; the answer is the evaluated policy — the same grants,
// masks and read classes the gates read — so the screen never interprets a
// role a second way.

type AccessPreview = components["schemas"]["AccessPreview"];
type Role = components["schemas"]["AccessPreviewRequest"]["role"];

// The objects worth a line in the preview: the record kinds a rep works.
const PREVIEW_OBJECTS = [
  "person",
  "organization",
  "lead",
  "deal",
  "project",
] as const;

function useAccessPreview(role: Role, teamIds: string[]) {
  return useQuery({
    // `stable`, not the reader's collation: this is a cache key, so the same
    // set of teams has to spell the same key wherever it is read.
    queryKey: ["access-preview", role, [...teamIds].sort(stable).join(",")],
    queryFn: async (): Promise<AccessPreview> => {
      const { data, error } = await api.POST("/users/access-preview", {
        body: { role, team_ids: teamIds },
      });
      if (error) throwProblem(error);
      return data;
    },
  });
}

export function AccessPreviewPanel({
  role,
  teamIds,
}: Readonly<{ role: Role; teamIds: string[] }>) {
  const t = useT();
  const preview = useAccessPreview(role, teamIds);
  return (
    <div className="users-access-preview" aria-live="polite">
      <p className="t-caption">{t("users.access.title")}</p>
      <QueryGate query={preview}>
        {(access) => <AccessSummary access={access} />}
      </QueryGate>
    </div>
  );
}

function AccessSummary({ access }: Readonly<{ access: AccessPreview }>) {
  const t = useT();
  const verbs = (object: string): string => {
    const grant = access.objects?.[object];
    if (!grant?.read) {
      return t("users.access.none");
    }
    const parts = [t("users.access.read")];
    if (grant.create || grant.update) parts.push(t("users.access.write"));
    if (grant.delete) parts.push(t("users.access.delete"));
    return parts.join(" · ");
  };
  const teams = (access.teams ?? []).map((team) => team.name).join(", ");
  return (
    <ul className="t-small users-access-list">
      <li>{t("users.access.identity")}</li>
      <li>
        {access.row_scope === "all"
          ? t("users.access.writesAll")
          : access.row_scope === "team"
            ? teams
              ? t("users.access.writesTeam", { teams })
              : t("users.access.writesTeamNone")
            : t("users.access.writesOwn")}
      </li>
      {PREVIEW_OBJECTS.map((object) => (
        <li key={object}>
          {t(`users.access.object.${object}` satisfies MessageKey)}:{" "}
          {verbs(object)}
        </li>
      ))}
      {(access.field_masks ?? []).map((mask) => (
        <li key={`${mask.object}.${mask.field}`}>
          {t("users.access.mask", {
            field: `${mask.object}.${mask.field}`,
            when:
              mask.condition === "always"
                ? t("users.access.maskAlways")
                : t("users.access.maskOutside"),
          })}
        </li>
      ))}
    </ul>
  );
}

// The name a team confirmation says, off the roster this card already holds.
// A team whose row has gone before the lookup runs falls back to its id rather
// than to an empty quote, which reads as a team with no name at all.
// Taken off the hook's own result rather than exported from `entityref`: the
// roster's element type is that hook's to state, and a second name for it here
// is a second thing to keep in step.
type RosterEntry = NonNullable<ReturnType<typeof useRoster>["data"]>[number];

function teamName(
  entries: readonly RosterEntry[] | undefined,
  id: string,
): string {
  const found = entries?.find((entry) => entry.id === id);
  // The roster is a union of people and teams under one cache key, so the
  // narrowing is real rather than ceremonial — a `name` is what makes it a team.
  return found && "name" in found ? found.name : id;
}

// The teams card: the workspace's teams, and the verbs that change them.
// Membership is what resolves who may EDIT whose records now that customer
// identity is readable by every seat, so this is where that is administered.

export function TeamsCard() {
  const t = useT();
  const toast = useToast();
  const plural = usePlural();
  const { locale } = useLocale();
  const qc = useQueryClient();
  // The shared roster read, not a second query of this card's own. Both spell
  // the same list under the same cache key, so whichever mounted first decided
  // what the other one read back — and only one of the two follows the
  // endpoint's cursor to the end.
  const teams = useRoster("team", true);
  const teamsPartial = useRosterPartial("team", true);
  // The one archive in this product with a way back: a team is archived by
  // PATCHing a flag, so the same endpoint restores it. Everything else called
  // "archive" here is a DELETE the contract offers no inverse for.
  // It takes the NAME as well as the id, rather than looking the name up again
  // on the way back. The archive that offered this Undo invalidated ["teams"],
  // so by the time a reader presses it the archived row may already be gone
  // from the roster — and a lookup that misses falls back to a uuid, which is
  // the one thing a confirmation must not call a team.
  const restore = useMutation({
    mutationFn: async ({ id }: { id: string; name: string }) => {
      const { error } = await api.PATCH("/teams/{id}", {
        params: { path: { id } },
        body: { archived: false },
      });
      if (error) throwProblem(error);
    },
    // An Undo that fails has to say so. The message it was offered from is
    // consumed the moment it is pressed, so a rejected restore would otherwise
    // leave the reader watching a confirmation disappear and believing the team
    // came back. Sticky, because a refusal is not a courtesy to withdraw after
    // three and a half seconds.
    onError: (error) => {
      toast.show(problemMessageOf(error, t), { mark: false, sticky: true });
    },
    onSuccess: (_restored, { name }) => {
      qc.invalidateQueries({ queryKey: ["teams"] });
      toast.show(t("users.teamRestored", { name }));
    },
  });
  const archive = useMutation({
    mutationFn: async (id: string) => {
      const { error } = await api.PATCH("/teams/{id}", {
        params: { path: { id } },
        body: { archived: true },
      });
      if (error) throwProblem(error);
    },
    onSuccess: (_archived, id) => {
      // The name is read BEFORE the roster refetch lands, or the row it comes
      // from is already gone by the time the sentence is built.
      const name = teamName(teams.data, id);
      qc.invalidateQueries({ queryKey: ["teams"] });
      toast.show(t("users.teamArchived", { name }), {
        action: {
          label: t("common.undo"),
          onAct: () => restore.mutate({ id, name }),
        },
      });
    },
  });
  return (
    // The create verb sits on the title's own line, which is where a card-level
    // create verb goes. It used to be the LAST ROW of the team list, labelled
    // "New team" beside a button reading "Create team" — a row that was not a
    // team, inside a list of teams, saying its own name twice.
    <Panel title={t("users.teamsTitle")} titleAction={<NewTeamAction />}>
      <PanelBody>
        <p className="settings-panel-sub">{t("users.teamsSub")}</p>
        {/* A refused archive belongs to the card, not to the row: the roster
            below is refetched on success, so the only thing left to say is that
            the write did not land. Callout's `danger` tone is what the rest of
            this tab says that with — a bare `role="alert"` span took its
            emphasis from nothing at all. */}
        {archive.isError && (
          <Callout tone="danger" live="alert">
            {problemMessageOf(archive.error, t)}
          </Callout>
        )}
        <QueryGate query={teams}>
          {(list) => {
            // The roster hook serves users and teams alike, so narrow to the
            // entries that actually carry a team's name rather than asserting
            // the shape.
            const rows = list.flatMap((entry) =>
              "name" in entry ? [entry] : [],
            );
            return rows.length === 0 && !teamsPartial ? (
              <EmptyState>
                <p className="t-small">{t("users.noTeamsYet")}</p>
              </EmptyState>
            ) : (
              // One team per row: the name and how many people are in it on the
              // left, the verb that archives it on the right, at the one x every
              // answer in settings sits at.
              <SettingList>
                {rows.map((team) => (
                  <SettingRow
                    key={team.id}
                    label={team.name}
                    // The roster's own count copy, which has a singular — this
                    // row said "1 members" for a team of one.
                    // The TEAM's own count key, not the roster's: this row
                    // counts members OF a team, while the roster above counts
                    // users of the installation. One key for both made the
                    // rename of one of them silently rewrite the other.
                    value={plural(
                      "users.teamMemberCount",
                      team.member_count ?? 0,
                      { count: formatNumber(team.member_count ?? 0, locale) },
                    )}
                    control={
                      <Button
                        small
                        variant="ghost"
                        iconOnly
                        aria-label={t("users.archiveTeam", { name: team.name })}
                        disabled={archive.isPending}
                        onClick={() => archive.mutate(team.id)}
                      >
                        <Trash2 aria-hidden />
                      </Button>
                    }
                  />
                ))}
              </SettingList>
            );
          }}
        </QueryGate>
        {/* The card lists what there is, so a list that stopped short of the
            end says so under the rows rather than reading as all of them. */}
        <RosterPartialNote partial={teamsPartial} />
      </PanelBody>
    </Panel>
  );
}

// Creating a team is one field, but it is the CARD's verb rather than one of the
// card's rows — so it reads on the title line and the field it needs opens in a
// dialog. The alternative kept a create form permanently open at the foot of a
// list whose every other row was a team.
function NewTeamAction() {
  const t = useT();
  const qc = useQueryClient();
  const titleId = useId();
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState("");
  // The name rides as the mutation's variable rather than through the closure:
  // react-query re-arms a mutation's options in a passive effect, so a submit
  // landing in that window would otherwise create a team under the PREVIOUS
  // name — which is the one thing a team is.
  const create = useMutation({
    mutationFn: async (name: string) => {
      const { data, error } = await api.POST("/teams", { body: { name } });
      if (error) throwProblem(error);
      return data;
    },
    onSuccess: () => {
      setDraft("");
      // The dialog closes on the write that landed, never before it: a refused
      // create has to leave the name where the admin typed it.
      setOpen(false);
      qc.invalidateQueries({ queryKey: ["teams"] });
    },
  });
  const ready = draft.trim() !== "" && !create.isPending;
  return (
    <>
      {/* Named for what it opens; the dialog's submit reads "Create team", so
          the two buttons on screen together are tellable apart. */}
      <Button small onClick={() => setOpen(true)}>
        {t("users.newTeamOpen")}
      </Button>
      <Modal open={open} onClose={() => setOpen(false)} labelledBy={titleId}>
        <form
          className="form-stack"
          onSubmit={(event) => {
            event.preventDefault();
            if (ready) create.mutate(draft.trim());
          }}
        >
          <h2 className="t-h3 modal-title" id={titleId}>
            {t("users.newTeamLabel")}
          </h2>
          <Field label={t("users.teamNameLabel")} required>
            {(control) => (
              <TextInput
                {...control}
                value={draft}
                placeholder={t("users.newTeamPlaceholder")}
                disabled={create.isPending}
                onChange={(event) => setDraft(event.target.value)}
              />
            )}
          </Field>
          {/* `.form-stack` stretches its children, so the submit takes its own
              trailing row rather than filling the dialog's width. */}
          <div className="form-actions">
            <Button type="submit" variant="primary" small disabled={!ready}>
              {t("users.createTeam")}
            </Button>
          </div>
          {create.isError && (
            <Callout tone="danger" live="alert">
              {problemMessageOf(create.error, t)}
            </Callout>
          )}
        </form>
      </Modal>
    </>
  );
}
