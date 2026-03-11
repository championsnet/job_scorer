import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { api } from "../api/client";
import { DataTable } from "../components/ui/DataTable";
import { EmptyState, ErrorState, LoadingState } from "../components/ui/QueryState";
import { PageHeader } from "../components/ui/PageHeader";
import { Surface } from "../components/ui/Surface";
import { formatScore } from "../lib/format";
import type { Job } from "../types";

type StageViewConfig = {
  title: string;
  queryStage: string;
  description: string;
};

const STAGE_VIEWS: Record<string, StageViewConfig> = {
  all_jobs: {
    title: "Scraped",
    queryStage: "all_jobs",
    description: "Raw scraped jobs with no exclusions applied yet.",
  },
  prefiltered: {
    title: "Prefiltered",
    queryStage: "prefiltered",
    description: "Shows kept jobs plus greyed-out jobs removed by duplicate detection or prefiltering.",
  },
  evaluated: {
    title: "LLM evaluated",
    queryStage: "promising",
    description: "Shows evaluated jobs, with non-promising jobs greyed out in the same table.",
  },
  promising: {
    title: "Promising",
    queryStage: "promising",
    description: "Shows promising jobs and the evaluated jobs that missed the threshold.",
  },
  final_evaluated: {
    title: "CV evaluated",
    queryStage: "notification",
    description: "Shows CV-evaluated jobs and greys out the ones that did not qualify for notification.",
  },
  notification: {
    title: "Notification",
    queryStage: "notification",
    description: "Shows notification-eligible jobs and why the rest were excluded after CV evaluation.",
  },
  validated_notification: {
    title: "Validated",
    queryStage: "validated_notification",
    description: "Shows validated jobs and the notification jobs rejected in the validation step.",
  },
  email_sent: {
    title: "Email sent",
    queryStage: "validated_notification",
    description: "Email uses the same validated set, so this mirrors validated jobs and validation exclusions.",
  },
};

const STAGE_ORDER = [
  "all_jobs",
  "prefiltered",
  "evaluated",
  "promising",
  "final_evaluated",
  "notification",
  "validated_notification",
  "email_sent",
] as const;

function getReason(job: Job): string {
  if (job.excluded) {
    return job.exclusionReason || "Excluded in this step";
  }
  return job.finalReasons?.[0] || job.reasons?.[0] || job.finalReason || job.reason || "-";
}

