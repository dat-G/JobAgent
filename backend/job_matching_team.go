package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const jobMatchingTeamName = "Legato Job Matching Team"
const maxPlannedJobMatchingAgents = 6
const minPlannedJobMatchingAgents = 3
const maxJobMatchingAgentConcurrency = 3
const maxJobMatchingAgentPromptWarningBytes = 80000
const maxJobMatchingAgentResultArrayItems = 6
const maxJobMatchingAgentResultStringRunes = 900
const maxJobMatchingResumeTextRunes = 60000
const radarEvidenceDiminishThreshold = 0.65
const radarEvidenceTailThreshold = 0.70
const radarEvidenceTailGain = 0.04
const radarEvidenceSoftCap = 0.88
const jobMatchingRadarDeviationTolerance = 3

type schoolTierConfig struct {
	Base           float64
	NoHighCap      int
	HighCap        int
	ExceptionalCap int
	LiftScale      float64
}

var schoolTierConfigs = map[string]schoolTierConfig{
	"T0":  {Base: 68, NoHighCap: 78, HighCap: 92, ExceptionalCap: 94, LiftScale: 0.25},
	"T1":  {Base: 62, NoHighCap: 72, HighCap: 86, ExceptionalCap: 88, LiftScale: 0.28},
	"T2":  {Base: 52, NoHighCap: 62, HighCap: 76, ExceptionalCap: 78, LiftScale: 0.30},
	"T3":  {Base: 46, NoHighCap: 56, HighCap: 68, ExceptionalCap: 72, LiftScale: 0.28},
	"T4A": {Base: 45, NoHighCap: 54, HighCap: 66, ExceptionalCap: 70, LiftScale: 0.24},
	"T4":  {Base: 38, NoHighCap: 52, HighCap: 60, ExceptionalCap: 62, LiftScale: 0.22},
	"T5":  {Base: 34, NoHighCap: 48, HighCap: 56, ExceptionalCap: 60, LiftScale: 0.20},
}

type cappedEvidenceBucketRule struct {
	SingleCap float64
	TotalCap  float64
}

var cappedEvidenceBucketRules = map[string]cappedEvidenceBucketRule{
	"lowImpactAwardCertificate": {SingleCap: 0.035, TotalCap: 0.08},
	"campusAward":               {SingleCap: 0.045, TotalCap: 0.10},
	"genericCampusRole":         {SingleCap: 0.045, TotalCap: 0.10},
	"untitledProject":           {SingleCap: 0.060, TotalCap: 0.15},
	"impactLow":                 {SingleCap: 0.060, TotalCap: 0.14},
	"impactMedium":              {SingleCap: 0.100, TotalCap: 0.24},
	"impactHigh":                {SingleCap: 0.160, TotalCap: 0.38},
	"impactExceptional":         {SingleCap: 0.240, TotalCap: 0.52},
}

type jobMatchingTeamEvent struct {
	Team            string `json:"team"`
	Workflow        string `json:"workflow"`
	AgentKey        string `json:"agent_key"`
	Agent           string `json:"agent"`
	AgentIndex      int    `json:"agent_index,omitempty"`
	AgentTotal      int    `json:"agent_total,omitempty"`
	Phase           string `json:"phase"`
	Perspective     string `json:"perspective,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	Focus           string `json:"focus,omitempty"`
	Complexity      string `json:"complexity,omitempty"`
	AgentCount      int    `json:"agent_count,omitempty"`
	Status          string `json:"status"`
	Message         string `json:"message"`
	Sequence        int    `json:"sequence"`
	PrestoSessionID string `json:"presto_session_id,omitempty"`
	PrestoRunID     string `json:"presto_run_id,omitempty"`
	PrestoEventType string `json:"presto_event_type,omitempty"`
	TokenChannel    string `json:"token_channel,omitempty"`
	TokenDelta      string `json:"token_delta,omitempty"`
	OutputPreview   string `json:"output_preview,omitempty"`
	Error           string `json:"error,omitempty"`
}

type jobMatchingAgentSpec struct {
	Key             string
	Name            string
	Phase           string
	Perspective     string
	ReasoningEffort string
	Focus           string
	AgentIndex      int
	AgentTotal      int
	Sequence        int
	Prompt          func(map[string]any) string
}

type jobMatchingAgentResult struct {
	Spec    jobMatchingAgentSpec
	Data    map[string]any
	Output  string
	RunID   string
	Session string
	Err     error
}

type prestoClient struct {
	base   *url.URL
	client *http.Client
}

type prestoSessionResponse struct {
	ID string `json:"id"`
}

type prestoRunResponse struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	Output    string `json:"output"`
	Error     string `json:"error"`
}

type prestoEventResponse struct {
	ID        string          `json:"id"`
	RunID     string          `json:"run_id"`
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data"`
	CreatedAt string          `json:"created_at"`
}

type jobMatchingAgentPlan struct {
	Complexity       string                    `json:"complexity"`
	ReasoningEffort  string                    `json:"reasoning_effort"`
	SynthesisEffort  string                    `json:"synthesis_effort"`
	Rationale        string                    `json:"rationale"`
	RecommendedScope string                    `json:"recommended_jobs_scope"`
	Agents           []jobMatchingPlannedAgent `json:"agents"`
}

type jobMatchingPlannedAgent struct {
	Key             string `json:"key"`
	Name            string `json:"name"`
	Phase           string `json:"phase"`
	Perspective     string `json:"perspective"`
	ReasoningEffort string `json:"reasoning_effort"`
	Focus           string `json:"focus"`
}

func (s Server) runLegatoJobMatchingTeam(ctx context.Context, job *DiagnosisJob, diagnosis Diagnosis) (*LegatoEnvelope, error) {
	client, err := s.newPrestoClient()
	if err != nil {
		return nil, err
	}
	return runLegatoJobMatchingTeamWithClient(ctx, job, diagnosis, client)
}

func runLegatoJobMatchingTeamWithClient(ctx context.Context, job *DiagnosisJob, diagnosis Diagnosis, client *prestoClient) (*LegatoEnvelope, error) {
	started := time.Now()
	teamCtx := buildJobMatchingTeamContext(ctx, job, diagnosis)
	emitJobMatchingTeamEvent(job, "running", jobMatchingTeamEvent{
		AgentKey: "team",
		Agent:    jobMatchingTeamName,
		Phase:    "orchestration",
		Status:   "running",
		Message:  "正在连接 Presto，准备由 Planner 动态派生 Agent。",
		Sequence: 0,
	})

	plannerSpec := jobMatchingAgentSpec{
		Key:             "adaptive_planner",
		Name:            "Adaptive Planner",
		Phase:           "planning",
		Perspective:     "agent_orchestration",
		ReasoningEffort: "high",
		Focus:           "根据简历复杂度和证据结构派生多视角 Agent plan",
		AgentIndex:      1,
		AgentTotal:      1,
		Sequence:        1,
		Prompt:          adaptivePlannerPrompt,
	}
	plannerResult, err := runJobMatchingAgent(ctx, job, client, teamCtx, plannerSpec)
	if err != nil {
		return nil, jobMatchingAgentError(plannerSpec, err)
	}
	plan, err := normalizeJobMatchingAgentPlan(plannerResult.Data)
	if err != nil {
		return nil, err
	}
	teamCtx["agent_plan"] = plan
	emitJobMatchingTeamEvent(job, "running", jobMatchingTeamEvent{
		AgentKey:        plannerSpec.Key,
		Agent:           plannerSpec.Name,
		AgentIndex:      plannerSpec.AgentIndex,
		AgentTotal:      plannerSpec.AgentTotal,
		Phase:           plannerSpec.Phase,
		Perspective:     plannerSpec.Perspective,
		ReasoningEffort: plannerSpec.ReasoningEffort,
		Focus:           plannerSpec.Focus,
		Complexity:      plan.Complexity,
		AgentCount:      len(plan.Agents),
		Status:          "done",
		Message:         fmt.Sprintf("Planner 已判定复杂度 %s，派生 %d 个并发视角 Agent。", plan.Complexity, len(plan.Agents)),
		Sequence:        plannerSpec.Sequence,
		PrestoSessionID: plannerResult.Session,
		PrestoRunID:     plannerResult.RunID,
		OutputPreview:   compactPreview(plan.Rationale, 120),
	})

	dynamicSpecs := specsFromJobMatchingPlan(plan)
	parallelResults, err := runJobMatchingAgentGroup(ctx, job, client, teamCtx, dynamicSpecs)
	if err != nil {
		return nil, err
	}
	perspectiveResults := make([]map[string]any, 0, len(parallelResults))
	for _, result := range parallelResults {
		compactData := compactJobMatchingAgentData(result.Data)
		teamCtx[result.Spec.Key] = compactData
		perspectiveResults = append(perspectiveResults, map[string]any{
			"agent_key":        result.Spec.Key,
			"agent":            result.Spec.Name,
			"phase":            result.Spec.Phase,
			"perspective":      result.Spec.Perspective,
			"reasoning_effort": result.Spec.ReasoningEffort,
			"focus":            result.Spec.Focus,
			"result":           compactData,
		})
	}
	teamCtx["perspective_results"] = perspectiveResults

	synthesisSpec := jobMatchingAgentSpec{
		Key:             "synthesis_arbiter",
		Name:            "Synthesis Arbiter",
		Phase:           "final_synthesis",
		Perspective:     "multi_view_decision",
		ReasoningEffort: plan.SynthesisEffort,
		Focus:           "综合 Planner 派生的所有视角结果，输出可渲染结构化岗位匹配",
		AgentIndex:      1,
		AgentTotal:      1,
		Sequence:        len(dynamicSpecs) + 2,
		Prompt:          synthesisArbiterPrompt,
	}
	reportWriter, err := runJobMatchingAgent(ctx, job, client, teamCtx, synthesisSpec)
	if err != nil {
		if isContextError(err) {
			return nil, jobMatchingAgentError(synthesisSpec, err)
		}
		reportWriter, err = retrySynthesisArbiter(ctx, job, client, teamCtx, synthesisSpec, err)
		if err != nil {
			return nil, err
		}
	}
	finalRaw := objectValue(reportWriter.Data["job_matching"])
	if len(finalRaw) == 0 {
		finalRaw = reportWriter.Data
	}
	if err := validateJobMatchingTeamOutputAgainstContext(finalRaw, teamCtx); err != nil {
		if ctx.Err() != nil {
			return nil, jobMatchingAgentError(synthesisSpec, ctx.Err())
		}
		reportWriter, err = retrySynthesisArbiter(ctx, job, client, teamCtx, synthesisSpec, err)
		if err != nil {
			return nil, err
		}
		finalRaw = objectValue(reportWriter.Data["job_matching"])
		if len(finalRaw) == 0 {
			finalRaw = reportWriter.Data
		}
		if err := validateJobMatchingTeamOutputAgainstContext(finalRaw, teamCtx); err != nil {
			emitJobMatchingAgentFailure(
				job,
				synthesisSpec,
				reportWriter.Session,
				reportWriter.RunID,
				"Synthesis Arbiter 输出仍未通过后端校验，岗位匹配失败。",
				err,
			)
			return nil, err
		}
	}

	emitJobMatchingTeamEvent(job, "running", jobMatchingTeamEvent{
		AgentKey:      "team",
		Agent:         jobMatchingTeamName,
		Phase:         "orchestration",
		Complexity:    plan.Complexity,
		AgentCount:    len(plan.Agents),
		Status:        "done",
		Message:       "动态 Presto Agent Team 已完成岗位推荐和匹配报告。",
		Sequence:      len(dynamicSpecs) + 3,
		OutputPreview: jobMatchingPreview(finalRaw),
	})

	return &LegatoEnvelope{
		Status:     "ok",
		Target:     "resume",
		Frontend:   "presto",
		Formatter:  "presto_job_matching_team",
		ElapsedMS:  int(time.Since(started) / time.Millisecond),
		Data:       map[string]any{"job_matching": finalRaw},
		Warnings:   []string{},
		Debug:      map[string]any{"agent_team": teamCtx, "agent_plan": plan},
		SourcePath: "legato://resume/job_matching_team",
	}, nil
}

