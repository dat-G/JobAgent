package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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

func TestItemBenchmarkBatchWorkersUsesEnvAndCaps(t *testing.T) {
	t.Setenv("ITEM_BENCHMARK_BATCH_WORKERS", "3")
	if got := itemBenchmarkBatchWorkers(); got != 3 {
		t.Fatalf("expected env batch workers 3, got %d", got)
	}

	t.Setenv("ITEM_BENCHMARK_BATCH_WORKERS", "12")
	if got := itemBenchmarkBatchWorkers(); got != maxItemBenchmarkBatchWorkers {
		t.Fatalf("expected capped batch workers %d, got %d", maxItemBenchmarkBatchWorkers, got)
	}
}

func TestDiagnosisJobSnapshotsDoNotShareAbilityProfileSlices(t *testing.T) {
	job := NewJobStore().Create(nil)
	diagnosis := Diagnosis{
		AbilityProfile: AbilityProfile{
			Awards: []AwardItem{{Name: "创新大赛", Result: "全国六强"}},
		},
	}
	job.SetDiagnosis(diagnosis)

	current := job.CurrentDiagnosis()
	job.Emit(DiagnosisEvent{
		Type:   "step.update",
		Step:   "profile",
		Status: "running",
		Data:   map[string]any{"ability_profile": current.AbilityProfile},
	})
	impact := 6.8
	current.AbilityProfile.Awards[0].ImpactFactor = &impact
	job.SetDiagnosis(current)

	unchanged := job.CurrentDiagnosis()
	unchanged.AbilityProfile.Awards[0].Name = "被外部修改"
	if got := job.CurrentDiagnosis().AbilityProfile.Awards[0].Name; got != "创新大赛" {
		t.Fatalf("expected CurrentDiagnosis to return an isolated copy, got %q", got)
	}

	events := job.Snapshot()["events"].([]DiagnosisEvent)
	data := events[0].Data.(map[string]any)
	profile := data["ability_profile"].(map[string]any)
	awards := profile["awards"].([]any)
	award := awards[0].(map[string]any)
	if _, ok := award["impact_factor"]; ok {
		t.Fatalf("expected emitted event snapshot to remain without later impact_factor, got %#v", award)
	}
}

func TestApplyResumeWorkflowItemBenchmarkMatchesAwardByTextWhenKeyMissing(t *testing.T) {
	diagnosis := Diagnosis{
		AbilityProfile: AbilityProfile{
			Awards: []AwardItem{{
				Name:          "创新促繁荣社邦青年创新大赛",
				Result:        "全国六强总决赛",
				EvidenceScope: "校外",
				Level:         6,
			}},
		},
	}
	result := &LegatoEnvelope{
		Data: map[string]any{
			"item_benchmark": []any{
				map[string]any{
					"item": map[string]any{
						"kind":           "award",
						"name":           "创新促繁荣社邦青年创新大赛",
						"result":         "全国六强总决赛",
						"evidence_scope": "校外",
					},
					"dimensions":    []any{"逻辑", "语言", "专业", "领导", "抗压", "成长"},
					"scores":        []any{0.18, 0.12, 0.2, 0.16, 0.14, 0.2},
					"impact_factor": 6.8,
				},
			},
		},
	}

	applied := applyResumeWorkflowItemBenchmark(&diagnosis, result)

	if applied != 1 {
		t.Fatalf("expected one applied benchmark, got %d", applied)
	}
	if diagnosis.AbilityProfile.Awards[0].ImpactFactor == nil || *diagnosis.AbilityProfile.Awards[0].ImpactFactor != 6.8 {
		t.Fatalf("expected award impact factor 6.8, got %#v", diagnosis.AbilityProfile.Awards[0].ImpactFactor)
	}
	if got := len(diagnosis.AbilityProfile.Awards[0].BenchmarkScores); got != 6 {
		t.Fatalf("expected six benchmark scores, got %d", got)
	}
	if got := diagnosis.AbilityProfile.BenchmarkStatus; got != "ready" {
		t.Fatalf("expected benchmark status ready, got %q", got)
	}
	if got := len(diagnosis.AbilityProfile.RadarData); got != len(benchmarkDimensionNames()) {
		t.Fatalf("expected radar data to be refreshed from item benchmark, got %d dimensions", got)
	}
}

