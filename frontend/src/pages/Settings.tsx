import { useEffect, useMemo, useState, type KeyboardEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import { ErrorState, LoadingState } from "../components/ui/QueryState";
import { PageHeader } from "../components/ui/PageHeader";
import { Surface } from "../components/ui/Surface";
import type { Policy } from "../types";

const MAX_JOBS_LIMIT = 1000;
const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/i;

const SCHEDULE_PRESETS = [
  { label: "Every hour", cron: "0 */1 * * *" },
  { label: "Every 2 hours", cron: "0 */2 * * *" },
  { label: "Every 4 hours", cron: "0 */4 * * *" },
  { label: "Every day", cron: "0 0 * * *" },
] as const;

const DATE_POSTED_OPTIONS = [
  "past hour",
  "past 2 hours",
  "past day",
  "past week",
  "past month",
  "past 24 hours",
] as const;

const FALLBACK_TIMEZONES = [
  "UTC",
  "Europe/Athens",
  "Europe/London",
  "Europe/Berlin",
  "America/New_York",
  "America/Los_Angeles",
  "Asia/Dubai",
  "Asia/Singapore",
  "Asia/Tokyo",
];

function getTimezoneOptions() {
  if (typeof Intl === "undefined" || typeof Intl.supportedValuesOf !== "function") {
    return FALLBACK_TIMEZONES;
  }
  const values = Intl.supportedValuesOf("timeZone");
  if (!values.includes("UTC")) {
    return ["UTC", ...values];
  }
  return values;
}

function formatBytes(bytes?: number) {
  if (bytes === undefined || Number.isNaN(bytes)) {
    return "-";
  }
  if (bytes < 1024) {
    return `${bytes} B`;
  }
  if (bytes < 1024 * 1024) {
    return `${(bytes / 1024).toFixed(1)} KB`;
  }
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function resolvePreset(cron: string): string {
  const preset = SCHEDULE_PRESETS.find((item) => item.cron === cron);
  return preset?.cron ?? SCHEDULE_PRESETS[0].cron;
}

function updatePolicy<K extends keyof Policy>(policy: Policy, key: K, value: Policy[K]): Policy {
  return { ...policy, [key]: value };
}

function FieldLabel({ label }: { label: string }) {
  return <span className="text-xs uppercase tracking-[0.18em] text-slate-500">{label}</span>;
}

function TextInput(props: {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
}) {
  return (
    <input
      value={props.value}
      onChange={(event) => props.onChange(event.target.value)}
      placeholder={props.placeholder}
      className="w-full rounded-xl border border-white/10 bg-slate-950/70 px-3 py-2 text-sm text-slate-200 outline-none"
    />
  );
}

function NumberInput(props: {
  value: number;
  onChange: (value: number) => void;
  min?: number;
  max?: number;
  step?: number;
}) {
  return (
    <input
      type="number"
      value={Number.isFinite(props.value) ? props.value : 0}
      min={props.min}
      max={props.max}
      step={props.step}
      onChange={(event) => props.onChange(Number(event.target.value))}
      className="w-full rounded-xl border border-white/10 bg-slate-950/70 px-3 py-2 text-sm text-slate-200 outline-none"
    />
  );
}

function ToggleSelect(props: {
  value: boolean;
  onChange: (value: boolean) => void;
}) {
  return (
    <select
      value={props.value ? "enabled" : "disabled"}
      onChange={(event) => props.onChange(event.target.value === "enabled")}
      className="w-full rounded-xl border border-white/10 bg-slate-950/70 px-3 py-2 text-sm text-slate-200 outline-none"
    >
      <option value="enabled">Enabled</option>
      <option value="disabled">Disabled</option>
    </select>
  );
}

function TextArea(props: {
  value: string;
  onChange: (value: string) => void;
  rows?: number;
  placeholder?: string;
}) {
  return (
    <textarea
      value={props.value}
      rows={props.rows ?? 4}
      onChange={(event) => props.onChange(event.target.value)}
      placeholder={props.placeholder}
      className="w-full rounded-xl border border-white/10 bg-slate-950/70 px-3 py-2 text-sm text-slate-200 outline-none"
    />
  );
}

function ChipListInput(props: {
  values: string[];
  onChange: (values: string[]) => void;
  placeholder: string;
  validate?: (value: string) => string | null;
}) {
  const [draft, setDraft] = useState("");
  const [error, setError] = useState<string | null>(null);

  const tryAdd = (raw: string) => {
    const value = raw.trim();
    if (!value) {
      return;
    }
    const validationError = props.validate?.(value) ?? null;
    if (validationError) {
      setError(validationError);
      return;
    }
    if (props.values.includes(value)) {
      setError("Already added.");
      return;
    }
    props.onChange([...props.values, value]);
    setDraft("");
    setError(null);
  };

  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "Enter" || event.key === "," || event.key === "Tab") {
      event.preventDefault();
      tryAdd(draft);
    } else if (event.key === "Backspace" && !draft && props.values.length > 0) {
      props.onChange(props.values.slice(0, -1));
    }
  };

  return (
    <div className="space-y-2">
      <div className="flex min-h-11 flex-wrap items-center gap-2 rounded-xl border border-white/10 bg-slate-950/70 p-2">
        {props.values.map((value) => (
          <span
            key={value}
            className="inline-flex items-center gap-2 rounded-full border border-cyan-300/30 bg-cyan-500/10 px-3 py-1 text-xs text-cyan-100"
          >
            {value}
            <button
              type="button"
              onClick={() => props.onChange(props.values.filter((item) => item !== value))}
              className="text-cyan-200 hover:text-white"
              aria-label={`Remove ${value}`}
            >
              ×
            </button>
          </span>
        ))}
        <input
          value={draft}
          onChange={(event) => {
            setDraft(event.target.value);
            setError(null);
          }}
          onBlur={() => tryAdd(draft)}
          onKeyDown={onKeyDown}
          placeholder={props.values.length === 0 ? props.placeholder : ""}
          className="min-w-[220px] flex-1 border-none bg-transparent px-1 py-1 text-sm text-slate-200 outline-none"
        />
      </div>
      {error ? <p className="text-xs text-rose-300">{error}</p> : null}
    </div>
  );
}