func retrySynthesisArbiter(ctx context.Context, job *DiagnosisJob, client *prestoClient, teamCtx map[string]any, spec jobMatchingAgentSpec, cause error) (jobMatchingAgentResult, error) {
	retryCtx := copyJobMatchingContext(teamCtx)
	retryCtx["synthesis_retry_reason"] = cause.Error()
	retryCtx["synthesis_retry_instruction"] = "Previous Synthesis Arbiter output failed backend parsing or validation. Return strict JSON only. student_radar must exactly follow radar_context.overall.scores within rounding tolerance; target_radar must equal selected_job.requirement_radar; report_sections must compare those two radars dimension by dimension."
	retrySpec := spec
	retrySpec.Focus = spec.Focus + "；上一次输出未通过后端校验，本次必须只返回严格 JSON。"
	emitJobMatchingTeamEvent(job, "running", jobMatchingTeamEvent{
		AgentKey:        retrySpec.Key,
		Agent:           retrySpec.Name,
		AgentIndex:      retrySpec.AgentIndex,
		AgentTotal:      retrySpec.AgentTotal,
		Phase:           retrySpec.Phase,
		Perspective:     retrySpec.Perspective,
		ReasoningEffort: retrySpec.ReasoningEffort,
		Focus:           retrySpec.Focus,
		Status:          "running",
		Message:         "Synthesis Arbiter 输出未通过校验，正在自动重试严格 JSON。",
		Sequence:        retrySpec.Sequence,
		Error:           cause.Error(),
	})
	result, err := runJobMatchingAgent(ctx, job, client, retryCtx, retrySpec)
	if err != nil {
		return result, fmt.Errorf("Synthesis Arbiter retry failed after initial error %q: %w", cause.Error(), err)
	}
	return result, nil
}

func (s Server) newPrestoClient() (*prestoClient, error) {
	base := s.prestoURL
	if base == nil {
		base = parseOptionalURL(legatoPrestoURL())
	}
	if base == nil {
		return nil, errors.New("PRESTO_URL 未配置，无法启动 Legato Job Matching Agent Team")
	}
	return &prestoClient{
		base: base,
		client: &http.Client{
			Timeout: 0,
		},
	}, nil
}

func runJobMatchingAgentGroup(ctx context.Context, job *DiagnosisJob, client *prestoClient, teamCtx map[string]any, specs []jobMatchingAgentSpec) ([]jobMatchingAgentResult, error) {
	results := make(chan jobMatchingAgentResult, len(specs))
	sem := make(chan struct{}, maxJobMatchingAgentConcurrency)
	var wg sync.WaitGroup
	for _, spec := range specs {
		spec := spec
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				err := jobMatchingAgentError(spec, ctx.Err())
				emitJobMatchingAgentFailure(job, spec, "", "", jobMatchingAgentFailureMessage(spec, ctx.Err()), ctx.Err())
				results <- jobMatchingAgentResult{Spec: spec, Err: err}
				return
			}
			result, err := runJobMatchingAgent(ctx, job, client, teamCtx, spec)
			if err != nil {
				result.Err = err
			}
			results <- result
		}()
	}
	wg.Wait()
	close(results)

	out := make([]jobMatchingAgentResult, 0, len(specs))
	var failures []string
	for result := range results {
		if result.Err != nil {
			failures = append(failures, result.Spec.Name+"："+result.Err.Error())
		}
		out = append(out, result)
	}
	if len(failures) > 0 {
		return out, errors.New(strings.Join(failures, "；"))
	}
	return out, nil
}

func runJobMatchingAgent(ctx context.Context, job *DiagnosisJob, client *prestoClient, teamCtx map[string]any, spec jobMatchingAgentSpec) (jobMatchingAgentResult, error) {
	prompt := spec.Prompt(teamCtx)
	jobID := ""
	if job != nil {
		jobID = job.ID
	}
	promptBytes := len(prompt)
	if promptBytes > maxJobMatchingAgentPromptWarningBytes {
		log.Printf("job matching prompt large job=%s agent=%s phase=%s prompt_bytes=%d", jobID, spec.Key, spec.Phase, promptBytes)
	} else {
		log.Printf("job matching prompt job=%s agent=%s phase=%s prompt_bytes=%d", jobID, spec.Key, spec.Phase, promptBytes)
	}
	output, sessionID, runID, err := client.runPromptStream(ctx, job, spec, prompt)
	result := jobMatchingAgentResult{Spec: spec, Output: output, RunID: runID, Session: sessionID}
	if err != nil {
		wrapped := jobMatchingAgentError(spec, err)
		result.Err = wrapped
		emitJobMatchingAgentFailure(job, spec, sessionID, runID, jobMatchingAgentFailureMessage(spec, err), err)
		return result, wrapped
	}
	data, err := extractJSONObject(output)
	if err != nil && ctx.Err() == nil {
		emitJobMatchingTeamEvent(job, "running", jobMatchingTeamEvent{
			AgentKey:        spec.Key,
			Agent:           spec.Name,
			AgentIndex:      spec.AgentIndex,
			AgentTotal:      spec.AgentTotal,
			Phase:           spec.Phase,
			Perspective:     spec.Perspective,
			ReasoningEffort: spec.ReasoningEffort,
			Focus:           spec.Focus,
			Status:          "running",
			Message:         spec.Name + " 返回的 JSON 无法解析，正在重试严格 JSON。",
			Sequence:        spec.Sequence,
			PrestoSessionID: sessionID,
			PrestoRunID:     runID,
			Error:           err.Error(),
		})
		retryPrompt := strictJSONRetryPrompt(prompt, err, output)
		output, sessionID, runID, err = client.runPromptStream(ctx, job, spec, retryPrompt)
		result = jobMatchingAgentResult{Spec: spec, Output: output, RunID: runID, Session: sessionID}
		if err != nil {
			wrapped := jobMatchingAgentError(spec, err)
			result.Err = wrapped
			emitJobMatchingAgentFailure(job, spec, sessionID, runID, jobMatchingAgentFailureMessage(spec, err), err)
			return result, wrapped
		}
		data, err = extractJSONObject(output)
	}
	if err != nil {
		result.Err = err
		emitJobMatchingTeamEvent(job, "failed", jobMatchingTeamEvent{
			AgentKey:        spec.Key,
			Agent:           spec.Name,
			AgentIndex:      spec.AgentIndex,
			AgentTotal:      spec.AgentTotal,
			Phase:           spec.Phase,
			Perspective:     spec.Perspective,
			ReasoningEffort: spec.ReasoningEffort,
			Focus:           spec.Focus,
			Status:          "failed",
			Message:         spec.Name + " 返回的 JSON 无法解析。",
			Sequence:        spec.Sequence,
			PrestoSessionID: sessionID,
			PrestoRunID:     runID,
			Error:           err.Error(),
		})
		return result, err
	}
	if spec.Key != "adaptive_planner" {
		if _, ok := data["agent_plan"]; ok {
			err := errors.New("dynamic Agent attempted to return a nested agent_plan")
			result.Err = err
			emitJobMatchingTeamEvent(job, "failed", jobMatchingTeamEvent{
				AgentKey:        spec.Key,
				Agent:           spec.Name,
				AgentIndex:      spec.AgentIndex,
				AgentTotal:      spec.AgentTotal,
				Phase:           spec.Phase,
				Perspective:     spec.Perspective,
				ReasoningEffort: spec.ReasoningEffort,
				Focus:           spec.Focus,
				Status:          "failed",
				Message:         spec.Name + " 尝试继续派生 Agent，已被后端拒绝。",
				Sequence:        spec.Sequence,
				PrestoSessionID: sessionID,
				PrestoRunID:     runID,
				Error:           err.Error(),
			})
			return result, err
		}
	}
	result.Data = data
	emitJobMatchingTeamEvent(job, "running", jobMatchingTeamEvent{
		AgentKey:        spec.Key,
		Agent:           spec.Name,
		AgentIndex:      spec.AgentIndex,
		AgentTotal:      spec.AgentTotal,
		Phase:           spec.Phase,
		Perspective:     spec.Perspective,
		ReasoningEffort: spec.ReasoningEffort,
		Focus:           spec.Focus,
		Status:          "done",
		Message:         spec.Name + " 已返回结构化结果。",
		Sequence:        spec.Sequence,
		PrestoSessionID: sessionID,
		PrestoRunID:     runID,
		OutputPreview:   compactPreview(output, 140),
	})
	return result, nil
}

