package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const jobMatchingTeamName = "Legato Job Matching Team"
const maxPlannedJobMatchingAgents = 6
const minPlannedJobMatchingAgents = 3
const maxJobMatchingAgentConcurrency = 3

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
	teamCtx := buildJobMatchingTeamContext(diagnosis)
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
		return nil, err
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
		teamCtx[result.Spec.Key] = result.Data
		perspectiveResults = append(perspectiveResults, map[string]any{
			"agent_key":        result.Spec.Key,
			"agent":            result.Spec.Name,
			"phase":            result.Spec.Phase,
			"perspective":      result.Spec.Perspective,
			"reasoning_effort": result.Spec.ReasoningEffort,
			"focus":            result.Spec.Focus,
			"result":           result.Data,
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
		reportWriter, err = retrySynthesisArbiter(ctx, job, client, teamCtx, synthesisSpec, err)
		if err != nil {
			return nil, err
		}
	}
	finalRaw := objectValue(reportWriter.Data["job_matching"])
	if len(finalRaw) == 0 {
		finalRaw = reportWriter.Data
	}
	if err := validateJobMatchingTeamOutput(finalRaw); err != nil {
		reportWriter, err = retrySynthesisArbiter(ctx, job, client, teamCtx, synthesisSpec, err)
		if err != nil {
			return nil, err
		}
		finalRaw = objectValue(reportWriter.Data["job_matching"])
		if len(finalRaw) == 0 {
			finalRaw = reportWriter.Data
		}
		if err := validateJobMatchingTeamOutput(finalRaw); err != nil {
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
	retryCtx["synthesis_retry_instruction"] = "Previous Synthesis Arbiter output failed backend parsing or validation. Return strict JSON only and exactly satisfy target_role, non-empty top_jobs, six student_radar dimensions, and six target_radar dimensions."
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
				results <- jobMatchingAgentResult{Spec: spec, Err: ctx.Err()}
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
	output, sessionID, runID, err := client.runPromptStream(ctx, job, spec, prompt)
	result := jobMatchingAgentResult{Spec: spec, Output: output, RunID: runID, Session: sessionID}
	if err != nil {
		result.Err = err
		return result, err
	}
	data, err := extractJSONObject(output)
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

func buildJobMatchingTeamContext(diagnosis Diagnosis) map[string]any {
	profile := diagnosis.AbilityProfile
	evidenceCount := len(profile.Awards) + len(profile.Experiences)
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
		"education":       profile.Education,
		"major_baseline":  profile.MajorBaseline,
		"student_radar":   profile.RadarData,
		"awards":          profile.Awards,
		"experiences":     profile.Experiences,
		"dimensions":      benchmarkDimensionNames(),
		"benchmark_state": map[string]any{"benchmark_status": profile.BenchmarkStatus, "major_baseline_status": profile.MajorBaselineStatus},
		"complexity_signals": map[string]any{
			"award_count":      len(profile.Awards),
			"experience_count": len(profile.Experiences),
			"evidence_count":   evidenceCount,
			"has_major_prior":  len(profile.MajorBaseline.Scores) > 0,
			"has_benchmark":    profile.BenchmarkStatus == "ready",
			"has_education":    len(profile.Education) > 0,
		},
		"principles": []string{
			"六维能力作为第一排序依据。",
			"经历和项目证据作为第二排序依据。",
			"学历作为门槛和风险约束，不单独决定排序。",
			"必须自动推荐岗位，不能要求用户主动输入目标岗位。",
			"至少覆盖本专业相关或扩展岗位，并在证据支持时给出跨专业可迁移岗位。",
		},
	}
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
- Return 3 to 6 agents. More evidence, cross-domain signals, ambiguous roles, or high-impact-but-risky evidence should use more agents and higher reasoning_effort.
- Required perspectives across the selected agents: ability fit, evidence quality, education threshold, role family mapping.
- For complex cases, add growth potential and counterfactual risk perspectives.
- Agents must judge from different angles and must not duplicate each other.
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
  "recommended_actions": ["补强动作"]
}
Rules:
- Only analyze from your assigned perspective. Do not summarize all perspectives.
- Give 2 to 5 job_hypotheses when your perspective supports them.
- confidence must be 0 to 100.
- Do not invent facts. If evidence is missing, state the missing proof.
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
    "method_summary": "一句话说明 Adaptive Planner 派生的多视角 Agent 如何用六维能力、经历证据、学历门槛和风险反证排序",
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
      "next_proof": "下一步最该补充的证据"
    },
    "top_jobs": [],
    "report_sections": [
      {"name":"逻辑","student":0,"role_need":0,"difference":0},
      {"name":"语言","student":0,"role_need":0,"difference":0},
      {"name":"专业","student":0,"role_need":0,"difference":0},
      {"name":"领导","student":0,"role_need":0,"difference":0},
      {"name":"抗压","student":0,"role_need":0,"difference":0},
      {"name":"成长","student":0,"role_need":0,"difference":0}
    ],
    "gap_details": [
      {"capability":"能力项","current":"当前证据","expected":"岗位要求","action":"建议动作","severity":"高|中|低"}
    ],
    "recommendations": ["职业发展建议"],
    "recommended_reasons": ["推荐理由"],
    "agent_notes": ["Adaptive Planner", "多视角 Agent", "Synthesis Arbiter"]
  }
}
Rules:
- You receive a validated agent_plan and perspective_results from dynamically spawned agents.
- Compare agreements and conflicts across perspectives before ranking.
- top_jobs must synthesize all useful job hypotheses from perspective_results, 3 to 5 jobs.
- selected_job must match rank 1 top_jobs.
- target_radar must equal selected_job.requirement_radar.
- report_sections must compare student_radar and target_radar dimension by dimension.
- If a dynamic Agent overreaches beyond evidence, down-weight it and mention the proof gap.
- Use source exactly: Legato Job Matching Team via Presto.`)
}

func teamPrompt(role string, ctx map[string]any, task string) string {
	return fmt.Sprintf(
		"You are %s in the Legato resume/job_matching backend Agent Team.\n"+
			"You are invoked by JobAgent Go backend through Presto. This is a real backend team run, not a simulated UI label.\n"+
			"Use the context and prior Agent outputs below. Output JSON only, no markdown fence, no commentary.\n\n"+
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
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimPrefix(trimmed, "```json")
		trimmed = strings.TrimPrefix(trimmed, "```")
		trimmed = strings.TrimSuffix(trimmed, "```")
		trimmed = strings.TrimSpace(trimmed)
	}
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start < 0 || end < start {
		return nil, errors.New("output does not contain a JSON object")
	}
	var data map[string]any
	decoder := json.NewDecoder(strings.NewReader(trimmed[start : end+1]))
	decoder.UseNumber()
	if err := decoder.Decode(&data); err != nil {
		return nil, err
	}
	return data, nil
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
