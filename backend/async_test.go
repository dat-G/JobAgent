package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestChunkBenchmarkInputsCapsRequestCount(t *testing.T) {
	items := make([]BenchmarkEvidenceInput, 7)
	for index := range items {
		items[index] = BenchmarkEvidenceInput{Kind: "award", Key: "award:test"}
	}

	chunks := chunkBenchmarkInputs(items, 5)
	if len(chunks) != 5 {
		t.Fatalf("expected 5 chunks, got %d", len(chunks))
	}
	total := 0
	for index, chunk := range chunks {
		if len(chunk) == 0 {
			t.Fatalf("chunk %d is empty", index)
		}
		total += len(chunk)
	}
	if total != len(items) {
		t.Fatalf("expected %d total items, got %d", len(items), total)
	}
}

func TestItemBenchmarkMaxRequestsUsesEnv(t *testing.T) {
	t.Setenv("ITEM_BENCHMARK_MAX_REQUESTS", "3")
	t.Setenv("BENCHMARK_MAX_REQUESTS", "")

	if got := itemBenchmarkMaxRequests(); got != 3 {
		t.Fatalf("expected env max requests 3, got %d", got)
	}
}

func TestJobMatchingTimeoutUsesDedicatedEnv(t *testing.T) {
	t.Setenv("JOB_MATCHING_TIMEOUT_SECONDS", "240")

	if got := jobMatchingTimeout(); got.Seconds() != 240 {
		t.Fatalf("expected job matching timeout 240s, got %s", got)
	}
}

func TestJobMatchingFailureLimitationsAreDeduplicatedAndCleared(t *testing.T) {
	message := "Job Matching 失败，岗位推荐和匹配雷达未生成，可点击匹配阶段继续或稍后重试。"
	limitations := addProductionLimitationOnce([]string{"其他限制"}, message)
	limitations = addProductionLimitationOnce(limitations, message)

	if got := len(limitations); got != 2 {
		t.Fatalf("expected deduplicated limitations length 2, got %d", got)
	}
	cleared := withoutJobMatchingFailureLimitations(limitations)
	if got := len(cleared); got != 1 || cleared[0] != "其他限制" {
		t.Fatalf("expected only unrelated limitation to remain, got %#v", cleared)
	}
}

