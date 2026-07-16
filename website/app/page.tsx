import { FailureReplay } from "./failure-replay";
import { HeroRouter } from "./hero-router";
import { InstallConsole } from "./install-console";
import { ProviderMatrix } from "./provider-matrix";
import { RequestLifecycle } from "./request-lifecycle";
import { SetupSession } from "./setup-session";
import { TuiTour } from "./tui-tour";

export default function HomePage() {
  return (
    <>
      <HeroRouter />
      <ProviderMatrix />
      <FailureReplay />
      <SetupSession />
      <RequestLifecycle />
      <TuiTour />
      <InstallConsole />
    </>
  );
}