func TestBenchmarkPrerequisiteWaitsForResumeProfileAndEvidence(t *testing.T) {
	diagnosis := Diagnosis{
		AbilityProfile: AbilityProfile{
			BasicInfo: BasicInfo{
				ResumeStatus: "等待 Legato 简历解析",
			},
			AwardsStatus:      "ready",
			ExperiencesStatus: "ready",
		},
	}
	if got := benchmarkPrerequisiteError(diagnosis); got == "" {
		t.Fatalf("expected benchmark to wait for resume profile")
	}

	diagnosis.AbilityProfile.BasicInfo = BasicInfo{
		Name:         "陈曦",
		School:       "东北农业大学",
		Major:        "计算机科学与技术",
		ResumeStatus: "Legato Resume workflow 已返回基础信息，frontend=pdfium_text，formatter=presto_resume_workflow_profile",
	}
	diagnosis.AbilityProfile.ExperiencesStatus = "refining"
	if got := benchmarkPrerequisiteError(diagnosis); got == "" {
		t.Fatalf("expected benchmark to wait for experience_hybrid terminal state")
	}

	diagnosis.AbilityProfile.ExperiencesStatus = "failed"
	if got := benchmarkPrerequisiteError(diagnosis); got == "" {
		t.Fatalf("expected benchmark to block failed experience_hybrid result")
	}

	diagnosis.AbilityProfile.ExperiencesStatus = "ready"
	if got := benchmarkPrerequisiteError(diagnosis); got != "" {
		t.Fatalf("expected benchmark prerequisites satisfied, got %q", got)
	}

	diagnosis.AbilityProfile.AwardsStatus = "empty"
	diagnosis.AbilityProfile.ExperiencesStatus = "empty"
	if got := benchmarkPrerequisiteError(diagnosis); got != "" {
		t.Fatalf("expected empty but completed evidence to satisfy prerequisites, got %q", got)
	}
}

func TestMatchingPrerequisiteRequiresReadyBenchmarkAndMajorBaseline(t *testing.T) {
	diagnosis := Diagnosis{
		AbilityProfile: AbilityProfile{
			BasicInfo: BasicInfo{
				Name:         "陈曦",
				School:       "东北农业大学",
				Major:        "计算机科学与技术",
				ResumeStatus: "Legato Resume workflow 已返回基础信息，frontend=pdfium_text，formatter=presto_resume_workflow_profile",
			},
			AwardsStatus:        "ready",
			ExperiencesStatus:   "ready",
			BenchmarkStatus:     "benchmarking",
			MajorBaselineStatus: "waiting",
		},
	}
	if got := matchingPrerequisiteError(diagnosis); got == "" {
		t.Fatalf("expected matching to wait for benchmarking")
	}

	diagnosis.AbilityProfile.BenchmarkStatus = "ready"
	diagnosis.AbilityProfile.MajorBaselineStatus = "waiting"
	if got := matchingPrerequisiteError(diagnosis); got == "" {
		t.Fatalf("expected matching to wait for Major Baseline")
	}

	diagnosis.AbilityProfile.MajorBaselineStatus = "empty"
	if got := matchingPrerequisiteError(diagnosis); got == "" {
		t.Fatalf("expected matching to block empty Major Baseline")
	}

	diagnosis.AbilityProfile.MajorBaselineStatus = "ready"
	if got := matchingPrerequisiteError(diagnosis); got != "" {
		t.Fatalf("expected matching prerequisites satisfied, got %q", got)
	}
}

func TestJobMatchingTimeoutUsesDedicatedEnv(t *testing.T) {
	t.Setenv("JOB_MATCHING_TIMEOUT_SECONDS", "240")

	if got := jobMatchingTimeout(); got.Seconds() != 240 {
		t.Fatalf("expected job matching timeout 240s, got %s", got)
	}
}

func TestJobMatchingTimeoutDefaultsToLongBudget(t *testing.T) {
	t.Setenv("JOB_MATCHING_TIMEOUT_SECONDS", "")

	if got := jobMatchingTimeout(); got != defaultJobMatchingTimeout || got.Seconds() != 600 {
		t.Fatalf("expected default job matching timeout 600s, got %s", got)
	}
}

