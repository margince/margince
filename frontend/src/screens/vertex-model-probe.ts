// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useState } from "react";
import { type Translator, useT } from "../i18n";
import { type AvailableModels, useModelProbe } from "./ai-models";

// Whether a Vertex lane's location serves its model, asked of Google while the
// lane is edited rather than left to the save to discover.

/** One probe: whether `location` serves `model`, and what to do if it does not. */
type ProbeTarget = Readonly<{
  location: string;
  model: string;
  clearIfUnserved: boolean;
}>;

type ProbedBinding = { provider: string; model: string };

/**
 * The probe behind one Vertex lane's model field. The editor tells it what the
 * reader did — re-pointed the lane, moved its location, picked a model — and
 * reads back the hint the model field shows.
 */
export function useVertexModelProbe<B extends ProbedBinding>({
  vertex,
  laneName,
  binding,
  location,
  available,
  onChange,
}: Readonly<{
  vertex: boolean;
  laneName: string;
  binding: B;
  location: string;
  available: AvailableModels | undefined;
  onChange: (next: B) => void;
}>) {
  const t = useT();
  const [target, setTarget] = useState<ProbeTarget | undefined>();
  const [cleared, setCleared] = useState<ProbeTarget | undefined>();
  const probe = useModelProbe(
    binding.provider,
    laneName,
    vertex ? target : undefined,
  );
  // The probe speaks for the field only while it asked about what the field
  // holds; a model typed since is a different question.
  const probed =
    vertex &&
    target !== undefined &&
    target.model === binding.model &&
    target.location === location;
  const unserved = probed && probe.data?.unavailable === "no_endpoint";
  // A location change that the model does not survive empties the field
  // rather than leaving a binding the save would refuse.
  useEffect(() => {
    if (unserved && target?.clearIfUnserved) {
      setCleared(target);
      // Once: the same model typed back in is flagged, not cleared again.
      setTarget({ ...target, clearIfUnserved: false });
      onChange({ ...binding, model: "" });
    }
  }, [unserved, target, binding, onChange]);

  return {
    hint: vertexModelHint({
      available,
      probe: probed ? probe.data : undefined,
      probing: probed && probe.isFetching,
      cleared,
      location,
      t,
    }),
    /** The lane was re-pointed: nothing asked so far is about it. */
    forget: () => {
      setTarget(undefined);
      setCleared(undefined);
    },
    /** The location moved: ask it about the model, and clear one it lacks. */
    relocate: (next: string) => {
      setCleared(undefined);
      setTarget(
        binding.model === ""
          ? undefined
          : { location: next, model: binding.model, clearIfUnserved: true },
      );
    },
    /** A model was chosen; one picked from the list is worth asking about. */
    picked: (model: string, fromList: boolean) => {
      setCleared(undefined);
      // A hand edit drops the question, so a pending clear cannot wipe it.
      setTarget(
        vertex && fromList
          ? { location, model, clearIfUnserved: false }
          : undefined,
      );
    },
  };
}

/**
 * What a Vertex lane's model field says: the probe's verdict on the chosen
 * model when there is one, else what the location's list could tell. Undefined
 * text falls through to the note every vendor shares.
 */
function vertexModelHint({
  available,
  probe,
  probing,
  cleared,
  location,
  t,
}: Readonly<{
  available: AvailableModels | undefined;
  probe: AvailableModels | undefined;
  probing: boolean;
  cleared: ProbeTarget | undefined;
  location: string;
  t: Translator;
}>): { text?: string; error?: string } {
  if (cleared) {
    return {
      text: t("aiRouting.probe.cleared", {
        model: cleared.model,
        location: cleared.location,
      }),
    };
  }
  if (probing) {
    return { text: t("aiRouting.probe.checking", { location }) };
  }
  if (probe?.unavailable === "no_endpoint") {
    return { error: t("aiRouting.probe.notServed", { location }) };
  }
  if (probe?.unavailable) {
    return { text: t("aiRouting.probe.unverified", { location }) };
  }
  if (probe) {
    return { text: t("aiRouting.probe.served", { location }) };
  }
  if (available?.unavailable === "no_key") {
    return { text: t("aiRouting.location.noKey") };
  }
  if (available && !available.unavailable && available.models.length === 0) {
    return { text: t("aiRouting.location.noModels", { location }) };
  }
  return {};
}