export default function Settings() {
  const timezoneOptions = useMemo(() => getTimezoneOptions(), []);
  const queryClient = useQueryClient();
  const { data, isLoading, error } = useQuery({
    queryKey: ["settings"],
    queryFn: api.getSettings,
  });
  const cvStatusQuery = useQuery({
    queryKey: ["settings", "cv"],
    queryFn: api.getCVStatus,
  });

  const [maxJobs, setMaxJobs] = useState(1000);
  const [scheduleCron, setScheduleCron] = useState("0 */1 * * *");
  const [schedulePreset, setSchedulePreset] = useState("0 */1 * * *");
  const [scheduleTimezone, setScheduleTimezone] = useState("UTC");
  const [scheduleEnabled, setScheduleEnabled] = useState(false);
  const [notificationEmails, setNotificationEmails] = useState<string[]>([]);
  const [policy, setPolicy] = useState<Policy | null>(null);
  const [saveMessage, setSaveMessage] = useState<string | null>(null);
  const [cvFile, setCvFile] = useState<File | null>(null);

  useEffect(() => {
    if (!data) {
      return;
    }
    const presetCron = resolvePreset(data.scheduleCron);
    setMaxJobs(Math.min(MAX_JOBS_LIMIT, data.maxJobs));
    setScheduleCron(presetCron);
    setSchedulePreset(presetCron);
    setScheduleTimezone(data.scheduleTimezone);
    setScheduleEnabled(data.scheduleEnabled);
    setNotificationEmails(data.notificationEmails ?? []);
    const detection = data.policy.filters.detectionLanguages ?? [];
    const primary = data.policy.filters.primaryLanguage;
    const normalizedDetection = detection.includes(primary) ? detection : [primary, ...detection];
    const normalizedDateSincePosted = DATE_POSTED_OPTIONS.includes(
      data.policy.scraper.dateSincePosted as (typeof DATE_POSTED_OPTIONS)[number],
    )
      ? data.policy.scraper.dateSincePosted
      : "past hour";
    setPolicy({
      ...data.policy,
      app: {
        ...data.policy.app,
        cronSchedule: presetCron,
      },
      filters: {
        ...data.policy.filters,
        detectionLanguages: normalizedDetection,
      },
      scraper: {
        ...data.policy.scraper,
        dateSincePosted: normalizedDateSincePosted,
      },
    });
  }, [data]);

  const saveMutation = useMutation({
    mutationFn: async () => {
      if (!policy) {
        throw new Error("Policy is not loaded yet.");
      }
      const normalizedDetectionLanguages = policy.filters.detectionLanguages.includes(policy.filters.primaryLanguage)
        ? policy.filters.detectionLanguages
        : [policy.filters.primaryLanguage, ...policy.filters.detectionLanguages];
      const normalizedPolicy: Policy = {
        ...policy,
        app: {
          ...policy.app,
          cronSchedule: scheduleCron,
        },
        filters: {
          ...policy.filters,
          detectionLanguages: normalizedDetectionLanguages,
        },
      };
      return api.updateSettings({
        policy: normalizedPolicy,
        maxJobs: Math.min(MAX_JOBS_LIMIT, Math.max(1, maxJobs)),
        scheduleCron,
        scheduleTimezone,
        scheduleEnabled,
        notificationEmails,
      });
    },
    onSuccess: () => {
      setSaveMessage("Settings saved.");
      queryClient.invalidateQueries({ queryKey: ["settings"] });
      queryClient.invalidateQueries({ queryKey: ["me"] });
    },
  });

  const uploadMutation = useMutation({
    mutationFn: async () => {
      if (!cvFile) {
        throw new Error("Select a CV file first.");
      }
      return api.uploadCV(cvFile);
    },
    onSuccess: () => {
      setSaveMessage("CV uploaded.");
      setCvFile(null);
      queryClient.invalidateQueries({ queryKey: ["settings"] });
      queryClient.invalidateQueries({ queryKey: ["settings", "cv"] });
    },
  });

  if (isLoading) {
    return <LoadingState title="Loading settings" description="Fetching account configuration and policy." />;
  }
  if (error) {
    return <ErrorState title="Settings failed to load" description={String(error)} />;
  }
  if (!policy) {
    return <LoadingState title="Loading policy" description="Preparing settings editor." />;
  }

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow="Account"
        title="Settings"
        description="Configure policy JSON, run cadence, job limits, notification recipients, and CV uploads."
      />

      <Surface className="p-5">
        <h2 className="text-lg font-semibold text-white">Execution controls</h2>
        <div className="mt-4 grid gap-4 md:grid-cols-2 xl:grid-cols-4">
          <label className="space-y-2">
            <FieldLabel label="Max jobs per run" />
            <NumberInput
              value={maxJobs}
              min={1}
              max={MAX_JOBS_LIMIT}
              onChange={(value) => setMaxJobs(Math.min(MAX_JOBS_LIMIT, Math.max(1, value)))}
            />
            <p className="text-xs text-slate-500">Limit: 1-{MAX_JOBS_LIMIT}</p>
          </label>
          <label className="space-y-2">
            <FieldLabel label="Schedule" />
            <select
              value={schedulePreset}
              onChange={(event) => {
                const value = event.target.value;
                setSchedulePreset(value);
                setScheduleCron(value);
              }}
              className="w-full rounded-xl border border-white/10 bg-slate-950/70 px-3 py-2 text-sm text-slate-200 outline-none"
            >
              {SCHEDULE_PRESETS.map((preset) => (
                <option key={preset.cron} value={preset.cron}>
                  {preset.label}
                </option>
              ))}
            </select>
          </label>
          <label className="space-y-2">
            <FieldLabel label="Timezone" />
            <select
              value={scheduleTimezone}
              onChange={(event) => setScheduleTimezone(event.target.value)}
              className="w-full rounded-xl border border-white/10 bg-slate-950/70 px-3 py-2 text-sm text-slate-200 outline-none"
            >
              {timezoneOptions.map((timezone) => (
                <option key={timezone} value={timezone}>
                  {timezone}
                </option>
              ))}
            </select>
          </label>
          <label className="space-y-2">
            <FieldLabel label="Scheduling" />
            <ToggleSelect value={scheduleEnabled} onChange={setScheduleEnabled} />
          </label>
        </div>
        <p className="mt-3 text-sm text-slate-400">
          Running:{" "}
          <span className="text-slate-200">
            {SCHEDULE_PRESETS.find((item) => item.cron === scheduleCron)?.label ?? "Every hour"}
          </span>
          {" "}in {scheduleTimezone}.
        </p>

        <label className="mt-4 block space-y-2">
          <FieldLabel label="Notification emails" />
          <ChipListInput
            values={notificationEmails}
            onChange={setNotificationEmails}
            placeholder="Type an email and press Enter"
            validate={(email) => (EMAIL_PATTERN.test(email) ? null : "Enter a valid email address.")}
          />
        </label>
      </Surface>

      <Surface className="p-5">
        <div className="flex items-start justify-between gap-4">
          <div>
            <h2 className="text-lg font-semibold text-white">Policy editor</h2>
            <p className="mt-1 text-sm text-slate-400">Edit every policy section with guardrails instead of raw JSON.</p>
          </div>
          <button
            type="button"
            onClick={() => saveMutation.mutate()}
            disabled={saveMutation.isPending}
            className="rounded-xl bg-cyan-400 px-4 py-2 text-sm font-semibold text-slate-950 transition hover:bg-cyan-300 disabled:cursor-not-allowed disabled:bg-slate-700 disabled:text-slate-400"
          >
            {saveMutation.isPending ? "Saving..." : "Save settings"}
          </button>
        </div>
        <div className="mt-4 space-y-6">
          <div className="grid gap-4 md:grid-cols-2">
            <label className="space-y-2">
              <FieldLabel label="Candidate target locations" />
              <ChipListInput
                values={policy.candidateProfile.targetLocations}
                onChange={(values) =>
                  setPolicy(updatePolicy(policy, "candidateProfile", { ...policy.candidateProfile, targetLocations: values }))
                }
                placeholder="Add preferred cities"
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Commute locations" />
              <ChipListInput
                values={policy.candidateProfile.commuteLocations}
                onChange={(values) =>
                  setPolicy(updatePolicy(policy, "candidateProfile", { ...policy.candidateProfile, commuteLocations: values }))
                }
                placeholder="Add nearby commute areas"
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Candidate languages" />
              <ChipListInput
                values={policy.candidateProfile.languages}
                onChange={(values) =>
                  setPolicy(updatePolicy(policy, "candidateProfile", { ...policy.candidateProfile, languages: values }))
                }
                placeholder="English"
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Desired fields" />
              <ChipListInput
                values={policy.candidateProfile.desiredFields}
                onChange={(values) =>
                  setPolicy(updatePolicy(policy, "candidateProfile", { ...policy.candidateProfile, desiredFields: values }))
                }
                placeholder="Marketing"
              />
            </label>
            <label className="space-y-2 md:col-span-2">
              <FieldLabel label="Allowed seniority levels" />
              <ChipListInput
                values={policy.candidateProfile.seniority}
                onChange={(values) =>
                  setPolicy(updatePolicy(policy, "candidateProfile", { ...policy.candidateProfile, seniority: values }))
                }
                placeholder="Junior"
              />
            </label>
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <label className="space-y-2">
              <FieldLabel label="LinkedIn job location IDs" />
              <ChipListInput
                values={policy.app.jobLocations}
                onChange={(values) => setPolicy(updatePolicy(policy, "app", { ...policy.app, jobLocations: values }))}
                placeholder="10000000"
              />
            </label>
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <label className="space-y-2">
              <FieldLabel label="Primary language" />
              <select
                value={policy.filters.primaryLanguage}
                onChange={(event) =>
                  setPolicy(updatePolicy(policy, "filters", { ...policy.filters, primaryLanguage: event.target.value }))
                }
                className="w-full rounded-xl border border-white/10 bg-slate-950/70 px-3 py-2 text-sm text-slate-200 outline-none"
              >
                {Array.from(new Set(policy.filters.detectionLanguages)).map((lang) => (
                  <option key={lang} value={lang}>
                    {lang}
                  </option>
                ))}
              </select>
            </label>
            <label className="space-y-2">
              <FieldLabel label="Detection languages" />
              <ChipListInput
                values={policy.filters.detectionLanguages}
                onChange={(values) => {
                  const normalized = values.length > 0 ? values : [policy.filters.primaryLanguage];
                  const withPrimary = normalized.includes(policy.filters.primaryLanguage)
                    ? normalized
                    : [policy.filters.primaryLanguage, ...normalized];
                  setPolicy(updatePolicy(policy, "filters", { ...policy.filters, detectionLanguages: withPrimary }));
                }}
                placeholder="english"
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Unwanted locations" />
              <ChipListInput
                values={policy.filters.unwantedLocations}
                onChange={(values) => setPolicy(updatePolicy(policy, "filters", { ...policy.filters, unwantedLocations: values }))}
                placeholder="EMEA"
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Unwanted title words" />
              <ChipListInput
                values={policy.filters.unwantedWordsInTitle}
                onChange={(values) => setPolicy(updatePolicy(policy, "filters", { ...policy.filters, unwantedWordsInTitle: values }))}
                placeholder="Senior"
              />
            </label>
            <label className="space-y-2 md:col-span-2">
              <FieldLabel label="Language red-flag keywords" />
              <ChipListInput
                values={policy.filters.redFlagLanguageKeywords}
                onChange={(values) =>
                  setPolicy(updatePolicy(policy, "filters", { ...policy.filters, redFlagLanguageKeywords: values }))
                }
                placeholder="german required"
              />
            </label>
            <label className="space-y-2 md:col-span-2">
              <FieldLabel label="Non-primary language keywords" />
              <ChipListInput
                values={policy.filters.nonPrimaryLanguageKeywords}
                onChange={(values) =>
                  setPolicy(updatePolicy(policy, "filters", { ...policy.filters, nonPrimaryLanguageKeywords: values }))
                }
                placeholder="stellenausschreibung"
              />
            </label>
            <label className="space-y-2 md:col-span-2">
              <FieldLabel label="Primary language indicators" />
              <ChipListInput
                values={policy.filters.primaryLanguageIndicators}
                onChange={(values) =>
                  setPolicy(updatePolicy(policy, "filters", { ...policy.filters, primaryLanguageIndicators: values }))
                }
                placeholder="english"
              />
            </label>
          </div>

          <div className="grid gap-4 md:grid-cols-3">
            <label className="space-y-2">
              <FieldLabel label="Min non-primary keyword count" />
              <NumberInput
                value={policy.filters.nonPrimaryKeywordMinCount}
                min={1}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "filters", { ...policy.filters, nonPrimaryKeywordMinCount: Math.max(1, value) }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Non-primary dominance threshold" />
              <NumberInput
                value={policy.filters.nonPrimaryDominanceThreshold}
                min={0}
                max={1}
                step={0.01}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "filters", { ...policy.filters, nonPrimaryDominanceThreshold: value }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Non-primary dominance ratio" />
              <NumberInput
                value={policy.filters.nonPrimaryDominanceRatio}
                min={0}
                step={0.01}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "filters", { ...policy.filters, nonPrimaryDominanceRatio: value }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Primary indicator min count" />
              <NumberInput
                value={policy.filters.primaryIndicatorMinCount}
                min={1}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "filters", { ...policy.filters, primaryIndicatorMinCount: Math.max(1, value) }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Primary indicator min confidence" />
              <NumberInput
                value={policy.filters.primaryIndicatorMinConfidence}
                min={0}
                max={1}
                step={0.01}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "filters", { ...policy.filters, primaryIndicatorMinConfidence: value }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Default primary threshold" />
              <NumberInput
                value={policy.filters.defaultPrimaryThreshold}
                min={0}
                max={1}
                step={0.01}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "filters", { ...policy.filters, defaultPrimaryThreshold: value }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Primary vs non-primary min delta" />
              <NumberInput
                value={policy.filters.primaryVsNonPrimaryMinDelta}
                min={0}
                max={1}
                step={0.01}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "filters", { ...policy.filters, primaryVsNonPrimaryMinDelta: value }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Min text length for language detect" />
              <NumberInput
                value={policy.filters.minTextLengthForLanguageDetect}
                min={1}
                onChange={(value) =>
                  setPolicy(
                    updatePolicy(policy, "filters", { ...policy.filters, minTextLengthForLanguageDetect: Math.max(1, value) }),
                  )
                }
              />
            </label>
          </div>

          <div className="grid gap-4">
            <label className="space-y-2">
              <FieldLabel label="Initial prompt template" />
              <TextArea
                value={policy.evaluation.initialPromptTemplate}
                rows={6}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "evaluation", { ...policy.evaluation, initialPromptTemplate: value }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Final prompt template" />
              <TextArea
                value={policy.evaluation.finalPromptTemplate}
                rows={6}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "evaluation", { ...policy.evaluation, finalPromptTemplate: value }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Batch prompt template" />
              <TextArea
                value={policy.evaluation.batchPromptTemplate}
                rows={6}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "evaluation", { ...policy.evaluation, batchPromptTemplate: value }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Validation prompt template" />
              <TextArea
                value={policy.evaluation.validationPromptTemplate}
                rows={5}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "evaluation", { ...policy.evaluation, validationPromptTemplate: value }))
                }
              />
            </label>
          </div>

          <div className="grid gap-4 md:grid-cols-4">
            <label className="space-y-2">
              <FieldLabel label="Max tokens (initial)" />
              <NumberInput
                value={policy.evaluation.maxTokens.initial}
                min={1}
                onChange={(value) =>
                  setPolicy(
                    updatePolicy(policy, "evaluation", {
                      ...policy.evaluation,
                      maxTokens: { ...policy.evaluation.maxTokens, initial: Math.max(1, value) },
                    }),
                  )
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Max tokens (final)" />
              <NumberInput
                value={policy.evaluation.maxTokens.final}
                min={1}
                onChange={(value) =>
                  setPolicy(
                    updatePolicy(policy, "evaluation", {
                      ...policy.evaluation,
                      maxTokens: { ...policy.evaluation.maxTokens, final: Math.max(1, value) },
                    }),
                  )
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Max tokens (batch)" />
              <NumberInput
                value={policy.evaluation.maxTokens.batch}
                min={1}
                onChange={(value) =>
                  setPolicy(
                    updatePolicy(policy, "evaluation", {
                      ...policy.evaluation,
                      maxTokens: { ...policy.evaluation.maxTokens, batch: Math.max(1, value) },
                    }),
                  )
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Max tokens (validation)" />
              <NumberInput
                value={policy.evaluation.maxTokens.validation}
                min={1}
                onChange={(value) =>
                  setPolicy(
                    updatePolicy(policy, "evaluation", {
                      ...policy.evaluation,
                      maxTokens: { ...policy.evaluation.maxTokens, validation: Math.max(1, value) },
                    }),
                  )
                }
              />
            </label>
          </div>

          <div className="grid gap-4 md:grid-cols-4">
            <label className="space-y-2">
              <FieldLabel label="CV prompt max length" />
              <NumberInput
                value={policy.evaluation.cvPromptTruncation.maxLength}
                min={1}
                onChange={(value) =>
                  setPolicy(
                    updatePolicy(policy, "evaluation", {
                      ...policy.evaluation,
                      cvPromptTruncation: { ...policy.evaluation.cvPromptTruncation, maxLength: Math.max(1, value) },
                    }),
                  )
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="CV prompt head length" />
              <NumberInput
                value={policy.evaluation.cvPromptTruncation.headLength}
                min={1}
                onChange={(value) =>
                  setPolicy(
                    updatePolicy(policy, "evaluation", {
                      ...policy.evaluation,
                      cvPromptTruncation: { ...policy.evaluation.cvPromptTruncation, headLength: Math.max(1, value) },
                    }),
                  )
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="CV prompt tail length" />
              <NumberInput
                value={policy.evaluation.cvPromptTruncation.tailLength}
                min={1}
                onChange={(value) =>
                  setPolicy(
                    updatePolicy(policy, "evaluation", {
                      ...policy.evaluation,
                      cvPromptTruncation: { ...policy.evaluation.cvPromptTruncation, tailLength: Math.max(1, value) },
                    }),
                  )
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Validation max size" />
              <NumberInput
                value={policy.evaluation.cvPromptTruncation.validationMaxSize}
                min={1}
                onChange={(value) =>
                  setPolicy(
                    updatePolicy(policy, "evaluation", {
                      ...policy.evaluation,
                      cvPromptTruncation: { ...policy.evaluation.cvPromptTruncation, validationMaxSize: Math.max(1, value) },
                    }),
                  )
                }
              />
            </label>
          </div>

          <div className="grid gap-4 md:grid-cols-3">
            <label className="space-y-2">
              <FieldLabel label="Promising score threshold (0-10)" />
              <NumberInput
                value={policy.pipeline.promisingScoreThreshold}
                min={0}
                max={10}
                step={0.1}
                onChange={(value) =>
                  setPolicy(
                    updatePolicy(policy, "pipeline", {
                      ...policy.pipeline,
                      promisingScoreThreshold: Math.max(0, Math.min(10, value)),
                    }),
                  )
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Batch size" />
              <NumberInput
                value={policy.pipeline.batchSize}
                min={1}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "pipeline", { ...policy.pipeline, batchSize: Math.max(1, value) }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Fallback max jobs" />
              <NumberInput
                value={policy.pipeline.individualFallbackMaxJobs}
                min={1}
                onChange={(value) =>
                  setPolicy(
                    updatePolicy(policy, "pipeline", { ...policy.pipeline, individualFallbackMaxJobs: Math.max(1, value) }),
                  )
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Enable final validation" />
              <ToggleSelect
                value={policy.pipeline.enableFinalValidation}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "pipeline", { ...policy.pipeline, enableFinalValidation: value }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Reject empty descriptions" />
              <ToggleSelect
                value={policy.pipeline.rejectEmptyDescriptions}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "pipeline", { ...policy.pipeline, rejectEmptyDescriptions: value }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Reject placeholder descriptions" />
              <ToggleSelect
                value={policy.pipeline.rejectPlaceholderDescription}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "pipeline", { ...policy.pipeline, rejectPlaceholderDescription: value }))
                }
              />
            </label>
            <label className="space-y-2 md:col-span-3">
              <FieldLabel label="Pipeline red flag phrases" />
              <ChipListInput
                values={policy.pipeline.redFlagPhrases}
                onChange={(values) => setPolicy(updatePolicy(policy, "pipeline", { ...policy.pipeline, redFlagPhrases: values }))}
                placeholder="german required"
              />
            </label>
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <label className="space-y-2">
              <FieldLabel label="Notification min final score (0-10)" />
              <NumberInput
                value={policy.notification.minFinalScore}
                min={0}
                max={10}
                step={0.1}
                onChange={(value) =>
                  setPolicy(
                    updatePolicy(policy, "notification", {
                      ...policy.notification,
                      minFinalScore: Math.max(0, Math.min(10, value)),
                    }),
                  )
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Email subject template" />
              <TextInput
                value={policy.notification.emailSubjectTemplate}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "notification", { ...policy.notification, emailSubjectTemplate: value }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Header title" />
              <TextInput
                value={policy.notification.headerTitle}
                onChange={(value) => setPolicy(updatePolicy(policy, "notification", { ...policy.notification, headerTitle: value }))}
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Header subtitle" />
              <TextInput
                value={policy.notification.headerSubtitle}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "notification", { ...policy.notification, headerSubtitle: value }))
                }
              />
            </label>
            <label className="space-y-2 md:col-span-2">
              <FieldLabel label="Summary template" />
              <TextArea
                value={policy.notification.summaryTemplate}
                rows={3}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "notification", { ...policy.notification, summaryTemplate: value }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Description title" />
              <TextInput
                value={policy.notification.descriptionTitle}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "notification", { ...policy.notification, descriptionTitle: value }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Reason title" />
              <TextInput
                value={policy.notification.reasonTitle}
                onChange={(value) => setPolicy(updatePolicy(policy, "notification", { ...policy.notification, reasonTitle: value }))}
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Apply button text" />
              <TextInput
                value={policy.notification.applyButtonText}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "notification", { ...policy.notification, applyButtonText: value }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Personalized title" />
              <TextInput
                value={policy.notification.personalizedTitle}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "notification", { ...policy.notification, personalizedTitle: value }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Personalized subtitle" />
              <TextInput
                value={policy.notification.personalizedSubtitle}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "notification", { ...policy.notification, personalizedSubtitle: value }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Footer line one" />
              <TextInput
                value={policy.notification.footerLineOne}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "notification", { ...policy.notification, footerLineOne: value }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Footer line two template" />
              <TextInput
                value={policy.notification.footerLineTwoTemplate}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "notification", { ...policy.notification, footerLineTwoTemplate: value }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Footer line three" />
              <TextInput
                value={policy.notification.footerLineThree}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "notification", { ...policy.notification, footerLineThree: value }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Description preview limit" />
              <NumberInput
                value={policy.notification.jobDescriptionPreviewLimit}
                min={1}
                onChange={(value) =>
                  setPolicy(
                    updatePolicy(policy, "notification", {
                      ...policy.notification,
                      jobDescriptionPreviewLimit: Math.max(1, value),
                    }),
                  )
                }
              />
            </label>
          </div>

          <div className="grid gap-4 md:grid-cols-3">
            <label className="space-y-2 md:col-span-3">
              <FieldLabel label="CV parser order" />
              <ChipListInput
                values={policy.cv.parserOrder}
                onChange={(values) => setPolicy(updatePolicy(policy, "cv", { ...policy.cv, parserOrder: values }))}
                placeholder="unipdf"
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Enable UniPDF" />
              <ToggleSelect
                value={policy.cv.enableUniPDF}
                onChange={(value) => setPolicy(updatePolicy(policy, "cv", { ...policy.cv, enableUniPDF: value }))}
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Allow env override UniPDF" />
              <ToggleSelect
                value={policy.cv.allowEnvOverrideUniPDF}
                onChange={(value) => setPolicy(updatePolicy(policy, "cv", { ...policy.cv, allowEnvOverrideUniPDF: value }))}
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Log parser used" />
              <ToggleSelect
                value={policy.cv.logParserUsed}
                onChange={(value) => setPolicy(updatePolicy(policy, "cv", { ...policy.cv, logParserUsed: value }))}
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Validate by heuristic" />
              <ToggleSelect
                value={policy.cv.validateCandidateByHeuristic}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "cv", { ...policy.cv, validateCandidateByHeuristic: value }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Min valid text length" />
              <NumberInput
                value={policy.cv.minValidTextLength}
                min={1}
                onChange={(value) => setPolicy(updatePolicy(policy, "cv", { ...policy.cv, minValidTextLength: Math.max(1, value) }))}
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Min letter ratio" />
              <NumberInput
                value={policy.cv.minLetterRatio}
                min={0}
                max={1}
                step={0.01}
                onChange={(value) => setPolicy(updatePolicy(policy, "cv", { ...policy.cv, minLetterRatio: value }))}
              />
            </label>
            <label className="space-y-2 md:col-span-3">
              <FieldLabel label="CV fallback text" />
              <TextArea
                value={policy.cv.fallbackText}
                rows={3}
                onChange={(value) => setPolicy(updatePolicy(policy, "cv", { ...policy.cv, fallbackText: value }))}
              />
            </label>
          </div>

          <div className="grid gap-4 md:grid-cols-3">
            <label className="space-y-2">
              <FieldLabel label="Posted within" />
              <select
                value={policy.scraper.dateSincePosted}
                onChange={(event) =>
                  setPolicy(updatePolicy(policy, "scraper", { ...policy.scraper, dateSincePosted: event.target.value }))
                }
                className="w-full rounded-xl border border-white/10 bg-slate-950/70 px-3 py-2 text-sm text-slate-200 outline-none"
              >
                {DATE_POSTED_OPTIONS.map((option) => (
                  <option key={option} value={option}>
                    {option}
                  </option>
                ))}
              </select>
            </label>
            <label className="space-y-2">
              <FieldLabel label="Pagination batch size" />
              <NumberInput
                value={policy.scraper.paginationBatchSize}
                min={1}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "scraper", { ...policy.scraper, paginationBatchSize: Math.max(1, value) }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Max consecutive errors" />
              <NumberInput
                value={policy.scraper.maxConsecutiveErrors}
                min={1}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "scraper", { ...policy.scraper, maxConsecutiveErrors: Math.max(1, value) }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Max request retries" />
              <NumberInput
                value={policy.scraper.maxRequestRetries}
                min={1}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "scraper", { ...policy.scraper, maxRequestRetries: Math.max(1, value) }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Consecutive backoff base seconds" />
              <NumberInput
                value={policy.scraper.consecutiveBackoffBaseSeconds}
                min={1}
                onChange={(value) =>
                  setPolicy(
                    updatePolicy(policy, "scraper", { ...policy.scraper, consecutiveBackoffBaseSeconds: Math.max(1, value) }),
                  )
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Retry backoff base seconds" />
              <NumberInput
                value={policy.scraper.retryBackoffBaseSeconds}
                min={1}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "scraper", { ...policy.scraper, retryBackoffBaseSeconds: Math.max(1, value) }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Retry jitter max ms" />
              <NumberInput
                value={policy.scraper.retryJitterMaxMs}
                min={0}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "scraper", { ...policy.scraper, retryJitterMaxMs: Math.max(0, value) }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Inter-batch delay min ms" />
              <NumberInput
                value={policy.scraper.interBatchDelayMinMs}
                min={0}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "scraper", { ...policy.scraper, interBatchDelayMinMs: Math.max(0, value) }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Inter-batch delay max ms" />
              <NumberInput
                value={policy.scraper.interBatchDelayMaxMs}
                min={0}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "scraper", { ...policy.scraper, interBatchDelayMaxMs: Math.max(0, value) }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Description delay min ms" />
              <NumberInput
                value={policy.scraper.descriptionDelayMinMs}
                min={0}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "scraper", { ...policy.scraper, descriptionDelayMinMs: Math.max(0, value) }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Description delay max ms" />
              <NumberInput
                value={policy.scraper.descriptionDelayMaxMs}
                min={0}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "scraper", { ...policy.scraper, descriptionDelayMaxMs: Math.max(0, value) }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Description max length" />
              <NumberInput
                value={policy.scraper.descriptionMaxLength}
                min={1}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "scraper", { ...policy.scraper, descriptionMaxLength: Math.max(1, value) }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Error body preview length" />
              <NumberInput
                value={policy.scraper.errorBodyPreviewLength}
                min={1}
                onChange={(value) =>
                  setPolicy(updatePolicy(policy, "scraper", { ...policy.scraper, errorBodyPreviewLength: Math.max(1, value) }))
                }
              />
            </label>
            <label className="space-y-2">
              <FieldLabel label="Verbose parse logs" />
              <ToggleSelect
                value={policy.scraper.verboseParseLogs}
                onChange={(value) => setPolicy(updatePolicy(policy, "scraper", { ...policy.scraper, verboseParseLogs: value }))}
              />
            </label>
            <label className="space-y-2 md:col-span-3">
              <FieldLabel label="CSS selectors" />
              <ChipListInput
                values={policy.scraper.selectors}
                onChange={(values) => setPolicy(updatePolicy(policy, "scraper", { ...policy.scraper, selectors: values }))}
                placeholder=".job-search-card"
              />
            </label>
          </div>
        </div>
        <div className="mt-6 rounded-xl border border-white/10 bg-slate-950/70 p-3">
          <p className="mb-2 text-xs uppercase tracking-[0.18em] text-slate-500">Live policy JSON preview</p>
          <pre className="max-h-72 overflow-auto text-xs text-slate-300">{JSON.stringify(policy, null, 2)}</pre>
        </div>
        {saveMutation.error ? <p className="mt-3 text-sm text-rose-300">{String(saveMutation.error)}</p> : null}
      </Surface>

      <Surface className="p-5">
        <h2 className="text-lg font-semibold text-white">CV upload</h2>
        <p className="mt-1 text-sm text-slate-400">Upload a PDF, Markdown, or text CV for account-scoped scoring.</p>
        <div className="mt-3 rounded-xl border border-white/10 bg-slate-950/70 px-3 py-2 text-sm text-slate-300">
          {cvStatusQuery.isLoading ? (
            <span>Checking uploaded CV status...</span>
          ) : cvStatusQuery.data?.hasCV ? (
            <div className="space-y-1">
              <span>
                Active CV is uploaded ({formatBytes(cvStatusQuery.data.sizeBytes)}), created{" "}
                {cvStatusQuery.data.createdAt ? new Date(cvStatusQuery.data.createdAt).toLocaleString() : "-"}.
              </span>
              {cvStatusQuery.data.textExtracted === false && (
                <p className="text-xs text-amber-400">
                  Text could not be extracted from this PDF — the evaluator will use fallback text. Try re-uploading as a plain-text (.txt) or Markdown (.md) CV for best results.
                </p>
              )}
            </div>
          ) : (
            <span>No CV uploaded yet.</span>
          )}
        </div>
        <div className="mt-4 flex flex-wrap items-center gap-3">
          <input
            type="file"
            accept=".pdf,.txt,.md,.markdown"
            onChange={(event) => setCvFile(event.target.files?.[0] ?? null)}
            className="rounded-xl border border-white/10 bg-slate-950/70 px-3 py-2 text-sm text-slate-300"
          />
          <button
            type="button"
            onClick={() => uploadMutation.mutate()}
            disabled={uploadMutation.isPending}
            className="rounded-xl border border-white/15 bg-white/4 px-4 py-2 text-sm font-medium text-slate-200 transition hover:bg-white/8 disabled:cursor-not-allowed disabled:text-slate-500"
          >
            {uploadMutation.isPending ? "Uploading..." : "Upload CV"}
          </button>
        </div>
        {cvFile ? <p className="mt-2 text-xs text-slate-400">Selected file: {cvFile.name}</p> : null}
        {uploadMutation.error ? <p className="mt-3 text-sm text-rose-300">{String(uploadMutation.error)}</p> : null}
      </Surface>

      {saveMessage ? <p className="text-sm text-emerald-300">{saveMessage}</p> : null}
    </div>
  );
}