func strictJSONRetryPrompt(originalPrompt string, cause error, output string) string {
	return originalPrompt + fmt.Sprintf(`

Backend retry instruction:
- The previous Agent output was not parseable strict JSON: %s
- Return one JSON object only. No markdown fence, no commentary, no trailing text.
- Do not use unquoted text values. Escape every newline inside JSON strings as \n.
- Preserve the task's required schema exactly.

Previous output preview:
%s`, cause.Error(), compactPreview(output, 900))
}

func jobMatchingAgentError(spec jobMatchingAgentSpec, err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), spec.Name+"（"+spec.Phase+"）") {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%s（%s）超过 Job Matching 总超时：%w", spec.Name, spec.Phase, err)
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("%s（%s）已被取消：%w", spec.Name, spec.Phase, err)
	}
	return err
}

func jobMatchingAgentFailureMessage(spec jobMatchingAgentSpec, err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return spec.Name + " 超过 Job Matching 总超时，模型或 Presto run 未在预算内返回终止事件。"
	}
	if errors.Is(err, context.Canceled) {
		return spec.Name + " 已被取消，Job Matching 流程提前终止。"
	}
	return spec.Name + " 运行失败。"
}

func isContextError(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}

func emitJobMatchingAgentFailure(job *DiagnosisJob, spec jobMatchingAgentSpec, sessionID string, runID string, message string, cause error) {
	errorText := ""
	if cause != nil {
		errorText = cause.Error()
	}
	emitJobMatchingTeamEvent(job, "failed", jobMatchingTeamEvent{
		AgentKey:        spec.Key,
		Agent:           spec.Name,
		AgentIndex:      spec.AgentIndex,
		AgentTotal:      spec.AgentTotal,
		Phase:           spec.Phase,
		Perspective:     spec.Perspective,
		ReasoningEffort: spec.ReasoningEffort,
		Focus:           spec.Focus,
		Status:          "failed",
		Message:         message,
		Sequence:        spec.Sequence,
		PrestoSessionID: sessionID,
		PrestoRunID:     runID,
		Error:           errorText,
	})
}

func (c *prestoClient) runPromptStream(ctx context.Context, job *DiagnosisJob, spec jobMatchingAgentSpec, prompt string) (string, string, string, error) {
	session, err := c.createSession(ctx, map[string]string{
		"app":              "legato",
		"workflow":         "resume",
		"team":             "job_matching",
		"agent":            spec.Key,
		"phase":            spec.Phase,
		"perspective":      spec.Perspective,
		"reasoning_effort": spec.ReasoningEffort,
	})
	if err != nil {
		return "", "", "", err
	}
	run, err := c.createRun(ctx, session.ID, prompt)
	if err != nil {
		return "", session.ID, "", err
	}
	emitJobMatchingTeamEvent(job, "running", jobMatchingTeamEvent{
		AgentKey:        spec.Key,
		Agent:           spec.Name,
		AgentIndex:      spec.AgentIndex,
		AgentTotal:      spec.AgentTotal,
		Phase:           spec.Phase,
		Perspective:     spec.Perspective,
		ReasoningEffort: spec.ReasoningEffort,
		Focus:           spec.Focus,
		Status:          "running",
		Message:         spec.Name + " 已创建 Presto run，等待模型事件。",
		Sequence:        spec.Sequence,
		PrestoSessionID: session.ID,
		PrestoRunID:     run.ID,
	})

	streamErr := c.streamRunEvents(ctx, run.ID, func(event prestoEventResponse) bool {
		if event.Type == "run.created" {
			return false
		}
		tokenChannel, tokenDelta := prestoTokenDelta(event)
		status := "streaming"
		if event.Type == "run.error" || event.Type == "run.failed" {
			status = "failed"
		}
		if event.Type == "run.done" || event.Type == "run.completed" {
			status = "done"
		}
		emitJobMatchingTeamEvent(job, "running", jobMatchingTeamEvent{
			AgentKey:        spec.Key,
			Agent:           spec.Name,
			AgentIndex:      spec.AgentIndex,
			AgentTotal:      spec.AgentTotal,
			Phase:           spec.Phase,
			Perspective:     spec.Perspective,
			ReasoningEffort: spec.ReasoningEffort,
			Focus:           spec.Focus,
			Status:          status,
			Message:         prestoEventMessage(spec.Name, event),
			Sequence:        spec.Sequence,
			PrestoSessionID: session.ID,
			PrestoRunID:     run.ID,
			PrestoEventType: event.Type,
			TokenChannel:    tokenChannel,
			TokenDelta:      tokenDelta,
		})
		return terminalPrestoEvent(event.Type)
	})
	if streamErr != nil {
		return "", session.ID, run.ID, streamErr
	}

	finished, err := c.waitRun(ctx, run.ID)
	if err != nil {
		return "", session.ID, run.ID, err
	}
	if finished.Error != "" {
		return "", session.ID, run.ID, errors.New(finished.Error)
	}
	if finished.Status != "completed" {
		return "", session.ID, run.ID, fmt.Errorf("Presto run ended with status %s", finished.Status)
	}
	if strings.TrimSpace(finished.Output) == "" {
		return "", session.ID, run.ID, errors.New("Presto run returned empty output")
	}
	return finished.Output, session.ID, run.ID, nil
}

func (c *prestoClient) createSession(ctx context.Context, metadata map[string]string) (prestoSessionResponse, error) {
	var session prestoSessionResponse
	err := c.requestJSON(ctx, http.MethodPost, "/sessions", map[string]any{"metadata": metadata}, &session)
	if err != nil {
		return session, err
	}
	if strings.TrimSpace(session.ID) == "" {
		return session, errors.New("Presto session response missing id")
	}
	return session, nil
}

func (c *prestoClient) createRun(ctx context.Context, sessionID string, prompt string) (prestoRunResponse, error) {
	var run prestoRunResponse
	err := c.requestJSON(ctx, http.MethodPost, "/sessions/"+url.PathEscape(sessionID)+"/runs", map[string]any{
		"message":      prompt,
		"async":        true,
		"token_stream": true,
	}, &run)
	if err != nil {
		return run, err
	}
	if strings.TrimSpace(run.ID) == "" {
		return run, errors.New("Presto run response missing id")
	}
	return run, nil
}

func (c *prestoClient) waitRun(ctx context.Context, runID string) (prestoRunResponse, error) {
	var last prestoRunResponse
	for attempt := 0; attempt < 20; attempt++ {
		var run prestoRunResponse
		if err := c.requestJSON(ctx, http.MethodGet, "/runs/"+url.PathEscape(runID), nil, &run); err != nil {
			return run, err
		}
		last = run
		if run.Status == "completed" || run.Status == "failed" || run.Status == "cancelled" {
			if run.Status == "completed" && strings.TrimSpace(run.Output) == "" && attempt < 19 {
				time.Sleep(80 * time.Millisecond)
				continue
			}
			return run, nil
		}
		select {
		case <-time.After(120 * time.Millisecond):
		case <-ctx.Done():
			return run, ctx.Err()
		}
	}
	return last, fmt.Errorf("Presto run %s did not finish", runID)
}

