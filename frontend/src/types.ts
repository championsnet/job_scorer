export type RunStatus = 'queued' | 'running' | 'success' | 'failed';

export interface RunStageCounts {
  all_jobs: number;
  prefiltered: number;
  evaluated: number;
  promising: number;
  final_evaluated: number;
  notification: number;
  validated: number;
  email_sent: number;
  duplicates_removed?: number;
}

export interface LLMUsageSnapshot {
  calls: number;
  input_tokens: number;
  cached_input_tokens: number;
  non_cached_input_tokens: number;
  billable_input_tokens: number;
  output_tokens: number;
  total_tokens: number;
}

export interface RunConfigSnapshot {
  locations: string[];
  max_jobs: number;
}

export interface NotificationResult {
  run_id: string;
  job_ids: string[];
  recipients_count: number;
  success_count: number;
  failed_count: number;
  completed_at: string;
  error_message?: string;
}

export interface RunSummary {
  run_id: string;
  status: RunStatus;
  started_at: string;
  completed_at?: string;
  duration_ms: number;
  stage_counts: RunStageCounts;
  config: RunConfigSnapshot;
  llm_usage: LLMUsageSnapshot;
  notification?: NotificationResult;
  error_message?: string;
}

export interface Job {
  jobId?: string;
  position: string;
  company: string;
  location: string;
  date?: string;
  salary?: string;
  jobUrl: string;
  companyLogo?: string;
  agoTime?: string;
  score?: number;
  reason?: string;
  reasons?: string[];
  excluded?: boolean;
  exclusionReason?: string;
  jobDescription?: string;
  finalScore?: number;
  finalReason?: string;
  finalReasons?: string[];
  createdAt?: string;
}

export interface StageJobsResponse {
  jobs: Job[];
  count: number;
  included_jobs?: Job[];
  included_count?: number;
  excluded_jobs?: Job[];
  excluded_count?: number;
  base_count?: number;
}

export interface AnalyticsOverview {
  runs: {
    total: number;
    success: number;
    failed: number;
    avg_duration_ms: number;
  };
  funnel: {
    all_jobs: number;
    prefiltered: number;
    evaluated: number;
    promising: number;
    final_evaluated: number;
    notification: number;
    validated: number;
    email_sent: number;
  };
  llm_usage: {
    calls: number;
    input_tokens: number;
    output_tokens: number;
  };
  recent_runs?: RunSummary[] | null;
}

export interface Policy {
  candidateProfile: {
    targetLocations: string[];
    commuteLocations: string[];
    languages: string[];
    desiredFields: string[];
    seniority: string[];
  };
  app: {
    cronSchedule: string;
    jobLocations: string[];
  };
  filters: {
    unwantedLocations: string[];
    unwantedWordsInTitle: string[];
    primaryLanguage: string;
    detectionLanguages: string[];
    redFlagLanguageKeywords: string[];
    requiredLanguageKeywords?: string[];
    nonPrimaryLanguageKeywords: string[];
    primaryLanguageIndicators: string[];
    nonPrimaryKeywordMinCount: number;
    nonPrimaryDominanceThreshold: number;
    nonPrimaryDominanceRatio: number;
    primaryIndicatorMinCount: number;
    primaryIndicatorMinConfidence: number;
    defaultPrimaryThreshold: number;
    primaryVsNonPrimaryMinDelta: number;
    minTextLengthForLanguageDetect: number;
  };
  evaluation: {
    initialPromptTemplate: string;
    finalPromptTemplate: string;
    batchPromptTemplate: string;
    validationPromptTemplate: string;
    maxTokens: {
      initial: number;
      final: number;
      batch: number;
      validation: number;
    };
    cvPromptTruncation: {
      maxLength: number;
      headLength: number;
      tailLength: number;
      validationMaxSize: number;
    };
  };
  pipeline: {
    promisingScoreThreshold: number;
    batchSize: number;
    individualFallbackMaxJobs: number;
    enableFinalValidation: boolean;
    rejectEmptyDescriptions: boolean;
    rejectPlaceholderDescription: boolean;
    redFlagPhrases: string[];
  };
  notification: {
    minFinalScore: number;
    emailSubjectTemplate: string;
    headerTitle: string;
    headerSubtitle: string;
    summaryTemplate: string;
    descriptionTitle: string;
    reasonTitle: string;
    applyButtonText: string;
    personalizedTitle: string;
    personalizedSubtitle: string;
    footerLineOne: string;
    footerLineTwoTemplate: string;
    footerLineThree: string;
    jobDescriptionPreviewLimit: number;
  };
  cv: {
    path: string;
    parserOrder: string[];
    enableUniPDF: boolean;
    fallbackText: string;
    minValidTextLength: number;
    minLetterRatio: number;
    allowEnvOverrideUniPDF: boolean;
    logParserUsed: boolean;
    validateCandidateByHeuristic: boolean;
  };
  scraper: {
    dateSincePosted: string;
    paginationBatchSize: number;
    maxConsecutiveErrors: number;
    maxRequestRetries: number;
    consecutiveBackoffBaseSeconds: number;
    retryBackoffBaseSeconds: number;
    retryJitterMaxMs: number;
    interBatchDelayMinMs: number;
    interBatchDelayMaxMs: number;
    descriptionDelayMinMs: number;
    descriptionDelayMaxMs: number;
    descriptionMaxLength: number;
    errorBodyPreviewLength: number;
    verboseParseLogs: boolean;
    selectors: string[];
  };
}

export interface AccountSettings {
  accountID: string;
  version: number;
  policy: Policy;
  maxJobs: number;
  scheduleCron: string;
  scheduleTimezone: string;
  scheduleEnabled: boolean;
  notificationEmails: string[];
  updatedAt?: string;
}

export interface CurrentUser {
  user_id: string;
  account_id: string;
  firebase_uid: string;
  email: string;
  email_verified: boolean;
  credit_balance: number;
  auth_bypass: boolean;
  run_credit_cost: number;
}

export interface CreditPackage {
  id: string;
  name: string;
  description: string;
  credits: number;
  price_id: string;
}

export interface BillingSummary {
  balance: number;
  packages: CreditPackage[];
  last_updated_at: string;
}

export interface AccountCVStatus {
  hasCV: boolean;
  id?: string;
  storagePath?: string;
  mimeType?: string;
  sizeBytes?: number;
  createdAt?: string;
  textExtracted?: boolean;
}
