import { FailureReplay } from "./failure-replay";
import { HeroRouter } from "./hero-router";
import { InstallConsole } from "./install-console";
import { LandingMotion } from "./landing-motion";
import { ProviderMatrix } from "./provider-matrix";
import { RequestLifecycle } from "./request-lifecycle";
import { SetupSession } from "./setup-session";
import { TuiTour } from "./tui-tour";

export default function HomePage() {
  return (
    <LandingMotion
      hero={<HeroRouter />}
      providers={<ProviderMatrix />}
      failure={<FailureReplay />}
      setup={<SetupSession />}
      lifecycle={<RequestLifecycle />}
      tui={<TuiTour />}
      install={<InstallConsole />}
    />
  );
}
