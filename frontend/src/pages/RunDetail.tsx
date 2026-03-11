import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { api } from "../api/client";
import { ErrorState, LoadingState } from "../components/ui/QueryState";
import { PageHeader } from "../components/ui/PageHeader";
import { StatCard } from "../components/ui/StatCard";
import { StatusBadge } from "../components/ui/StatusBadge";
import { Surface } from "../components/ui/Surface";
import { compactNumber, formatDateTime, formatDuration } from "../lib/format";

const STAGE_CONFIG: { apiStage: string; countKey: string; label: string }[] = [
  { apiStage: "all_jobs", countKey: "all_jobs", label: "Scraped" },
  { apiStage: "prefiltered", countKey: "prefiltered", label: "Prefiltered" },
  { apiStage: "evaluated", countKey: "evaluated", label: "LLM evaluated" },
  { apiStage: "promising", countKey: "promising", label: "Promising" },
  { apiStage: "final_evaluated", countKey: "final_evaluated", label: "CV evaluated" },
  { apiStage: "notification", countKey: "notification", label: "Notification" },
  { apiStage: "validated_notification", countKey: "validated", label: "Validated" },
  { apiStage: "email_sent", countKey: "email_sent", label: "Email sent" },
];

export default function RunDetail() {
  const { runId } = useParams<{ runId: string }>();
  const { data: run, isLoading, error } = useQuery({
    queryKey: ["run", runId],
    queryFn: () => api.getRun(runId!),
    enabled: !!runId,
  });

  if (isLoading) {
    return <LoadingState title="Loading run detail" description="Fetching metrics, stage counts, and notification outcomes." />;
  }

  if (error) {
    return <ErrorState title="Run detail failed to load" description={String(error)} />;
  }

  if (!run) {
    return <ErrorState title="Run not found" description="The requested run ID could not be loaded." />;
  }

  const counts = run.stage_counts as unknown as Record<string, number>;
  const locationList = Array.isArray(run.config?.locations) ? run.config.locations : [];
  const startedLabel = run.status === "queued" ? "Waiting for worker" : formatDateTime(run.started_at);

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow="Run detail"
        title={run.run_id}
        description={
          <div className="flex flex-wrap items-center gap-3">
            <Link to="/runs" className="text-cyan-300 hover:text-cyan-200">
              Back to runs
            </Link>
            <StatusBadge status={run.status} />
          </div>
        }
      />

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <StatCard label="Started" value={startedLabel} helper="Run start time" />
        <StatCard label="Duration" value={formatDuration(run.duration_ms)} tone="cyan" helper="Total runtime" />
        <StatCard label="LLM calls" value={run.llm_usage.calls} tone="violet" helper={`${compactNumber(run.llm_usage.total_tokens)} total tokens`} />
        <StatCard label="Email sent" value={run.stage_counts.email_sent} tone="emerald" helper={`${run.stage_counts.promising} promising jobs`} />
      </div>

      {run.error_message ? (
        <Surface className="border-rose-500/20 bg-rose-950/20 p-5">
          <p className="text-xs font-semibold uppercase tracking-[0.18em] text-rose-300">Run error</p>
          <p className="mt-2 text-sm text-rose-100">{run.error_message}</p>
        </Surface>
      ) : null}

      <Surface className="p-5">
        <div className="mb-6">
          <h2 className="text-lg font-semibold text-white">Pipeline timeline</h2>
          <p className="mt-1 text-sm text-slate-400">Follow the run from scrape through delivery, with direct links into each stage payload.</p>
        </div>
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
          {STAGE_CONFIG.map(({ apiStage, countKey, label }, index) => {
            const count = counts[countKey] ?? 0;
            return (
              <Link
                key={`${apiStage}-${countKey}-${index}`}
                to={`/runs/${run.run_id}/stages/${apiStage}`}
                className="group rounded-2xl border border-white/10 bg-white/[0.03] p-4 transition hover:border-cyan-400/25 hover:bg-cyan-400/[0.06]"
              >
                <div className="flex items-start justify-between gap-3">
                  <p className="text-sm font-medium text-white">{label}</p>
                  <span className="rounded-full bg-white/[0.06] px-2 py-1 text-[11px] text-slate-400">Stage {index + 1}</span>
                </div>
                <p className="mt-4 text-3xl font-semibold tracking-tight text-cyan-200">{count}</p>
                <p className="mt-2 text-xs text-slate-500 group-hover:text-slate-400">Open jobs in this stage</p>
              </Link>
            );
          })}
        </div>
      </Surface>

      <div className="grid gap-6 xl:grid-cols-[1fr_1fr]">
        <Surface className="p-5">
          <h2 className="text-lg font-semibold text-white">Run configuration</h2>
          <div className="mt-5 grid gap-4 sm:grid-cols-2">
            <div className="rounded-2xl bg-white/[0.03] p-4">
              <p className="text-xs uppercase tracking-[0.18em] text-slate-500">Locations</p>
              <p className="mt-2 text-sm text-slate-200">{locationList.join(", ") || "-"}</p>
            </div>
            <div className="rounded-2xl bg-white/[0.03] p-4">
              <p className="text-xs uppercase tracking-[0.18em] text-slate-500">Max jobs</p>
              <p className="mt-2 text-sm text-slate-200">{run.config.max_jobs}</p>
            </div>
          </div>
        </Surface>

        <Surface className="p-5">
          <h2 className="text-lg font-semibold text-white">LLM usage snapshot</h2>
          <div className="mt-5 grid gap-4 sm:grid-cols-2">
            <div className="rounded-2xl bg-white/[0.03] p-4">
              <p className="text-xs uppercase tracking-[0.18em] text-slate-500">Input tokens</p>
              <p className="mt-2 text-sm text-slate-200">{compactNumber(run.llm_usage.input_tokens)}</p>
            </div>
            <div className="rounded-2xl bg-white/[0.03] p-4">
              <p className="text-xs uppercase tracking-[0.18em] text-slate-500">Output tokens</p>
              <p className="mt-2 text-sm text-slate-200">{compactNumber(run.llm_usage.output_tokens)}</p>
            </div>
            <div className="rounded-2xl bg-white/[0.03] p-4">
              <p className="text-xs uppercase tracking-[0.18em] text-slate-500">Cached input</p>
              <p className="mt-2 text-sm text-slate-200">{compactNumber(run.llm_usage.cached_input_tokens)}</p>
            </div>
            <div className="rounded-2xl bg-white/[0.03] p-4">
              <p className="text-xs uppercase tracking-[0.18em] text-slate-500">Billable input</p>
              <p className="mt-2 text-sm text-slate-200">{compactNumber(run.llm_usage.billable_input_tokens)}</p>
            </div>
          </div>
        </Surface>
      </div>

      {run.notification ? (
        <Surface className="p-5">
          <h2 className="text-lg font-semibold text-white">Notification outcome</h2>
          <div className="mt-5 grid gap-4 md:grid-cols-3">
            <div className="rounded-2xl bg-white/[0.03] p-4">
              <p className="text-xs uppercase tracking-[0.18em] text-slate-500">Recipients</p>
              <p className="mt-2 text-sm text-slate-200">{run.notification.recipients_count}</p>
            </div>
            <div className="rounded-2xl bg-white/[0.03] p-4">
              <p className="text-xs uppercase tracking-[0.18em] text-slate-500">Successful sends</p>
              <p className="mt-2 text-sm text-slate-200">{run.notification.success_count}</p>
            </div>
            <div className="rounded-2xl bg-white/[0.03] p-4">
              <p className="text-xs uppercase tracking-[0.18em] text-slate-500">Failed sends</p>
              <p className="mt-2 text-sm text-slate-200">{run.notification.failed_count}</p>
            </div>
          </div>
          {run.notification.error_message ? (
            <p className="mt-4 text-sm text-rose-300">{run.notification.error_message}</p>
          ) : null}
        </Surface>
      ) : null}
    </div>
  );
}
