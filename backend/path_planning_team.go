package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

const pathPlanningTeamName = "Legato Path Planning Team"
const minPlannedPathAgents = 3
const maxPlannedPathAgents = 5

type pathPlanningAgentPlan struct {
	Complexity      string                    `json:"complexity"`
	ReasoningEffort string                    `json:"reasoning_effort"`
	SynthesisEffort string                    `json:"synthesis_effort"`
	Rationale       string                    `json:"rationale"`
	Horizon         string                    `json:"horizon"`
	Agents          []jobMatchingPlannedAgent `json:"agents"`
}

func (s Server) runLegatoPathPlanningTeam(ctx context.Context, job *DiagnosisJob, diagnosis Diagnosis) (*LegatoEnvelope, error) {
	client, err := s.newPrestoClient()
	if err != nil {
		return nil, err
	}
	return runLegatoPathPlanningTeamWithClient(ctx, job, diagnosis, client)
}

func runLegatoPathPlanningTeamWithClient(ctx context.Context, job *DiagnosisJob, diagnosis Diagnosis, client *prestoClient) (*LegatoEnvelope, error) {
	started := time.Now()
	teamCtx := buildPathPlanningTeamContext(diagnosis)
	emitPathPlanningTeamEvent(job, "running", jobMatchingTeamEvent{
		AgentKey: "team",
		Agent:    pathPlanningTeamName,
		Phase:    "orchestration",
		Status:   "running",
		Message:  "岗位匹配已完成，正在启动 Path Planner 生成成长路径 Agent plan。",
		Sequence: 0,
	})

	plannerSpec := jobMatchingAgentSpec{
		Key:             "path_planner",
		Name:            "Path Planner",
		Phase:           "planning",
		Perspective:     "path_orchestration",
		ReasoningEffort: "high",
		Focus:           "根据岗位匹配结果、差距明细和证据结构派生路径规划 Agent plan",
		AgentIndex:      1,
		AgentTotal:      1,
		Sequence:        1,
		Prompt:          pathPlannerPrompt,
	}
	plannerResult, err := runPathPlanningAgent(ctx, job, client, teamCtx, plannerSpec)
	if err != nil {
		return nil, jobMatchingAgentError(plannerSpec, err)
	}
	plan, err := normalizePathPlanningAgentPlan(plannerResult.Data)
	if err != nil {
		return nil, err
	}
	teamCtx["agent_plan"] = plan
	emitPathPlanningTeamEvent(job, "running", jobMatchingTeamEvent{
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
		Message:         fmt.Sprintf("Path Planner 已判定复杂度 %s，派生 %d 个路径规划 Agent。", plan.Complexity, len(plan.Agents)),
		Sequence:        plannerSpec.Sequence,
		PrestoSessionID: plannerResult.Session,
		PrestoRunID:     plannerResult.RunID,
		OutputPreview:   compactPreview(plan.Rationale, 120),
	})

	specs := specsFromPathPlanningPlan(plan)
	parallelResults, err := runPathPlanningAgentGroup(ctx, job, client, teamCtx, specs)
	if err != nil {
		return nil, err
	}
	agentResults := make([]map[string]any, 0, len(parallelResults))
	for _, result := range parallelResults {
		compactData := compactJobMatchingAgentData(result.Data)
		teamCtx[result.Spec.Key] = compactData
		agentResults = append(agentResults, map[string]any{
			"agent_key":        result.Spec.Key,
			"agent":            result.Spec.Name,
			"phase":            result.Spec.Phase,
			"perspective":      result.Spec.Perspective,
			"reasoning_effort": result.Spec.ReasoningEffort,
			"focus":            result.Spec.Focus,
			"result":           compactData,
		})
	}
	teamCtx["path_agent_results"] = agentResults

	synthesisSpec := jobMatchingAgentSpec{
		Key:             "path_synthesis_arbiter",
		Name:            "Path Synthesis Arbiter",
		Phase:           "final_synthesis",
		Perspective:     "path_plan_synthesis",
		ReasoningEffort: plan.SynthesisEffort,
		Focus:           "综合路径规划 Agent 结果，输出可渲染的阶段目标、周任务和达标标准",
		AgentIndex:      1,
		AgentTotal:      1,
		Sequence:        len(specs) + 2,
		Prompt:          pathSynthesisPrompt,
	}
	writer, err := runPathPlanningAgent(ctx, job, client, teamCtx, synthesisSpec)
	if err != nil {
		return nil, err
	}
	raw := objectValue(writer.Data["path_plan"])
	if len(raw) == 0 {
		raw = writer.Data
	}
	planOutput, err := normalizePathPlanOutput(raw)
	if err != nil {
		return nil, err
	}

	emitPathPlanningTeamEvent(job, "running", jobMatchingTeamEvent{
		AgentKey:      "team",
		Agent:         pathPlanningTeamName,
		Phase:         "orchestration",
		Complexity:    plan.Complexity,
		AgentCount:    len(plan.Agents),
		Status:        "done",
		Message:       "Path Planning Agent Team 已完成阶段目标、周任务和达标标准。",
		Sequence:      len(specs) + 3,
		OutputPreview: pathPlanningPreview(planOutput),
	})

	return &LegatoEnvelope{
		Status:     "ok",
		Target:     "resume",
		Frontend:   "presto",
		Formatter:  "presto_path_planning_team",
		ElapsedMS:  int(time.Since(started) / time.Millisecond),
		Data:       map[string]any{"path_plan": planOutput},
		Warnings:   []string{},
		Debug:      map[string]any{"agent_team": teamCtx, "agent_plan": plan},
		SourcePath: "legato://resume/path_planning_team",
	}, nil
}

