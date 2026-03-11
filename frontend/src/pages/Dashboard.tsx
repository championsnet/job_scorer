import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { api } from "../api/client";
import { DataTable } from "../components/ui/DataTable";
import { LoadingState, ErrorState, EmptyState } from "../components/ui/QueryState";
import { PageHeader } from "../components/ui/PageHeader";
import { StatCard } from "../components/ui/StatCard";
import { StatusBadge } from "../components/ui/StatusBadge";
import { Surface } from "../components/ui/Surface";
import { compactNumber, formatDuration, formatPercent } from "../lib/format";
import type { RunSummary } from "../types";

const STAGE_ORDER = [
  "all_jobs",
  "prefiltered",
  "promising",
  "notification",
  "validated",
] as const;

const STAGE_LABELS: Record<string, string> = {
  all_jobs: "Scraped",
  prefiltered: "Prefiltered",
  promising: "Promising",
  notification: "Notification",
  validated: "Validated",
};

export default function Dashboard() {
  const { data, isLoading, error } = useQuery({
    queryKey: ["analytics"],
    queryFn: api.getAnalyticsOverview,
  });

  if (isLoading) {
    return (
      <LoadingState
        title="Loading dashboard"
        description="Fetching run summaries, funnel analytics, and recent activity."
      />
    );
  }

  if (error) {
    return <ErrorState title="Dashboard failed to load" description={String(error)} />;
  }

  if (!data) {
    return <EmptyState title="No analytics available" description="Run the pipeline to populate the dashboard." />;
  }

  const { runs, funnel, llm_usage, recent_runs } = data;
  const recentRuns = recent_runs ?? [];
  const latestRun = recentRuns[0];
  const funnelData = STAGE_ORDER.map((key) => ({
    name: STAGE_LABELS[key],
    count: funnel[key as keyof typeof funnel] ?? 0,
  }));
  const statusData = [
    { name: "Success", value: runs.success, color: "#34d399" },
    { name: "Failed", value: runs.failed, color: "#fb7185" },
  ].filter((item) => item.value > 0);

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow="Overview"
        title="Pipeline dashboard"
        description="Understand recent activity at a glance, spot failed runs, and drill directly into the runs that matter."
        actions={
          latestRun ? (
            <Link
              to={`/runs/${latestRun.run_id}`}
              className="rounded-xl border border-cyan-400/20 bg-cyan-400/10 px-4 py-2 text-sm font-medium text-cyan-200 transition hover:bg-cyan-400/15"
            >
              Open latest run
            </Link>
          ) : null
        }
      />

      <Surface className="overflow-hidden p-6">
        <div className="grid gap-8 lg:grid-cols-[1.35fr_0.95fr]">
          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.18em] text-cyan-300">Today</p>
            <h2 className="mt-3 text-3xl font-semibold tracking-tight text-white">
              {runs.failed > 0 ? "There are runs that need attention." : "Pipeline health is looking stable."}
            </h2>
            <p className="mt-3 max-w-2xl text-sm leading-6 text-slate-400">
              {runs.total > 0
                ? `${runs.total} tracked runs with ${formatPercent(runs.success, runs.total)} success rate. ${funnel.validated} jobs reached the validated notification stage across recent history.`
                : "No runs have been tracked yet. Trigger the pipeline from the Runs page to start building analytics."}
            </p>
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <StatCard label="Tracked runs" value={runs.total} helper="Across the stored run history" />
            <StatCard label="Success rate" value={formatPercent(runs.success, runs.total)} tone="emerald" helper={`${runs.failed} failed runs`} />
            <StatCard label="Avg duration" value={formatDuration(runs.avg_duration_ms)} tone="cyan" helper="Mean wall-clock completion time" />
            <StatCard label="LLM traffic" value={compactNumber(llm_usage.calls)} tone="violet" helper={`${compactNumber(llm_usage.input_tokens)} input tokens`} />
          </div>
        </div>
      </Surface>

      <div className="grid gap-6 xl:grid-cols-[1.2fr_0.8fr]">
        <Surface className="p-5">
          <div className="mb-5 flex items-end justify-between gap-4">
            <div>
              <h2 className="text-lg font-semibold text-white">Pipeline funnel</h2>
              <p className="mt-1 text-sm text-slate-400">How jobs contract from scrape through final notification.</p>
            </div>
          </div>
          <div className="h-80">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={funnelData} layout="vertical" margin={{ left: 32, right: 12 }}>
                <CartesianGrid stroke="#1e293b" strokeDasharray="3 3" />
                <XAxis type="number" stroke="#64748b" />
                <YAxis type="category" dataKey="name" width={92} stroke="#94a3b8" />
                <Tooltip
                  cursor={{ fill: "rgba(148,163,184,0.08)" }}
                  contentStyle={{
                    backgroundColor: "#0f172a",
                    border: "1px solid rgba(148,163,184,0.2)",
                    borderRadius: "14px",
                  }}
                  labelStyle={{ color: "#e2e8f0" }}
                />
                <Bar dataKey="count" fill="#22d3ee" radius={[0, 10, 10, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </div>
        </Surface>

        <div className="space-y-6">
          <Surface className="p-5">
            <h2 className="text-lg font-semibold text-white">Run outcomes</h2>
            <p className="mt-1 text-sm text-slate-400">Current balance between successful and failed pipeline runs.</p>
            <div className="mt-5 h-64">
              {statusData.length > 0 ? (
                <ResponsiveContainer width="100%" height="100%">
                  <PieChart>
                    <Pie data={statusData} dataKey="value" nameKey="name" innerRadius={54} outerRadius={88} paddingAngle={3}>
                      {statusData.map((entry) => (
                        <Cell key={entry.name} fill={entry.color} />
                      ))}
                    </Pie>
                    <Tooltip
                      contentStyle={{
                        backgroundColor: "#0f172a",
                        border: "1px solid rgba(148,163,184,0.2)",
                        borderRadius: "14px",
                      }}
                    />
                  </PieChart>
                </ResponsiveContainer>
              ) : (
                <EmptyState title="No run outcomes yet" description="Once runs complete, the success/failure split will appear here." />
              )}
            </div>
          </Surface>

          {latestRun ? (
            <Surface className="p-5">
              <div className="flex items-start justify-between gap-4">
                <div>
                  <p className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Latest run</p>
                  <p className="mt-2 font-mono text-sm text-white">{latestRun.run_id}</p>
                </div>
                <StatusBadge status={latestRun.status} />
              </div>
              <div className="mt-5 grid grid-cols-3 gap-3">
                <div className="rounded-xl bg-white/[0.04] p-3">
                  <p className="text-xs text-slate-500">Scraped</p>
                  <p className="mt-1 text-xl font-semibold text-white">{latestRun.stage_counts.all_jobs}</p>
                </div>
                <div className="rounded-xl bg-white/[0.04] p-3">
                  <p className="text-xs text-slate-500">Promising</p>
                  <p className="mt-1 text-xl font-semibold text-white">{latestRun.stage_counts.promising}</p>
                </div>
                <div className="rounded-xl bg-white/[0.04] p-3">
                  <p className="text-xs text-slate-500">Emailed</p>
                  <p className="mt-1 text-xl font-semibold text-white">{latestRun.stage_counts.email_sent}</p>
                </div>
              </div>
            </Surface>
          ) : null}
        </div>
      </div>

      <DataTable
        title="Recent runs"
        description="The latest pipeline activity, optimized for quick scanning and direct drill-down."
        summary={`${recentRuns.length} recent runs returned by the analytics endpoint`}
      >
        <table className="min-w-full text-sm">
          <thead className="bg-white/[0.02] text-left text-slate-400">
            <tr>
              <th className="px-5 py-3 font-medium">Run</th>
              <th className="px-5 py-3 font-medium">Status</th>
              <th className="px-5 py-3 font-medium">Duration</th>
              <th className="px-5 py-3 font-medium">Scraped</th>
              <th className="px-5 py-3 font-medium">Promising</th>
              <th className="px-5 py-3 font-medium">Emailed</th>
            </tr>
          </thead>
          <tbody>
            {recentRuns.map((run: RunSummary) => (
              <tr key={run.run_id} className="border-t border-white/5 text-slate-300 transition hover:bg-white/[0.03]">
                <td className="px-5 py-4">
                  <Link to={`/runs/${run.run_id}`} className="font-mono text-xs text-cyan-300 hover:text-cyan-200">
                    {run.run_id}
                  </Link>
                </td>
                <td className="px-5 py-4">
                  <StatusBadge status={run.status} />
                </td>
                <td className="px-5 py-4">{formatDuration(run.duration_ms)}</td>
                <td className="px-5 py-4">{run.stage_counts.all_jobs}</td>
                <td className="px-5 py-4">{run.stage_counts.promising}</td>
                <td className="px-5 py-4">{run.stage_counts.email_sent}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </DataTable>
    </div>
  );
}