func TestRunLegatoJobMatchingTeamStreamsPrestoAgents(t *testing.T) {
	var mu sync.Mutex
	sessionAgents := map[string]string{}
	runAgents := map[string]string{}
	seenAgents := map[string]bool{}
	fakeTransport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		path := strings.Trim(r.URL.Path, "/")
		status := http.StatusOK
		headers := http.Header{"Content-Type": []string{"application/json"}}
		var body string
		switch {
		case r.Method == http.MethodPost && path == "sessions":
			var req struct {
				Metadata map[string]string `json:"metadata"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				return nil, fmt.Errorf("decode session request: %w", err)
			}
			agent := req.Metadata["agent"]
			sessionID := "sess_" + agent
			mu.Lock()
			sessionAgents[sessionID] = agent
			seenAgents[agent] = true
			mu.Unlock()
			status = http.StatusCreated
			body = compactJSON(map[string]any{"id": sessionID})
		case r.Method == http.MethodPost && strings.HasPrefix(path, "sessions/") && strings.HasSuffix(path, "/runs"):
			parts := strings.Split(path, "/")
			sessionID := parts[1]
			mu.Lock()
			agent := sessionAgents[sessionID]
			runID := fmt.Sprintf("run_%d_%s", len(runAgents)+1, agent)
			runAgents[runID] = agent
			mu.Unlock()
			status = http.StatusCreated
			body = compactJSON(map[string]any{"id": runID, "session_id": sessionID, "status": "queued"})
		case r.Method == http.MethodGet && strings.HasPrefix(path, "runs/") && strings.HasSuffix(path, "/events"):
			parts := strings.Split(path, "/")
			runID := parts[1]
			headers.Set("Content-Type", "text/event-stream")
			body = fmt.Sprintf(
				"id: e1\nevent: run.started\ndata: {\"type\":\"run.started\",\"run_id\":%q}\n\n"+
					"id: e2\nevent: model.started\ndata: {\"type\":\"model.started\",\"run_id\":%q,\"step\":1}\n\n"+
					"id: e3\nevent: model.done\ndata: {\"type\":\"model.done\",\"run_id\":%q,\"step\":1}\n\n"+
					"id: e4\nevent: run.done\ndata: {\"type\":\"run.done\",\"run_id\":%q,\"step\":1}\n\n",
				runID, runID, runID, runID,
			)
		case r.Method == http.MethodGet && strings.HasPrefix(path, "runs/"):
			parts := strings.Split(path, "/")
			runID := parts[1]
			mu.Lock()
			agent := runAgents[runID]
			mu.Unlock()
			body = compactJSON(map[string]any{
				"id":         runID,
				"session_id": "sess_" + agent,
				"status":     "completed",
				"output":     fakeJobMatchingAgentOutput(agent),
			})
		default:
			return nil, fmt.Errorf("unexpected fake Presto request: %s %s", r.Method, r.URL.Path)
		}
		return &http.Response{
			StatusCode: status,
			Header:     headers,
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	})

	client := &prestoClient{
		base:   parseOptionalURL("http://presto.test"),
		client: &http.Client{Transport: fakeTransport},
	}
	job := NewJobStore().Create(nil)
	diagnosis := Diagnosis{AbilityProfile: AbilityProfile{
		BasicInfo: BasicInfo{Name: "陈曦", School: "东北农业大学", Major: "计算机科学与技术", Degree: "本科"},
		Education: []EducationItem{{
			School:     "东北农业大学",
			Degree:     "本科",
			Department: "电气与信息学院",
			Major:      "计算机科学与技术",
			Is211:      true,
			RuankeRank: 120,
		}},
		RadarData: []ScoreDimension{
			{Name: "逻辑", Score: 74, MaxScore: 100},
			{Name: "语言", Score: 64, MaxScore: 100},
			{Name: "专业", Score: 80, MaxScore: 100},
			{Name: "领导", Score: 62, MaxScore: 100},
			{Name: "抗压", Score: 68, MaxScore: 100},
			{Name: "成长", Score: 76, MaxScore: 100},
		},
	}}

	envelope, err := runLegatoJobMatchingTeamWithClient(context.Background(), job, diagnosis, client)
	if err != nil {
		t.Fatalf("run team: %v", err)
	}
	raw := objectValue(envelope.Data["job_matching"])
	if got := stringValue(raw["target_role"]); got != "前端开发工程师" {
		t.Fatalf("expected target role from report writer, got %q", got)
	}
	for _, agent := range []string{"adaptive_planner", "ability_fit_agent", "evidence_quality_agent", "education_threshold_agent", "role_family_agent", "synthesis_arbiter"} {
		if !seenAgents[agent] {
			t.Fatalf("expected fake Presto to see agent %q", agent)
		}
	}
	snapshot := job.Snapshot()
	events, ok := snapshot["events"].([]DiagnosisEvent)
	if !ok {
		t.Fatalf("expected diagnosis events in snapshot")
	}
	streamEvents := 0
	synthesisStarted := false
	synthesisDone := false
	for _, event := range events {
		data, _ := event.Data.(map[string]any)
		teamEvent, ok := data["agent_team_event"].(jobMatchingTeamEvent)
		if !ok {
			continue
		}
		streamEvents++
		if teamEvent.AgentKey == "synthesis_arbiter" {
			if teamEvent.Status == "running" || teamEvent.Status == "streaming" {
				synthesisStarted = true
			}
			if teamEvent.Status == "done" {
				synthesisDone = true
			}
		}
	}
	if streamEvents == 0 {
		t.Fatalf("expected Agent Team events to be emitted")
	}
	if !synthesisStarted || !synthesisDone {
		t.Fatalf("expected Synthesis Arbiter stream events, started=%v done=%v", synthesisStarted, synthesisDone)
	}
}

func TestApplyResumeWorkflowExperienceHybridReplacesExperienceList(t *testing.T) {
	diagnosis := Diagnosis{
		AbilityProfile: AbilityProfile{
			Experiences: []ExperienceItem{{
				Type:         "实习",
				Role:         "旧公司 / 旧岗位",
				Contribution: "旧贡献",
				Level:        3,
				HybridStatus: "pending",
			}},
		},
	}
	result := &LegatoEnvelope{
		Data: map[string]any{
			"experience": []any{
				map[string]any{
					"type":           "任职",
					"role":           "哈尔滨威科赛斯生物科技有限公司 / 生物实验员",
					"contribution":   "生物实验员岗位实践",
					"level":          float64(4),
					"evidence_scope": "校外",
				},
				map[string]any{
					"type":           "科研项目",
					"role":           "疫苗制备项目 / 负责人",
					"contribution":   "制备疫苗并验证免疫效果",
					"level":          float64(6),
					"evidence_scope": "校外",
				},
			},
		},
	}

	applyResumeWorkflowExperience(&diagnosis, result, true)

	if got := len(diagnosis.AbilityProfile.Experiences); got != 2 {
		t.Fatalf("expected hybrid to replace with 2 experiences, got %d", got)
	}
	if got := diagnosis.AbilityProfile.Experiences[0].Role; got != "哈尔滨威科赛斯生物科技有限公司 / 生物实验员" {
		t.Fatalf("expected first role from hybrid result, got %q", got)
	}
	if got := diagnosis.AbilityProfile.ExperiencesStatus; got != "ready" {
		t.Fatalf("expected experiences status ready, got %q", got)
	}
	for index, item := range diagnosis.AbilityProfile.Experiences {
		if item.HybridStatus != "ready" {
			t.Fatalf("expected item %d hybrid status ready, got %q", index, item.HybridStatus)
		}
	}
}

func TestApplyResumeWorkflowJobMatchingMapsRadarAndTopJobs(t *testing.T) {
	targetRadar := []any{
		map[string]any{"name": "逻辑", "score": float64(76)},
		map[string]any{"name": "语言", "score": float64(64)},
		map[string]any{"name": "专业", "score": float64(82)},
		map[string]any{"name": "领导", "score": float64(68)},
		map[string]any{"name": "抗压", "score": float64(72)},
		map[string]any{"name": "成长", "score": float64(78)},
	}
	result := &LegatoEnvelope{
		Data: map[string]any{
			"job_matching": map[string]any{
				"target_role":    "前端开发工程师",
				"overall_match":  float64(81),
				"match_level":    "高潜力匹配",
				"source":         "Legato Job Matching Team via Presto",
				"method_summary": "六维能力优先，经历证据第二，学历门槛通过。",
				"fit_summary":    "专业和经历支撑前端方向。",
				"student_radar": []any{
					map[string]any{"name": "逻辑", "score": float64(72)},
					map[string]any{"name": "语言", "score": float64(62)},
					map[string]any{"name": "专业", "score": float64(79)},
					map[string]any{"name": "领导", "score": float64(66)},
					map[string]any{"name": "抗压", "score": float64(70)},
					map[string]any{"name": "成长", "score": float64(75)},
				},
				"target_radar": targetRadar,
				"selected_job": map[string]any{
					"rank":              float64(1),
					"title":             "前端开发工程师",
					"category":          "本专业相关",
					"match":             float64(81),
					"ability_match":     float64(82),
					"experience_match":  float64(76),
					"education_gate":    "通过",
					"requirement_radar": targetRadar,
					"reasons":           []any{"专业背景相关", "项目证据可迁移"},
					"next_proof":        "补充工程化测试证据",
				},
				"top_jobs": []any{
					map[string]any{
						"rank":              float64(1),
						"title":             "前端开发工程师",
						"category":          "本专业相关",
						"match":             float64(81),
						"ability_match":     float64(82),
						"experience_match":  float64(76),
						"education_gate":    "通过",
						"requirement_radar": targetRadar,
						"reasons":           []any{"专业背景相关"},
						"next_proof":        "补充工程化测试证据",
					},
				},
				"report_sections": []any{},
				"gap_details": []any{
					map[string]any{"capability": "工程化测试", "current": "未体现", "expected": "需要测试证据", "action": "补 Playwright", "severity": "高"},
				},
				"recommendations":     []any{"先补作品集"},
				"recommended_reasons": []any{"经历相关"},
				"agent_notes":         []any{"六维能力优先"},
			},
		},
	}
	diagnosis := Diagnosis{}

	applyResumeWorkflowJobMatching(&diagnosis, result)

	if got := diagnosis.MatchingResult.TargetRole; got != "前端开发工程师" {
		t.Fatalf("expected target role mapped, got %q", got)
	}
	if got := len(diagnosis.MatchingResult.TargetRadar); got != 6 {
		t.Fatalf("expected target radar length 6, got %d", got)
	}
	if got := len(diagnosis.AbilityProfile.TopJobs); got != 1 {
		t.Fatalf("expected one top job, got %d", got)
	}
	if got := diagnosis.AbilityProfile.TopJobs[0].RequirementRadar[2].Score; got != 82 {
		t.Fatalf("expected professional target score 82, got %d", got)
	}
	if got := diagnosis.AbilityProfile.BasicInfo.TargetRole; got != "前端开发工程师" {
		t.Fatalf("expected basic info target role updated, got %q", got)
	}
}

func fakeJobMatchingAgentOutput(agent string) string {
	radar := []map[string]any{
		{"name": "逻辑", "score": 74},
		{"name": "语言", "score": 64},
		{"name": "专业", "score": 80},
		{"name": "领导", "score": 62},
		{"name": "抗压", "score": 68},
		{"name": "成长", "score": 76},
	}
	targetRadar := []map[string]any{
		{"name": "逻辑", "score": 78},
		{"name": "语言", "score": 62},
		{"name": "专业", "score": 86},
		{"name": "领导", "score": 64},
		{"name": "抗压", "score": 72},
		{"name": "成长", "score": 78},
	}
	switch agent {
	case "adaptive_planner":
		return compactJSON(map[string]any{
			"agent_plan": map[string]any{
				"complexity":             "complex",
				"reasoning_effort":       "high",
				"synthesis_effort":       "high",
				"recommended_jobs_scope": "本专业相关、本专业扩展和跨专业可迁移岗位。",
				"rationale":              "证据包含专业背景、项目经历和岗位方向不确定性，需要多视角判断。",
				"agents": []map[string]any{
					{"key": "ability_fit_agent", "name": "Ability Fit Agent", "phase": "capability", "perspective": "六维能力与岗位能力模型", "reasoning_effort": "high", "focus": "根据学生雷达判断可胜任岗位族"},
					{"key": "evidence_quality_agent", "name": "Evidence Quality Agent", "phase": "evidence", "perspective": "经历证据质量与可验证性", "reasoning_effort": "medium", "focus": "判断项目、奖项、实习证据能支撑哪些岗位"},
					{"key": "education_threshold_agent", "name": "Education Threshold Agent", "phase": "education", "perspective": "学历和专业门槛", "reasoning_effort": "medium", "focus": "判断学校、专业、学历是否构成门槛或风险"},
					{"key": "role_family_agent", "name": "Role Family Agent", "phase": "role_mapping", "perspective": "岗位族映射与迁移可能", "reasoning_effort": "high", "focus": "给出本专业相关、扩展和跨专业可迁移岗位假设"},
				},
			},
		})
	case "ability_fit_agent", "evidence_quality_agent", "education_threshold_agent", "role_family_agent":
		return compactJSON(map[string]any{
			"agent":            agent,
			"perspective":      "测试视角",
			"reasoning_effort": "high",
			"focus":            "测试 focus",
			"summary":          "前端方向证据较强。",
			"job_hypotheses": []map[string]any{
				{"title": "前端开发工程师", "category": "本专业相关", "support": "专业和项目相关", "risk": "工程化证据不足", "next_proof": "补测试覆盖", "confidence": 82},
				{"title": "Web 全栈开发", "category": "本专业扩展", "support": "前端基础可迁移", "risk": "后端证据不足", "next_proof": "补接口项目", "confidence": 76},
			},
			"signals":             []string{"专业相关"},
			"risks":               []string{"量化结果不足"},
			"recommended_actions": []string{"补作品集"},
		})
	default:
		return compactJSON(map[string]any{
			"job_matching": map[string]any{
				"target_role":    "前端开发工程师",
				"overall_match":  82,
				"match_level":    "高潜力匹配",
				"source":         "Legato Job Matching Team via Presto",
				"method_summary": "backend Agent Team 按六维能力、经历证据、学历门槛排序。",
				"fit_summary":    "专业和项目支撑前端方向。",
				"student_radar":  radar,
				"target_radar":   targetRadar,
				"selected_job": map[string]any{
					"rank":              1,
					"title":             "前端开发工程师",
					"category":          "本专业相关",
					"match":             82,
					"ability_match":     84,
					"experience_match":  78,
					"education_gate":    "通过",
					"fit_summary":       "专业和项目支撑前端方向。",
					"risk":              "工程化证据不足",
					"requirement_radar": targetRadar,
					"reasons":           []string{"专业相关", "项目相关"},
					"next_proof":        "补测试覆盖",
				},
				"top_jobs": []map[string]any{
					{"rank": 1, "title": "前端开发工程师", "category": "本专业相关", "match": 82, "ability_match": 84, "experience_match": 78, "education_gate": "通过", "fit_summary": "专业和项目支撑前端方向。", "risk": "工程化证据不足", "requirement_radar": targetRadar, "reasons": []string{"专业相关", "项目相关"}, "next_proof": "补测试覆盖"},
					{"rank": 2, "title": "Web 全栈开发", "category": "本专业扩展", "match": 76, "ability_match": 77, "experience_match": 70, "education_gate": "通过", "fit_summary": "前端可迁移到全栈。", "risk": "后端证据不足", "requirement_radar": targetRadar, "reasons": []string{"前端基础"}, "next_proof": "补接口项目"},
					{"rank": 3, "title": "测试开发工程师", "category": "跨专业可迁移", "match": 68, "ability_match": 70, "experience_match": 58, "education_gate": "通过", "fit_summary": "工程基础可迁移。", "risk": "测试工具链不足", "requirement_radar": targetRadar, "reasons": []string{"工程基础"}, "next_proof": "补 Playwright"},
				},
				"report_sections": []map[string]any{
					{"name": "逻辑", "student": 74, "role_need": 78, "difference": -4},
					{"name": "语言", "student": 64, "role_need": 62, "difference": 2},
					{"name": "专业", "student": 80, "role_need": 86, "difference": -6},
					{"name": "领导", "student": 62, "role_need": 64, "difference": -2},
					{"name": "抗压", "student": 68, "role_need": 72, "difference": -4},
					{"name": "成长", "student": 76, "role_need": 78, "difference": -2},
				},
				"gap_details":         []map[string]any{{"capability": "工程化", "current": "证据不足", "expected": "测试和性能证据", "action": "补项目报告", "severity": "高"}},
				"recommendations":     []string{"先补工程化项目证据"},
				"recommended_reasons": []string{"专业和项目相关"},
				"agent_notes":         []string{"Adaptive Planner", "多视角 Agent", "Synthesis Arbiter"},
			},
		})
	}
}