func (c *prestoClient) streamRunEvents(ctx context.Context, runID string, onEvent func(prestoEventResponse) bool) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint("/runs/"+url.PathEscape(runID)+"/events"), nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Presto event stream failed with %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var eventType string
	var dataLines []string
	dispatch := func() bool {
		if eventType == "" && len(dataLines) == 0 {
			return false
		}
		event := prestoEventResponse{Type: eventType, Data: json.RawMessage(strings.Join(dataLines, "\n"))}
		if len(event.Data) > 0 {
			var envelope map[string]any
			if err := json.Unmarshal(event.Data, &envelope); err == nil {
				if nestedType := stringValue(envelope["type"]); nestedType != "" && event.Type == "" {
					event.Type = nestedType
				}
				event.RunID = stringValue(envelope["run_id"])
			}
		}
		done := onEvent(event)
		eventType = ""
		dataLines = nil
		return done
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if dispatch() {
				return nil
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	dispatch()
	return nil
}

func (c *prestoClient) requestJSON(ctx context.Context, method string, path string, payload any, target any) error {
	var body io.Reader
	if payload != nil {
		raw, err := marshalNoEscape(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint(path), body)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Presto request failed with %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	decoder := json.NewDecoder(resp.Body)
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func (c *prestoClient) endpoint(path string) string {
	target := *c.base
	basePath := strings.TrimRight(target.Path, "/")
	target.Path = basePath + "/" + strings.TrimLeft(path, "/")
	return target.String()
}

func emitJobMatchingTeamEvent(job *DiagnosisJob, outerStatus string, event jobMatchingTeamEvent) {
	if job == nil {
		return
	}
	if event.Team == "" {
		event.Team = jobMatchingTeamName
	}
	if event.Workflow == "" {
		event.Workflow = "resume/job_matching"
	}
	if event.Status == "" {
		event.Status = "running"
	}
	job.Emit(DiagnosisEvent{
		Type:    "step.update",
		Step:    "matching",
		Status:  outerStatus,
		Message: event.Message,
		Data: map[string]any{
			"agent_team_event": event,
		},
	})
}

func buildJobMatchingTeamContext(ctx context.Context, job *DiagnosisJob, diagnosis Diagnosis) map[string]any {
	profile := diagnosis.AbilityProfile
	evidenceCount := len(profile.Awards) + len(profile.Experiences)
	resumeText, resumeTextMeta := resumeTextContext(ctx, job)
	radarContext := buildJobMatchingRadarContext(profile)
	return map[string]any{
		"basic_info": map[string]any{
			"name":            profile.BasicInfo.Name,
			"sex":             profile.BasicInfo.Sex,
			"birth_year":      profile.BasicInfo.BirthYear,
			"school":          profile.BasicInfo.School,
			"major":           profile.BasicInfo.Major,
			"degree":          profile.BasicInfo.Degree,
			"graduation_year": profile.BasicInfo.GraduationYear,
			"transcript_use":  profile.BasicInfo.TranscriptUse,
		},
		"education":             profile.Education,
		"major_baseline":        profile.MajorBaseline,
		"student_radar":         profile.RadarData,
		"radar_context":         radarContext,
		"awards":                profile.Awards,
		"experiences":           profile.Experiences,
		"resume_full_text":      resumeText,
		"resume_full_text_meta": resumeTextMeta,
		"dimensions":            benchmarkDimensionNames(),
		"benchmark_state":       map[string]any{"benchmark_status": profile.BenchmarkStatus, "major_baseline_status": profile.MajorBaselineStatus},
		"complexity_signals": map[string]any{
			"award_count":      len(profile.Awards),
			"experience_count": len(profile.Experiences),
			"evidence_count":   evidenceCount,
			"has_major_prior":  len(profile.MajorBaseline.Scores) > 0,
			"has_benchmark":    profile.BenchmarkStatus == "ready",
			"has_education":    len(profile.Education) > 0,
		},
		"principles": []string{
			"radar_context.overall 和 student_radar 是强依据；若二者冲突，优先使用 radar_context.overall 并解释原因。",
			"校内/校外 evidence_scope 的六维分布可用于发展建议：校内建议应指向课程、社团、实验室、校内项目或竞赛；校外建议应指向实习、开源、作品集、企业项目、社会竞赛或行业认证。",
			"经历和项目证据作为第二排序依据。",
			"学历作为门槛和风险约束，不单独决定排序。",
			"成绩单是可选增强材料，不上传不等于强缺失；除非岗位明确强筛成绩，不默认提及 GPA 缺失。",
			"如果有较高 GPA、核心课程高分或课程项目佐证，表达为会更有信服力，而不是缺失或否定。",
			"必须自动推荐岗位，不能要求用户主动输入目标岗位。",
			"至少覆盖本专业相关或扩展岗位，并在证据支持时给出跨专业可迁移岗位。",
		},
	}
}

func resumeTextContext(ctx context.Context, job *DiagnosisJob) (string, map[string]any) {
	meta := map[string]any{"status": "missing"}
	if job == nil {
		return "", meta
	}
	file, ok := firstFileByKind(job.Files, "resume")
	if !ok || strings.TrimSpace(file.Path) == "" {
		return "", meta
	}
	if text, fallbackErr := readPlainResumeText(file.Path); fallbackErr == nil && text != "" {
		originalChars := len([]rune(text))
		text, truncated := truncateJobMatchingResumeText(text)
		meta = map[string]any{
			"status":         "ready_fallback",
			"chars":          originalChars,
			"included_chars": len([]rune(text)),
			"truncated":      truncated,
			"frontend":       "plain_file",
			"formatter":      "local_plain_text",
		}
		return text, meta
	}
	result, err := runLegato(ctx, file.Path, "resume", "source_text")
	if err == nil && result != nil {
		text := stringValue(result.Data["resume_text"])
		originalChars := len([]rune(text))
		text, truncated := truncateJobMatchingResumeText(text)
		meta = map[string]any{
			"status":         "ready",
			"chars":          originalChars,
			"included_chars": len([]rune(text)),
			"truncated":      truncated,
			"frontend":       result.Frontend,
			"formatter":      result.Formatter,
		}
		return text, meta
	}
	errorText := ""
	if err != nil {
		errorText = err.Error()
	}
	meta = map[string]any{"status": "unavailable", "error": truncateRunes(errorText, 220)}
	return "", meta
}

func readPlainResumeText(path string) (string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".md", ".markdown", ".txt", ".text":
	default:
		return "", fmt.Errorf("resume file is not plain text: %s", ext)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func truncateJobMatchingResumeText(value string) (string, bool) {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= maxJobMatchingResumeTextRunes {
		return value, false
	}
	return string(runes[:maxJobMatchingResumeTextRunes]), true
}

func buildJobMatchingRadarContext(profile AbilityProfile) map[string]any {
	overall := aggregateEvidenceRadar(profile, "")
	campus := aggregateEvidenceRadar(profile, "校内")
	external := aggregateEvidenceRadar(profile, "校外")
	if len(profile.RadarData) == len(benchmarkDimensionNames()) {
		if overall.Count == 0 {
			overall.Count = benchmarkedEvidenceCount(profile.Awards, profile.Experiences)
		}
		overall.Scores = profile.RadarData
		overall.Source = "profile.radar_data"
	}
	return map[string]any{
		"overall":          radarSummaryMap(overall),
		"campus":           radarSummaryMap(campus),
		"external":         radarSummaryMap(external),
		"major_baseline":   profile.MajorBaseline,
		"usage_note":       "student_radar should follow overall scores when available. campus/external scores are for development recommendations, not separate job ranking caps.",
		"dimensions":       benchmarkDimensionNames(),
		"benchmark_status": profile.BenchmarkStatus,
	}
}

func benchmarkedEvidenceCount(awards []AwardItem, experiences []ExperienceItem) int {
	count := 0
	for _, item := range awards {
		if len(item.BenchmarkScores) >= len(benchmarkDimensionNames()) && item.ImpactFactor != nil {
			count++
		}
	}
	for _, item := range experiences {
		if len(item.BenchmarkScores) >= len(benchmarkDimensionNames()) && item.ImpactFactor != nil {
			count++
		}
	}
	return count
}

type aggregatedRadar struct {
	Scores []ScoreDimension
	Count  int
	Source string
}

type radarEvidenceItem struct {
	Kind            string
	Name            string
	Result          string
	ExperienceType  string
	Role            string
	Contribution    string
	EvidenceScope   string
	Level           float64
	ImpactFactor    *float64
	BenchmarkScores []float64
}

type radarEvidenceQuality struct {
	HighImpactCount        int
	ExceptionalImpactCount int
	CappedLowCount         int
}

type radarAcademicBaseline struct {
	Base           int
	TranscriptBase int
	Scores         []int
	MajorFamily    string
	Source         string
}

type radarSchoolTier struct {
	Key    string
	Config schoolTierConfig
}

type radarDimensionContributions struct {
	Regular []float64
	Capped  map[string][]float64
}

func aggregateEvidenceRadar(profile AbilityProfile, scope string) aggregatedRadar {
	dimensions := benchmarkDimensionNames()
	items := radarEvidenceItems(profile, scope)
	if len(items) == 0 {
		return aggregatedRadar{Count: 0, Source: "empty"}
	}
	contributions := make([]radarDimensionContributions, len(dimensions))
	for index := range contributions {
		contributions[index] = radarEmptyDimensionContributions()
	}
	for _, item := range items {
		strength := radarEvidenceStrength(item.Level, item.ImpactFactor)
		bucketKey := radarCappedEvidenceBucketKey(item)
		rule, hasRule := cappedEvidenceBucketRules[bucketKey]
		for index := range dimensions {
			contribution := clampFloat(item.BenchmarkScores[index], 0, 1) * strength
			contribution = clampFloat(contribution, 0, 0.96)
			if contribution <= 0 {
				continue
			}
			if hasRule {
				contributions[index].Capped[bucketKey] = append(contributions[index].Capped[bucketKey], minFloat(contribution, rule.SingleCap))
			} else {
				contributions[index].Regular = append(contributions[index].Regular, contribution)
			}
		}
	}
	quality := radarEvidenceQualitySummary(items)
	academicBaseline := radarAcademicBaselineVector(profile)
	schoolTier := radarSchoolTierProfile(profile)
	includeAcademicPrior := scope != "校外"
	scores := make([]ScoreDimension, len(dimensions))
	for index, name := range dimensions {
		dimensionContributions := contributions[index]
		capped := make([]float64, 0, len(dimensionContributions.Capped))
		for bucketKey, values := range dimensionContributions.Capped {
			rule, ok := cappedEvidenceBucketRules[bucketKey]
			if !ok {
				continue
			}
			if score := radarCappedEvidenceBucket(values, rule.TotalCap); score > 0 {
				capped = append(capped, score)
			}
		}
		evidenceScore := int(radarDiminishingEvidenceScore(append(dimensionContributions.Regular, capped...))*100 + 0.5)
		score := radarCombineEvidenceWithSchoolTier(
			evidenceScore,
			academicBaseline,
			schoolTier,
			quality,
			len(items),
			index,
			includeAcademicPrior,
		)
		scores[index] = ScoreDimension{Name: name, Score: score, MaxScore: 100}
	}
	return aggregatedRadar{Scores: scores, Count: len(items), Source: "evidence_benchmark"}
}

func radarEvidenceItems(profile AbilityProfile, scope string) []radarEvidenceItem {
	items := make([]radarEvidenceItem, 0, len(profile.Awards)+len(profile.Experiences))
	for _, item := range profile.Awards {
		if len(item.BenchmarkScores) < len(benchmarkDimensionNames()) || item.ImpactFactor == nil {
			continue
		}
		itemScope := normalizeEvidenceScope(item.EvidenceScope, "award", item.Name, item.Result, "", "", "")
		if scope != "" && itemScope != scope {
			continue
		}
		items = append(items, radarEvidenceItem{
			Kind:            "award",
			Name:            item.Name,
			Result:          item.Result,
			EvidenceScope:   itemScope,
			Level:           item.Level,
			ImpactFactor:    item.ImpactFactor,
			BenchmarkScores: item.BenchmarkScores,
		})
	}
	for _, item := range profile.Experiences {
		if len(item.BenchmarkScores) < len(benchmarkDimensionNames()) || item.ImpactFactor == nil {
			continue
		}
		itemScope := normalizeEvidenceScope(item.EvidenceScope, "experience", "", "", item.Type, item.Role, item.Contribution)
		if scope != "" && itemScope != scope {
			continue
		}
		items = append(items, radarEvidenceItem{
			Kind:            "experience",
			ExperienceType:  item.Type,
			Role:            item.Role,
			Contribution:    item.Contribution,
			EvidenceScope:   itemScope,
			Level:           float64(item.Level),
			ImpactFactor:    item.ImpactFactor,
			BenchmarkScores: item.BenchmarkScores,
		})
	}
	return items
}

func radarEmptyDimensionContributions() radarDimensionContributions {
	capped := make(map[string][]float64, len(cappedEvidenceBucketRules))
	for bucketKey := range cappedEvidenceBucketRules {
		capped[bucketKey] = nil
	}
	return radarDimensionContributions{Capped: capped}
}

func radarEvidenceStrength(level float64, impact *float64) float64 {
	levelValue := clampFloat(level, 0, 10) / 10
	if impact == nil {
		if levelValue == 0 {
			return 0
		}
		return levelValue
	}
	impactValue := clampFloat(*impact, 0, 10) / 10
	return clampFloat(levelValue*0.4+impactValue*0.6, 0, 1)
}

func radarCappedEvidenceBucketKey(item radarEvidenceItem) string {
	if radarIsLowImpactAwardOrCertificateEvidence(item) {
		return "lowImpactAwardCertificate"
	}
	if item.Kind == "award" && item.EvidenceScope == "校内" {
		return "campusAward"
	}
	if radarIsGenericCampusRoleEvidence(item) {
		return "genericCampusRole"
	}
	if radarIsUntitledProfessionalProjectEvidence(item) {
		return "untitledProject"
	}
	return radarImpactEvidenceBucketKey(item)
}

func radarImpactEvidenceBucketKey(item radarEvidenceItem) string {
	signal := item.Level
	if item.ImpactFactor != nil {
		signal = *item.ImpactFactor
	}
	switch {
	case signal >= 8.5:
		return "impactExceptional"
	case signal >= 7:
		return "impactHigh"
	case signal >= 5.5:
		return "impactMedium"
	default:
		return "impactLow"
	}
}

func radarEvidenceQualitySummary(items []radarEvidenceItem) radarEvidenceQuality {
	var quality radarEvidenceQuality
	for _, item := range items {
		bucketKey := radarCappedEvidenceBucketKey(item)
		switch bucketKey {
		case "lowImpactAwardCertificate", "campusAward", "genericCampusRole", "untitledProject", "impactLow":
			quality.CappedLowCount++
		}
		impact := 0.0
		if item.ImpactFactor != nil {
			impact = *item.ImpactFactor
		}
		if impact >= 8.5 && bucketKey == "impactExceptional" {
			quality.ExceptionalImpactCount++
		} else if impact >= 7 && (bucketKey == "impactHigh" || bucketKey == "impactExceptional") {
			quality.HighImpactCount++
		}
	}
	return quality
}

func radarIsLowImpactAwardOrCertificateEvidence(item radarEvidenceItem) bool {
	if item.Kind != "award" {
		return false
	}
	text := strings.ToLower(radarEvidenceText(item))
	impact := -1.0
	if item.ImpactFactor != nil {
		impact = *item.ImpactFactor
	}
	return (impact >= 0 && impact <= 3.5) ||
		(item.Level > 0 && item.Level <= 3) ||
		containsAny(text, "证书", "cet", "英语四级", "英语六级", "计算机二级", "nisp一级", "奖学金", "优秀学生", "优秀学生干部", "三好学生", "标兵")
}

func radarIsGenericCampusRoleEvidence(item radarEvidenceItem) bool {
	if item.Kind != "experience" || item.EvidenceScope != "校内" {
		return false
	}
	text := strings.ToLower(radarEvidenceText(item))
	return containsAny(text, "学生会", "社团", "班长", "团支书", "部长", "主席", "干部", "优秀学生", "组织活动")
}

func radarIsUntitledProfessionalProjectEvidence(item radarEvidenceItem) bool {
	if item.Kind != "experience" {
		return false
	}
	text := strings.ToLower(item.ExperienceType + item.Role + item.Contribution)
	if strings.TrimSpace(text) == "" {
		return false
	}
	if containsAny(text, "实习", "比赛", "竞赛", "任职", "社团", "学生会", "班长", "部长", "主席") {
		return false
	}
	if !containsAny(text, "项目", "科研", "研究", "课题", "实验", "系统", "平台", "模型", "算法", "开发", "漏洞", "测试", "数据") {
		return false
	}
	return !radarHasConcreteProjectTitle(item.Role)
}

func radarHasConcreteProjectTitle(role string) bool {
	normalized := strings.ReplaceAll(strings.TrimSpace(role), " ", "")
	if normalized == "" || len([]rune(normalized)) < 4 {
		return false
	}
	genericTitles := map[string]bool{
		"项目": true, "科研项目": true, "研究项目": true, "项目经历": true, "科研经历": true,
		"参与者": true, "参与人": true, "成员": true, "队员": true, "负责人": true,
		"核心成员": true, "角色未解析": true, "未解析": true,
	}
	return !genericTitles[normalized]
}

func radarEvidenceText(item radarEvidenceItem) string {
	return item.Name + item.Result + item.ExperienceType + item.Role + item.Contribution + item.EvidenceScope
}

func radarCappedEvidenceBucket(contributions []float64, cap float64) float64 {
	if len(contributions) == 0 {
		return 0
	}
	return minFloat(radarDiminishingEvidenceScore(contributions), cap)
}

func radarDiminishingEvidenceScore(contributions []float64) float64 {
	values := make([]float64, 0, len(contributions))
	for _, contribution := range contributions {
		if contribution > 0 {
			values = append(values, contribution)
		}
	}
	sort.Slice(values, func(left, right int) bool {
		return values[left] > values[right]
	})
	score := 0.0
	for _, contribution := range values {
		score = radarAddDiminishingEvidence(score, contribution)
	}
	return score
}

func radarAddDiminishingEvidence(score float64, contribution float64) float64 {
	rawDelta := (1 - score) * contribution
	if rawDelta <= 0 {
		return score
	}
	if score < radarEvidenceDiminishThreshold {
		roomBeforeThreshold := radarEvidenceDiminishThreshold - score
		if rawDelta <= roomBeforeThreshold {
			return minFloat(radarEvidenceSoftCap, score+rawDelta)
		}
		overflow := rawDelta - roomBeforeThreshold
		nextScore := radarEvidenceDiminishThreshold + overflow*radarEvidenceTailGainAt(radarEvidenceDiminishThreshold)
		return minFloat(radarEvidenceSoftCap, nextScore)
	}
	nextScore := score + rawDelta*radarEvidenceTailGainAt(score)
	return minFloat(radarEvidenceSoftCap, nextScore)
}

func radarEvidenceTailGainAt(score float64) float64 {
	if score <= radarEvidenceDiminishThreshold {
		return 0.15
	}
	if score >= radarEvidenceTailThreshold {
		return radarEvidenceTailGain
	}
	progress := (score - radarEvidenceDiminishThreshold) / (radarEvidenceTailThreshold - radarEvidenceDiminishThreshold)
	return radarEvidenceTailGain + (0.15-radarEvidenceTailGain)*(1-progress)*(1-progress)
}

func radarCombineEvidenceWithSchoolTier(evidenceScore int, academicBaseline radarAcademicBaseline, schoolTier radarSchoolTier, quality radarEvidenceQuality, itemCount int, dimensionIndex int, includeAcademicPrior bool) int {
	capValue := radarSchoolTierScoreCap(schoolTier.Config, quality)
	if !includeAcademicPrior {
		if itemCount == 0 {
			return 0
		}
		return clampInt(evidenceScore, 0, capValue)
	}
	prior := radarAcademicPriorForDimension(academicBaseline, schoolTier, dimensionIndex)
	if itemCount == 0 {
		return clampInt(int(prior+0.5), 0, capValue)
	}
	lift := float64(evidenceScore) * schoolTier.Config.LiftScale
	return clampInt(int(minFloat(float64(capValue), prior+lift)+0.5), 0, 100)
}

func radarSchoolTierScoreCap(config schoolTierConfig, quality radarEvidenceQuality) int {
	if quality.ExceptionalImpactCount > 0 {
		return config.ExceptionalCap
	}
	if quality.HighImpactCount > 0 {
		return config.HighCap
	}
	return config.NoHighCap
}

func radarAcademicPriorForDimension(academicBaseline radarAcademicBaseline, schoolTier radarSchoolTier, dimensionIndex int) float64 {
	scores := academicBaseline.Scores
	if len(scores) != len(benchmarkDimensionNames()) {
		scores = []int{50, 50, 50, 50, 50, 50}
	}
	total := 0
	valid := 0
	for _, score := range scores {
		total += score
		valid++
	}
	baselineMean := 50.0
	if valid > 0 {
		baselineMean = float64(total) / float64(valid)
	}
	dimensionOffset := 0.0
	if dimensionIndex >= 0 && dimensionIndex < len(scores) {
		dimensionOffset = float64(scores[dimensionIndex]) - baselineMean
	}
	transcriptOffset := clampFloat(float64(academicBaseline.TranscriptBase-50), -8, 12) * 0.45
	return clampFloat(schoolTier.Config.Base+transcriptOffset+dimensionOffset*0.55, 25, 85)
}

func radarAcademicBaselineVector(profile AbilityProfile) radarAcademicBaseline {
	if len(profile.MajorBaseline.Scores) == len(benchmarkDimensionNames()) {
		scores := make([]int, len(profile.MajorBaseline.Scores))
		for index, score := range profile.MajorBaseline.Scores {
			scores[index] = clampInt(score, 25, 85)
		}
		base := profile.MajorBaseline.BaseScore
		if base == 0 {
			base = 50
		}
		return radarAcademicBaseline{
			Base:           clampInt(base, 30, 85),
			TranscriptBase: radarAcademicBaseScore(profile),
			Scores:         scores,
			MajorFamily:    profile.MajorBaseline.MajorFamily,
			Source:         fallbackString(profile.MajorBaseline.Source, "major_baseline"),
		}
	}
	base := radarAcademicBaseScore(profile)
	majorText := strings.ToLower(profile.BasicInfo.Major)
	for _, item := range radarNormalizedEducation(profile) {
		majorText += strings.ToLower(item.Major)
	}
	isStem := containsAny(majorText, "计算机", "软件", "数据", "网络", "信息", "数学", "统计", "电子", "电气", "自动化", "工程", "物理", "化学", "生物")
	hasMajor := strings.TrimSpace(majorText) != ""
	scores := []int{base, base, base, base - 10, base - 4, base}
	if isStem {
		scores[0] += 3
		scores[1] -= 2
	} else {
		scores[1] += 3
	}
	if hasMajor {
		scores[2] += 5
	}
	for index, score := range scores {
		scores[index] = clampInt(score, 25, 85)
	}
	return radarAcademicBaseline{Base: base, TranscriptBase: base, Scores: scores, MajorFamily: "未知", Source: "fallback"}
}

func radarAcademicBaseScore(profile AbilityProfile) int {
	text := profile.BasicInfo.TranscriptUse
	if text == "" {
		return 50
	}
	if match := regexp.MustCompile(`(?i)GPA[:：]?\s*([0-9]+(?:\.[0-9]+)?)`).FindStringSubmatch(text); len(match) == 2 {
		value := floatValue(match[1])
		if value > 0 && value <= 4.3 {
			return radarAcademicAverageToPrior(80 + (value-3)*15)
		}
		if value > 0 && value <= 5 {
			return radarAcademicAverageToPrior(80 + (value-3.5)*10)
		}
		return radarAcademicAverageToPrior(value)
	}
	if match := regexp.MustCompile(`(?:均分|平均分|平均成绩)[:：]?\s*([0-9]+(?:\.[0-9]+)?)`).FindStringSubmatch(text); len(match) == 2 {
		return radarAcademicAverageToPrior(floatValue(match[1]))
	}
	return 50
}

func radarAcademicAverageToPrior(average float64) int {
	if average <= 0 {
		return 50
	}
	return int(clampFloat(50+(average-80)*1.6, 35, 78) + 0.5)
}

func radarSchoolTierProfile(profile AbilityProfile) radarSchoolTier {
	educations := radarNormalizedEducation(profile)
	tiers := make([]string, 0, len(educations))
	hasIndependent := false
	for _, education := range educations {
		if radarIsIndependentCollegeEducation(education) {
			hasIndependent = true
		}
		if tier := radarEducationTierKey(education); tier != "" {
			tiers = append(tiers, tier)
		}
	}
	sort.Slice(tiers, func(left, right int) bool {
		return radarSchoolTierRank(tiers[left]) < radarSchoolTierRank(tiers[right])
	})
	tierKey := "T3"
	if len(tiers) > 0 {
		tierKey = tiers[0]
	}
	if hasIndependent {
		bestNonIndependent := ""
		for _, tier := range tiers {
			if tier != "T4" {
				bestNonIndependent = tier
				break
			}
		}
		if bestNonIndependent == "T0" || bestNonIndependent == "T1" || bestNonIndependent == "T2" {
			tierKey = "T4A"
		} else {
			tierKey = "T4"
		}
	}
	config, ok := schoolTierConfigs[tierKey]
	if !ok {
		tierKey = "T3"
		config = schoolTierConfigs[tierKey]
	}
	return radarSchoolTier{Key: tierKey, Config: config}
}

func radarNormalizedEducation(profile AbilityProfile) []EducationItem {
	if len(profile.Education) > 0 {
		return profile.Education
	}
	if profile.BasicInfo.School == "" && profile.BasicInfo.Major == "" && profile.BasicInfo.Degree == "" {
		return nil
	}
	return []EducationItem{{
		School: profile.BasicInfo.School,
		Major:  profile.BasicInfo.Major,
		Degree: profile.BasicInfo.Degree,
	}}
}

func radarEducationTierKey(item EducationItem) string {
	if radarIsIndependentCollegeEducation(item) {
		return "T4"
	}
	if strings.Contains(item.Degree, "专科") {
		return "T5"
	}
	rank := item.RuankeRank
	switch {
	case item.Is985 || (rank > 0 && rank <= 50):
		return "T0"
	case item.Is211 || item.IsDoubleFirstClass || (rank > 0 && rank <= 150):
		return "T1"
	case rank > 0 && rank <= 250:
		return "T2"
	default:
		return "T3"
	}
}

func radarIsIndependentCollegeEducation(item EducationItem) bool {
	if item.SchoolKind == "independent_college" {
		return true
	}
	school := strings.TrimSpace(item.School)
	return regexp.MustCompile(`大学.+学院$`).MatchString(school) && !strings.Contains(school, "大学院")
}

func radarSchoolTierRank(tierKey string) int {
	switch tierKey {
	case "T0":
		return 0
	case "T1":
		return 1
	case "T2":
		return 2
	case "T3":
		return 3
	case "T4A":
		return 4
	case "T4":
		return 5
	case "T5":
		return 6
	default:
		return 3
	}
}

func minFloat(left float64, right float64) float64 {
	if left < right {
		return left
	}
	return right
}

func radarSummaryMap(radar aggregatedRadar) map[string]any {
	out := map[string]any{
		"source": radar.Source,
		"count":  radar.Count,
		"scores": radar.Scores,
	}
	if len(radar.Scores) == 0 {
		return out
	}
	strongest := radar.Scores[0]
	weakest := radar.Scores[0]
	for _, item := range radar.Scores[1:] {
		if item.Score > strongest.Score {
			strongest = item
		}
		if item.Score < weakest.Score {
			weakest = item
		}
	}
	out["strongest"] = strongest
	out["weakest"] = weakest
	return out
}

func adaptivePlannerPrompt(ctx map[string]any) string {
	return teamPrompt("Adaptive Planner", ctx, `
Return strict JSON only:
{
  "agent_plan": {
    "complexity": "simple|standard|complex|high_complexity",
    "reasoning_effort": "low|medium|high|xhigh",
    "synthesis_effort": "high|xhigh",
    "recommended_jobs_scope": "本次岗位探索范围",
    "rationale": "为什么需要这些视角 Agent",
    "agents": [
      {
        "key": "ascii_snake_case_key",
        "name": "Agent display name",
        "phase": "capability|evidence|education|role_mapping|risk|growth|market",
        "perspective": "这个 Agent 的判断视角",
        "reasoning_effort": "low|medium|high|xhigh",
        "focus": "这个 Agent 需要重点判断的问题"
      }
    ]
  }
}
Rules:
- You are the first Agent. Decide the team shape from resume complexity, evidence volume, benchmark status, education signals and skill ambiguity.
- Treat radar_context.overall as the strongest ability-profile signal. If radar_context has campus/external splits, assign at least one Agent focus to translate those gaps into student development actions.
- Return 3 to 6 agents. More evidence, cross-domain signals, ambiguous roles, or high-impact-but-risky evidence should use more agents and higher reasoning_effort.
- Required perspectives across the selected agents: ability fit, evidence quality, education threshold, role family mapping.
- For complex cases, add growth potential and counterfactual risk perspectives.
- Agents must judge from different angles and must not duplicate each other.
- Do not create an Agent just to penalize missing transcript or GPA. Transcript is optional supporting evidence, not a required input.
- key must be lower ascii snake_case. No spaces. No markdown.
- Do not recommend only one narrow role family in the plan.`)
}

func normalizeJobMatchingAgentPlan(data map[string]any) (jobMatchingAgentPlan, error) {
	raw := objectValue(data["agent_plan"])
	if len(raw) == 0 {
		raw = data
	}
	plan := jobMatchingAgentPlan{
		Complexity:       normalizePlanComplexity(stringValue(raw["complexity"])),
		ReasoningEffort:  normalizePlanEffort(stringValue(raw["reasoning_effort"]), "high"),
		SynthesisEffort:  normalizePlanEffort(stringValue(raw["synthesis_effort"]), "high"),
		Rationale:        truncateRunes(stringValue(raw["rationale"]), 220),
		RecommendedScope: truncateRunes(stringValue(raw["recommended_jobs_scope"]), 140),
	}
	items := objectArray(raw["agents"])
	if len(items) < minPlannedJobMatchingAgents {
		return plan, fmt.Errorf("Adaptive Planner must return at least %d agents", minPlannedJobMatchingAgents)
	}
	if len(items) > maxPlannedJobMatchingAgents {
		items = items[:maxPlannedJobMatchingAgents]
	}
	seen := map[string]bool{}
	for index, item := range items {
		agent := jobMatchingPlannedAgent{
			Key:             sanitizeAgentKey(stringValue(item["key"]), index),
			Name:            truncateRunes(stringValue(item["name"]), 48),
			Phase:           truncateRunes(normalizePlanPhase(stringValue(item["phase"])), 32),
			Perspective:     truncateRunes(stringValue(item["perspective"]), 120),
			ReasoningEffort: normalizePlanEffort(stringValue(item["reasoning_effort"]), plan.ReasoningEffort),
			Focus:           truncateRunes(stringValue(item["focus"]), 180),
		}
		if agent.Name == "" {
			agent.Name = "Perspective Agent"
		}
		if agent.Perspective == "" {
			agent.Perspective = agent.Phase
		}
		if agent.Focus == "" {
			agent.Focus = "从指定视角判断岗位适配度、风险和补证据动作。"
		}
		for seen[agent.Key] {
			agent.Key = fmt.Sprintf("%s_%d", agent.Key, index+1)
		}
		seen[agent.Key] = true
		plan.Agents = append(plan.Agents, agent)
	}
	if len(plan.Agents) < minPlannedJobMatchingAgents {
		return plan, fmt.Errorf("Adaptive Planner returned too few valid agents")
	}
	if plan.Rationale == "" {
		plan.Rationale = "Planner 根据简历复杂度、证据数量、教育门槛和岗位不确定性派生多视角 Agent。"
	}
	if plan.RecommendedScope == "" {
		plan.RecommendedScope = "本专业相关、本专业扩展和跨专业可迁移岗位。"
	}
	return plan, nil
}

func specsFromJobMatchingPlan(plan jobMatchingAgentPlan) []jobMatchingAgentSpec {
	specs := make([]jobMatchingAgentSpec, 0, len(plan.Agents))
	for index, planned := range plan.Agents {
		planned := planned
		specs = append(specs, jobMatchingAgentSpec{
			Key:             planned.Key,
			Name:            planned.Name,
			Phase:           planned.Phase,
			Perspective:     planned.Perspective,
			ReasoningEffort: planned.ReasoningEffort,
			Focus:           planned.Focus,
			AgentIndex:      index + 1,
			AgentTotal:      len(plan.Agents),
			Sequence:        index + 2,
			Prompt: func(ctx map[string]any) string {
				return perspectiveAgentPrompt(planned, ctx)
			},
		})
	}
	return specs
}

func perspectiveAgentPrompt(agent jobMatchingPlannedAgent, ctx map[string]any) string {
	task := fmt.Sprintf(`
Return strict JSON only:
{
  "agent": %q,
  "perspective": %q,
  "reasoning_effort": %q,
  "focus": %q,
  "summary": "本视角的判断结论",
  "job_hypotheses": [
    {
      "title": "可能适合的岗位",
      "category": "本专业相关|本专业扩展|跨专业可迁移",
      "support": "支持理由",
      "risk": "主要风险",
      "next_proof": "需要补充的证据",
      "confidence": 0
    }
  ],
  "signals": ["关键证据信号"],
  "risks": ["关键风险"],
  "questions_for_synthesis": ["需要 Synthesis Arbiter 到 resume_full_text 查验的疑点"],
  "recommended_actions": ["补强动作"]
}
Rules:
- Only analyze from your assigned perspective. Do not summarize all perspectives.
- Give 2 to 5 job_hypotheses when your perspective supports them.
- confidence must be 0 to 100.
- Use radar_context.overall as a strong ability baseline. Use radar_context.campus and radar_context.external to propose student development actions for campus and external contexts.
- Structured awards/experiences may omit facts present in resume_full_text. If a proof seems absent, first phrase it as a question for Synthesis Arbiter to check in resume_full_text.
- Do not say the student did not provide something unless both structured evidence and resume_full_text clearly support that absence.
- Distinguish hard gaps from soft questions. Only call a gap hard when it is essential for the job family and cannot be found in structured evidence or resume_full_text.
- Transcript and GPA are optional. Do not list missing transcript/GPA as a default risk. When relevant, say: 如果有较高 GPA、核心课程高分或课程项目佐证，会更有信服力.
- Do not invent facts. If evidence is uncertain, state the uncertainty and the simple verification question for Synthesis Arbiter.
- Keep output concise enough for the final Synthesis Arbiter to compare with other agents.`, agent.Key, agent.Perspective, agent.ReasoningEffort, agent.Focus)
	return teamPrompt(agent.Name, ctx, task)
}

func normalizePlanComplexity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "simple", "low":
		return "simple"
	case "complex", "high":
		return "complex"
	case "high_complexity", "very_complex", "xhigh":
		return "high_complexity"
	default:
		return "standard"
	}
}

func normalizePlanEffort(value string, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low", "medium", "high", "xhigh":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		if fallback == "" {
			return "medium"
		}
		return normalizePlanEffort(fallback, "medium")
	}
}

