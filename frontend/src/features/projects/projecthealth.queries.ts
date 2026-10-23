import {
  skipToken,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { throwProblem } from "../../lib/problem";
import { project360Key } from "./project360.key";

// How a project is going, as somebody judged it on a day.
//
// The history is read whole rather than only its head: a delivery review asks
// what was said in March, and a card that could only answer "now" would send
// the reader to the timeline for the rest.

export type ProjectHealthAssessment =
  components["schemas"]["ProjectHealthAssessment"];
export type ProjectHealthState = components["schemas"]["ProjectHealthState"];

export function projectHealthKey(projectId: string) {
  return ["project-health", projectId] as const;
}

export function useProjectHealth(projectId: string | undefined) {
  return useQuery({
    queryKey: projectHealthKey(projectId ?? ""),
    queryFn: projectId
      ? async () => {
          const { data, error, response } = await api.GET(
            "/projects/{id}/health-assessments",
            { params: { path: { id: projectId } } },
          );
          if (error || !response.ok) {
            throwProblem(error);
          }
          return data.data;
        }
      : skipToken,
  });
}

export function useRecordProjectHealth(projectId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (
      body: components["schemas"]["CreateProjectHealthAssessmentRequest"],
    ) => {
      const { data, error, response } = await api.POST(
        "/projects/{id}/health-assessments",
        { params: { path: { id: projectId } }, body },
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
    // The project page carries the current reading in its own payload, so both
    // are invalidated: a card that refreshed while the header above it kept the
    // old judgement would be the page disagreeing with itself.
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: projectHealthKey(projectId) });
      qc.invalidateQueries({ queryKey: project360Key(projectId) });
    },
  });
}

export function useCorrectProjectHealth(projectId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (args: {
      assessmentId: string;
      body: components["schemas"]["CreateProjectHealthCorrectionRequest"];
    }) => {
      const { data, error, response } = await api.POST(
        "/projects/{id}/health-assessments/{assessment_id}/corrections",
        {
          params: {
            path: { id: projectId, assessment_id: args.assessmentId },
          },
          body: args.body,
        },
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: projectHealthKey(projectId) });
      qc.invalidateQueries({ queryKey: project360Key(projectId) });
    },
  });
}