func buildPathPlanningTeamContext(diagnosis Diagnosis) map[string]any {
	profile := diagnosis.AbilityProfile
	match := diagnosis.MatchingResult
	return map[string]any{
		"basic_info": map[string]any{
			"name":            profile.BasicInfo.Name,
			"school":          profile.BasicInfo.School,
			"major":           profile.BasicInfo.Major,
			"degree":          profile.BasicInfo.Degree,
			"graduation_year": profile.BasicInfo.GraduationYear,
			"target_role":     match.TargetRole,
		},
		"education":            profile.Education,
		"student_radar":        profile.RadarData,
		"radar_context":        buildJobMatchingRadarContext(profile),
		"awards":               profile.Awards,
		"experiences":          profile.Experiences,
		"matching_result":      match,
		"selected_job":         match.SelectedJob,
		"top_jobs":             profile.TopJobs,
		"report_sections":      match.ReportSections,
		"gap_details":          match.GapDetails,
		"development_actions":  match.DevelopmentActions,
		"recommendations":      match.Recommendations,
		"dimensions":           benchmarkDimensionNames(),
		"required_path_shape":  "2 到 4 个阶段，每阶段必须包含阶段目标、3 到 5 个周任务、达标标准、交付物和资源。",
		"sequencing_principle": "路径规划只能在岗位匹配完成后生成，必须围绕首选岗位和最大能力差距排序。",
		"task_design_principles": []string{
			"阶段目标必须能直接解释为何服务于 target_role。",
			"周任务必须可执行，避免泛泛学习建议。",
			"达标标准必须可验证，尽量包含数量、作品、链接、报告、面试通过率或评分阈值。",
			"校内任务优先使用课程项目、实验室、竞赛、社团或导师任务；校外任务优先使用实习、开源、作品集、企业项目、行业认证或模拟面试。",
			"不能把 optional transcript/GPA 缺失写成硬性路径任务。",
		},
	}
}