export default function StageJobs() {
  const { runId, stage } = useParams<{ runId: string; stage: string }>();
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const view = STAGE_VIEWS[stage || ""] ?? {
    title: stage || "Stage",
    queryStage: stage || "",
    description: "Stage view",
  };
  const currentStageIndex = STAGE_ORDER.findIndex((item) => item === stage);
  const previousStage = currentStageIndex > 0 ? STAGE_ORDER[currentStageIndex - 1] : null;
  const nextStage =
    currentStageIndex >= 0 && currentStageIndex < STAGE_ORDER.length - 1
      ? STAGE_ORDER[currentStageIndex + 1]
      : null;

  const { data, isLoading, error } = useQuery({
    queryKey: ["runStage", runId, stage, view.queryStage],
    queryFn: () => api.getRunStageJobs(runId!, view.queryStage),
    enabled: !!runId && !!view.queryStage,
  });

  const shouldUsePrefilterFallback =
    view.queryStage === "prefiltered" &&
    !!runId &&
    !!data &&
    (data.included_jobs?.length ?? data.jobs?.length ?? 0) === 0 &&
    (data.excluded_jobs?.length ?? 0) === 0;

  const { data: allJobsFallback, isLoading: isFallbackLoading } = useQuery({
    queryKey: ["runStageFallback", runId, "all_jobs"],
    queryFn: () => api.getRunStageJobs(runId!, "all_jobs"),
    enabled: shouldUsePrefilterFallback,
  });

  const includedJobs = useMemo(
    () => (data?.included_jobs ?? data?.jobs ?? []).map((job) => ({ ...job, excluded: false })),
    [data]
  );
  const excludedJobs = useMemo(
    () => (data?.excluded_jobs ?? []).map((job) => ({ ...job, excluded: true })),
    [data]
  );
  const fallbackExcludedJobs = useMemo(
    () =>
      shouldUsePrefilterFallback
        ? (allJobsFallback?.jobs ?? []).map((job) => ({
            ...job,
            excluded: true,
            exclusionReason: "Excluded during prefiltering",
          }))
        : [],
    [allJobsFallback, shouldUsePrefilterFallback]
  );

  const effectiveExcludedJobs = excludedJobs.length > 0 ? excludedJobs : fallbackExcludedJobs;
  const rows = [...includedJobs, ...effectiveExcludedJobs];
  const baseCount = data?.base_count ?? rows.length;
  const keptCount = data?.included_count ?? includedJobs.length;

  useEffect(() => {
    if (!rows.length) {
      setSelectedKey(null);
      return;
    }

    const hasSelected = rows.some((job) => (job.jobId || job.jobUrl) === selectedKey);
    if (!hasSelected) {
      setSelectedKey(rows[0].jobId || rows[0].jobUrl);
    }
  }, [rows, selectedKey]);

  const selectedJob = rows.find((job) => (job.jobId || job.jobUrl) === selectedKey) ?? rows[0];

  if (isLoading || isFallbackLoading) {
    return <LoadingState title="Loading stage jobs" description="Fetching kept and excluded jobs for this step." />;
  }

  if (error) {
    return <ErrorState title="Stage jobs failed to load" description={String(error)} />;
  }

  if (!rows.length) {
    return (
      <div className="space-y-8">
        <PageHeader
          eyebrow="Stage view"
          title={`${view.title} jobs`}
          description={
            <Link to={`/runs/${runId}`} className="text-cyan-300 hover:text-cyan-200">
              Back to run detail
            </Link>
          }
          actions={
            <>
              {previousStage ? (
                <Link
                  to={`/runs/${runId}/stages/${previousStage}`}
                  className="rounded-xl border border-white/15 bg-white/3 px-3 py-2 text-sm text-slate-200 transition hover:border-cyan-400/30 hover:bg-cyan-400/10"
                >
                  Previous: {STAGE_VIEWS[previousStage]?.title ?? previousStage}
                </Link>
              ) : (
                <span className="rounded-xl border border-white/10 bg-slate-900/70 px-3 py-2 text-sm text-slate-500">
                  First stage
                </span>
              )}
              {nextStage ? (
                <Link
                  to={`/runs/${runId}/stages/${nextStage}`}
                  className="rounded-xl border border-cyan-400/25 bg-cyan-400/10 px-3 py-2 text-sm text-cyan-200 transition hover:bg-cyan-400/15"
                >
                  Next: {STAGE_VIEWS[nextStage]?.title ?? nextStage}
                </Link>
              ) : (
                <span className="rounded-xl border border-white/10 bg-slate-900/70 px-3 py-2 text-sm text-slate-500">
                  Last stage
                </span>
              )}
            </>
          }
        />
        <EmptyState title="No jobs stored for this stage" description="The selected stage does not have persisted jobs for this run." />
      </div>
    );
  }

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow="Stage view"
        title={`${view.title} jobs`}
        description={
          <div className="space-y-2">
            <div className="flex flex-wrap items-center gap-3">
              <Link to={`/runs/${runId}`} className="text-cyan-300 hover:text-cyan-200">
                Back to run detail
              </Link>
              <span className="text-sm text-slate-500">
                Kept {keptCount} of {baseCount} jobs in this view
              </span>
            </div>
            <p className="text-sm text-slate-500">{view.description}</p>
          </div>
        }
        actions={
          <>
            {previousStage ? (
              <Link
                to={`/runs/${runId}/stages/${previousStage}`}
                className="rounded-xl border border-white/15 bg-white/3 px-3 py-2 text-sm text-slate-200 transition hover:border-cyan-400/30 hover:bg-cyan-400/10"
              >
                Previous: {STAGE_VIEWS[previousStage]?.title ?? previousStage}
              </Link>
            ) : (
              <span className="rounded-xl border border-white/10 bg-slate-900/70 px-3 py-2 text-sm text-slate-500">
                First stage
              </span>
            )}
            {nextStage ? (
              <Link
                to={`/runs/${runId}/stages/${nextStage}`}
                className="rounded-xl border border-cyan-400/25 bg-cyan-400/10 px-3 py-2 text-sm text-cyan-200 transition hover:bg-cyan-400/15"
              >
                Next: {STAGE_VIEWS[nextStage]?.title ?? nextStage}
              </Link>
            ) : (
              <span className="rounded-xl border border-white/10 bg-slate-900/70 px-3 py-2 text-sm text-slate-500">
                Last stage
              </span>
            )}
          </>
        }
      />

      <div className="grid gap-6 xl:grid-cols-[1.2fr_0.8fr]">
        <DataTable
          title="Stage comparison"
          description={`Run ${runId}`}
          summary={`${includedJobs.length} kept, ${effectiveExcludedJobs.length} excluded`}
        >
          <table className="min-w-full text-sm">
            <thead className="bg-white/2 text-left text-slate-400">
              <tr>
                <th className="px-5 py-3 font-medium">Position</th>
                <th className="px-5 py-3 font-medium">Company</th>
                <th className="px-5 py-3 font-medium">Location</th>
                <th className="px-5 py-3 font-medium">Score</th>
                <th className="px-5 py-3 font-medium">Final</th>
                <th className="px-5 py-3 font-medium">Decision</th>
                <th className="px-5 py-3 font-medium">Reason</th>
              </tr>
            </thead>
            <tbody>
              {includedJobs.map((job) => {
                const key = job.jobId || job.jobUrl;
                const selected = key === (selectedJob.jobId || selectedJob.jobUrl);
                return (
                  <tr
                    key={key}
                    onClick={() => setSelectedKey(key)}
                    className={`cursor-pointer border-t border-white/5 text-slate-300 transition ${
                      selected ? "bg-cyan-400/8" : "hover:bg-white/3"
                    }`}
                  >
                    <td className="px-5 py-4 font-medium text-white">{job.position}</td>
                    <td className="px-5 py-4">{job.company}</td>
                    <td className="px-5 py-4">{job.location}</td>
                    <td className="px-5 py-4">{formatScore(job.score)}</td>
                    <td className="px-5 py-4">{formatScore(job.finalScore)}</td>
                    <td className="px-5 py-4 text-emerald-300">Kept</td>
                    <td className="px-5 py-4 text-slate-400">{getReason(job)}</td>
                  </tr>
                );
              })}

              {effectiveExcludedJobs.length > 0 ? (
                <tr className="border-t border-white/10 bg-slate-950/70">
                  <td colSpan={7} className="px-5 py-3 text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">
                    Excluded in this step
                  </td>
                </tr>
              ) : null}

              {effectiveExcludedJobs.map((job) => {
                const key = job.jobId || job.jobUrl;
                const selected = key === (selectedJob.jobId || selectedJob.jobUrl);
                return (
                  <tr
                    key={key}
                    onClick={() => setSelectedKey(key)}
                    className={`cursor-pointer border-t border-white/5 text-slate-500 transition ${
                      selected ? "bg-slate-800/80" : "bg-slate-900/55 hover:bg-slate-800/70"
                    }`}
                  >
                    <td className="px-5 py-4 font-medium text-slate-300">{job.position}</td>
                    <td className="px-5 py-4">{job.company}</td>
                    <td className="px-5 py-4">{job.location}</td>
                    <td className="px-5 py-4">{formatScore(job.score)}</td>
                    <td className="px-5 py-4">{formatScore(job.finalScore)}</td>
                    <td className="px-5 py-4 text-slate-400">Excluded</td>
                    <td className="px-5 py-4 text-slate-400">{getReason(job)}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </DataTable>

        <div className="xl:sticky xl:top-6 xl:self-start">
        <Surface className="p-5">
          <p className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Selected job</p>
          <div className="mt-3 flex items-start justify-between gap-3">
            <div>
              <h2 className="text-xl font-semibold text-white">{selectedJob.position}</h2>
              <p className="mt-2 text-sm text-slate-400">
                {selectedJob.company} {selectedJob.location ? `| ${selectedJob.location}` : ""}
              </p>
            </div>
            <span
              className={`rounded-full px-3 py-1 text-xs font-semibold ${
                selectedJob.excluded
                  ? "bg-slate-700 text-slate-300"
                  : "bg-emerald-500/15 text-emerald-300"
              }`}
            >
              {selectedJob.excluded ? "Excluded" : "Kept"}
            </span>
          </div>

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

          <div className="mt-5 rounded-2xl bg-white/3 p-4">
            <p className="text-xs uppercase tracking-[0.18em] text-slate-500">
              {selectedJob.excluded ? "Exclusion reason" : "Reason"}
            </p>
            <p className="mt-2 text-sm leading-6 text-slate-300">{getReason(selectedJob)}</p>
          </div>

          {selectedJob.jobDescription ? (
            <div className="mt-4 rounded-2xl bg-white/3 p-4">
              <p className="text-xs uppercase tracking-[0.18em] text-slate-500">Description</p>
              <p className="mt-2 max-h-64 overflow-auto text-sm leading-6 text-slate-300">
                {selectedJob.jobDescription}
              </p>
            </div>
          ) : null}

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
    </div>
  );
}
