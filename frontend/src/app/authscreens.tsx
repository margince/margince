import { type ComponentProps, Suspense } from "react";
import { lazy } from "../i18n/lazy";
import { AuthSplash } from "./bootsplash";

// On the essential catalog, so a signed-out reader never waits for the complete one.
const SignIn = lazy(
  async () => ({
    default: (await import("../features/sign-in/auth")).AuthScreen,
  }),
  "essential",
);
const Availability = lazy(
  async () => ({
    default: (await import("../features/sign-in/auth")).AvailabilityScreen,
  }),
  "essential",
);
const Setup = lazy(
  async () => ({
    default: (await import("../features/sign-in/setupclaim")).SetupClaimScreen,
  }),
  "essential",
);

export function AuthScreen(props: ComponentProps<typeof SignIn>) {
  return (
    <Suspense fallback={<AuthSplash />}>
      <SignIn {...props} />
    </Suspense>
  );
}
export function AvailabilityScreen(props: ComponentProps<typeof Availability>) {
  return (
    <Suspense fallback={<AuthSplash />}>
      <Availability {...props} />
    </Suspense>
  );
}
export function SetupClaimScreen(props: ComponentProps<typeof Setup>) {
  return (
    <Suspense fallback={<AuthSplash />}>
      <Setup {...props} />
    </Suspense>
  );
}

const Skew = lazy(async () => ({
  default: (await import("../features/settings/releaseskew")).ReleaseSkewScreen,
}));
export function ReleaseSkewScreen(props: ComponentProps<typeof Skew>) {
  return (
    <Suspense fallback={<AuthSplash />}>
      <Skew {...props} />
    </Suspense>
  );
}