func pathPlannerPrompt(ctx map[string]any) string {
	return pathPlanningPrompt("Path Planner", ctx, `
Return strict JSON only:
{
  "agent_plan": {
    "complexity": "simple|standard|complex|high_complexity",
    "reasoning_effort": "low|medium|high|xhigh",
    "synthesis_effort": "high|xhigh",
    "horizon": "60天|90天|120天",
    "rationale": "为什么需要这些路径规划 Agent",
    "agents": [
      {
        "key": "ascii_snake_case_key",
        "name": "Agent display name",
        "phase": "stage_goal|weekly_tasks|evidence_delivery|acceptance|schedule_risk",
        "perspective": "这个 Agent 的路径规划视角",
        "reasoning_effort": "low|medium|high|xhigh",
        "focus": "这个 Agent 需要重点设计的问题"
      }
    ]
  }
}
Rules:
- You run only after Job Matching is complete. Use selected_job, report_sections, gap_details and development_actions as required inputs.
- Return 3 to 5 agents.
- Required perspectives: stage_goal, weekly_tasks, acceptance.
- Add evidence_delivery when proof gaps or weak evidence exist. Add schedule_risk for complex or high-risk plans.
- The plan must lead to a rendered PathPlan containing 阶段目标、周任务、达标标准.
- key must be lower ascii snake_case. No markdown.`)
}

func pathPlanningAgentPrompt(agent jobMatchingPlannedAgent, ctx map[string]any) string {
	task := fmt.Sprintf(`
Return strict JSON only:
{
  "agent": %q,
  "perspective": %q,
  "focus": %q,
  "summary": "本视角规划结论",
  "stage_recommendations": [
    {
      "stage": "阶段名称",
      "goal": "阶段目标",
      "task_direction": "任务方向",
      "deliverable": "交付物",
      "acceptance": ["达标标准"],
      "risk": "执行风险"
    }
  ],
  "weekly_tasks": [
    {"week":"第 1 周","task":"任务","metric":"达标指标","priority":"高|中|低"}
  ],
  "resources": [
    {"label":"资源名","url":"https://example.com"}
  ],
  "guardrails": ["不要做什么或如何避免无效投入"]
}
Rules:
- Only design from your assigned perspective.
- Every task must serve selected_job or a named gap_detail.
- Prefer measurable deliverables over vague learning.
- Do not invent credentials or achievements. Plan how to produce new evidence.
- Keep output compact and one-line JSON strings.`, agent.Key, agent.Perspective, agent.Focus)
	return pathPlanningPrompt(agent.Name, ctx, task)
}

func pathSynthesisPrompt(ctx map[string]any) string {
	return pathPlanningPrompt("Path Synthesis Arbiter", ctx, `
Return this exact JSON shape and strict JSON only:
{
  "path_plan": {
    "export_formats": ["PDF","Word"],
    "stages": [
      {
        "stage": "第 1 阶段，0 到 30 天",
        "goal": "阶段目标，必须绑定首选岗位和主要短板",
        "weeks": [
          {"week":"第 1 周","task":"具体任务","metric":"达标指标","priority":"高|中|低"}
        ],
        "resources": [
          {"label":"资源名","url":"https://example.com"}
        ],
        "standards": ["达标标准"],
        "deliverable": "阶段交付物"
      }
    ]
  }
}
Rules:
- You receive selected_job, report_sections, gap_details, development_actions and path_agent_results.
- Output 2 to 4 stages.
- Every stage must include a clear stage goal, 3 to 5 weekly tasks, at least 2 acceptance standards, and one deliverable.
- Across the full plan, include at least one task for each high or medium gap_detail unless it duplicates a stronger development_action.
- weekly tasks must be ordered and have priority 高/中/低.
- metric must be externally checkable: count, score, link, report, deployed project, interview pass criterion, code review result, or documented artifact.
- resources must use stable official or generally reachable URLs. Prefer documentation, GitHub, MDN, Playwright, OWASP, web.dev, Kaggle, arXiv only when relevant.
- Do not include fake companies, paid courses, or fabricated resource URLs.
- No simulated-data labels. This is the real Legato Path Planning Team output.
- Keep all Chinese text concise. No markdown fences.`)
}

