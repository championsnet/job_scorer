const API_BASE = import.meta.env.VITE_API_URL
  ? `${import.meta.env.VITE_API_URL.replace(/\/$/, "")}/api`
  : "/api";

type TokenProvider = () => Promise<string | null>;

let tokenProvider: TokenProvider = async () => null;

export function setAuthTokenProvider(provider: TokenProvider) {
  tokenProvider = provider;
}

type FetchApiOptions = {
  skipAuth?: boolean;
  isFormData?: boolean;
};

async function fetchApi<T>(
  path: string,
  options?: RequestInit,
  fetchOptions?: FetchApiOptions
): Promise<T> {
  const isFormData = fetchOptions?.isFormData ?? false;
  const headers = new Headers(options?.headers ?? {});

  if (!isFormData && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  if (!fetchOptions?.skipAuth) {
    const token = await tokenProvider();
    if (token) {
      headers.set("Authorization", `Bearer ${token}`);
    }
  }

  const response = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers,
  });

  if (!response.ok) {
    const errPayload = await response.json().catch(() => ({ error: response.statusText }));
    throw new Error((errPayload as { error?: string }).error || response.statusText);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  return response.json();
}

export const api = {
  getCurrentUser: () => fetchApi<import("../types").CurrentUser>("/v1/me"),

  getSettings: () => fetchApi<import("../types").AccountSettings>("/v1/settings"),

  updateSettings: (payload: Partial<import("../types").AccountSettings>) =>
    fetchApi<import("../types").AccountSettings>("/v1/settings", {
      method: "PUT",
      body: JSON.stringify(payload),
    }),

  uploadCV: (file: File) => {
    const form = new FormData();
    form.append("cv", file);
    return fetchApi<{ id: string; storagePath: string }>("/v1/settings/cv", {
      method: "POST",
      body: form,
    }, { isFormData: true });
  },

  getCVStatus: () => fetchApi<import("../types").AccountCVStatus>("/v1/settings/cv"),

  triggerRun: (opts?: { forceReeval?: boolean }) =>
    fetchApi<{
      status: string;
      request_id: string;
      run_id: string;
      status_url: string;
      message: string;
    }>("/v1/runs", {
      method: "POST",
      body: JSON.stringify({ force_reeval: opts?.forceReeval ?? false }),
    }),

  getRunRequest: (requestId: string) =>
    fetchApi<{
      request_id: string;
      run_id: string;
      status: string;
      started_at?: string;
      completed_at?: string;
      error?: string;
    }>(`/v1/runs/requests/${requestId}`),

  listRuns: (limit = 50) =>
    fetchApi<{ runs: import("../types").RunSummary[] }>(`/v1/runs?limit=${limit}`),

  getRun: (runId: string) => fetchApi<import("../types").RunSummary>(`/v1/runs/${runId}`),

  getRunStageJobs: (runId: string, stage: string) =>
    fetchApi<import("../types").StageJobsResponse>(`/v1/runs/${runId}/stages/${stage}`),

  getAnalyticsOverview: () => fetchApi<import("../types").AnalyticsOverview>("/v1/analytics/overview"),

  getBillingSummary: () => fetchApi<import("../types").BillingSummary>("/v1/billing/summary"),

  createCheckout: (packageId: string) =>
    fetchApi<{ session_id: string; url: string }>("/v1/billing/checkout", {
      method: "POST",
      body: JSON.stringify({ package_id: packageId }),
    }),
};