func normalizePlanPhase(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "capability", "evidence", "education", "role_mapping", "risk", "growth", "market":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "role_mapping"
	}
}

func sanitizeAgentKey(value string, index int) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			if builder.Len() > 0 {
				builder.WriteRune(r)
			}
		case r == '_' || r == '-' || r == ' ':
			if builder.Len() > 0 {
				builder.WriteRune('_')
			}
		}
	}
	key := strings.Trim(builder.String(), "_")
	reserved := map[string]bool{
		"":                  true,
		"team":              true,
		"planner":           true,
		"adaptive_planner":  true,
		"synthesizer":       true,
		"synthesis_arbiter": true,
	}
	if reserved[key] {
		key = fmt.Sprintf("perspective_%d", index+1)
	}
	if len(key) > 40 {
		key = strings.Trim(key[:40], "_")
	}
	return key
}

func truncateRunes(value string, limit int) string {
	value = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 32 {
			return -1
		}
		return r
	}, value))
	if limit <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func synthesisArbiterPrompt(ctx map[string]any) string {
	return teamPrompt("Synthesis Arbiter", ctx, `
Return this exact JSON shape and strict JSON only:
{
  "job_matching": {
    "target_role": "首选岗位名称",
    "overall_match": 0,
    "match_level": "强匹配|高潜力匹配|可迁移匹配|需补证据",
    "source": "Legato Job Matching Team via Presto",
    "method_summary": "一句话说明 Adaptive Planner 派生的多视角 Agent 如何用雷达画像、全文核验、经历证据、学历门槛和风险反证排序",
    "fit_summary": "面向学生展示的简短中文描述",
    "student_radar": [
      {"name":"逻辑","score":0},
      {"name":"语言","score":0},
      {"name":"专业","score":0},
      {"name":"领导","score":0},
      {"name":"抗压","score":0},
      {"name":"成长","score":0}
    ],
    "target_radar": [
      {"name":"逻辑","score":0},
      {"name":"语言","score":0},
      {"name":"专业","score":0},
      {"name":"领导","score":0},
      {"name":"抗压","score":0},
      {"name":"成长","score":0}
    ],
    "selected_job": {
      "rank": 1,
      "title": "岗位名称",
      "category": "本专业相关|本专业扩展|跨专业可迁移",
      "match": 0,
      "ability_match": 0,
      "experience_match": 0,
      "education_gate": "通过|有风险|高impact项目可突破|不建议",
      "education_gate_status": "pass|risk|stretch|blocked",
      "evidence_strength": "strong|medium|weak",
      "fit_summary": "该岗位与学生的简短匹配说明",
      "risk": "主要风险",
      "requirement_radar": [
        {"name":"逻辑","score":0},
        {"name":"语言","score":0},
        {"name":"专业","score":0},
        {"name":"领导","score":0},
        {"name":"抗压","score":0},
        {"name":"成长","score":0}
      ],
      "reasons": ["推荐理由1", "推荐理由2"],
      "proof_gaps": ["仍缺少的证据1", "仍缺少的证据2"],
      "next_proof": "下一步最该补充的证据"
    },
    "top_jobs": [],
    "report_sections": [
      {"name":"逻辑","student":0,"role_need":0,"difference":0,"status":"advantage|fit|limited|gap","note":"一句话解释"},
      {"name":"语言","student":0,"role_need":0,"difference":0,"status":"advantage|fit|limited|gap","note":"一句话解释"},
      {"name":"专业","student":0,"role_need":0,"difference":0,"status":"advantage|fit|limited|gap","note":"一句话解释"},
      {"name":"领导","student":0,"role_need":0,"difference":0,"status":"advantage|fit|limited|gap","note":"一句话解释"},
      {"name":"抗压","student":0,"role_need":0,"difference":0,"status":"advantage|fit|limited|gap","note":"一句话解释"},
      {"name":"成长","student":0,"role_need":0,"difference":0,"status":"advantage|fit|limited|gap","note":"一句话解释"}
    ],
    "gap_details": [
      {"capability":"能力项","current":"当前证据","expected":"岗位要求","action":"建议动作，优先写成校内或校外可执行活动","severity":"高|中|低"}
    ],
    "development_actions": [
      {"priority":"高|中|低","scope":"校内|校外","description":"一条可执行提升动作，不要包含【】前缀"}
    ],
    "recommendations": ["职业发展建议摘要，不要包含【校内·高优先级】这类前缀"],
    "recommended_reasons": ["推荐理由"],
    "agent_notes": ["Adaptive Planner", "多视角 Agent", "Synthesis Arbiter"]
  }
}
Rules:
- You receive a validated agent_plan and perspective_results from dynamically spawned agents.
- Compare agreements and conflicts across perspectives before ranking.
- You also receive resume_full_text. It is the arbiter source for facts that structured awards/experiences may have omitted.
- If a dynamic Agent marks something as missing or raises questions_for_synthesis, first check resume_full_text and structured evidence. If resume_full_text contains supporting evidence, do not keep it as a proof gap.
- Do not say the student did not provide a fact unless resume_full_text and structured evidence both make that absence clear.
- Treat transcript and GPA as optional supporting evidence. Do not mention transcript/GPA absence by default. When useful, phrase it as optional credibility: 如果有较高 GPA、核心课程高分或课程项目佐证，会更有信服力.
- radar_context.overall is the strong baseline for student_radar. Use radar_context.campus and radar_context.external to write practical development recommendations.
- student_radar should follow radar_context.overall when available; otherwise use major_baseline plus item benchmark evidence. Do not ignore the radar just because an Agent narrative is more persuasive.
- development_actions is the UI table source. Return 6 to 10 rows, sorted by priority high to low. Each row must have priority 高/中/低, scope 校内/校外, and a concrete description. Do not put scope or priority inside description.
- Recommendations must be student-development oriented summaries, not only job-screening oriented, and must not duplicate the structured prefix pattern. Include campus actions for campus radar gaps and external actions for external radar gaps through development_actions.
- top_jobs must synthesize all useful job hypotheses from perspective_results, 3 to 5 jobs.
- selected_job must match rank 1 top_jobs.
- Every job in selected_job and top_jobs must include education_gate_status and evidence_strength.
- target_radar must equal selected_job.requirement_radar.
- report_sections must compare student_radar and target_radar dimension by dimension.
- report_sections.status must be derived from difference: advantage >= 6, fit >= -3, limited >= -12, gap < -12.
- If a dynamic Agent overreaches beyond evidence, down-weight it and mention the proof gap.
- Do not create a hard gap from optional documents or from structure-only omissions that can be checked in resume_full_text.
- Keep output compact: top_jobs 3 to 5 jobs, reasons up to 3 per job, gap_details up to 4, development_actions 6 to 10, recommendations up to 4, agent_notes up to 3.
- Keep every Chinese text field concise. No long paragraphs. No markdown fences.
- Use source exactly: Legato Job Matching Team via Presto.`)
}