func normalizePathPlanningAgentPlan(data map[string]any) (pathPlanningAgentPlan, error) {
	raw := objectValue(data["agent_plan"])
	if len(raw) == 0 {
		raw = data
	}
	plan := pathPlanningAgentPlan{
		Complexity:      normalizePlanComplexity(stringValue(raw["complexity"])),
		ReasoningEffort: normalizePlanEffort(stringValue(raw["reasoning_effort"]), "high"),
		SynthesisEffort: normalizePlanEffort(stringValue(raw["synthesis_effort"]), "high"),
		Rationale:       truncateRunes(stringValue(raw["rationale"]), 220),
		Horizon:         truncateRunes(stringValue(raw["horizon"]), 40),
	}
	items := objectArray(raw["agents"])
	if len(items) < minPlannedPathAgents {
		return plan, fmt.Errorf("Path Planner must return at least %d agents", minPlannedPathAgents)
	}
	if len(items) > maxPlannedPathAgents {
		items = items[:maxPlannedPathAgents]
	}
	seen := map[string]bool{}
	for index, item := range items {
		agent := jobMatchingPlannedAgent{
			Key:             sanitizeAgentKey(stringValue(item["key"]), index),
			Name:            truncateRunes(stringValue(item["name"]), 48),
			Phase:           normalizePathPlanPhase(stringValue(item["phase"])),
			Perspective:     truncateRunes(stringValue(item["perspective"]), 120),
			ReasoningEffort: normalizePlanEffort(stringValue(item["reasoning_effort"]), plan.ReasoningEffort),
			Focus:           truncateRunes(stringValue(item["focus"]), 180),
		}
		if agent.Name == "" {
			agent.Name = "Path Agent"
		}
		if agent.Perspective == "" {
			agent.Perspective = agent.Phase
		}
		if agent.Focus == "" {
			agent.Focus = "把岗位差距转成阶段目标、周任务和达标标准。"
		}
		for seen[agent.Key] {
			agent.Key = fmt.Sprintf("%s_%d", agent.Key, index+1)
		}
		seen[agent.Key] = true
		plan.Agents = append(plan.Agents, agent)
	}
	if len(plan.Agents) < minPlannedPathAgents {
		return plan, errors.New("Path Planner returned too few valid agents")
	}
	if plan.Rationale == "" {
		plan.Rationale = "Path Planner 根据岗位匹配结果、六维差距和证据缺口派生路径规划 Agent。"
	}
	if plan.Horizon == "" {
		plan.Horizon = "90天"
	}
	return plan, nil
}

func normalizePathPlanPhase(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "stage_goal", "weekly_tasks", "evidence_delivery", "acceptance", "schedule_risk":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "weekly_tasks"
	}
}

func specsFromPathPlanningPlan(plan pathPlanningAgentPlan) []jobMatchingAgentSpec {
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
				return pathPlanningAgentPrompt(planned, ctx)
			},
		})
	}
	return specs
}

func runPathPlanningAgentGroup(ctx context.Context, job *DiagnosisJob, client *prestoClient, teamCtx map[string]any, specs []jobMatchingAgentSpec) ([]jobMatchingAgentResult, error) {
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
				emitPathPlanningAgentFailure(job, spec, "", "", pathPlanningAgentFailureMessage(spec, ctx.Err()), ctx.Err())
				results <- jobMatchingAgentResult{Spec: spec, Err: err}
				return
			}
			result, err := runPathPlanningAgent(ctx, job, client, teamCtx, spec)
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

