// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The verbs on a project's stakeholders card.
//
// The card read the seats and offered nothing: a person putting a sponsor on a
// project, or taking a departed one off, had no control anywhere in the app and
// had to reach the endpoint through an agent tool. The read was on three
// surfaces and the write on none.
//
// They live here rather than beside the generic relationships panel because the
// contract puts them on the project's OWN endpoints — /projects/{id}/stakeholders
// — which is also why `project_stakeholder` is the one stakeholder kind that
// panel does not create. The endpoint carries two rules the generic surface
// cannot: write authority over the project row, and one seat per person.
//
// That last rule is what makes a role CHANGE the same act as an add: the PUT is
// idempotent per person, so naming somebody already seated re-roles them rather
// than seating them twice, and the dialog says so.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, Field } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../design-system/recordpicker";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";
import { projectRoleLabel } from "./record360";

type SetProjectStakeholderRequest =
  components["schemas"]["SetProjectStakeholderRequest"];
type ProjectStakeholderRole = SetProjectStakeholderRequest["role"];

// Every seat the contract admits, in the order it declares them: the five deal
// roles a project inherits, then the four a body of work running past close
// needs. Typed as the request's own role, so a role the contract drops stops
// compiling here rather than earning a 422 in front of a reader.
const PROJECT_STAKEHOLDER_ROLES: readonly ProjectStakeholderRole[] = [
  "champion",
  "economic_buyer",
  "blocker",
  "influencer",
  "user",
  "sponsor",
  "project_lead",
  "delivery_lead",
  "subject_matter_expert",
];

// The one key the project page reads under (project360.tsx). A guess here
// leaves the card showing the state before the write.
function invalidateProject(
  queryClient: ReturnType<typeof useQueryClient>,
  projectId: string,
) {
  queryClient.invalidateQueries({ queryKey: ["project", projectId, "360"] });
}

async function searchPeople(query: string): Promise<RecordPickerCandidate[]> {
  const { data, error } = await api.GET("/people", {
    params: { query: { q: query, limit: 10 } },
  });
  if (error) {
    throwProblem(error);
  }
  return data.data.map((person) => ({
    id: person.id,
    name: person.full_name,
  }));
}

/**
 * Seat somebody on the project, or move somebody already seated to another
 * role — one dialog, because the endpoint makes them one write.
 */
export function AddProjectStakeholder({
  projectId,
}: Readonly<{ projectId: string }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [person, setPerson] = useState<RecordPickerCandidate | null>(null);
  const [role, setRole] = useState<ProjectStakeholderRole>(
    PROJECT_STAKEHOLDER_ROLES[0],
  );

  // The pick and the role arrive as the mutation's VARIABLES rather than
  // through this closure: react-query re-arms a mutation's options in a passive
  // effect, so a submit landing between the commit that enables the button and
  // that effect runs the previous render's function — where nobody had been
  // picked yet.
  const seat = useMutation({
    mutationFn: async (chosen: {
      personId: string;
      role: ProjectStakeholderRole;
    }) => {
      const { error } = await api.PUT("/projects/{id}/stakeholders", {
        params: { path: { id: projectId } },
        body: { person_id: chosen.personId, role: chosen.role },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      invalidateProject(queryClient, projectId);
      close();
    },
  });

  function close() {
    setOpen(false);
    setPerson(null);
    setRole(PROJECT_STAKEHOLDER_ROLES[0]);
    seat.reset();
  }

  return (
    <>
      <Button
        small
        onClick={() => setOpen(true)}
        data-testid="add-project-stakeholder"
      >
        {t("project.stakeholders.add")}
      </Button>
      <ConfirmModal
        open={open}
        onClose={close}
        title={t("project.stakeholders.add")}
        // "Add", not "Save": nothing here is being edited, and a reader who
        // pressed "Add stakeholder" should not have to work out whether "Save"
        // means the same thing.
        confirmLabel={t("project.stakeholders.addConfirm")}
        confirmDisabled={person === null}
        onConfirm={() => {
          if (person) {
            seat.mutate({ personId: person.id, role });
          }
        }}
        pending={seat.isPending}
        error={seat.isError ? problemMessageOf(seat.error, t) : null}
      >
        <div
          style={{
            display: "flex",
            flexDirection: "column",
            gap: "var(--space-2)",
          }}
        >
          {/* Says what a second seating does before anybody tries it: the same
              person named again is a re-role, not a duplicate row. */}
          <p className="t-caption">{t("project.stakeholders.addHint")}</p>
          <RecordPicker
            label={t("project.stakeholders.searchLabel")}
            searchTargets={searchPeople}
            onPick={setPerson}
            selected={person}
            disabled={seat.isPending}
          />
          <Field label={t("rel.role")}>
            {(control) => (
              <Select
                {...control}
                value={role}
                onChange={(value) => {
                  if (isStakeholderRole(value)) {
                    setRole(value);
                  }
                }}
                options={PROJECT_STAKEHOLDER_ROLES.map((value) => ({
                  value,
                  label: projectRoleLabel(value, t),
                }))}
              />
            )}
          </Field>
        </div>
      </ConfirmModal>
    </>
  );
}

// The Select hands back a bare string, and the request's role is a closed
// vocabulary — narrowing here is what keeps the body typed without an
// assertion.
function isStakeholderRole(value: string): value is ProjectStakeholderRole {
  return PROJECT_STAKEHOLDER_ROLES.some((role) => role === value);
}

/**
 * Take a person off the project.
 *
 * Two steps, like every other remove on a record page: the endpoint archives
 * the edge, and the card gives no way back.
 */
export function RemoveProjectStakeholder({
  projectId,
  personId,
  personName,
  returnFocusTo,
}: Readonly<{
  projectId: string;
  personId: string;
  // Absent when the caller may not read that person — the seat is still
  // reported, and still removable, so the dialog names the seat rather than
  // pretending to a name it was not given.
  personName: string | null | undefined;
  // Where focus lands once the dialog closes. A successful removal unmounts the
  // row this button sits in, so restoring focus to the trigger would hand a
  // keyboard reader a detached node and drop them on document.body. The card
  // names a surviving element — the same rule ProjectLinks keeps for its own
  // detach, one card up the page.
  returnFocusTo?: () => HTMLElement | null;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);

  const detach = useMutation({
    mutationFn: async (person: string) => {
      const { error } = await api.DELETE(
        "/projects/{id}/stakeholders/{person_id}",
        { params: { path: { id: projectId, person_id: person } } },
      );
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      invalidateProject(queryClient, projectId);
      setOpen(false);
    },
  });

  return (
    <>
      <Button
        small
        variant="danger"
        onClick={() => setOpen(true)}
        data-testid="remove-project-stakeholder"
        aria-label={t("project.stakeholders.removeOne", {
          name: personName ?? t("coverage.seatWithheld"),
        })}
      >
        {t("rel.remove")}
      </Button>
      <ConfirmModal
        open={open}
        onClose={() => {
          setOpen(false);
          detach.reset();
        }}
        title={t("project.stakeholders.removeTitle")}
        confirmLabel={t("rel.remove")}
        confirmVariant="danger"
        returnFocusTo={returnFocusTo}
        onConfirm={() => detach.mutate(personId)}
        pending={detach.isPending}
        error={detach.isError ? problemMessageOf(detach.error, t) : null}
      >
        <p>
          {t("project.stakeholders.removeConfirm", {
            name: personName ?? t("coverage.seatWithheld"),
          })}
        </p>
      </ConfirmModal>
    </>
  );
}