func teamPrompt(role string, ctx map[string]any, task string) string {
	return fmt.Sprintf(
		"You are %s in the Legato resume/job_matching backend Agent Team.\n"+
			"You are invoked by JobAgent Go backend through Presto. This is a real backend team run, not a simulated UI label.\n"+
			"Use the context and prior Agent outputs below. Output JSON only, no markdown fence, no commentary.\n"+
			"Every JSON string value must stay on one line; if a newline is necessary, escape it as \\n.\n\n"+
			"Context:\n%s\n\nTask:\n%s",
		role,
		compactJSON(ctx),
		strings.TrimSpace(task),
	)
}

func compactJSON(value any) string {
	raw, err := marshalNoEscape(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func copyJobMatchingContext(ctx map[string]any) map[string]any {
	out := make(map[string]any, len(ctx)+2)
	for key, value := range ctx {
		out[key] = value
	}
	return out
}

func compactJobMatchingAgentData(data map[string]any) map[string]any {
	if len(data) == 0 {
		return data
	}
	out := map[string]any{}
	preferredKeys := []string{
		"agent",
		"perspective",
		"reasoning_effort",
		"focus",
		"summary",
		"job_hypotheses",
		"top_jobs",
		"roles",
		"role_scores",
		"education_gate",
		"signals",
		"risks",
		"strengths",
		"gaps",
		"evidence_notes",
		"questions_for_synthesis",
		"market_notes",
		"recommended_actions",
		"next_steps",
		"rationale",
	}
	for _, key := range preferredKeys {
		if value, ok := data[key]; ok {
			out[key] = compactJobMatchingAgentValue(value, 0)
		}
	}
	if len(out) > 0 {
		return out
	}
	return compactJobMatchingAgentMap(data, 0)
}

func compactJobMatchingAgentMap(data map[string]any, depth int) map[string]any {
	out := make(map[string]any, len(data))
	for key, value := range data {
		if strings.Contains(strings.ToLower(key), "reasoning_content") {
			continue
		}
		out[key] = compactJobMatchingAgentValue(value, depth+1)
	}
	return out
}

func compactJobMatchingAgentValue(value any, depth int) any {
	switch typed := value.(type) {
	case string:
		limit := maxJobMatchingAgentResultStringRunes
		if depth > 1 {
			limit = maxJobMatchingAgentResultStringRunes / 2
		}
		return truncateRunes(typed, limit)
	case []any:
		limit := maxJobMatchingAgentResultArrayItems
		if len(typed) < limit {
			limit = len(typed)
		}
		out := make([]any, 0, limit)
		for index := 0; index < limit; index++ {
			out = append(out, compactJobMatchingAgentValue(typed[index], depth+1))
		}
		return out
	case map[string]any:
		return compactJobMatchingAgentMap(typed, depth+1)
	default:
		return typed
	}
}

func marshalNoEscape(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(buffer.Bytes()), nil
}

func extractJSONObject(text string) (map[string]any, error) {
	trimmed := trimJSONFence(text)
	candidates := balancedJSONObjectCandidates(trimmed)
	if len(candidates) == 0 {
		return nil, errors.New("output does not contain a JSON object")
	}
	var firstErr error
	for _, candidate := range candidates {
		if data, err := decodeJSONObjectCandidate(candidate); err == nil {
			return data, nil
		} else if firstErr == nil {
			firstErr = err
		}
		if data, err := decodeJSONObjectCandidate(escapeJSONControlCharsInStrings(candidate)); err == nil {
			return data, nil
		} else if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, errors.New("output does not contain a valid JSON object")
}

func trimJSONFence(text string) string {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "```") {
		return trimmed
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) == 0 || !strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
		return trimmed
	}
	lines = lines[1:]
	if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
		lines = lines[:len(lines)-1]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func balancedJSONObjectCandidates(source string) []string {
	var candidates []string
	for start := 0; start < len(source); start++ {
		if source[start] != '{' {
			continue
		}
		if end := balancedJSONObjectEnd(source, start); end >= start {
			candidates = append(candidates, source[start:end+1])
		}
	}
	return candidates
}

func balancedJSONObjectEnd(source string, start int) int {
	depth := 0
	inString := false
	escaped := false
	for index := start; index < len(source); index++ {
		char := source[index]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch char {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch char {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func decodeJSONObjectCandidate(candidate string) (map[string]any, error) {
	var data map[string]any
	decoder := json.NewDecoder(strings.NewReader(candidate))
	decoder.UseNumber()
	if err := decoder.Decode(&data); err != nil {
		return nil, err
	}
	return data, nil
}

func escapeJSONControlCharsInStrings(candidate string) string {
	var builder strings.Builder
	builder.Grow(len(candidate))
	inString := false
	escaped := false
	for index := 0; index < len(candidate); index++ {
		char := candidate[index]
		if !inString {
			builder.WriteByte(char)
			if char == '"' {
				inString = true
				escaped = false
			}
			continue
		}
		if escaped {
			builder.WriteByte(char)
			escaped = false
			continue
		}
		switch char {
		case '\\':
			builder.WriteByte(char)
			escaped = true
		case '"':
			builder.WriteByte(char)
			inString = false
		case '\n':
			builder.WriteString(`\n`)
		case '\r':
			builder.WriteString(`\r`)
		case '\t':
			builder.WriteString(`\t`)
		default:
			if char < 0x20 {
				builder.WriteString(fmt.Sprintf(`\u%04x`, char))
				continue
			}
			builder.WriteByte(char)
		}
	}
	return builder.String()
}

func validateJobMatchingTeamOutput(raw map[string]any) error {
	if len(raw) == 0 {
		return errors.New("job_matching output is empty")
	}
	if stringValue(raw["target_role"]) == "" {
		return errors.New("job_matching.target_role is required")
	}
	if len(objectArray(raw["top_jobs"])) == 0 {
		return errors.New("job_matching.top_jobs is required")
	}
	if len(buildScoreDimensions(raw["student_radar"])) != len(benchmarkDimensionNames()) {
		return errors.New("job_matching.student_radar must contain six dimensions")
	}
	if len(buildScoreDimensions(raw["target_radar"])) != len(benchmarkDimensionNames()) {
		return errors.New("job_matching.target_radar must contain six dimensions")
	}
	return nil
}

func validateJobMatchingTeamOutputAgainstContext(raw map[string]any, teamCtx map[string]any) error {
	if err := validateJobMatchingTeamOutput(raw); err != nil {
		return err
	}
	expected := authoritativeRadarFromTeamContext(teamCtx)
	if len(expected) != len(benchmarkDimensionNames()) {
		return nil
	}
	actual := canonicalScoreDimensions(buildScoreDimensions(raw["student_radar"]))
	if len(actual) != len(expected) {
		return errors.New("job_matching.student_radar cannot be compared with authoritative radar")
	}
	for index, expectedItem := range expected {
		actualItem := actual[index]
		if absInt(actualItem.Score-expectedItem.Score) > jobMatchingRadarDeviationTolerance {
			return fmt.Errorf(
				"job_matching.student_radar deviates from Benchmark profile on %s: got %d, expected %d",
				expectedItem.Name,
				actualItem.Score,
				expectedItem.Score,
			)
		}
	}
	return nil
}

func authoritativeRadarFromTeamContext(teamCtx map[string]any) []ScoreDimension {
	radarContext := objectValue(teamCtx["radar_context"])
	overall := objectValue(radarContext["overall"])
	if scores := scoreDimensionsFromAny(overall["scores"]); len(scores) == len(benchmarkDimensionNames()) {
		return scores
	}
	return scoreDimensionsFromAny(teamCtx["student_radar"])
}

func scoreDimensionsFromAny(value any) []ScoreDimension {
	switch typed := value.(type) {
	case []ScoreDimension:
		return canonicalScoreDimensions(typed)
	case []any:
		return canonicalScoreDimensions(buildScoreDimensions(typed))
	default:
		return nil
	}
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func prestoEventMessage(agentName string, event prestoEventResponse) string {
	step := ""
	if len(event.Data) > 0 {
		var payload map[string]any
		if err := json.Unmarshal(event.Data, &payload); err == nil {
			if value := intValue(payload["step"]); value > 0 {
				step = fmt.Sprintf(" step %d", value)
			}
			if message := stringValue(payload["message"]); message != "" {
				return agentName + "：" + message
			}
		}
	}
	switch event.Type {
	case "run.started":
		return agentName + "：Presto run 已启动。"
	case "model.started":
		return agentName + "：模型" + step + "开始推理。"
	case "model.delta":
		return agentName + "：正在输出。"
	case "model.done":
		return agentName + "：模型" + step + "完成推理。"
	case "tool.started":
		return agentName + "：工具调用开始。"
	case "tool.done":
		return agentName + "：工具调用完成。"
	case "tool.error":
		return agentName + "：工具调用失败。"
	case "run.done", "run.completed":
		return agentName + "：Presto run 已完成。"
	case "run.error", "run.failed":
		return agentName + "：Presto run 失败。"
	default:
		if event.Type != "" {
			return agentName + "：" + event.Type
		}
		return agentName + "：收到 Presto 事件。"
	}
}

func prestoTokenDelta(event prestoEventResponse) (string, string) {
	if event.Type != "model.delta" || len(event.Data) == 0 {
		return "", ""
	}
	var payload map[string]any
	if err := json.Unmarshal(event.Data, &payload); err != nil {
		return "", ""
	}
	data, _ := payload["data"].(map[string]any)
	channel := stringValue(data["channel"])
	if channel != "content" {
		return channel, ""
	}
	return channel, stringValue(data["text"])
}

func terminalPrestoEvent(eventType string) bool {
	return eventType == "run.done" || eventType == "run.completed" || eventType == "run.error" || eventType == "run.failed"
}

func compactPreview(text string, limit int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if len([]rune(text)) <= limit {
		return text
	}
	runes := []rune(text)
	return string(runes[:limit]) + "..."
}

func jobMatchingPreview(raw map[string]any) string {
	role := stringValue(raw["target_role"])
	score := intValue(raw["overall_match"])
	if role == "" {
		return "岗位匹配结果已生成。"
	}
	if score > 0 {
		return fmt.Sprintf("%s，匹配度 %d。", role, score)
	}
	return role + "，匹配结果已生成。"
}