func runPathPlanningAgent(ctx context.Context, job *DiagnosisJob, client *prestoClient, teamCtx map[string]any, spec jobMatchingAgentSpec) (jobMatchingAgentResult, error) {
	prompt := spec.Prompt(teamCtx)
	jobID := ""
	if job != nil {
		jobID = job.ID
	}
	log.Printf("path planning prompt job=%s agent=%s phase=%s prompt_bytes=%d", jobID, spec.Key, spec.Phase, len(prompt))
	output, sessionID, runID, err := client.runPathPlanningPromptStream(ctx, job, spec, prompt)
	result := jobMatchingAgentResult{Spec: spec, Output: output, RunID: runID, Session: sessionID}
	if err != nil {
		wrapped := jobMatchingAgentError(spec, err)
		result.Err = wrapped
		emitPathPlanningAgentFailure(job, spec, sessionID, runID, pathPlanningAgentFailureMessage(spec, err), err)
		return result, wrapped
	}
	data, err := extractJSONObject(output)
	if err != nil && ctx.Err() == nil {
		emitPathPlanningTeamEvent(job, "running", jobMatchingTeamEvent{
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
		output, sessionID, runID, err = client.runPathPlanningPromptStream(ctx, job, spec, retryPrompt)
		result = jobMatchingAgentResult{Spec: spec, Output: output, RunID: runID, Session: sessionID}
		if err != nil {
			wrapped := jobMatchingAgentError(spec, err)
			result.Err = wrapped
			emitPathPlanningAgentFailure(job, spec, sessionID, runID, pathPlanningAgentFailureMessage(spec, err), err)
			return result, wrapped
		}
		data, err = extractJSONObject(output)
	}
	if err != nil {
		result.Err = err
		emitPathPlanningAgentFailure(job, spec, sessionID, runID, spec.Name+" 返回的 JSON 无法解析。", err)
		return result, err
	}
	result.Data = data
	emitPathPlanningTeamEvent(job, "running", jobMatchingTeamEvent{
		AgentKey:        spec.Key,
		Agent:           spec.Name,
		AgentIndex:      spec.AgentIndex,
		AgentTotal:      spec.AgentTotal,
		Phase:           spec.Phase,
		Perspective:     spec.Perspective,
		ReasoningEffort: spec.ReasoningEffort,
		Focus:           spec.Focus,
		Status:          "done",
		Message:         spec.Name + " 已返回结构化路径规划结果。",
		Sequence:        spec.Sequence,
		PrestoSessionID: sessionID,
		PrestoRunID:     runID,
		OutputPreview:   compactPreview(output, 140),
	})
	return result, nil
}

func (c *prestoClient) runPathPlanningPromptStream(ctx context.Context, job *DiagnosisJob, spec jobMatchingAgentSpec, prompt string) (string, string, string, error) {
	session, err := c.createSession(ctx, map[string]string{
		"app":              "legato",
		"workflow":         "resume",
		"team":             "path_planning",
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
	emitPathPlanningTeamEvent(job, "running", jobMatchingTeamEvent{
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
		emitPathPlanningTeamEvent(job, "running", jobMatchingTeamEvent{
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

func normalizePathPlanOutput(raw map[string]any) (PathPlan, error) {
	plan := PathPlan{ExportFormats: []string{"PDF", "Word"}}
	if formats := stringArrayValue(raw["export_formats"]); len(formats) > 0 {
		plan.ExportFormats = formats
	}
	for _, stageRaw := range objectArray(raw["stages"]) {
		stage := PlanStage{
			Stage:       truncateRunes(stringValue(stageRaw["stage"]), 80),
			Goal:        truncateRunes(stringValue(stageRaw["goal"]), 180),
			Deliverable: truncateRunes(stringValue(stageRaw["deliverable"]), 140),
			Standards:   cleanPathStringList(stageRaw["standards"], 5, 120),
		}
		for _, weekRaw := range objectArray(stageRaw["weeks"]) {
			week := WeeklyTask{
				Week:     truncateRunes(stringValue(weekRaw["week"]), 24),
				Task:     truncateRunes(stringValue(weekRaw["task"]), 180),
				Metric:   truncateRunes(stringValue(weekRaw["metric"]), 120),
				Priority: normalizePathPriority(stringValue(weekRaw["priority"])),
			}
			if week.Week == "" || week.Task == "" || week.Metric == "" {
				continue
			}
			stage.Weeks = append(stage.Weeks, week)
		}
		for _, resourceRaw := range objectArray(stageRaw["resources"]) {
			resource := Resource{
				Label: truncateRunes(stringValue(resourceRaw["label"]), 60),
				URL:   truncateRunes(stringValue(resourceRaw["url"]), 180),
			}
			if resource.Label == "" || !strings.HasPrefix(resource.URL, "http") {
				continue
			}
			stage.Resources = append(stage.Resources, resource)
			if len(stage.Resources) >= 4 {
				break
			}
		}
		if stage.Stage == "" || stage.Goal == "" || len(stage.Weeks) < 2 || len(stage.Standards) == 0 {
			continue
		}
		if stage.Deliverable == "" {
			stage.Deliverable = "阶段作品、复盘文档和可验证证据"
		}
		plan.Stages = append(plan.Stages, stage)
	}
	if len(plan.Stages) < 2 {
		return plan, errors.New("Path Planning output must contain at least 2 valid stages")
	}
	if len(plan.Stages) > 4 {
		plan.Stages = plan.Stages[:4]
	}
	return plan, nil
}

func cleanPathStringList(value any, limit int, runeLimit int) []string {
	items := stringArrayValue(value)
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = truncateRunes(item, runeLimit)
		if item != "" {
			out = append(out, item)
		}
		if len(out) >= limit {
			break
		}
	}
	return out
}

func normalizePathPriority(value string) string {
	switch strings.TrimSpace(value) {
	case "高", "中", "低":
		return strings.TrimSpace(value)
	default:
		return "中"
	}
}

func pathPlanningPreview(plan PathPlan) string {
	if len(plan.Stages) == 0 {
		return "Path Planning Team 已生成路径规划。"
	}
	return compactPreview(plan.Stages[0].Stage+"："+plan.Stages[0].Goal, 140)
}

func pathPlanningPrompt(role string, ctx map[string]any, task string) string {
	return fmt.Sprintf(
		"You are %s in the Legato resume/path_planning backend Agent Team.\n"+
			"You are invoked only after Job Matching has completed. This is a real backend workflow, not a simulated UI label.\n"+
			"Use the context and prior Agent outputs below. Output JSON only, no markdown fence, no commentary.\n"+
			"Every JSON string value must stay on one line; if a newline is necessary, escape it as \\n.\n\n"+
			"Context:\n%s\n\nTask:\n%s",
		role,
		compactJSON(ctx),
		strings.TrimSpace(task),
	)
}

func emitPathPlanningAgentFailure(job *DiagnosisJob, spec jobMatchingAgentSpec, sessionID string, runID string, message string, cause error) {
	errorText := ""
	if cause != nil {
		errorText = cause.Error()
	}
	emitPathPlanningTeamEvent(job, "failed", jobMatchingTeamEvent{
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

func pathPlanningAgentFailureMessage(spec jobMatchingAgentSpec, err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return spec.Name + " 超过 Path Planning 总超时，模型或 Presto run 未在预算内返回终止事件。"
	}
	if errors.Is(err, context.Canceled) {
		return spec.Name + " 已被取消，Path Planning 流程提前终止。"
	}
	return spec.Name + " 运行失败。"
}

func emitPathPlanningTeamEvent(job *DiagnosisJob, outerStatus string, event jobMatchingTeamEvent) {
	if job == nil {
		return
	}
	event.Team = pathPlanningTeamName
	event.Workflow = "resume/path_planning"
	if event.Status == "" {
		event.Status = "running"
	}
	job.Emit(DiagnosisEvent{
		Type:    "step.update",
		Step:    "path",
		Status:  outerStatus,
		Message: event.Message,
		Data: map[string]any{
			"agent_team_event": event,
		},
	})
}
