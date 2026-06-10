import { createFlinkClient } from "./index";

declare global {
  interface Window {
    flink: ReturnType<typeof createFlinkClient>;
    createFlinkClient: typeof createFlinkClient;
  }
}

window.createFlinkClient = createFlinkClient;
window.flink = createFlinkClient();
