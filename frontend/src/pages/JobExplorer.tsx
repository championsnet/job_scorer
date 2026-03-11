import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { DataTable } from "../components/ui/DataTable";
import { EmptyState, ErrorState, LoadingState } from "../components/ui/QueryState";
import { PageHeader } from "../components/ui/PageHeader";
import { Surface } from "../components/ui/Surface";
import { formatScore } from "../lib/format";
import type { Job } from "../types";

const STAGES = [
  "all_jobs",
  "prefiltered",
  "evaluated",
  "promising",
  "final_evaluated",
  "notification",
  "validated_notification",
  "email_sent",
] as const;

const STAGE_LABELS: Record<string, string> = {
  all_jobs: "Scraped",
  prefiltered: "Prefiltered",
  evaluated: "LLM evaluated",
  promising: "Promising",
  final_evaluated: "CV evaluated",
  notification: "Notification",
  validated_notification: "Validated",
  email_sent: "Email sent",
};

function getPrimaryReason(job: Job): string {
  return (
    job.finalReasons?.[0] ||
    job.reasons?.[0] ||
    job.finalReason ||
    job.reason ||
    "No scoring rationale stored for this job."
  );
}

export default function JobExplorer() {
  const [runId, setRunId] = useState("");
  const [stage, setStage] = useState<(typeof STAGES)[number]>("all_jobs");
  const [search, setSearch] = useState("");
  const [selectedJobKey, setSelectedJobKey] = useState<string | null>(null);

  const { data: runs } = useQuery({
    queryKey: ["runs", 100],
    queryFn: () => api.listRuns(100),
  });

  const { data: jobsData, isLoading, error } = useQuery({
    queryKey: ["jobs", runId, stage],
    queryFn: () => api.getRunStageJobs(runId, stage),
    enabled: !!runId,
  });

  const jobs = jobsData?.jobs ?? [];
  const filteredJobs = useMemo(() => {
    if (!search) return jobs;
    const query = search.toLowerCase();
    return jobs.filter(
      (job) =>
        job.position?.toLowerCase().includes(query) ||
        job.company?.toLowerCase().includes(query) ||
        job.location?.toLowerCase().includes(query)
    );
  }, [jobs, search]);

  useEffect(() => {
    if (!filteredJobs.length) {
      setSelectedJobKey(null);
      return;
    }

    const hasSelected = filteredJobs.some((job) => (job.jobId || job.jobUrl) === selectedJobKey);
    if (!hasSelected) {
      setSelectedJobKey(filteredJobs[0].jobId || filteredJobs[0].jobUrl);
    }
  }, [filteredJobs, selectedJobKey]);

  const selectedJob =
    filteredJobs.find((job) => (job.jobId || job.jobUrl) === selectedJobKey) ?? filteredJobs[0];

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow="Explorer"
        title="Job explorer"
        description="Inspect jobs from any run and stage, then preview the underlying reasons and metadata without leaving the page."
      />

      <Surface className="p-5">
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
          <label className="space-y-2">
            <span className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Run</span>
            <select
              value={runId}
              onChange={(event) => setRunId(event.target.value)}
              className="w-full rounded-xl border border-white/10 bg-slate-950/70 px-3 py-2 text-sm text-slate-200 outline-none"
            >
              <option value="">Select run</option>
              {(runs?.runs ?? []).map((run) => (
                <option key={run.run_id} value={run.run_id}>
                  {run.run_id}
                </option>
              ))}
            </select>
          </label>

          <label className="space-y-2">
            <span className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Stage</span>
            <select
              value={stage}
              onChange={(event) => setStage(event.target.value as (typeof STAGES)[number])}
              className="w-full rounded-xl border border-white/10 bg-slate-950/70 px-3 py-2 text-sm text-slate-200 outline-none"
            >
              {STAGES.map((stageKey) => (
                <option key={stageKey} value={stageKey}>
                  {STAGE_LABELS[stageKey]}
                </option>
              ))}
            </select>
          </label>

          <label className="space-y-2 xl:col-span-2">
            <span className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Search</span>
            <input
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Search by position, company, or location"
              className="w-full rounded-xl border border-white/10 bg-slate-950/70 px-3 py-2 text-sm text-slate-200 outline-none placeholder:text-slate-500"
            />
          </label>
        </div>
      </Surface>

      {!runId ? (
        <EmptyState title="Select a run" description="Choose a run first to load stage-level job output." />
      ) : isLoading ? (
        <LoadingState title="Loading jobs" description="Fetching the selected run and stage payload." />
      ) : error ? (
        <ErrorState title="Jobs failed to load" description={String(error)} />
      ) : filteredJobs.length === 0 ? (
        <EmptyState
          title="No jobs matched"
          description={
            jobs.length === 0
              ? "This stage has no stored jobs."
              : "Try a broader search or switch to a different pipeline stage."
          }
        />
      ) : (
        <div className="grid gap-6 xl:grid-cols-[1.15fr_0.85fr]">
          <DataTable
            title="Stage jobs"
            description={`${STAGE_LABELS[stage]} stage for run ${runId}`}
            summary={`Showing ${filteredJobs.length} of ${jobs.length} jobs`}
          >
            <table className="min-w-full text-sm">
              <thead className="bg-white/2 text-left text-slate-400">
                <tr>
                  <th className="px-5 py-3 font-medium">Position</th>
                  <th className="px-5 py-3 font-medium">Company</th>
                  <th className="px-5 py-3 font-medium">Location</th>
                  <th className="px-5 py-3 font-medium">Score</th>
                  <th className="px-5 py-3 font-medium">Final</th>
                </tr>
              </thead>
              <tbody>
                {filteredJobs.map((job) => {
                  const key = job.jobId || job.jobUrl;
                  const selected = key === (selectedJob.jobId || selectedJob.jobUrl);
                  return (
                    <tr
                      key={key}
                      onClick={() => setSelectedJobKey(key)}
                      className={`cursor-pointer border-t border-white/5 text-slate-300 transition ${
                        selected ? "bg-cyan-400/8" : "hover:bg-white/3"
                      }`}
                    >
                      <td className="px-5 py-4">
                        <div className="font-medium text-white">{job.position}</div>
                        <div className="mt-1 text-xs text-slate-500">{getPrimaryReason(job).slice(0, 90)}</div>
                      </td>
                      <td className="px-5 py-4">{job.company}</td>
                      <td className="px-5 py-4">{job.location}</td>
                      <td className="px-5 py-4">{formatScore(job.score)}</td>
                      <td className="px-5 py-4">{formatScore(job.finalScore)}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </DataTable>

          <div className="xl:sticky xl:top-6 xl:self-start">
          <Surface className="p-5">
            <p className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Selected job</p>
            <h2 className="mt-3 text-xl font-semibold text-white">{selectedJob.position}</h2>
            <p className="mt-2 text-sm text-slate-400">
              {selectedJob.company} {selectedJob.location ? `| ${selectedJob.location}` : ""}
            </p>

            <div className="mt-5 grid grid-cols-2 gap-3">
              <div className="rounded-2xl bg-white/3 p-4">
                <p className="text-xs uppercase tracking-[0.18em] text-slate-500">Score</p>
                <p className="mt-2 text-lg font-semibold text-white">{formatScore(selectedJob.score)}</p>
              </div>
              <div className="rounded-2xl bg-white/3 p-4">
                <p className="text-xs uppercase tracking-[0.18em] text-slate-500">Final score</p>
                <p className="mt-2 text-lg font-semibold text-white">{formatScore(selectedJob.finalScore)}</p>
              </div>
            </div>

            <div className="mt-5 space-y-4">
              <div className="rounded-2xl bg-white/3 p-4">
                <p className="text-xs uppercase tracking-[0.18em] text-slate-500">Primary rationale</p>
                <p className="mt-2 text-sm leading-6 text-slate-300">{getPrimaryReason(selectedJob)}</p>
              </div>

              {selectedJob.jobDescription ? (
                <div className="rounded-2xl bg-white/3 p-4">
                  <p className="text-xs uppercase tracking-[0.18em] text-slate-500">Description</p>
                  <p className="mt-2 max-h-64 overflow-auto text-sm leading-6 text-slate-300">
                    {selectedJob.jobDescription}
                  </p>
                </div>
              ) : null}
            </div>

            {selectedJob.jobUrl ? (
              <a
                href={selectedJob.jobUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="mt-5 inline-flex rounded-xl border border-cyan-400/20 bg-cyan-400/10 px-4 py-2 text-sm font-medium text-cyan-200 transition hover:bg-cyan-400/15"
              >
                Open job posting
              </a>
            ) : null}
          </Surface>
          </div>
        </div>
      )}
    </div>
  );
}
