import { type ComponentProps, Suspense } from "react";
import { lazy } from "../../i18n/lazy";

const Evidence = lazy(async () => ({
  default: (await import("./reporting.evidence")).ReportingEvidenceDrawer,
}));
const Initiative = lazy(async () => ({
  default: (await import("./reporting.initiativedrawer")).InitiativeDrawer,
}));

export function ReportingEvidenceDrawer(
  props: ComponentProps<typeof Evidence>,
) {
  return (
    <Suspense fallback={null}>
      <Evidence {...props} />
    </Suspense>
  );
}

export function InitiativeDrawer(props: ComponentProps<typeof Initiative>) {
  return (
    <Suspense fallback={null}>
      <Initiative {...props} />
    </Suspense>
  );
}
