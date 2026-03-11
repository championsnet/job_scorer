import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import { DataTable } from "../components/ui/DataTable";
import { ErrorState, EmptyState, LoadingState } from "../components/ui/QueryState";
import { PageHeader } from "../components/ui/PageHeader";
import { StatCard } from "../components/ui/StatCard";
import { StatusBadge } from "../components/ui/StatusBadge";
import { Surface } from "../components/ui/Surface";
import { formatDateTime, formatDuration } from "../lib/format";
import type { RunSummary } from "../types";

const SESSION_KEY = "job_scorer_active_request_id";

function readStoredRequestId(): string | null {
  try {
    return sessionStorage.getItem(SESSION_KEY);
  } catch {
    return null;
  }
}

function writeStoredRequestId(id: string | null) {
  try {
    if (id) {
      sessionStorage.setItem(SESSION_KEY, id);
    } else {
      sessionStorage.removeItem(SESSION_KEY);
    }
  } catch {
    // sessionStorage unavailable (private mode, etc.)
  }
}

export default function Runs() {
  const [limit, setLimit] = useState(50);
  const [forceReeval, setForceReeval] = useState(false);
  const [activeRequestId, setActiveRequestId] = useState<string | null>(readStoredRequestId);
  const queryClient = useQueryClient();

  // Auto-poll the runs list whenever there are queued or running rows.
  const { data, isLoading, error } = useQuery({
    queryKey: ["runs", limit],
    queryFn: () => api.listRuns(limit),
    refetchInterval: (query) => {
      const runs = query.state.data?.runs ?? [];
      const hasLive = runs.some((r) => r.status === "queued" || r.status === "running");
      return hasLive ? 5000 : false;
    },
  });

  const requestStatus = useQuery({
    queryKey: ["run-request", activeRequestId],
    queryFn: () => api.getRunRequest(activeRequestId!),
    enabled: !!activeRequestId,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      if (status === "queued") return 3000;
      if (status === "running") return 2000;
      return false;
    },
  });

  const triggerMutation = useMutation({
    mutationFn: () => api.triggerRun({ forceReeval }),
    onSuccess: (result) => {
      setActiveRequestId(result.request_id);
      writeStoredRequestId(result.request_id);
      queryClient.invalidateQueries({ queryKey: ["runs"] });
      queryClient.invalidateQueries({ queryKey: ["analytics"] });
    },
  });

  useEffect(() => {
    const status = requestStatus.data?.status;
    if (status === "success" || status === "failed") {
      queryClient.invalidateQueries({ queryKey: ["runs"] });
      queryClient.invalidateQueries({ queryKey: ["analytics"] });
      setActiveRequestId(null);
      writeStoredRequestId(null);
    }
  }, [queryClient, requestStatus.data?.status]);

  if (isLoading) {
    return <LoadingState title="Loading runs" description="Retrieving recent pipeline runs and summaries." />;
  }

  if (error) {
    return <ErrorState title="Runs failed to load" description={String(error)} />;
  }

  const runs = data?.runs ?? [];
  const latestRun = runs[0];
  const hasLiveRow = runs.some((r) => r.status === "queued" || r.status === "running");
  const isRunActive = triggerMutation.isPending || requestStatus.data?.status === "running" || requestStatus.data?.status === "queued" || hasLiveRow;

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow="Operations"
        title="Runs"
        description="Trigger new searches, monitor active requests, and compare outcomes across recent runs."
        actions={
          <>
            <select
              value={limit}
              onChange={(event) => setLimit(Number(event.target.value))}
              className="rounded-xl border border-white/10 bg-slate-950/60 px-3 py-2 text-sm text-slate-200 outline-none ring-0"
            >
              <option value={20}>20 runs</option>
              <option value={50}>50 runs</option>
              <option value={100}>100 runs</option>
            </select>
            <label className="flex cursor-pointer items-center gap-2 select-none">
              <div
                onClick={() => setForceReeval((v) => !v)}
                className={`relative h-5 w-9 rounded-full transition-colors ${forceReeval ? "bg-amber-400" : "bg-slate-700"}`}
              >
                <span
                  className={`absolute top-0.5 left-0.5 h-4 w-4 rounded-full bg-white shadow transition-transform ${forceReeval ? "translate-x-4" : "translate-x-0"}`}
                />
              </div>
              <span className="text-sm text-slate-300">Re-evaluate duplicates</span>
            </label>
            <button
              onClick={() => triggerMutation.mutate()}
              disabled={isRunActive}
              className="rounded-xl bg-cyan-400 px-4 py-2 text-sm font-semibold text-slate-950 transition hover:bg-cyan-300 disabled:cursor-not-allowed disabled:bg-slate-700 disabled:text-slate-400"
            >
              {isRunActive ? "Run in progress" : "Trigger new run"}
            </button>
          </>
        }
      />

      <div className="grid gap-4 md:grid-cols-3">
        <StatCard label="Loaded runs" value={runs.length} helper="Currently displayed in the table below" />
        <StatCard
          label="Active request"
          value={requestStatus.data?.status ?? (hasLiveRow ? "in progress" : "idle")}
          tone={isRunActive ? "amber" : "emerald"}
          helper={
            activeRequestId
              ? `Tracking request ${activeRequestId}`
              : hasLiveRow
              ? "A run is queued or running — polling for updates"
              : "No active request in this session"
          }
        />
        <StatCard label="Latest duration" value={latestRun ? formatDuration(latestRun.duration_ms) : "-"} tone="cyan" helper={latestRun ? latestRun.run_id : "No runs available yet"} />
      </div>

      {requestStatus.data ? (
        <Surface
          className={`p-5 ${
            requestStatus.data.status === "failed"
              ? "border-rose-500/20 bg-rose-950/20"
              : requestStatus.data.status === "success"
              ? "border-emerald-500/20 bg-emerald-950/10"
              : "border-amber-500/20 bg-amber-950/10"
          }`}
        >
          <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
            <div>
              <p className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-400">Request tracker</p>
              <p className="mt-2 text-sm text-slate-200">
                Request <span className="font-mono text-xs">{requestStatus.data.request_id}</span> is currently{" "}
                <strong className="capitalize">{requestStatus.data.status}</strong>.
              </p>
              {requestStatus.data.error ? (
                <p className="mt-2 text-sm text-rose-300">{requestStatus.data.error}</p>
              ) : null}
            </div>
            <StatusBadge status={requestStatus.data.status as "queued" | "success" | "failed" | "running"} />
          </div>
        </Surface>
      ) : hasLiveRow ? (
        <Surface className="border-amber-500/20 bg-amber-950/10 p-5">
          <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
            <div>
              <p className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-400">Run in progress</p>
              <p className="mt-2 text-sm text-slate-200">
                A run is currently <strong>queued or running</strong>. Refreshing automatically every 5 seconds.
              </p>
            </div>
            <StatusBadge status="queued" />
          </div>
        </Surface>
      ) : null}

      {triggerMutation.isError ? (
        <ErrorState title="Unable to start a run" description={String(triggerMutation.error)} />
      ) : null}

      {runs.length === 0 ? (
        <EmptyState title="No runs yet" description="Trigger the first run to start building your pipeline history." />
      ) : (
        <DataTable
          title="Run history"
          description="Every stored run with stage counts, status, and duration."
          summary={`${runs.length} runs shown`}
        >
          <table className="min-w-full text-sm">
            <thead className="bg-white/2 text-left text-slate-400">
              <tr>
                <th className="px-5 py-3 font-medium">Run</th>
                <th className="px-5 py-3 font-medium">Status</th>
                <th className="px-5 py-3 font-medium">Started</th>
                <th className="px-5 py-3 font-medium">Duration</th>
                <th className="px-5 py-3 font-medium">Scraped</th>
                <th className="px-5 py-3 font-medium">Promising</th>
                <th className="px-5 py-3 font-medium">Final</th>
                <th className="px-5 py-3 font-medium">Emailed</th>
              </tr>
            </thead>
            <tbody>
              {runs.map((run: RunSummary) => (
                <tr key={run.run_id} className="border-t border-white/5 text-slate-300 transition hover:bg-white/3">
                  <td className="px-5 py-4">
                    <Link to={`/runs/${run.run_id}`} className="font-mono text-xs text-cyan-300 hover:text-cyan-200">
                      {run.run_id}
                    </Link>
                  </td>
                  <td className="px-5 py-4">
                    <StatusBadge status={run.status} />
                  </td>
                  <td className="px-5 py-4">{formatDateTime(run.started_at)}</td>
                  <td className="px-5 py-4">{formatDuration(run.duration_ms)}</td>
                  <td className="px-5 py-4">{run.stage_counts.all_jobs}</td>
                  <td className="px-5 py-4">{run.stage_counts.promising}</td>
                  <td className="px-5 py-4">{run.stage_counts.final_evaluated}</td>
                  <td className="px-5 py-4">{run.stage_counts.email_sent}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </DataTable>
      )}
    </div>
  );
}