func TestPathPlanningTimeoutUsesDedicatedEnv(t *testing.T) {
	t.Setenv("PATH_PLANNING_TIMEOUT_SECONDS", "360")

	if got := pathPlanningTimeout(); got.Seconds() != 360 {
		t.Fatalf("expected path planning timeout 360s, got %s", got)
	}
}

func TestPathPlanningTimeoutDefaultsToLongBudget(t *testing.T) {
	t.Setenv("PATH_PLANNING_TIMEOUT_SECONDS", "")

	if got := pathPlanningTimeout(); got != defaultPathPlanningTimeout || got.Seconds() != 600 {
		t.Fatalf("expected default path planning timeout 600s, got %s", got)
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

func TestNormalizePathPlanOutputRequiresStagesWeeksAndStandards(t *testing.T) {
	raw := map[string]any{
		"export_formats": []any{"PDF", "Word"},
		"stages": []any{
			map[string]any{
				"stage":       "第 1 阶段，0 到 30 天",
				"goal":        "补齐首选岗位核心证据",
				"deliverable": "作品集与复盘文档",
				"weeks": []any{
					map[string]any{"week": "第 1 周", "task": "梳理岗位能力矩阵", "metric": "完成 1 份矩阵", "priority": "高"},
					map[string]any{"week": "第 2 周", "task": "补齐项目 README", "metric": "上线 1 个仓库", "priority": "中"},
				},
				"standards": []any{"矩阵覆盖 6 个维度", "项目可被面试官访问"},
			},
			map[string]any{
				"stage":       "第 2 阶段，31 到 60 天",
				"goal":        "补齐工程化证据",
				"deliverable": "测试报告和性能报告",
				"weeks": []any{
					map[string]any{"week": "第 3 周", "task": "增加 E2E 测试", "metric": "核心路径 3 条", "priority": "高"},
					map[string]any{"week": "第 4 周", "task": "完成性能优化", "metric": "Lighthouse 90+", "priority": "高"},
				},
				"standards": []any{"测试报告可展示", "性能前后对比清晰"},
			},
		},
	}

	plan, err := normalizePathPlanOutput(raw)
	if err != nil {
		t.Fatalf("expected valid path plan, got %v", err)
	}
	if len(plan.Stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(plan.Stages))
	}
	if len(plan.Stages[0].Weeks) != 2 || len(plan.Stages[0].Standards) != 2 {
		t.Fatalf("expected weeks and standards preserved, got %#v", plan.Stages[0])
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

func TestJobMatchingTeamContextIncludesResumeTextAndScopeRadar(t *testing.T) {
	resumeText := "姓名：陈测试\n项目经历：隐藏的开源作品集，负责前端性能优化。\n校内经历：实验室科研项目。"
	resumePath := filepath.Join(t.TempDir(), "resume.md")
	if err := os.WriteFile(resumePath, []byte(resumeText), 0o644); err != nil {
		t.Fatalf("write resume: %v", err)
	}
	job := NewJobStore().Create([]SavedUpload{{
		SourceFile: SourceFile{Kind: "resume", Name: "resume.md", Size: int64(len(resumeText))},
		Path:       resumePath,
	}})
	impact := 9.0
	diagnosis := Diagnosis{AbilityProfile: AbilityProfile{
		BasicInfo: BasicInfo{Name: "陈测试", School: "测试大学", Major: "计算机科学与技术"},
		Awards: []AwardItem{{
			Name:            "校内科研项目",
			Result:          "实验室项目",
			Level:           7,
			ImpactFactor:    &impact,
			EvidenceScope:   "校内",
			BenchmarkScores: []float64{0.72, 0.20, 0.82, 0.40, 0.52, 0.62},
		}},
		Experiences: []ExperienceItem{{
			Type:            "实习",
			Role:            "企业前端开发",
			Contribution:    "性能优化",
			Level:           8,
			ImpactFactor:    &impact,
			EvidenceScope:   "校外",
			BenchmarkScores: []float64{0.66, 0.42, 0.78, 0.32, 0.70, 0.74},
		}},
		BenchmarkStatus: "ready",
	}}

	ctx := buildJobMatchingTeamContext(context.Background(), job, diagnosis)
	if got := stringValue(ctx["resume_full_text"]); !strings.Contains(got, "隐藏的开源作品集") {
		t.Fatalf("expected resume_full_text to include raw resume evidence, got %q", got)
	}
	meta := objectValue(ctx["resume_full_text_meta"])
	if got := stringValue(meta["status"]); got != "ready_fallback" {
		t.Fatalf("expected plain-text fallback status, got %q", got)
	}
	radarContext := objectValue(ctx["radar_context"])
	campus := objectValue(radarContext["campus"])
	external := objectValue(radarContext["external"])
	if got := intValue(campus["count"]); got != 1 {
		t.Fatalf("expected campus radar count 1, got %d", got)
	}
	if got := intValue(external["count"]); got != 1 {
		t.Fatalf("expected external radar count 1, got %d", got)
	}
	overall := objectValue(radarContext["overall"])
	scores, ok := overall["scores"].([]ScoreDimension)
	if !ok {
		t.Fatalf("expected overall radar scores to be []ScoreDimension, got %#v", overall["scores"])
	}
	if got := len(scores); got != len(benchmarkDimensionNames()) {
		t.Fatalf("expected six overall radar dimensions, got %d", got)
	}
}

func TestRefreshAbilityProfileRadarDataUsesCumulativeAbilityScale(t *testing.T) {
	impactHigh := 8.5
	impactMedium := 6.8
	diagnosis := Diagnosis{AbilityProfile: AbilityProfile{
		BasicInfo: BasicInfo{Major: "计算机科学与技术"},
		Education: []EducationItem{{
			School:     "测试大学",
			Major:      "计算机科学与技术",
			Degree:     "本科",
			Is211:      true,
			RuankeRank: 120,
		}},
		MajorBaselineStatus: "ready",
		MajorBaseline: MajorBaseline{
			BaseScore:  52,
			Dimensions: benchmarkDimensionNames(),
			Scores:     []int{56, 46, 59, 42, 49, 53},
		},
		Awards: []AwardItem{
			{
				Name:            "创新大赛",
				Result:          "全国六强",
				EvidenceScope:   "校外",
				Level:           7,
				ImpactFactor:    &impactMedium,
				BenchmarkScores: []float64{0.28, 0.08, 0.22, 0.12, 0.18, 0.12},
			},
			{
				Name:            "数学建模竞赛",
				Result:          "省一等奖",
				EvidenceScope:   "校外",
				Level:           8,
				ImpactFactor:    &impactHigh,
				BenchmarkScores: []float64{0.30, 0.06, 0.28, 0.05, 0.20, 0.11},
			},
		},
		Experiences: []ExperienceItem{
			{
				Type:            "科研项目",
				Role:            "核心成员",
				Contribution:    "完成数据处理和模型开发",
				EvidenceScope:   "校内",
				Level:           8,
				ImpactFactor:    &impactHigh,
				BenchmarkScores: []float64{0.20, 0.08, 0.36, 0.12, 0.12, 0.12},
			},
		},
	}}

	count := refreshAbilityProfileRadarData(&diagnosis.AbilityProfile)

	if count != 3 {
		t.Fatalf("expected three benchmarked evidence items, got %d", count)
	}
	if got := len(diagnosis.AbilityProfile.RadarData); got != len(benchmarkDimensionNames()) {
		t.Fatalf("expected six radar dimensions, got %d", got)
	}
	if got := len(diagnosis.AbilityProfile.RadarSeries); got < 1 {
		t.Fatalf("expected backend radar series to be generated, got %d", got)
	}
	if got := diagnosis.AbilityProfile.RadarSeries[0].Key; got != "overall" {
		t.Fatalf("expected first radar series to be overall, got %q", got)
	}
	total := 0
	for _, item := range diagnosis.AbilityProfile.RadarData {
		total += item.Score
	}
	if total < 250 {
		t.Fatalf("expected cumulative ability scale, got distribution-like total %d from %#v", total, diagnosis.AbilityProfile.RadarData)
	}
}

func TestRefreshAbilityProfileRadarDataKeepsCampusPriorWithoutCampusEvidence(t *testing.T) {
	impact := 7.2
	profile := AbilityProfile{
		BasicInfo: BasicInfo{Major: "计算机科学与技术"},
		Education: []EducationItem{{
			School:     "测试大学",
			Major:      "计算机科学与技术",
			Degree:     "本科",
			RuankeRank: 180,
		}},
		MajorBaselineStatus: "ready",
		MajorBaseline: MajorBaseline{
			BaseScore:  48,
			Dimensions: benchmarkDimensionNames(),
			Scores:     []int{52, 42, 56, 38, 46, 50},
			Source:     "major_baseline",
		},
		Awards: []AwardItem{{
			Name:            "创新大赛",
			Result:          "省二等奖",
			EvidenceScope:   "校外",
			Level:           7,
			ImpactFactor:    &impact,
			BenchmarkScores: []float64{0.24, 0.12, 0.26, 0.12, 0.14, 0.12},
		}},
		BenchmarkStatus: "ready",
	}

	refreshAbilityProfileRadarData(&profile)

	var campus *RadarSeries
	var external *RadarSeries
	for index := range profile.RadarSeries {
		switch profile.RadarSeries[index].Key {
		case "campus":
			campus = &profile.RadarSeries[index]
		case "external":
			external = &profile.RadarSeries[index]
		}
	}
	if campus == nil {
		t.Fatalf("expected campus radar series to be kept when campus evidence is absent, got %#v", profile.RadarSeries)
	}
	if campus.Count != 0 {
		t.Fatalf("expected campus evidence count 0, got %d", campus.Count)
	}
	if got := len(campus.Scores); got != len(benchmarkDimensionNames()) {
		t.Fatalf("expected campus prior to have six dimensions, got %d", got)
	}
	if campus.Source == "empty" {
		t.Fatalf("expected campus prior source, got %q", campus.Source)
	}
	if external == nil || external.Count != 1 {
		t.Fatalf("expected external radar series to keep the benchmarked item, got %#v", profile.RadarSeries)
	}

	radarContext := buildJobMatchingRadarContext(profile)
	campusContext := objectValue(radarContext["campus"])
	if got := intValue(campusContext["count"]); got != 0 {
		t.Fatalf("expected job matching campus context count 0, got %d", got)
	}
	if scores, ok := campusContext["scores"].([]ScoreDimension); !ok || len(scores) != len(benchmarkDimensionNames()) {
		t.Fatalf("expected job matching campus context to include prior scores, got %#v", campusContext["scores"])
	}
}

func TestJobMatchingPromptsTreatMissingEvidenceAsSynthesisQuestion(t *testing.T) {
	ctx := map[string]any{
		"resume_full_text": "项目经历：开源作品集。成绩单未上传。",
		"radar_context": map[string]any{
			"overall": map[string]any{"scores": benchmarkDimensionNames()},
		},
	}
	agent := jobMatchingPlannedAgent{
		Key:             "evidence_agent",
		Name:            "Evidence Agent",
		Phase:           "evidence",
		Perspective:     "证据质量",
		ReasoningEffort: "high",
		Focus:           "查验证据是否足以支持岗位推荐",
	}
	perspectivePrompt := perspectiveAgentPrompt(agent, ctx)
	for _, want := range []string{"questions_for_synthesis", "resume_full_text", "Transcript and GPA are optional"} {
		if !strings.Contains(perspectivePrompt, want) {
			t.Fatalf("expected perspective prompt to contain %q", want)
		}
	}
	synthesisPrompt := synthesisArbiterPrompt(ctx)
	for _, want := range []string{"resume_full_text", "如果有较高 GPA", "radar_context.overall"} {
		if !strings.Contains(synthesisPrompt, want) {
			t.Fatalf("expected synthesis prompt to contain %q", want)
		}
	}
}

func TestJobMatchingValidationRejectsStudentRadarDeviation(t *testing.T) {
	expected := []ScoreDimension{
		{Name: "逻辑", Score: 62, MaxScore: 100},
		{Name: "语言", Score: 52, MaxScore: 100},
		{Name: "专业", Score: 68, MaxScore: 100},
		{Name: "领导", Score: 49, MaxScore: 100},
		{Name: "抗压", Score: 58, MaxScore: 100},
		{Name: "成长", Score: 60, MaxScore: 100},
	}
	target := []any{
		map[string]any{"name": "逻辑", "score": 70, "max_score": 100},
		map[string]any{"name": "语言", "score": 55, "max_score": 100},
		map[string]any{"name": "专业", "score": 75, "max_score": 100},
		map[string]any{"name": "领导", "score": 55, "max_score": 100},
		map[string]any{"name": "抗压", "score": 60, "max_score": 100},
		map[string]any{"name": "成长", "score": 65, "max_score": 100},
	}
	raw := map[string]any{
		"target_role":   "数据工程师",
		"target_radar":  target,
		"student_radar": target,
		"top_jobs": []any{
			map[string]any{"title": "数据工程师", "requirement_radar": target},
		},
	}
	teamCtx := map[string]any{
		"radar_context": map[string]any{
			"overall": map[string]any{"scores": expected},
		},
	}

	if err := validateJobMatchingTeamOutputAgainstContext(raw, teamCtx); err == nil {
		t.Fatal("expected validation to reject student_radar that deviates from Benchmark profile")
	}
}

func TestCompactJobMatchingAgentDataLimitsSynthesisContext(t *testing.T) {
	longText := strings.Repeat("x", maxJobMatchingAgentResultStringRunes+200)
	hypotheses := []any{}
	for index := 0; index < maxJobMatchingAgentResultArrayItems+2; index++ {
		hypotheses = append(hypotheses, map[string]any{"title": fmt.Sprintf("岗位%d", index), "support": longText})
	}
	compact := compactJobMatchingAgentData(map[string]any{
		"summary":           longText,
		"job_hypotheses":    hypotheses,
		"reasoning_content": longText,
	})

	if _, ok := compact["reasoning_content"]; ok {
		t.Fatalf("expected reasoning_content to be dropped from synthesis context")
	}
	if got := len([]rune(stringValue(compact["summary"]))); got > maxJobMatchingAgentResultStringRunes {
		t.Fatalf("expected compact summary at most %d runes, got %d", maxJobMatchingAgentResultStringRunes, got)
	}
	items, ok := compact["job_hypotheses"].([]any)
	if !ok {
		t.Fatalf("expected compact job_hypotheses slice, got %#v", compact["job_hypotheses"])
	}
	if got := len(items); got != maxJobMatchingAgentResultArrayItems {
		t.Fatalf("expected %d compact hypotheses, got %d", maxJobMatchingAgentResultArrayItems, got)
	}
}

func TestExtractJSONObjectEscapesRawNewlinesInStrings(t *testing.T) {
	raw := `{
  "agent": "role_family_mapper",
  "summary": "第一行
第二行",
  "job_hypotheses": [
    {"title": "数据产品经理", "support": "技术证据
产品证据"}
  ]
}`

	data, err := extractJSONObject(raw)
	if err != nil {
		t.Fatalf("expected raw newlines in strings to be repaired, got %v", err)
	}
	if got := stringValue(data["summary"]); got != "第一行\n第二行" {
		t.Fatalf("expected repaired newline string, got %q", got)
	}
	items := objectArray(data["job_hypotheses"])
	if got := stringValue(items[0]["support"]); got != "技术证据\n产品证据" {
		t.Fatalf("expected nested repaired newline string, got %q", got)
	}
}

func TestExtractJSONObjectUsesFirstValidBalancedObject(t *testing.T) {
	raw := `模型先返回结构化内容 {"summary":"岗位匹配可用","job_hypotheses":[{"title":"数据分析师"}]} 后续流式尾巴 {bad: è}`

	data, err := extractJSONObject(raw)
	if err != nil {
		t.Fatalf("expected first valid object to be extracted, got %v", err)
	}
	if got := stringValue(data["summary"]); got != "岗位匹配可用" {
		t.Fatalf("expected summary from first object, got %q", got)
	}
	items := objectArray(data["job_hypotheses"])
	if got := stringValue(items[0]["title"]); got != "数据分析师" {
		t.Fatalf("expected nested title from first object, got %q", got)
	}
}

func TestRunJobMatchingAgentRetriesMalformedJSON(t *testing.T) {
	var mu sync.Mutex
	runOutputs := map[string]string{}
	agentAttempts := map[string]int{}
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
			status = http.StatusCreated
			body = compactJSON(map[string]any{"id": "sess_" + req.Metadata["agent"]})
		case r.Method == http.MethodPost && strings.HasPrefix(path, "sessions/") && strings.HasSuffix(path, "/runs"):
			parts := strings.Split(path, "/")
			agent := strings.TrimPrefix(parts[1], "sess_")
			mu.Lock()
			agentAttempts[agent]++
			attempt := agentAttempts[agent]
			runID := fmt.Sprintf("run_%s_%d", agent, attempt)
			if attempt == 1 {
				runOutputs[runID] = `{"agent":"growth","summary":"首次输出"后续中文}`
			} else {
				runOutputs[runID] = compactJSON(map[string]any{
					"agent":            "growth",
					"perspective":      "成长差距",
					"reasoning_effort": "high",
					"focus":            "测试重试",
					"summary":          "重试后返回严格 JSON",
					"job_hypotheses": []map[string]any{
						{"title": "数据分析师", "confidence": 78},
					},
				})
			}
			mu.Unlock()
			status = http.StatusCreated
			body = compactJSON(map[string]any{"id": runID, "session_id": parts[1], "status": "queued"})
		case r.Method == http.MethodGet && strings.HasPrefix(path, "runs/") && strings.HasSuffix(path, "/events"):
			parts := strings.Split(path, "/")
			runID := parts[1]
			headers.Set("Content-Type", "text/event-stream")
			body = fmt.Sprintf(
				"id: e1\nevent: run.started\ndata: {\"type\":\"run.started\",\"run_id\":%q}\n\n"+
					"id: e2\nevent: run.done\ndata: {\"type\":\"run.done\",\"run_id\":%q}\n\n",
				runID, runID,
			)
		case r.Method == http.MethodGet && strings.HasPrefix(path, "runs/"):
			parts := strings.Split(path, "/")
			mu.Lock()
			output := runOutputs[parts[1]]
			mu.Unlock()
			body = compactJSON(map[string]any{
				"id":         parts[1],
				"session_id": "sess_growth_agent",
				"status":     "completed",
				"output":     output,
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
	spec := jobMatchingAgentSpec{
		Key:             "growth_agent",
		Name:            "成长差距与建议 Agent",
		Phase:           "growth",
		Perspective:     "成长差距",
		ReasoningEffort: "high",
		Focus:           "测试 JSON 重试",
		AgentIndex:      1,
		AgentTotal:      1,
		Sequence:        1,
		Prompt: func(map[string]any) string {
			return "Return strict JSON only."
		},
	}

	result, err := runJobMatchingAgent(context.Background(), NewJobStore().Create(nil), client, map[string]any{}, spec)
	if err != nil {
		t.Fatalf("expected malformed first output to be retried, got %v", err)
	}
	if got := stringValue(result.Data["summary"]); got != "重试后返回严格 JSON" {
		t.Fatalf("expected retried JSON data, got %q", got)
	}
	mu.Lock()
	attempts := agentAttempts["growth_agent"]
	mu.Unlock()
	if attempts != 2 {
		t.Fatalf("expected two Presto attempts, got %d", attempts)
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
				"source":         "LegatoJobMatchingTeamviaPresto",
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
	diagnosis := Diagnosis{AbilityProfile: AbilityProfile{RadarData: []ScoreDimension{
		{Name: "逻辑", Score: 73, MaxScore: 100},
		{Name: "语言", Score: 62, MaxScore: 100},
		{Name: "专业", Score: 79, MaxScore: 100},
		{Name: "领导", Score: 66, MaxScore: 100},
		{Name: "抗压", Score: 70, MaxScore: 100},
		{Name: "成长", Score: 75, MaxScore: 100},
	}}}

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
	if got := diagnosis.MatchingResult.Source; got != "Legato Job Matching Team via Presto" {
		t.Fatalf("expected normalized source, got %q", got)
	}
	if got := diagnosis.MatchingResult.SelectedJob.EducationGateStatus; got != "pass" {
		t.Fatalf("expected normalized education gate status pass, got %q", got)
	}
	if got := diagnosis.MatchingResult.StudentRadar[0].Score; got != 73 {
		t.Fatalf("expected student radar to follow ability_profile.radar_data, got %d", got)
	}
	if got := diagnosis.MatchingResult.ReportSections[0].Student; got != 73 {
		t.Fatalf("expected report rows to be recalculated from authoritative radar, got %d", got)
	}
	if got := diagnosis.MatchingResult.ReportSections[2].Status; got != "fit" {
		t.Fatalf("expected professional dimension status fit, got %q", got)
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
