package controller

import (
	"fmt"

	"job-scorer/models"
	"job-scorer/services/filter"
)

type stageViewConfig struct {
	baseStage string
}

var comparisonStages = map[string]stageViewConfig{
	"prefiltered":           {baseStage: "all_jobs"},
	"promising":             {baseStage: "evaluated"},
	"notification":          {baseStage: "final_evaluated"},
	"validated_notification": {baseStage: "notification"},
}

func (jc *JobController) GetRunStageView(runID, stage string) ([]*models.Job, []*models.Job, int, error) {
	included, err := jc.GetRunStageJobs(runID, stage)
	if err != nil {
		return nil, nil, 0, err
	}
	if included == nil {
		included = []*models.Job{}
	}

	cfg, ok := comparisonStages[stage]
	if !ok {
		return included, []*models.Job{}, len(included), nil
	}

	excluded, err := jc.checkpoint.LoadCheckpointByStage(runID, stage+"_excluded")
	if err != nil {
		return nil, nil, 0, err
	}
	if len(excluded) > 0 {
		return included, excluded, len(included) + len(excluded), nil
	}

	baseJobs, err := jc.GetRunStageJobs(runID, cfg.baseStage)
	if err != nil {
		return nil, nil, 0, err
	}
	if baseJobs == nil {
		baseJobs = []*models.Job{}
	}

	reconstructed := reconstructExcludedJobs(baseJobs, included, stage)
	return included, reconstructed, len(baseJobs), nil
}

func (jc *JobController) saveExcludedCheckpoint(jobs []*models.Job, stage string, metadata map[string]interface{}) {
	if err := jc.checkpoint.SaveCheckpoint(jobs, stage+"_excluded", metadata); err != nil {
		jc.logger.Error("Failed to save %s excluded jobs checkpoint: %v", stage, err)
	}
}

func reconstructExcludedJobs(baseJobs, includedJobs []*models.Job, stage string) []*models.Job {
	includedKeys := make(map[string]bool, len(includedJobs))
	for _, job := range includedJobs {
		includedKeys[jobKey(job)] = true
	}

	excluded := make([]*models.Job, 0)
	for _, job := range baseJobs {
		if includedKeys[jobKey(job)] {
			continue
		}

		clone := cloneJob(job)
		clone.Excluded = true
		switch stage {
		case "promising":
			clone.ExclusionReason = derivePromisingReason(job)
		case "notification":
			clone.ExclusionReason = deriveNotificationReason(job)
		default:
			clone.ExclusionReason = fmt.Sprintf("Excluded in the %s step; a detailed persisted reason is unavailable for this run", stage)
		}
		excluded = append(excluded, clone)
	}
	return excluded
}

func cloneJob(job *models.Job) *models.Job {
	if job == nil {
		return nil
	}
	copyJob := *job
	if job.Reasons != nil {
		copyJob.Reasons = append([]string{}, job.Reasons...)
	}
	if job.FinalReasons != nil {
		copyJob.FinalReasons = append([]string{}, job.FinalReasons...)
	}
	return &copyJob
}

func jobKey(job *models.Job) string {
	if job == nil {
		return ""
	}
	if job.JobID != "" {
		return job.JobID
	}
	return job.JobURL
}

func derivePromisingReason(job *models.Job) string {
	if job == nil {
		return "Excluded before the promising stage"
	}
	if job.Score == nil {
		return "Excluded because the initial LLM evaluation did not produce a score"
	}
	return "Excluded because the initial score was below the promising threshold"
}

func deriveNotificationReason(job *models.Job) string {
	if job == nil {
		return "Excluded before the notification stage"
	}
	if job.FinalScore == nil {
		if job.FinalReason != "" {
			return job.FinalReason
		}
		if len(job.FinalReasons) > 0 {
			return job.FinalReasons[0]
		}
		return "Excluded because CV evaluation did not produce a final score"
	}
	if job.FinalReason != "" {
		return fmt.Sprintf("Excluded because the final score was below the notification threshold. %s", job.FinalReason)
	}
	return "Excluded because the final score was below the notification threshold"
}

func buildPrefilterExcludedJobs(allJobs, newJobs, includedJobs []*models.Job, filterService *filter.Filter) []*models.Job {
	newJobKeys := make(map[string]bool, len(newJobs))
	for _, job := range newJobs {
		newJobKeys[jobKey(job)] = true
	}

	includedKeys := make(map[string]bool, len(includedJobs))
	for _, job := range includedJobs {
		includedKeys[jobKey(job)] = true
	}

	excluded := make([]*models.Job, 0)
	for _, job := range allJobs {
		key := jobKey(job)
		if includedKeys[key] {
			continue
		}

		clone := cloneJob(job)
		clone.Excluded = true
		if !newJobKeys[key] {
			if job.JobID != "" {
				clone.ExclusionReason = fmt.Sprintf("Skipped because this job was already processed in a previous run (ID: %s)", job.JobID)
			} else {
				clone.ExclusionReason = "Skipped because this job was already processed in a previous run"
			}
		} else if filterService != nil {
			clone.ExclusionReason = filterService.PrefilterReason(job)
		}
		if clone.ExclusionReason == "" {
			clone.ExclusionReason = "Excluded during prefiltering"
		}
		excluded = append(excluded, clone)
	}

	return excluded
}

func buildPromisingExcludedJobs(baseJobs, includedJobs []*models.Job, filterService *filter.Filter, threshold float64) []*models.Job {
	return buildExcludedFromSubset(baseJobs, includedJobs, func(job *models.Job) string {
		if filterService == nil {
			return derivePromisingReason(job)
		}
		reason := filterService.PromisingExclusionReason(job, threshold)
		if reason == "" {
			return derivePromisingReason(job)
		}
		return reason
	})
}

func buildNotificationExcludedJobs(baseJobs, includedJobs []*models.Job, filterService *filter.Filter) []*models.Job {
	return buildExcludedFromSubset(baseJobs, includedJobs, func(job *models.Job) string {
		if filterService == nil {
			return deriveNotificationReason(job)
		}
		reason := filterService.NotificationExclusionReason(job)
		if reason == "" {
			return deriveNotificationReason(job)
		}
		if job.FinalReason != "" && job.FinalScore != nil {
			return fmt.Sprintf("%s. %s", reason, job.FinalReason)
		}
		return reason
	})
}

func buildExcludedFromSubset(baseJobs, includedJobs []*models.Job, reasonFn func(job *models.Job) string) []*models.Job {
	includedKeys := make(map[string]bool, len(includedJobs))
	for _, job := range includedJobs {
		includedKeys[jobKey(job)] = true
	}

	excluded := make([]*models.Job, 0)
	for _, job := range baseJobs {
		if includedKeys[jobKey(job)] {
			continue
		}

		clone := cloneJob(job)
		clone.Excluded = true
		if reasonFn != nil {
			clone.ExclusionReason = reasonFn(job)
		}
		if clone.ExclusionReason == "" {
			clone.ExclusionReason = "Excluded in this stage"
		}
		excluded = append(excluded, clone)
	}
	return excluded
}
