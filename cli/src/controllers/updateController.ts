import type { APIClient } from "../api/client.js";
import { getInitialUpdateJobForView, isUpdateActive } from "../components/UpdateView.js";
import type { AppAction, AppState } from "../types.js";

export type UpdateControllerApi = Pick<APIClient, "getUpdateCheck" | "getUpdateJob" | "applyUpdate">;

interface UpdateControllerOptions {
  api: UpdateControllerApi;
  dispatch: (action: AppAction) => void;
  appendSystem: (content: string) => void;
  getState: () => AppState;
}

/** Update check/apply/polling orchestration independent of Ink rendering. */
export function createUpdateController({ api, dispatch, appendSystem, getState }: UpdateControllerOptions) {
  const loadUpdate = async () => {
    dispatch({ type: "SET_VIEW", view: "update" });
    dispatch({ type: "SET_UPDATE_STATE", loading: true, confirming: false });
    try {
      const check = await api.getUpdateCheck(true);
      const job = await api.getUpdateJob();
      const visibleJob = getInitialUpdateJobForView(job);
      dispatch({ type: "SET_UPDATE_STATE", check, job: visibleJob, loading: false, applying: isUpdateActive(visibleJob) });
    } catch (error) {
      dispatch({ type: "SET_UPDATE_STATE", loading: false, applying: false });
      appendSystem(`Failed to load update status: ${(error as Error).message}`);
    }
  };

  const checkForUpdatesSilently = async () => {
    try {
      const check = await api.getUpdateCheck(false);
      dispatch({ type: "SET_UPDATE_STATE", check });
    } catch {
      // Update checks are informational; keep startup quiet.
    }
  };

  const applyUpdate = async () => {
    const state = getState();
    if (!state.updateCheck?.canApply || !state.updateCheck.latest || state.updateApplying) {
      appendSystem("No applicable update is available.");
      return;
    }
    dispatch({ type: "SET_UPDATE_STATE", applying: true, confirming: false });
    try {
      const job = await api.applyUpdate(state.updateCheck.latest);
      dispatch({ type: "SET_UPDATE_STATE", job, applying: true });
      appendSystem(`Update started for ${state.updateCheck.latest}. The update view will keep showing progress while the service is reachable.`);
    } catch (error) {
      dispatch({ type: "SET_UPDATE_STATE", applying: false });
      appendSystem(`Failed to start update: ${(error as Error).message}`);
    }
  };

  const refreshUpdateJob = async (isCancelled: () => boolean) => {
    try {
      const job = await api.getUpdateJob();
      if (isCancelled()) return;
      dispatch({ type: "SET_UPDATE_STATE", job, applying: isUpdateActive(job) });
    } catch {
      if (!isCancelled()) dispatch({ type: "SET_UPDATE_STATE", applying: false });
    }
  };

  return { loadUpdate, checkForUpdatesSilently, applyUpdate, refreshUpdateJob };
}
