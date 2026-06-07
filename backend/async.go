package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const defaultDiagnosisTimeout = 120 * time.Second
const defaultDiagnosisTempRetention = 2 * time.Hour

type JobStore struct {
	seq  uint64
	jobs map[string]*DiagnosisJob
	mu   sync.RWMutex
}

type DiagnosisJob struct {
	ID          string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Files       []SavedUpload
	Diagnosis   Diagnosis
	Error       string
	TempDir     string
	events      []DiagnosisEvent
	subscribers map[chan DiagnosisEvent]struct{}
	nextEventID uint64
	mu          sync.Mutex
}

type SavedUpload struct {
	SourceFile
	Path string `json:"-"`
}

type DiagnosisEvent struct {
	ID      string `json:"id"`
	JobID   string `json:"job_id"`
	Type    string `json:"type"`
	Step    string `json:"step,omitempty"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
	Time    string `json:"time"`
}

type LegatoEnvelope struct {
	Status        string         `json:"status"`
	Target        string         `json:"target"`
	SourcePath    string         `json:"source_path"`
	Frontend      string         `json:"frontend"`
	Formatter     string         `json:"formatter"`
	ElapsedMS     int            `json:"elapsed_ms"`
	MarkdownChars int            `json:"markdown_chars"`
	Data          map[string]any `json:"data"`
	Warnings      []string       `json:"warnings"`
	Debug         map[string]any `json:"debug,omitempty"`
	Error         string         `json:"error"`
}

type legatoStageResult struct {
	target string
	result *LegatoEnvelope
	err    error
}

type BenchmarkRequest struct {
	Items []BenchmarkEvidenceInput `json:"items"`
}

type BenchmarkEvidenceInput struct {
	Kind           string  `json:"kind"`
	Key            string  `json:"key"`
	Name           string  `json:"name"`
	Result         string  `json:"result"`
	EvidenceScope  string  `json:"evidence_scope,omitempty"`
	ExperienceType string  `json:"experience_type,omitempty"`
	Role           string  `json:"role,omitempty"`
	Contribution   string  `json:"contribution,omitempty"`
	Level          float64 `json:"level,omitempty"`
}

func NewJobStore() *JobStore {
	return &JobStore{jobs: make(map[string]*DiagnosisJob)}
}

func (s *JobStore) Create(files []SavedUpload) *DiagnosisJob {
	now := time.Now().UTC()
	id := "diag_" + strconv.FormatUint(atomic.AddUint64(&s.seq, 1), 10)
	job := &DiagnosisJob{
		ID:          id,
		Status:      "queued",
		CreatedAt:   now,
		UpdatedAt:   now,
		Files:       files,
		subscribers: make(map[chan DiagnosisEvent]struct{}),
	}
	s.mu.Lock()
	s.jobs[id] = job
	s.mu.Unlock()
	return job
}

func (s *JobStore) Get(id string) (*DiagnosisJob, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	return job, ok
}

func (j *DiagnosisJob) Emit(event DiagnosisEvent) {
	now := time.Now().UTC()
	event.ID = "evt_" + strconv.FormatUint(atomic.AddUint64(&j.nextEventID, 1), 10)
	event.JobID = j.ID
	event.Time = now.Format(time.RFC3339)

	j.mu.Lock()
	switch event.Type {
	case "job.created":
		j.Status = "queued"
	case "job.started":
		j.Status = "running"
	case "job.done":
		j.Status = "completed"
	case "job.failed":
		j.Status = "failed"
	}
	j.UpdatedAt = now
	j.events = append(j.events, event)
	subscribers := make([]chan DiagnosisEvent, 0, len(j.subscribers))
	for ch := range j.subscribers {
		subscribers = append(subscribers, ch)
	}
	j.mu.Unlock()

	for _, ch := range subscribers {
		select {
		case ch <- event:
		default:
		}
	}
}

func (j *DiagnosisJob) SetDiagnosis(diagnosis Diagnosis) {
	j.mu.Lock()
	j.Diagnosis = diagnosis
	j.UpdatedAt = time.Now().UTC()
	j.mu.Unlock()
}

func (j *DiagnosisJob) SetError(message string) {
	j.mu.Lock()
	j.Error = message
	j.UpdatedAt = time.Now().UTC()
	j.mu.Unlock()
}

func (j *DiagnosisJob) Subscribe(lastEventID string) ([]DiagnosisEvent, <-chan DiagnosisEvent, func()) {
	ch := make(chan DiagnosisEvent, 64)
	j.mu.Lock()
	replay := eventsAfter(j.events, lastEventID)
	j.subscribers[ch] = struct{}{}
	j.mu.Unlock()
	unsubscribe := func() {
		j.mu.Lock()
		delete(j.subscribers, ch)
		j.mu.Unlock()
	}
	return replay, ch, unsubscribe
}

func (j *DiagnosisJob) Snapshot() map[string]any {
	j.mu.Lock()
	defer j.mu.Unlock()
	return map[string]any{
		"id":         j.ID,
		"status":     j.Status,
		"created_at": j.CreatedAt,
		"updated_at": j.UpdatedAt,
		"diagnosis":  j.Diagnosis,
		"error":      j.Error,
		"events":     j.events,
	}
}

func (j *DiagnosisJob) CurrentDiagnosis() Diagnosis {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.Diagnosis
}

func eventsAfter(events []DiagnosisEvent, lastEventID string) []DiagnosisEvent {
	if lastEventID == "" {
		return append([]DiagnosisEvent(nil), events...)
	}
	for index, event := range events {
		if event.ID == lastEventID {
			return append([]DiagnosisEvent(nil), events[index+1:]...)
		}
	}
	return append([]DiagnosisEvent(nil), events...)
}

func (s Server) startDiagnosisJob(w http.ResponseWriter, r *http.Request) {
	files, err := saveDiagnosisUploads(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if file, ok := firstFileByKind(files, "resume"); !ok || file.Path == "" {
		cleanupDiagnosisTempDir(tempDirFromFiles(files))
		writeError(w, http.StatusBadRequest, "强制 Legato 模式需要上传可解析的简历文件")
		return
	}
	job := s.jobs.Create(files)
	job.TempDir = tempDirFromFiles(files)
	job.Emit(DiagnosisEvent{Type: "job.created", Status: "queued", Message: "诊断任务已创建"})
	go s.runDiagnosisJob(job)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"job_id":     job.ID,
		"status":     "queued",
		"events_url": "/api/diagnosis/" + job.ID + "/events",
	})
}

func (s Server) handleDiagnosisJob(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/diagnosis/"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "diagnosis job not found")
		return
	}
	job, ok := s.jobs.Get(parts[0])
	if !ok {
		writeError(w, http.StatusNotFound, "diagnosis job not found")
		return
	}
	if len(parts) == 2 && parts[1] == "events" {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		streamDiagnosisEvents(w, r, job)
		return
	}
	if len(parts) == 2 && parts[1] == "benchmark" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		s.handleDiagnosisBenchmark(w, r, job)
		return
	}
	if len(parts) == 1 && r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, job.Snapshot())
		return
	}
	writeError(w, http.StatusNotFound, "diagnosis job not found")
}

func streamDiagnosisEvents(w http.ResponseWriter, r *http.Request, job *DiagnosisJob) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	lastEventID := r.Header.Get("Last-Event-ID")
	if after := r.URL.Query().Get("after"); after != "" {
		lastEventID = after
	}
	replay, events, unsubscribe := job.Subscribe(lastEventID)
	defer unsubscribe()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	for _, event := range replay {
		if writeDiagnosisSSE(w, event) != nil {
			return
		}
		flusher.Flush()
	}

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			if writeDiagnosisSSE(w, event) != nil {
				return
			}
			flusher.Flush()
		case <-ticker.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s Server) handleDiagnosisBenchmark(w http.ResponseWriter, r *http.Request, job *DiagnosisJob) {
	var req BenchmarkRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid Benchmark request")
		return
	}
	items := sanitizeBenchmarkInputs(req.Items)
	diagnosis := job.CurrentDiagnosis()
	if len(items) == 0 {
		diagnosis.AbilityProfile.BenchmarkStatus = "empty"
		job.SetDiagnosis(diagnosis)
		writeJSON(w, http.StatusOK, map[string]any{"ability_profile": diagnosis.AbilityProfile})
		return
	}
	if err := ensureResumeFileAvailable(job); err != nil {
		diagnosis.AbilityProfile.BenchmarkStatus = "failed"
		diagnosis.AbilityProfile.MajorBaselineStatus = "failed"
		diagnosis.ProductionLimitations = append(diagnosis.ProductionLimitations, err.Error())
		job.SetDiagnosis(diagnosis)
		job.Emit(DiagnosisEvent{
			Type:    "step.update",
			Step:    "profile",
			Status:  "failed",
			Message: "Benchmark 无法继续，上传简历临时文件不可用",
			Data: map[string]any{
				"ability_profile":        diagnosis.AbilityProfile,
				"production_limitations": diagnosis.ProductionLimitations,
				"error":                  err.Error(),
			},
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"ability_profile":        diagnosis.AbilityProfile,
			"production_limitations": diagnosis.ProductionLimitations,
			"error":                  err.Error(),
		})
		return
	}

	diagnosis.AbilityProfile.BenchmarkStatus = "benchmarking"
	diagnosis.AbilityProfile.MajorBaselineStatus = "benchmarking"
	job.SetDiagnosis(diagnosis)
	job.Emit(DiagnosisEvent{
		Type:    "step.update",
		Step:    "profile",
		Status:  "running",
		Message: "Benchmark 正在并发评估证据影响因子和专业六维基线",
		Data:    map[string]any{"ability_profile": diagnosis.AbilityProfile},
	})

	ctx, cancel := context.WithTimeout(r.Context(), diagnosisTimeout())
	defer cancel()
	majorBaselineInput := buildMajorBaselineStageInput(diagnosis)
	batches := chunkBenchmarkInputs(items, itemBenchmarkMaxRequests())
	type itemBenchmarkBatchCall struct {
		index     int
		total     int
		itemCount int
		result    *LegatoEnvelope
		err       error
	}
	type majorBaselineCall struct {
		result *LegatoEnvelope
		err    error
	}
	itemBenchmarkCh := make(chan itemBenchmarkBatchCall, len(batches))
	majorBaselineCh := make(chan majorBaselineCall, 1)
	for index, batch := range batches {
		index := index
		batch := batch
		go func() {
			result, err := s.runResumeWorkflowStageWithInput(
				ctx,
				job,
				"item_benchmark",
				fmt.Sprintf("简历 Item Benchmark 第 %d/%d 批正在评估 %d 条证据", index+1, len(batches), len(batch)),
				map[string]any{
					"items":       batch,
					"max_workers": 1,
				},
			)
			itemBenchmarkCh <- itemBenchmarkBatchCall{
				index:     index,
				total:     len(batches),
				itemCount: len(batch),
				result:    result,
				err:       err,
			}
		}()
	}
	go func() {
		result, err := s.runResumeWorkflowStageWithInput(
			ctx,
			job,
			"major_baseline",
			"简历 Major Baseline 正在思考专业对六维基础分布的影响",
			majorBaselineInput,
		)
		majorBaselineCh <- majorBaselineCall{result: result, err: err}
	}()

	pendingItemBatches := len(batches)
	majorBaselinePending := true
	itemBenchmarkApplied := 0
	var benchmarkErrors []string
	for pendingItemBatches > 0 || majorBaselinePending {
		select {
		case batch := <-itemBenchmarkCh:
			pendingItemBatches--
			diagnosis = job.CurrentDiagnosis()
			if batch.err != nil {
				message := fmt.Sprintf("Item Benchmark 第 %d/%d 批失败：%s", batch.index+1, batch.total, batch.err.Error())
				benchmarkErrors = append(benchmarkErrors, message)
				log.Printf("diagnosis %s legato item_benchmark batch %d/%d failed: %v", job.ID, batch.index+1, batch.total, batch.err)
				diagnosis.AbilityProfile.BenchmarkStatus = "benchmarking"
				job.SetDiagnosis(diagnosis)
				job.Emit(DiagnosisEvent{
					Type:    "step.update",
					Step:    "profile",
					Status:  "running",
					Message: benchmarkProgressMessage(pendingItemBatches, majorBaselinePending, itemBenchmarkApplied, true),
					Data: map[string]any{
						"ability_profile": diagnosis.AbilityProfile,
						"benchmark_batch": map[string]any{
							"index":      batch.index,
							"total":      batch.total,
							"item_count": batch.itemCount,
							"status":     "failed",
							"pending":    pendingItemBatches,
						},
					},
				})
				continue
			}
			applied := applyResumeWorkflowItemBenchmark(&diagnosis, batch.result)
			if applied == 0 {
				message := fmt.Sprintf("Item Benchmark 第 %d/%d 批未返回可应用评分", batch.index+1, batch.total)
				benchmarkErrors = append(benchmarkErrors, message)
				diagnosis.AbilityProfile.BenchmarkStatus = "benchmarking"
				job.SetDiagnosis(diagnosis)
				job.Emit(DiagnosisEvent{
					Type:    "step.update",
					Step:    "profile",
					Status:  "running",
					Message: benchmarkProgressMessage(pendingItemBatches, majorBaselinePending, itemBenchmarkApplied, true),
					Data: map[string]any{
						"ability_profile": diagnosis.AbilityProfile,
						"benchmark_batch": map[string]any{
							"index":      batch.index,
							"total":      batch.total,
							"item_count": batch.itemCount,
							"applied":    0,
							"status":     "failed",
							"pending":    pendingItemBatches,
						},
					},
				})
				continue
			}
			itemBenchmarkApplied += applied
			if pendingItemBatches > 0 || majorBaselinePending {
				diagnosis.AbilityProfile.BenchmarkStatus = "benchmarking"
			}
			job.SetDiagnosis(diagnosis)
			job.Emit(DiagnosisEvent{
				Type:    "step.update",
				Step:    "profile",
				Status:  benchmarkStepStatus(pendingItemBatches, majorBaselinePending),
				Message: benchmarkProgressMessage(pendingItemBatches, majorBaselinePending, itemBenchmarkApplied, len(benchmarkErrors) > 0),
				Data: map[string]any{
					"ability_profile": diagnosis.AbilityProfile,
					"benchmark_batch": map[string]any{
						"index":      batch.index,
						"total":      batch.total,
						"item_count": batch.itemCount,
						"applied":    applied,
						"status":     "ready",
						"pending":    pendingItemBatches,
					},
				},
			})
		case baseline := <-majorBaselineCh:
			majorBaselinePending = false
			majorBaselineCh = nil
			diagnosis = job.CurrentDiagnosis()
			if baseline.err != nil {
				message := "Major Baseline 失败：" + baseline.err.Error()
				benchmarkErrors = append(benchmarkErrors, message)
				log.Printf("diagnosis %s legato major_baseline failed: %v", job.ID, baseline.err)
				diagnosis.AbilityProfile.MajorBaselineStatus = "failed"
				job.SetDiagnosis(diagnosis)
				job.Emit(DiagnosisEvent{
					Type:    "step.update",
					Step:    "profile",
					Status:  "running",
					Message: benchmarkProgressMessage(pendingItemBatches, majorBaselinePending, itemBenchmarkApplied, true),
					Data: map[string]any{
						"ability_profile": diagnosis.AbilityProfile,
						"major_baseline":  map[string]any{"status": "failed"},
					},
				})
				continue
			}
			applyResumeWorkflowMajorBaseline(&diagnosis, baseline.result)
			job.SetDiagnosis(diagnosis)
			job.Emit(DiagnosisEvent{
				Type:    "step.update",
				Step:    "profile",
				Status:  benchmarkStepStatus(pendingItemBatches, majorBaselinePending),
				Message: benchmarkProgressMessage(pendingItemBatches, majorBaselinePending, itemBenchmarkApplied, len(benchmarkErrors) > 0),
				Data: map[string]any{
					"ability_profile": diagnosis.AbilityProfile,
					"major_baseline":  map[string]any{"status": "ready"},
				},
			})
		case <-ctx.Done():
			benchmarkErrors = append(benchmarkErrors, "Benchmark 超时："+ctx.Err().Error())
			pendingItemBatches = 0
			majorBaselinePending = false
		}
	}

	diagnosis = job.CurrentDiagnosis()
	if len(benchmarkErrors) > 0 {
		errMessage := strings.Join(benchmarkErrors, "；")
		diagnosis.AbilityProfile.BenchmarkStatus = "failed"
		if diagnosis.AbilityProfile.MajorBaselineStatus == "benchmarking" {
			diagnosis.AbilityProfile.MajorBaselineStatus = "failed"
		}
		diagnosis.ProductionLimitations = append(diagnosis.ProductionLimitations, "Benchmark 失败，六维分布、impact_factor 或专业六维基线未生成，可从失败阶段重试。")
		job.SetDiagnosis(diagnosis)
		job.Emit(DiagnosisEvent{
			Type:    "step.update",
			Step:    "profile",
			Status:  "failed",
			Message: "Benchmark 失败，点击 dock 中红色画像阶段可继续重试",
			Data: map[string]any{
				"ability_profile":        diagnosis.AbilityProfile,
				"production_limitations": diagnosis.ProductionLimitations,
				"error":                  errMessage,
			},
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"ability_profile":        diagnosis.AbilityProfile,
			"production_limitations": diagnosis.ProductionLimitations,
			"error":                  errMessage,
		})
		return
	}

	if itemBenchmarkApplied == 0 {
		diagnosis.AbilityProfile.BenchmarkStatus = "empty"
	} else {
		diagnosis.AbilityProfile.BenchmarkStatus = "ready"
	}
	job.SetDiagnosis(diagnosis)
	job.Emit(DiagnosisEvent{
		Type:    "step.update",
		Step:    "profile",
		Status:  "done",
		Message: "Benchmark 已返回，证据卡片已补充 impact_factor，雷达图已补充专业六维基线",
		Data:    map[string]any{"ability_profile": diagnosis.AbilityProfile},
	})
	diagnosis, matchingErr := s.runJobMatchingStage(ctx, job, diagnosis)
	if matchingErr != nil {
		diagnosis.ProductionLimitations = append(diagnosis.ProductionLimitations, "Job Matching 失败，岗位推荐和匹配雷达未生成，可重新生成或稍后重试。")
		job.SetDiagnosis(diagnosis)
		job.Emit(DiagnosisEvent{
			Type:    "step.update",
			Step:    "matching",
			Status:  "failed",
			Message: "Legato Job Matching 失败，岗位匹配模块未生成",
			Data: map[string]any{
				"ability_profile":        diagnosis.AbilityProfile,
				"production_limitations": diagnosis.ProductionLimitations,
				"error":                  matchingErr.Error(),
			},
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"ability_profile":        diagnosis.AbilityProfile,
			"production_limitations": diagnosis.ProductionLimitations,
			"matching_error":         matchingErr.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ability_profile":  diagnosis.AbilityProfile,
		"matching_result":  diagnosis.MatchingResult,
		"top_jobs":         diagnosis.AbilityProfile.TopJobs,
		"generated_at":     diagnosis.GeneratedAt,
		"match_generated":  true,
		"matching_source":  diagnosis.MatchingResult.Source,
		"job_matching_run": "Legato Job Matching Team via Presto",
	})
}

func (s Server) runJobMatchingStage(ctx context.Context, job *DiagnosisJob, diagnosis Diagnosis) (Diagnosis, error) {
	job.Emit(DiagnosisEvent{
		Type:    "step.update",
		Step:    "matching",
		Status:  "running",
		Message: "Legato Job Matching Team 正在连接 Presto 并生成岗位推荐",
		Data: map[string]any{
			"ability_profile": diagnosis.AbilityProfile,
		},
	})
	result, err := s.runLegatoJobMatchingTeam(ctx, job, diagnosis)
	if err != nil {
		return diagnosis, err
	}
	applyResumeWorkflowJobMatching(&diagnosis, result)
	diagnosis.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	job.SetDiagnosis(diagnosis)
	job.Emit(DiagnosisEvent{
		Type:    "step.update",
		Step:    "matching",
		Status:  "done",
		Message: "Legato Job Matching Team 已返回岗位推荐、目标雷达和差距明细",
		Data: map[string]any{
			"matching_result": diagnosis.MatchingResult,
			"top_jobs":        diagnosis.AbilityProfile.TopJobs,
			"generated_at":    diagnosis.GeneratedAt,
		},
	})
	return diagnosis, nil
}

func writeDiagnosisSSE(w io.Writer, event DiagnosisEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		data = []byte(`{"type":"error","status":"failed","message":"event marshal failed"}`)
	}
	if _, err := fmt.Fprintf(w, "id: %s\n", event.ID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", event.Type); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return err
	}
	return nil
}

func (s Server) runDiagnosisJob(job *DiagnosisJob) {
	ctx, cancel := context.WithTimeout(context.Background(), diagnosisTimeout())
	defer cancel()
	defer scheduleDiagnosisTempCleanup(job.TempDir)

	files := sourceFiles(job.Files)
	diagnosis := mockDiagnosis(DiagnosisRequest{Files: files})
	diagnosis.Mode = "legato-required"
	transcriptUse := "未上传成绩单"
	if _, ok := firstFileByKind(job.Files, "transcript"); ok {
		transcriptUse = "等待 Legato 成绩单解析"
	}
	diagnosis.AbilityProfile.BasicInfo = BasicInfo{
		ResumeStatus:  "等待 Legato 简历解析",
		TranscriptUse: transcriptUse,
	}
	diagnosis.AbilityProfile.Education = []EducationItem{}
	diagnosis.AbilityProfile.RadarData = []ScoreDimension{}
	diagnosis.AbilityProfile.EvidenceSummary = []EvidenceItem{}
	diagnosis.AbilityProfile.AwardsStatus = "waiting"
	diagnosis.AbilityProfile.Awards = []AwardItem{}
	diagnosis.AbilityProfile.ExperiencesStatus = "waiting"
	diagnosis.AbilityProfile.Experiences = []ExperienceItem{}
	diagnosis.AbilityProfile.BenchmarkStatus = "waiting"
	diagnosis.AbilityProfile.MajorBaselineStatus = "waiting"
	diagnosis.AbilityProfile.MajorBaseline = MajorBaseline{}
	diagnosis.AbilityProfile.TopJobs = []MatchedJob{}
	diagnosis.MatchingResult = MatchingResult{}
	diagnosis.ProductionLimitations = []string{
		"简历必须由 Legato 后端成功解析，否则诊断任务失败。",
		"成绩单是可选增强材料；如解析失败，本次诊断会跳过成绩单证据并继续生成结果。",
		"岗位推荐、匹配差距和目标岗位雷达由 Benchmark 后的 Legato Job Matching Team via Presto 生成。",
		"成长路径仍为模拟数据，待接入真实规划 Agent。",
		"其他材料当前只记录文件元信息，尚未纳入真实解析。",
	}

	job.SetDiagnosis(diagnosis)
	job.Emit(DiagnosisEvent{Type: "job.started", Status: "running", Message: "已开始诊断，Legato 解析为必需步骤"})

	resumeStages := []struct {
		stage   string
		message string
	}{
		{stage: "profile", message: "简历基础信息 Agent 正在抽取姓名、学校、专业和学历"},
		{stage: "certifications_awards", message: "简历证书奖项 Agent 正在结构化荣誉与证书"},
		{stage: "experience_hybrid", message: "简历经历 hybrid Agent 正在整体整理项目、实习和活动经历"},
	}
	results := make(chan legatoStageResult, len(resumeStages)+1)
	for _, stage := range resumeStages {
		stage := stage
		go func() {
			result, err := s.runResumeWorkflowStage(ctx, job, stage.stage, stage.message)
			results <- legatoStageResult{
				target: "resume_" + stage.stage,
				result: result,
				err:    err,
			}
		}()
	}
	go func() {
		result, err := s.runLegatoStage(ctx, job, "transcript", "transcript_agent", "成绩单解析 Agent 正在处理")
		results <- legatoStageResult{
			target: "transcript",
			result: result,
			err:    err,
		}
	}()

	job.Emit(DiagnosisEvent{Type: "step.update", Step: "profile", Status: "running", Message: "画像 Agent 已启动，等待材料解析结果"})
	resumeParsed := false
	var resumeFailures []string
	var requiredFailures []string
	var optionalWarnings []string

	for completed := 0; completed < cap(results); {
		select {
		case stage := <-results:
			completed++
			if stage.err != nil {
				message := legatoTargetLabel(stage.target) + "失败：" + stage.err.Error()
				log.Printf("diagnosis %s legato %s failed: %v", job.ID, stage.target, stage.err)
				if isResumeLegatoTarget(stage.target) {
					resumeFailures = append(resumeFailures, message)
					markResumeEvidenceStatus(&diagnosis, stage.target, "failed")
				} else {
					optionalWarnings = append(optionalWarnings, message)
					applyOptionalLegatoFailure(&diagnosis, stage.target, stage.err)
					job.SetDiagnosis(diagnosis)
				}
				continue
			}
			if stage.result != nil && stage.result.Data != nil {
				switch stage.target {
				case "resume_profile":
					applyResumeWorkflowProfile(&diagnosis, stage.result)
					resumeParsed = true
				case "resume_certifications_awards":
					applyResumeWorkflowCertifications(&diagnosis, stage.result)
					resumeParsed = true
				case "resume_experience":
					applyResumeWorkflowExperience(&diagnosis, stage.result, false)
					resumeParsed = true
				case "resume_experience_hybrid":
					applyResumeWorkflowExperience(&diagnosis, stage.result, true)
					resumeParsed = true
				case "transcript":
					applyTranscriptLegato(&diagnosis, stage.result)
				}
				job.SetDiagnosis(diagnosis)
				job.Emit(DiagnosisEvent{
					Type:    "step.update",
					Step:    "profile",
					Status:  "running",
					Message: "画像 Agent 已收到" + legatoTargetLabel(stage.target) + "结果",
					Data:    map[string]any{"ability_profile": diagnosis.AbilityProfile},
				})
			}
		}
	}
	if !resumeParsed {
		if len(resumeFailures) > 0 {
			requiredFailures = append(requiredFailures, resumeFailures...)
		} else {
			requiredFailures = append(requiredFailures, "简历 workflow 未返回有效结构化结果")
		}
	} else if len(resumeFailures) > 0 {
		optionalWarnings = append(optionalWarnings, resumeFailures...)
		diagnosis.ProductionLimitations = append(diagnosis.ProductionLimitations, "简历 workflow 部分子任务失败，已使用已返回的结构化数据继续生成。")
	}
	if len(requiredFailures) > 0 {
		message := "简历 Legato 解析失败，诊断已停止"
		job.SetError(strings.Join(requiredFailures, "；"))
		job.Emit(DiagnosisEvent{
			Type:    "job.failed",
			Status:  "failed",
			Message: message,
			Data: map[string]any{
				"errors": requiredFailures,
			},
		})
		return
	}
	job.Emit(DiagnosisEvent{
		Type:    "step.update",
		Step:    "resume_agent",
		Status:  "done",
		Message: "简历 Resume workflow 已返回可用结构化数据",
	})
	if len(optionalWarnings) > 0 {
		job.Emit(DiagnosisEvent{
			Type:    "step.update",
			Step:    "profile",
			Status:  "running",
			Message: "可选材料解析失败，已跳过对应证据并继续诊断",
			Data: map[string]any{
				"warnings":               optionalWarnings,
				"ability_profile":        diagnosis.AbilityProfile,
				"production_limitations": diagnosis.ProductionLimitations,
			},
		})
	}

	job.Emit(DiagnosisEvent{
		Type:    "step.update",
		Step:    "profile",
		Status:  "running",
		Message: "experience_hybrid 已返回最终经历，画像 Agent 正在合并证据",
		Data:    map[string]any{"ability_profile": diagnosis.AbilityProfile},
	})

	job.Emit(DiagnosisEvent{Type: "step.update", Step: "matching", Status: "running", Message: "岗位匹配 Agent 正在计算推荐岗位"})
	job.Emit(DiagnosisEvent{Type: "step.update", Step: "path", Status: "running", Message: "路径规划 Agent 正在生成阶段任务"})
	var downstream sync.WaitGroup
	downstream.Add(1)
	go func() {
		defer downstream.Done()
		time.Sleep(260 * time.Millisecond)
		job.Emit(DiagnosisEvent{
			Type:    "step.update",
			Step:    "path",
			Status:  "done",
			Message: "成长路径已生成，可查看路径模块",
			Data:    map[string]any{"path_plan": diagnosis.PathPlan},
		})
	}()
	downstreamDone := make(chan struct{})
	go func() {
		downstream.Wait()
		close(downstreamDone)
	}()

	downstreamPending := true
	for downstreamPending {
		select {
		case <-downstreamDone:
			downstreamPending = false
			downstreamDone = nil
		case <-ctx.Done():
			downstreamPending = false
		}
	}

	job.Emit(DiagnosisEvent{Type: "step.update", Step: "outputs", Status: "running", Message: "导出 Agent 正在整理结构化输出"})
	shortPause()
	diagnosis.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	job.SetDiagnosis(diagnosis)
	job.Emit(DiagnosisEvent{
		Type:    "step.update",
		Step:    "outputs",
		Status:  "done",
		Message: "结构化输出已就绪",
		Data: map[string]any{
			"backend_requirements":   diagnosis.BackendRequirements,
			"production_limitations": diagnosis.ProductionLimitations,
			"generated_at":           diagnosis.GeneratedAt,
		},
	})
	job.Emit(DiagnosisEvent{
		Type:    "job.done",
		Status:  "completed",
		Message: "异步诊断完成",
		Data:    map[string]any{"diagnosis": diagnosis},
	})
}

func (s Server) runLegatoStage(ctx context.Context, job *DiagnosisJob, target string, step string, runningMessage string) (*LegatoEnvelope, error) {
	file, ok := firstFileByKind(job.Files, target)
	if !ok {
		job.Emit(DiagnosisEvent{Type: "step.update", Step: step, Status: "done", Message: "未上传对应材料，跳过该 Agent"})
		return nil, nil
	}
	if file.Path == "" {
		err := errors.New("缺少上传文件路径，无法调用 Legato")
		job.Emit(DiagnosisEvent{
			Type:    "step.update",
			Step:    step,
			Status:  "failed",
			Message: "Legato " + target + " 解析失败",
			Data: map[string]any{
				"source": file.SourceFile,
				"error":  err.Error(),
			},
		})
		return nil, err
	}

	job.Emit(DiagnosisEvent{Type: "step.update", Step: step, Status: "running", Message: runningMessage})
	result, err := runLegato(ctx, file.Path, target)
	if err != nil {
		job.Emit(DiagnosisEvent{
			Type:    "step.update",
			Step:    step,
			Status:  "failed",
			Message: "Legato " + target + " 解析失败",
			Data: map[string]any{
				"source": file.SourceFile,
				"error":  err.Error(),
			},
		})
		return nil, err
	}
	job.Emit(DiagnosisEvent{
		Type:    "step.update",
		Step:    step,
		Status:  "done",
		Message: "Legato " + target + " 解析完成",
		Data: map[string]any{
			"source": file.SourceFile,
			"legato": result,
		},
	})
	return result, nil
}

func (s Server) runResumeWorkflowStage(ctx context.Context, job *DiagnosisJob, workflowStage string, runningMessage string) (*LegatoEnvelope, error) {
	file, ok := firstFileByKind(job.Files, "resume")
	step := "resume_agent"
	if !ok {
		err := errors.New("缺少简历文件，无法调用 Legato resume workflow")
		job.Emit(DiagnosisEvent{Type: "step.update", Step: step, Status: "failed", Message: err.Error()})
		return nil, err
	}
	if file.Path == "" {
		err := errors.New("缺少上传文件路径，无法调用 Legato resume workflow")
		job.Emit(DiagnosisEvent{
			Type:    "step.update",
			Step:    step,
			Status:  "failed",
			Message: err.Error(),
			Data: map[string]any{
				"source": file.SourceFile,
				"stage":  workflowStage,
				"error":  err.Error(),
			},
		})
		return nil, err
	}

	job.Emit(DiagnosisEvent{
		Type:    "step.update",
		Step:    step,
		Status:  "running",
		Message: runningMessage,
		Data: map[string]any{
			"source": file.SourceFile,
			"stage":  workflowStage,
		},
	})
	result, err := runLegato(ctx, file.Path, "resume", workflowStage)
	if err != nil {
		job.Emit(DiagnosisEvent{
			Type:    "step.update",
			Step:    step,
			Status:  "failed",
			Message: "Legato resume workflow " + workflowStage + " 解析失败",
			Data: map[string]any{
				"source": file.SourceFile,
				"stage":  workflowStage,
				"error":  err.Error(),
			},
		})
		return nil, err
	}
	job.Emit(DiagnosisEvent{
		Type:    "step.update",
		Step:    step,
		Status:  "running",
		Message: "Legato " + legatoTargetLabel("resume_"+workflowStage) + "已返回结构化数据",
		Data: map[string]any{
			"source": file.SourceFile,
			"stage":  workflowStage,
			"legato": result,
		},
	})
	return result, nil
}

func (s Server) runResumeWorkflowStageWithInput(ctx context.Context, job *DiagnosisJob, workflowStage string, runningMessage string, stageInput any) (*LegatoEnvelope, error) {
	file, ok := firstFileByKind(job.Files, "resume")
	step := "profile"
	if workflowStage == "job_matching" {
		step = "matching"
	}
	if !ok || file.Path == "" {
		err := errors.New("缺少简历文件，无法调用 Legato resume workflow")
		job.Emit(DiagnosisEvent{Type: "step.update", Step: step, Status: "failed", Message: err.Error()})
		return nil, err
	}

	inputFile, err := os.CreateTemp("", "jobagent-legato-stage-input-*.json")
	if err != nil {
		return nil, err
	}
	inputPath := inputFile.Name()
	defer os.Remove(inputPath)
	encoder := json.NewEncoder(inputFile)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(stageInput); err != nil {
		inputFile.Close()
		return nil, err
	}
	if err := inputFile.Close(); err != nil {
		return nil, err
	}

	job.Emit(DiagnosisEvent{
		Type:    "step.update",
		Step:    step,
		Status:  "running",
		Message: runningMessage,
		Data: map[string]any{
			"source": file.SourceFile,
			"stage":  workflowStage,
		},
	})
	result, err := runLegatoWithWorkflowStageInput(ctx, file.Path, "resume", workflowStage, inputPath)
	if err != nil {
		job.Emit(DiagnosisEvent{
			Type:    "step.update",
			Step:    step,
			Status:  "failed",
			Message: "Legato resume workflow " + workflowStage + " 解析失败",
			Data: map[string]any{
				"source": file.SourceFile,
				"stage":  workflowStage,
				"error":  err.Error(),
			},
		})
		return nil, err
	}
	job.Emit(DiagnosisEvent{
		Type:    "step.update",
		Step:    step,
		Status:  "running",
		Message: "Legato " + legatoTargetLabel("resume_"+workflowStage) + "已返回结构化数据",
		Data: map[string]any{
			"source": file.SourceFile,
			"stage":  workflowStage,
			"legato": result,
		},
	})
	return result, nil
}

func runLegato(ctx context.Context, sourcePath string, target string, workflowStage ...string) (*LegatoEnvelope, error) {
	workflowStageInput := ""
	return runLegatoWithOptions(ctx, sourcePath, target, workflowStageInput, workflowStage...)
}

func runLegatoWithWorkflowStageInput(ctx context.Context, sourcePath string, target string, workflowStage string, workflowStageInput string) (*LegatoEnvelope, error) {
	return runLegatoWithOptions(ctx, sourcePath, target, workflowStageInput, workflowStage)
}

func runLegatoWithOptions(ctx context.Context, sourcePath string, target string, workflowStageInput string, workflowStage ...string) (*LegatoEnvelope, error) {
	root := legatoRoot()
	if root == "" {
		return nil, errors.New("cannot locate Agents/legato")
	}
	python := legatoPython(root)
	args := []string{
		"-m", "legato.cli",
		sourcePath,
		"--target", target,
		"--timeout-ms", legatoTimeoutMS(),
	}
	if target == "resume" || target == "chat" {
		args = append(args, "--workflow", target)
		if len(workflowStage) > 0 && strings.TrimSpace(workflowStage[0]) != "" {
			args = append(args, "--workflow-stage", strings.TrimSpace(workflowStage[0]))
		}
		if strings.TrimSpace(workflowStageInput) != "" {
			args = append(args, "--workflow-stage-input", workflowStageInput)
		}
		if target == "chat" {
			args = append(args, "--debug")
		}
	}
	if legatoUsePresto() {
		args = append(args, "--presto-url", legatoPrestoURL())
	} else {
		args = append(args, "--no-presto")
	}
	cmd := exec.CommandContext(ctx, python, args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PYTHONPATH="+pythonPath(root))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s failed: %w: %s", target, err, strings.TrimSpace(string(output)))
	}
	var envelope LegatoEnvelope
	if err := json.Unmarshal(output, &envelope); err != nil {
		return nil, fmt.Errorf("cannot parse Legato %s output: %w", target, err)
	}
	if envelope.Status == "failed" || envelope.Error != "" {
		return nil, fmt.Errorf("Legato %s failed: %s", target, envelope.Error)
	}
	return &envelope, nil
}

func saveDiagnosisUploads(r *http.Request) ([]SavedUpload, error) {
	r.Body = http.MaxBytesReader(nilResponseWriter{}, r.Body, maxUploadBytes)
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "application/json") {
		return nil, errors.New("强制 Legato 模式只支持 multipart/form-data 文件上传")
	}
	if !strings.HasPrefix(contentType, "multipart/form-data") {
		return nil, errors.New("请使用 multipart/form-data 或 application/json")
	}
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		return nil, errors.New("上传内容无法读取，请检查文件大小和格式")
	}
	tempDir, err := os.MkdirTemp("", "jobagent-diagnosis-*")
	if err != nil {
		return nil, errors.New("无法创建上传临时目录")
	}
	var files []SavedUpload
	for _, spec := range []struct {
		key  string
		kind string
	}{
		{key: "resume", kind: "resume"},
		{key: "transcript", kind: "transcript"},
		{key: "other", kind: "other"},
	} {
		saved, err := saveFilesFromForm(r.MultipartForm, spec.key, spec.kind, tempDir)
		if err != nil {
			return nil, err
		}
		files = append(files, saved...)
	}
	return files, nil
}

type nilResponseWriter struct{}

func (nilResponseWriter) Header() http.Header       { return http.Header{} }
func (nilResponseWriter) Write([]byte) (int, error) { return 0, nil }
func (nilResponseWriter) WriteHeader(int)           {}

func saveFilesFromForm(form *multipart.Form, key string, kind string, tempDir string) ([]SavedUpload, error) {
	if form == nil || form.File == nil {
		return nil, nil
	}
	headers := form.File[key]
	files := make([]SavedUpload, 0, len(headers))
	for index, header := range headers {
		source, err := header.Open()
		if err != nil {
			return nil, errors.New("上传文件无法打开")
		}
		defer source.Close()
		name := safeFilename(header.Filename)
		target := filepath.Join(tempDir, fmt.Sprintf("%s_%d_%s", kind, index+1, name))
		destination, err := os.Create(target)
		if err != nil {
			return nil, errors.New("上传文件无法保存")
		}
		if _, err := io.Copy(destination, source); err != nil {
			_ = destination.Close()
			return nil, errors.New("上传文件保存失败")
		}
		if err := destination.Close(); err != nil {
			return nil, errors.New("上传文件保存失败")
		}
		files = append(files, SavedUpload{
			SourceFile: SourceFile{Kind: kind, Name: header.Filename, Size: header.Size},
			Path:       target,
		})
	}
	return files, nil
}

func sourceFiles(files []SavedUpload) []SourceFile {
	out := make([]SourceFile, 0, len(files))
	for _, file := range files {
		out = append(out, file.SourceFile)
	}
	return out
}

func firstFileByKind(files []SavedUpload, kind string) (SavedUpload, bool) {
	for _, file := range files {
		if file.Kind == kind {
			return file, true
		}
	}
	return SavedUpload{}, false
}

func tempDirFromFiles(files []SavedUpload) string {
	for _, file := range files {
		if file.Path != "" {
			return filepath.Dir(file.Path)
		}
	}
	return ""
}

func ensureResumeFileAvailable(job *DiagnosisJob) error {
	file, ok := firstFileByKind(job.Files, "resume")
	if !ok || file.Path == "" {
		return errors.New("缺少简历文件，无法继续 Benchmark")
	}
	if _, err := os.Stat(file.Path); err != nil {
		name := strings.TrimSpace(file.SourceFile.Name)
		if name == "" {
			name = filepath.Base(file.Path)
		}
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("上传简历临时文件已过期或被清理，无法从当前任务继续 Benchmark：%s；请重新生成诊断", name)
		}
		return fmt.Errorf("上传简历文件不可读取，无法继续 Benchmark：%w", err)
	}
	return nil
}

func scheduleDiagnosisTempCleanup(tempDir string) {
	if tempDir == "" {
		return
	}
	retention := diagnosisTempRetention()
	if retention <= 0 {
		cleanupDiagnosisTempDir(tempDir)
		return
	}
	time.AfterFunc(retention, func() {
		cleanupDiagnosisTempDir(tempDir)
	})
}

func cleanupDiagnosisTempDir(tempDir string) {
	if tempDir == "" {
		return
	}
	if err := os.RemoveAll(tempDir); err != nil {
		log.Printf("cleanup diagnosis temp dir failed: %v", err)
	}
}

func diagnosisTempRetention() time.Duration {
	raw := strings.TrimSpace(os.Getenv("JOBAGENT_TEMP_RETENTION_MINUTES"))
	if raw == "" {
		return defaultDiagnosisTempRetention
	}
	minutes, err := strconv.Atoi(raw)
	if err != nil {
		log.Printf("invalid JOBAGENT_TEMP_RETENTION_MINUTES=%q, using default %s", raw, defaultDiagnosisTempRetention)
		return defaultDiagnosisTempRetention
	}
	if minutes <= 0 {
		return 0
	}
	return time.Duration(minutes) * time.Minute
}

func legatoRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	candidates := []string{
		filepath.Join(cwd, "Agents", "legato"),
		filepath.Join(cwd, "..", "Agents", "legato"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(filepath.Join(candidate, "legato", "cli.py")); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func legatoPython(root string) string {
	if python := strings.TrimSpace(os.Getenv("LEGATO_PYTHON")); python != "" {
		return python
	}
	candidate := filepath.Join(root, ".venv", "bin", "python")
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		return candidate
	}
	return "python3"
}

func legatoUsePresto() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("LEGATO_USE_PRESTO")))
	return value == "" || value == "1" || value == "true" || value == "yes" || value == "on"
}

func legatoPrestoURL() string {
	for _, key := range []string{"LEGATO_PRESTO_URL", "PRESTO_URL"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return strings.TrimRight(value, "/")
		}
	}
	return "http://127.0.0.1:8080"
}

func legatoTimeoutMS() string {
	value := strings.TrimSpace(os.Getenv("LEGATO_TIMEOUT_MS"))
	if value == "" {
		return "60000"
	}
	return value
}

func itemBenchmarkMaxRequests() int {
	for _, key := range []string{"ITEM_BENCHMARK_MAX_REQUESTS", "BENCHMARK_MAX_REQUESTS"} {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}
		count, err := strconv.Atoi(value)
		if err != nil || count <= 0 {
			log.Printf("invalid %s=%q, using 5", key, value)
			return 5
		}
		return count
	}
	return 5
}

func diagnosisTimeout() time.Duration {
	value := strings.TrimSpace(os.Getenv("DIAGNOSIS_TIMEOUT_SECONDS"))
	if value == "" {
		return defaultDiagnosisTimeout
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		log.Printf("invalid DIAGNOSIS_TIMEOUT_SECONDS=%q, using %s", value, defaultDiagnosisTimeout)
		return defaultDiagnosisTimeout
	}
	return time.Duration(seconds) * time.Second
}

func pythonPath(root string) string {
	if existing := strings.TrimSpace(os.Getenv("PYTHONPATH")); existing != "" {
		return root + string(os.PathListSeparator) + existing
	}
	return root
}

func safeFilename(name string) string {
	name = filepath.Base(name)
	name = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		default:
			return r
		}
	}, name)
	if strings.TrimSpace(name) == "" {
		return "upload.bin"
	}
	return name
}

func shortPause() {
	time.Sleep(180 * time.Millisecond)
}

func legatoTargetLabel(target string) string {
	switch target {
	case "resume":
		return "简历解析"
	case "resume_profile":
		return "简历基础信息"
	case "resume_certifications_awards":
		return "简历证书奖项"
	case "resume_experience":
		return "简历经历"
	case "resume_experience_hybrid":
		return "简历经历 hybrid"
	case "resume_experience_hybrid_item":
		return "简历经历 hybrid item"
	case "resume_item_benchmark":
		return "简历 Item Benchmark"
	case "resume_major_baseline":
		return "简历 Major Baseline"
	case "resume_job_matching":
		return "简历 Job Matching"
	case "transcript":
		return "成绩单解析"
	default:
		return target
	}
}

func isResumeLegatoTarget(target string) bool {
	return target == "resume" || strings.HasPrefix(target, "resume_")
}

func applyOptionalLegatoFailure(diagnosis *Diagnosis, target string, err error) {
	switch target {
	case "transcript":
		diagnosis.AbilityProfile.BasicInfo.TranscriptUse = "成绩单解析失败，已跳过可选成绩单证据"
		diagnosis.AbilityProfile.EvidenceSummary = append(diagnosis.AbilityProfile.EvidenceSummary, EvidenceItem{
			Category: "Legato 成绩单解析",
			Summary:  "可选成绩单解析失败：" + err.Error(),
			Signal:   "未纳入本次能力映射",
		})
		diagnosis.ProductionLimitations = append(diagnosis.ProductionLimitations, "本次上传的成绩单解析失败，课程与 GPA 证据未纳入能力画像。")
	default:
		diagnosis.ProductionLimitations = append(diagnosis.ProductionLimitations, legatoTargetLabel(target)+"失败，已跳过该可选材料。")
	}
}

func applyResumeWorkflowProfile(diagnosis *Diagnosis, result *LegatoEnvelope) {
	data := result.Data
	identity, _ := data["identity"].(map[string]any)
	if name := stringValue(identity["name"]); name != "" {
		diagnosis.AbilityProfile.BasicInfo.Name = name
	}
	if sex := stringValue(identity["sex"]); sex != "" {
		diagnosis.AbilityProfile.BasicInfo.Sex = sex
	}
	if birthYear := stringValue(identity["birth_year"]); birthYear != "" {
		diagnosis.AbilityProfile.BasicInfo.BirthYear = birthYear
	}
	diagnosis.AbilityProfile.Education = buildEducationItems(objectArray(data["education"]))
	if education := firstObject(data, "education"); education != nil {
		if school := stringValue(education["school"]); school != "" {
			diagnosis.AbilityProfile.BasicInfo.School = school
		}
		if major := stringValue(education["major"]); major != "" {
			diagnosis.AbilityProfile.BasicInfo.Major = major
		}
		if degree := stringValue(education["degree_level"]); degree != "" {
			diagnosis.AbilityProfile.BasicInfo.Degree = degree
		} else if degree := stringValue(education["degree"]); degree != "" {
			diagnosis.AbilityProfile.BasicInfo.Degree = degree
		}
	}
	diagnosis.AbilityProfile.BasicInfo.ResumeStatus = fmt.Sprintf("Legato Resume workflow 已返回基础信息，frontend=%s，formatter=%s", result.Frontend, result.Formatter)
	diagnosis.AbilityProfile.EvidenceSummary = append([]EvidenceItem{
		{
			Category: "Legato 简历基础信息",
			Summary:  fmt.Sprintf("Resume workflow profile 段耗时 %d ms，抽取字符数 %d。", result.ElapsedMS, result.MarkdownChars),
			Signal:   "真实简历结构化已接入",
		},
	}, diagnosis.AbilityProfile.EvidenceSummary...)
	applyLegatoWarnings(diagnosis, "简历基础信息", result)
}

func buildEducationItems(items []map[string]any) []EducationItem {
	education := make([]EducationItem, 0, len(items))
	for _, item := range items {
		school := stringValue(item["school"])
		major := stringValue(item["major"])
		department := stringValue(item["department"])
		degree := stringValue(item["degree_level"])
		if degree == "" {
			degree = stringValue(item["degree"])
		}
		tags, _ := item["school_tags"].(map[string]any)
		if school == "" && tags != nil {
			school = stringValue(tags["matched_school"])
		}
		if school == "" && major == "" && department == "" && degree == "" {
			continue
		}
		education = append(education, EducationItem{
			School:             school,
			Degree:             degree,
			Department:         department,
			Major:              major,
			Is985:              boolValue(tags["is_985"]),
			Is211:              boolValue(tags["is_211"]),
			IsDoubleFirstClass: boolValue(tags["is_double_first_class"]),
			RuankeRank:         intValue(tags["ruanke_rank"]),
			SchoolKind:         stringValue(tags["school_kind"]),
			ParentSchool:       stringValue(tags["parent_school"]),
		})
	}
	return education
}

func applyResumeWorkflowCertifications(diagnosis *Diagnosis, result *LegatoEnvelope) {
	items := objectArray(result.Data["certifications_awards"])
	diagnosis.AbilityProfile.Awards = buildAwardItems(items)
	if len(diagnosis.AbilityProfile.Awards) == 0 {
		diagnosis.AbilityProfile.AwardsStatus = "empty"
	} else {
		diagnosis.AbilityProfile.AwardsStatus = "ready"
	}
	summary := fmt.Sprintf("Resume workflow 已解析证书和奖项 %d 条。", len(items))
	if len(items) > 0 {
		names := make([]string, 0, minInt(len(items), 3))
		for _, item := range items[:minInt(len(items), 3)] {
			if name := stringValue(item["name"]); name != "" {
				names = append(names, name)
			}
		}
		if len(names) > 0 {
			summary += " 代表项：" + strings.Join(names, "、") + "。"
		}
	}
	diagnosis.AbilityProfile.EvidenceSummary = append(diagnosis.AbilityProfile.EvidenceSummary, EvidenceItem{
		Category: "Legato 简历证书奖项",
		Summary:  summary,
		Signal:   "证书与竞赛证据已结构化",
	})
	applyLegatoWarnings(diagnosis, "简历证书奖项", result)
}

func applyResumeWorkflowExperience(diagnosis *Diagnosis, result *LegatoEnvelope, hybrid bool) {
	items := objectArray(result.Data["experience"])
	diagnosis.AbilityProfile.Experiences = buildExperienceItems(items)
	if hybrid {
		markPendingExperienceHybridItems(diagnosis, "ready")
	} else {
		markPendingExperienceHybridItems(diagnosis, "pending")
	}
	if len(diagnosis.AbilityProfile.Experiences) == 0 {
		if hybrid {
			diagnosis.AbilityProfile.ExperiencesStatus = "empty"
		} else {
			diagnosis.AbilityProfile.ExperiencesStatus = "refining"
		}
	} else {
		if hybrid {
			diagnosis.AbilityProfile.ExperiencesStatus = "ready"
		} else {
			diagnosis.AbilityProfile.ExperiencesStatus = "refining"
		}
	}
	summary := fmt.Sprintf("Resume workflow 已解析经历 %d 条。", len(items))
	category := "Legato 简历经历"
	signal := "项目、实习和活动经历已结构化"
	warningLabel := "简历经历"
	if hybrid {
		category = "Legato 简历经历 hybrid"
		signal = "高精度经历分析已替换快速结果"
		warningLabel = "简历经历 hybrid"
		summary = fmt.Sprintf("experience_hybrid 已解析经历 %d 条。", len(items))
	}
	if len(items) > 0 {
		if contribution := stringValue(items[0]["contribution"]); contribution != "" {
			summary += " 代表经历：" + contribution + "。"
		}
	}
	diagnosis.AbilityProfile.EvidenceSummary = append(diagnosis.AbilityProfile.EvidenceSummary, EvidenceItem{
		Category: category,
		Summary:  summary,
		Signal:   signal,
	})
	applyLegatoWarnings(diagnosis, warningLabel, result)
}

func applyResumeWorkflowItemBenchmark(diagnosis *Diagnosis, result *LegatoEnvelope) int {
	items := objectArray(result.Data["item_benchmark"])
	if len(items) == 0 {
		diagnosis.AbilityProfile.BenchmarkStatus = "empty"
		return 0
	}

	applied := 0
	for _, benchmark := range items {
		item := objectValue(benchmark["item"])
		kind := stringValue(item["kind"])
		key := stringValue(item["key"])
		dimensions := stringArrayValue(benchmark["dimensions"])
		if len(dimensions) == 0 {
			dimensions = benchmarkDimensionNames()
		}
		scores := floatArrayValue(benchmark["scores"])
		if len(scores) != len(dimensions) || len(scores) == 0 {
			continue
		}
		impact := clampFloat(floatValue(benchmark["impact_factor"]), 0, 10)
		scope := normalizeEvidenceScope(stringValue(item["evidence_scope"]), kind, stringValue(item["name"]), stringValue(item["result"]), stringValue(item["type"]), stringValue(item["role"]), stringValue(item["contribution"]))
		if kind == "experience" {
			if index, ok := benchmarkIndexFromKey(key, "experience", len(diagnosis.AbilityProfile.Experiences)); ok {
				diagnosis.AbilityProfile.Experiences[index].ImpactFactor = floatPtr(impact)
				diagnosis.AbilityProfile.Experiences[index].BenchmarkDimensions = dimensions
				diagnosis.AbilityProfile.Experiences[index].BenchmarkScores = scores
				diagnosis.AbilityProfile.Experiences[index].EvidenceScope = scope
				diagnosis.AbilityProfile.Experiences[index].ScoreSource = "Legato level + item_benchmark impact_factor"
				applied++
			}
			continue
		}
		if index, ok := benchmarkIndexFromKey(key, "award", len(diagnosis.AbilityProfile.Awards)); ok {
			diagnosis.AbilityProfile.Awards[index].ImpactFactor = floatPtr(impact)
			diagnosis.AbilityProfile.Awards[index].BenchmarkDimensions = dimensions
			diagnosis.AbilityProfile.Awards[index].BenchmarkScores = scores
			diagnosis.AbilityProfile.Awards[index].EvidenceScope = scope
			diagnosis.AbilityProfile.Awards[index].ScoreSource = "Legato level + item_benchmark impact_factor"
			applied++
		}
	}

	if applied == 0 {
		diagnosis.AbilityProfile.BenchmarkStatus = "empty"
		return 0
	}
	diagnosis.AbilityProfile.BenchmarkStatus = "ready"
	diagnosis.AbilityProfile.EvidenceSummary = append(diagnosis.AbilityProfile.EvidenceSummary, EvidenceItem{
		Category: "Legato Item Benchmark",
		Summary:  fmt.Sprintf("已为 %d 条证据补充六维分布和 impact_factor。", applied),
		Signal:   "证据影响因子已结构化",
	})
	applyLegatoWarnings(diagnosis, "Item Benchmark", result)
	return applied
}

func buildMajorBaselineStageInput(diagnosis Diagnosis) map[string]any {
	return map[string]any{
		"basic_info":     diagnosis.AbilityProfile.BasicInfo,
		"education":      diagnosis.AbilityProfile.Education,
		"transcript_use": diagnosis.AbilityProfile.BasicInfo.TranscriptUse,
	}
}

func buildJobMatchingStageInput(diagnosis Diagnosis) map[string]any {
	return map[string]any{
		"basic_info":     diagnosis.AbilityProfile.BasicInfo,
		"education":      diagnosis.AbilityProfile.Education,
		"major_baseline": diagnosis.AbilityProfile.MajorBaseline,
		"awards":         diagnosis.AbilityProfile.Awards,
		"experiences":    diagnosis.AbilityProfile.Experiences,
		"dimensions":     benchmarkDimensionNames(),
	}
}

func applyResumeWorkflowMajorBaseline(diagnosis *Diagnosis, result *LegatoEnvelope) {
	baseline := objectValue(result.Data["major_baseline"])
	if len(baseline) == 0 {
		diagnosis.AbilityProfile.MajorBaselineStatus = "empty"
		return
	}
	dimensions := stringArrayValue(baseline["dimensions"])
	if len(dimensions) == 0 {
		dimensions = benchmarkDimensionNames()
	}
	scores := intArrayValue(baseline["scores"])
	if len(scores) != len(dimensions) || len(scores) == 0 {
		diagnosis.AbilityProfile.MajorBaselineStatus = "empty"
		return
	}
	baseScore := intValue(baseline["base_score"])
	if baseScore == 0 {
		baseScore = 50
	} else {
		baseScore = clampInt(baseScore, 30, 85)
	}
	diagnosis.AbilityProfile.MajorBaseline = MajorBaseline{
		MajorName:   stringValue(baseline["major_name"]),
		MajorFamily: stringValue(baseline["major_family"]),
		BaseScore:   baseScore,
		Dimensions:  dimensions,
		Scores:      scores,
		Rationale:   stringValue(baseline["rationale"]),
		Confidence:  clampFloat(floatValue(baseline["confidence"]), 0, 1),
		Source:      stringValue(baseline["source"]),
	}
	diagnosis.AbilityProfile.MajorBaselineStatus = "ready"
	majorFamily := diagnosis.AbilityProfile.MajorBaseline.MajorFamily
	if majorFamily == "" {
		majorFamily = "未知"
	}
	diagnosis.AbilityProfile.EvidenceSummary = append(diagnosis.AbilityProfile.EvidenceSummary, EvidenceItem{
		Category: "Legato Major Baseline",
		Summary:  fmt.Sprintf("已根据%s专业背景生成六维能力 prior。", majorFamily),
		Signal:   "专业培养 prior 已结构化",
	})
	applyLegatoWarnings(diagnosis, "Major Baseline", result)
}

func applyResumeWorkflowJobMatching(diagnosis *Diagnosis, result *LegatoEnvelope) {
	raw := objectValue(result.Data["job_matching"])
	if len(raw) == 0 {
		return
	}
	topJobs := buildMatchedJobs(raw["top_jobs"])
	selectedJob := buildMatchedJob(objectValue(raw["selected_job"]), 1)
	if selectedJob.Title == "" && len(topJobs) > 0 {
		selectedJob = topJobs[0]
	}
	if len(topJobs) == 0 && selectedJob.Title != "" {
		topJobs = []MatchedJob{selectedJob}
	}
	targetRole := stringValue(raw["target_role"])
	if targetRole == "" {
		targetRole = selectedJob.Title
	}
	studentRadar := buildScoreDimensions(raw["student_radar"])
	targetRadar := buildScoreDimensions(raw["target_radar"])
	if len(targetRadar) == 0 {
		targetRadar = selectedJob.RequirementRadar
	}
	if selectedJob.Title != "" && len(selectedJob.RequirementRadar) == 0 {
		selectedJob.RequirementRadar = targetRadar
	}
	diagnosis.MatchingResult = MatchingResult{
		TargetRole:      targetRole,
		OverallMatch:    clampInt(intValue(raw["overall_match"]), 0, 100),
		MatchLevel:      stringValue(raw["match_level"]),
		Source:          stringValue(raw["source"]),
		MethodSummary:   stringValue(raw["method_summary"]),
		FitSummary:      stringValue(raw["fit_summary"]),
		SelectedJob:     selectedJob,
		StudentRadar:    studentRadar,
		TargetRadar:     targetRadar,
		ReportSections:  buildReportRows(raw["report_sections"]),
		GapDetails:      buildGapDetails(raw["gap_details"]),
		Recommendations: stringArrayValue(raw["recommendations"]),
		Reasons:         stringArrayValue(raw["recommended_reasons"]),
		AgentNotes:      stringArrayValue(raw["agent_notes"]),
	}
	if diagnosis.MatchingResult.OverallMatch == 0 && selectedJob.Match > 0 {
		diagnosis.MatchingResult.OverallMatch = selectedJob.Match
	}
	if diagnosis.MatchingResult.MatchLevel == "" {
		diagnosis.MatchingResult.MatchLevel = matchLevelFromScore(diagnosis.MatchingResult.OverallMatch)
	}
	if diagnosis.MatchingResult.Source == "" {
		diagnosis.MatchingResult.Source = "Legato Job Matching Team via Presto"
	}
	if len(diagnosis.MatchingResult.ReportSections) == 0 && len(studentRadar) == len(targetRadar) {
		diagnosis.MatchingResult.ReportSections = reportRowsFromRadar(studentRadar, targetRadar)
	}
	diagnosis.AbilityProfile.TopJobs = topJobs
	if targetRole != "" {
		diagnosis.AbilityProfile.BasicInfo.TargetRole = targetRole
	}
	diagnosis.AbilityProfile.EvidenceSummary = append(diagnosis.AbilityProfile.EvidenceSummary, EvidenceItem{
		Category: "Legato Job Matching",
		Summary:  fmt.Sprintf("已推荐 %d 个岗位，首选岗位为%s。", len(topJobs), fallbackString(targetRole, "未命名岗位")),
		Signal:   "岗位推荐和目标能力雷达已结构化",
	})
	applyLegatoWarnings(diagnosis, "Job Matching", result)
}

func buildMatchedJobs(value any) []MatchedJob {
	items := objectArray(value)
	jobs := make([]MatchedJob, 0, minInt(len(items), 5))
	for index, item := range items {
		job := buildMatchedJob(item, index+1)
		if job.Title == "" {
			continue
		}
		job.Rank = len(jobs) + 1
		jobs = append(jobs, job)
		if len(jobs) >= 5 {
			break
		}
	}
	return jobs
}

func buildMatchedJob(item map[string]any, defaultRank int) MatchedJob {
	if len(item) == 0 {
		return MatchedJob{}
	}
	title := stringValue(item["title"])
	if title == "" {
		title = stringValue(item["role"])
	}
	if title == "" {
		return MatchedJob{}
	}
	return MatchedJob{
		Rank:             clampInt(defaultInt(intValue(item["rank"]), defaultRank), 1, 99),
		Title:            title,
		Category:         stringValue(item["category"]),
		Match:            clampInt(intValue(item["match"]), 0, 100),
		AbilityMatch:     clampInt(intValue(item["ability_match"]), 0, 100),
		ExperienceMatch:  clampInt(intValue(item["experience_match"]), 0, 100),
		EducationGate:    stringValue(item["education_gate"]),
		FitSummary:       stringValue(item["fit_summary"]),
		Risk:             stringValue(item["risk"]),
		RequirementRadar: buildScoreDimensions(item["requirement_radar"]),
		Reasons:          stringArrayValue(item["reasons"]),
		NextProof:        stringValue(item["next_proof"]),
	}
}

func buildScoreDimensions(value any) []ScoreDimension {
	items := objectArray(value)
	if len(items) == 0 {
		return nil
	}
	dimensions := make([]ScoreDimension, 0, len(items))
	for _, item := range items {
		name := stringValue(item["name"])
		if name == "" {
			name = stringValue(item["dimension"])
		}
		score := clampInt(intValue(item["score"]), 0, 100)
		maxScore := intValue(item["max_score"])
		if maxScore == 0 {
			maxScore = 100
		}
		if name == "" {
			continue
		}
		dimensions = append(dimensions, ScoreDimension{
			Name:     name,
			Score:    score,
			MaxScore: clampInt(maxScore, 1, 100),
			Level:    stringValue(item["level"]),
			Reason:   stringValue(item["reason"]),
		})
	}
	return dimensions
}

func buildReportRows(value any) []ReportRow {
	items := objectArray(value)
	rows := make([]ReportRow, 0, len(items))
	for _, item := range items {
		name := stringValue(item["name"])
		if name == "" {
			name = stringValue(item["capability"])
		}
		if name == "" {
			continue
		}
		student := clampInt(intValue(item["student"]), 0, 100)
		roleNeed := clampInt(defaultInt(intValue(item["role_need"]), intValue(item["target"])), 0, 100)
		difference := intValue(item["difference"])
		if difference == 0 {
			difference = student - roleNeed
		}
		rows = append(rows, ReportRow{
			Name:       name,
			Student:    student,
			RoleNeed:   roleNeed,
			Difference: clampInt(difference, -100, 100),
		})
	}
	return rows
}

func buildGapDetails(value any) []GapDetail {
	items := objectArray(value)
	gaps := make([]GapDetail, 0, len(items))
	for _, item := range items {
		capability := stringValue(item["capability"])
		if capability == "" {
			capability = stringValue(item["name"])
		}
		if capability == "" {
			continue
		}
		gaps = append(gaps, GapDetail{
			Capability: capability,
			Current:    stringValue(item["current"]),
			Expected:   stringValue(item["expected"]),
			Action:     stringValue(item["action"]),
			Severity:   stringValue(item["severity"]),
		})
	}
	return gaps
}

func reportRowsFromRadar(studentRadar []ScoreDimension, targetRadar []ScoreDimension) []ReportRow {
	targetByName := make(map[string]int, len(targetRadar))
	for _, item := range targetRadar {
		targetByName[item.Name] = item.Score
	}
	rows := make([]ReportRow, 0, len(studentRadar))
	for _, item := range studentRadar {
		roleNeed := targetByName[item.Name]
		rows = append(rows, ReportRow{
			Name:       item.Name,
			Student:    item.Score,
			RoleNeed:   roleNeed,
			Difference: item.Score - roleNeed,
		})
	}
	return rows
}

func matchLevelFromScore(score int) string {
	switch {
	case score >= 85:
		return "强匹配"
	case score >= 75:
		return "高潜力匹配"
	case score >= 65:
		return "可迁移匹配"
	default:
		return "需补证据"
	}
}

func fallbackString(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func defaultInt(value int, fallback int) int {
	if value != 0 {
		return value
	}
	return fallback
}

func benchmarkIndexFromKey(key string, prefix string, length int) (int, bool) {
	needle := prefix + ":"
	if !strings.HasPrefix(key, needle) {
		return 0, false
	}
	index, err := strconv.Atoi(strings.TrimPrefix(key, needle))
	if err != nil || index < 0 || index >= length {
		return 0, false
	}
	return index, true
}

func markResumeEvidenceStatus(diagnosis *Diagnosis, target string, status string) {
	switch target {
	case "resume_certifications_awards":
		diagnosis.AbilityProfile.AwardsStatus = status
	case "resume_experience":
		diagnosis.AbilityProfile.ExperiencesStatus = status
	case "resume_experience_hybrid":
		if len(diagnosis.AbilityProfile.Experiences) == 0 {
			diagnosis.AbilityProfile.ExperiencesStatus = status
		}
	}
}

func chunkBenchmarkInputs(items []BenchmarkEvidenceInput, maxRequests int) [][]BenchmarkEvidenceInput {
	if len(items) == 0 {
		return nil
	}
	if maxRequests <= 0 {
		maxRequests = 5
	}
	requestCount := minInt(maxRequests, len(items))
	chunks := make([][]BenchmarkEvidenceInput, requestCount)
	for index, item := range items {
		bucket := index % requestCount
		chunks[bucket] = append(chunks[bucket], item)
	}
	return chunks
}

func benchmarkStepStatus(pendingItemBatches int, majorBaselinePending bool) string {
	if pendingItemBatches > 0 || majorBaselinePending {
		return "running"
	}
	return "done"
}

func benchmarkProgressMessage(pendingItemBatches int, majorBaselinePending bool, applied int, hasFailures bool) string {
	parts := []string{fmt.Sprintf("Item Benchmark 已补充 %d 条证据", applied)}
	if pendingItemBatches > 0 {
		parts = append(parts, fmt.Sprintf("剩余 %d 批", pendingItemBatches))
	}
	if majorBaselinePending {
		parts = append(parts, "等待 Major Baseline")
	}
	if hasFailures {
		parts = append(parts, "部分批次失败")
	}
	return strings.Join(parts, "，")
}

func markPendingExperienceHybridItems(diagnosis *Diagnosis, status string) {
	for index := range diagnosis.AbilityProfile.Experiences {
		if diagnosis.AbilityProfile.Experiences[index].HybridStatus == "" || diagnosis.AbilityProfile.Experiences[index].HybridStatus == "pending" {
			diagnosis.AbilityProfile.Experiences[index].HybridStatus = status
		}
	}
}

func buildAwardItems(items []map[string]any) []AwardItem {
	awards := make([]AwardItem, 0, len(items))
	for _, item := range items {
		name := stringValue(item["name"])
		result := stringValue(item["result"])
		if name == "" && result == "" {
			continue
		}
		score, signal, reason := scoreAwardEvidence(name, result)
		level := floatValue(item["level"])
		if level <= 0 {
			level = float64(score) / 10
		}
		level = clampFloat(level, 0, 10)
		awards = append(awards, AwardItem{
			Name:          name,
			Result:        result,
			EvidenceScope: normalizeEvidenceScope(stringValue(item["evidence_scope"]), "award", name, result, "", "", ""),
			Level:         level,
			Signal:        signal,
			Reason:        reason,
			DataSource:    "Legato Resume workflow",
			ScoreSource:   "Legato fast level，benchmark impact_factor 待返回",
		})
	}
	return awards
}

func buildExperienceItems(items []map[string]any) []ExperienceItem {
	experiences := make([]ExperienceItem, 0, len(items))
	for _, item := range items {
		contribution := cleanLegatoText(stringValue(item["contribution"]))
		role := cleanLegatoText(stringValue(item["role"]))
		experienceType := cleanLegatoText(stringValue(item["type"]))
		if isSectionOnlyContribution(contribution, experienceType) {
			contribution = ""
		}
		if contribution == "" && role == "" && experienceType == "" {
			continue
		}
		if contribution == "" {
			contribution = fallbackExperienceContribution(experienceType, role)
		}
		level := clampInt(intValue(item["level"]), 0, 10)
		score := level * 10
		signal, reason := experienceSignal(score)
		experiences = append(experiences, ExperienceItem{
			Type:          experienceType,
			Role:          role,
			Contribution:  contribution,
			EvidenceScope: normalizeEvidenceScope(stringValue(item["evidence_scope"]), "experience", "", "", experienceType, role, contribution),
			Level:         level,
			Signal:        signal,
			Reason:        reason,
			DataSource:    "Legato Resume workflow",
			ScoreSource:   "Legato fast level，benchmark impact_factor 待返回",
		})
	}
	return experiences
}

func scoreAwardEvidence(name string, result string) (int, string, string) {
	text := strings.ToLower(name + " " + result)
	score := 52
	reason := "奖项已结构化，但缺少个人贡献描述，按保守规则评分。"
	isCET6 := containsAny(text, "英语六级", "cet-6", "cet6", "六级")
	isBasicCertificate := isCET6 || containsAny(text, "计算机等级", "wps", "office")
	isGenericHonor := containsAny(text, "三好学生", "优秀学生干部", "优秀团员", "奖学金", "标兵", "红旗手")

	if isCET6 {
		score = 52
		reason = "基础证书可证明英语能力，按 Legato 评分约定不作为高价值岗位能力证据。"
		if containsAny(text, "600", "610", "620", "630", "640", "650", "660", "670", "680", "690", "700") {
			score = 62
		} else if containsAny(text, "55", "56", "57", "58", "59") {
			score = 58
		}
	}
	if containsAny(text, "计算机等级", "wps", "office") {
		score = maxInt(score, 38)
		reason = "基础技能证书可作能力补充，但岗位区分度有限。"
	}
	if containsAny(text, "三好学生", "优秀学生干部", "优秀团员", "奖学金", "标兵", "红旗手") {
		score = maxInt(score, 42)
		reason = "泛荣誉可补充综合素质，但缺少具体任务、动作和结果。"
	}
	if containsAny(text, "校级", "学校", "院级") {
		score = maxInt(score, 46)
		reason = "校内奖项可作为学习或组织能力补充，证据强度需要具体经历支撑。"
	}
	if containsAny(text, "省", "赛区", "区域", "华北", "华东", "华南", "东北", "西北", "西南") {
		score = maxInt(score, 62)
		reason = "区域或省级奖项具备一定选择性，可补充能力证据。"
	}
	if containsAny(text, "全国", "国家级", "国家", "国际", "global", "national") {
		score = maxInt(score, 68)
		reason = "全国或国家级奖项具备较强选择性，但仍需个人贡献描述来提高可信度。"
	}
	if containsAny(text, "数学建模", "程序设计", "蓝桥", "互联网+", "挑战杯", "acm", "icpc", "算法", "大创", "创新创业") {
		score = maxInt(score, 66)
		reason = "技术或专业竞赛与岗位能力更相关，可补充问题拆解、建模或工程实践证据。"
	}
	if containsAny(text, "一等奖", "第一名", "冠军", "金奖") {
		score += 8
	} else if containsAny(text, "二等奖", "第二名", "亚军", "银奖") {
		score += 5
	} else if containsAny(text, "三等奖", "第三名", "铜奖") {
		score += 3
	}
	if isBasicCertificate {
		if isCET6 && containsAny(text, "600", "610", "620", "630", "640", "650", "660", "670", "680", "690", "700") {
			score = minInt(score, 62)
		} else {
			score = minInt(score, 58)
		}
		reason = "基础证书可证明英语或工具能力，按 Legato 评分约定不作为高价值岗位能力证据。"
	}
	if isGenericHonor {
		score = minInt(score, 46)
	}
	score = clampInt(score, 0, 82)
	return score, scoreBand(score), reason
}

func experienceSignal(score int) (string, string) {
	switch {
	case score >= 80:
		return "高价值经历", "Legato level 显示该经历具备较强相关性、ownership 或结果证据，可作为核心简历素材。"
	case score >= 70:
		return "强证据", "经历与推荐岗位较相关，已有具体贡献，建议补充量化结果和技术取舍。"
	case score >= 50:
		return "有效证据", "经历有可用贡献描述，但深度、范围或结果证据仍需补强。"
	case score >= 30:
		return "普通经历", "经历可补充背景，但与岗位核心能力的直接关系或结果证据偏弱。"
	case score > 0:
		return "弱证据", "经历信息较少，暂不适合作为主推卖点。"
	default:
		return "无效证据", "Legato 未识别到可用经历评分。"
	}
}

func scoreBand(score int) string {
	switch {
	case score >= 80:
		return "高价值证据"
	case score >= 65:
		return "较强证据"
	case score >= 50:
		return "基础证据"
	case score >= 35:
		return "弱证据"
	default:
		return "低信号"
	}
}

func cleanLegatoText(text string) string {
	text = strings.TrimSpace(text)
	for strings.HasPrefix(text, "#") {
		text = strings.TrimSpace(strings.TrimPrefix(text, "#"))
	}
	return strings.Trim(text, " ：:")
}

func isSectionOnlyContribution(contribution string, experienceType string) bool {
	normalized := strings.ReplaceAll(contribution, " ", "")
	if normalized == "" {
		return false
	}
	sectionNames := []string{"经历", "实习经历", "项目经历", "校园经历", "工作经历", "竞赛经历", "社会实践"}
	for _, name := range sectionNames {
		if normalized == name {
			return true
		}
	}
	if experienceType != "" && (normalized == experienceType || normalized == experienceType+"经历") {
		return true
	}
	return false
}

func fallbackExperienceContribution(experienceType string, role string) string {
	label := experienceType
	if label == "" {
		label = "经历"
	}
	if role != "" {
		return fmt.Sprintf("Legato 已识别到%s（%s），但贡献细节仍需补充。", label, role)
	}
	return fmt.Sprintf("Legato 已识别到%s，但贡献细节仍需补充。", label)
}

func applyResumeLegato(diagnosis *Diagnosis, result *LegatoEnvelope) {
	data := result.Data
	if name := nestedString(data, "identity", "name"); name != "" {
		diagnosis.AbilityProfile.BasicInfo.Name = name
	}
	if sex := nestedString(data, "identity", "sex"); sex != "" {
		diagnosis.AbilityProfile.BasicInfo.Sex = sex
	}
	if birthYear := nestedString(data, "identity", "birth_year"); birthYear != "" {
		diagnosis.AbilityProfile.BasicInfo.BirthYear = birthYear
	}
	if name := nestedString(data, "candidate", "name"); name != "" {
		diagnosis.AbilityProfile.BasicInfo.Name = name
	}
	diagnosis.AbilityProfile.Education = buildEducationItems(objectArray(data["education"]))
	if education := firstObject(data, "education"); education != nil {
		if school := stringValue(education["school"]); school != "" {
			diagnosis.AbilityProfile.BasicInfo.School = school
		}
		if major := stringValue(education["major"]); major != "" {
			diagnosis.AbilityProfile.BasicInfo.Major = major
		}
		if degree := stringValue(education["degree_level"]); degree != "" {
			diagnosis.AbilityProfile.BasicInfo.Degree = degree
		} else if degree := stringValue(education["degree"]); degree != "" {
			diagnosis.AbilityProfile.BasicInfo.Degree = degree
		}
	}
	diagnosis.AbilityProfile.BasicInfo.ResumeStatus = fmt.Sprintf("Legato 已解析简历，frontend=%s，formatter=%s", result.Frontend, result.Formatter)
	diagnosis.AbilityProfile.EvidenceSummary = append([]EvidenceItem{
		{
			Category: "Legato 简历解析",
			Summary:  fmt.Sprintf("解析耗时 %d ms，抽取字符数 %d。", result.ElapsedMS, result.MarkdownChars),
			Signal:   "真实材料解析已接入",
		},
	}, diagnosis.AbilityProfile.EvidenceSummary...)
	applyLegatoWarnings(diagnosis, "简历", result)
}

func applyTranscriptLegato(diagnosis *Diagnosis, result *LegatoEnvelope) {
	courseCount := len(arrayValue(result.Data["courses"]))
	gpa := nestedString(result.Data, "summary", "gpa")
	summary := fmt.Sprintf("Legato 已解析成绩单，课程记录 %d 条。", courseCount)
	if gpa != "" {
		summary += " GPA：" + gpa + "。"
	}
	diagnosis.AbilityProfile.BasicInfo.TranscriptUse = summary
	diagnosis.AbilityProfile.EvidenceSummary = append(diagnosis.AbilityProfile.EvidenceSummary, EvidenceItem{
		Category: "Legato 成绩单解析",
		Summary:  summary,
		Signal:   "课程证据可用于能力映射",
	})
	applyLegatoWarnings(diagnosis, "成绩单", result)
}

func applyLegatoWarnings(diagnosis *Diagnosis, label string, result *LegatoEnvelope) {
	warnings := visibleLegatoWarnings(result.Warnings)
	if len(warnings) == 0 {
		return
	}
	summary := strings.Join(warnings, "；")
	diagnosis.AbilityProfile.EvidenceSummary = append(diagnosis.AbilityProfile.EvidenceSummary, EvidenceItem{
		Category: "Legato " + label + "解析警告",
		Summary:  summary,
		Signal:   "已保留解析降级信息",
	})
	diagnosis.ProductionLimitations = append(diagnosis.ProductionLimitations, "Legato "+label+"解析警告："+summary)
}

func visibleLegatoWarnings(warnings []string) []string {
	out := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		warning = strings.TrimSpace(warning)
		if warning == "" || warning == "normalized markdown before formatting" {
			continue
		}
		out = append(out, warning)
	}
	return out
}

func sanitizeBenchmarkInputs(items []BenchmarkEvidenceInput) []BenchmarkEvidenceInput {
	out := make([]BenchmarkEvidenceInput, 0, len(items))
	for index, item := range items {
		kind := strings.TrimSpace(item.Kind)
		if kind != "award" && kind != "experience" {
			continue
		}
		key := strings.TrimSpace(item.Key)
		if key == "" {
			key = fmt.Sprintf("%s:%d", kind, index)
		}
		clean := BenchmarkEvidenceInput{
			Kind:           kind,
			Key:            key,
			Name:           cleanLegatoText(item.Name),
			Result:         cleanLegatoText(item.Result),
			EvidenceScope:  normalizeEvidenceScope(item.EvidenceScope, kind, item.Name, item.Result, item.ExperienceType, item.Role, item.Contribution),
			ExperienceType: cleanLegatoText(item.ExperienceType),
			Role:           cleanLegatoText(item.Role),
			Contribution:   cleanLegatoText(item.Contribution),
			Level:          clampFloat(item.Level, 0, 10),
		}
		if clean.Name == "" && clean.Result == "" && clean.Role == "" && clean.Contribution == "" {
			continue
		}
		out = append(out, clean)
	}
	return out
}

func nestedString(data map[string]any, objectKey string, fieldKey string) string {
	object, ok := data[objectKey].(map[string]any)
	if !ok {
		return ""
	}
	return stringValue(object[fieldKey])
}

func firstObject(data map[string]any, key string) map[string]any {
	items := arrayValue(data[key])
	if len(items) == 0 {
		return nil
	}
	object, _ := items[0].(map[string]any)
	return object
}

func arrayValue(value any) []any {
	items, _ := value.([]any)
	return items
}

func objectArray(value any) []map[string]any {
	items := arrayValue(value)
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if object, ok := item.(map[string]any); ok {
			out = append(out, object)
		}
	}
	return out
}

func objectValue(value any) map[string]any {
	object, _ := value.(map[string]any)
	if object == nil {
		return map[string]any{}
	}
	return object
}

func stringArrayValue(value any) []string {
	items := arrayValue(value)
	out := make([]string, 0, len(items))
	for _, item := range items {
		text := stringValue(item)
		if text != "" {
			out = append(out, text)
		}
	}
	return out
}

func floatArrayValue(value any) []float64 {
	items := arrayValue(value)
	out := make([]float64, 0, len(items))
	for _, item := range items {
		out = append(out, clampFloat(floatValue(item), 0, 1))
	}
	return out
}

func intArrayValue(value any) []int {
	items := arrayValue(value)
	out := make([]int, 0, len(items))
	for _, item := range items {
		out = append(out, clampInt(intValue(item), 0, 100))
	}
	return out
}

func stringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		number, _ := typed.Int64()
		return int(number)
	case string:
		number, _ := strconv.Atoi(strings.TrimSpace(typed))
		return number
	default:
		return 0
	}
}

func floatValue(value any) float64 {
	switch typed := value.(type) {
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case float64:
		return typed
	case json.Number:
		number, _ := typed.Float64()
		return number
	case string:
		number, _ := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return number
	default:
		return 0
	}
}

func boolValue(value any) bool {
	typed, _ := value.(bool)
	return typed
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(text, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func normalizeEvidenceScope(value string, kind string, name string, result string, experienceType string, role string, contribution string) string {
	value = strings.TrimSpace(value)
	if value == "校内" || value == "校外" {
		return value
	}
	text := value + name + result + experienceType + role + contribution
	lowered := strings.ToLower(text)
	if containsAny(lowered, "校外", "全国", "国家级", "省级", "省", "市级", "市", "区域", "赛区", "国际", "企业", "公司", "集团", "实习", "英语四级", "英语六级", "cet", "计算机等级", "蓝桥", "acm", "icpc", "ctf", "挑战杯", "互联网+") {
		return "校外"
	}
	if containsAny(lowered, "校内", "校级", "院级", "学院", "学校", "校学生会", "院学生会", "学生会", "社团", "协会", "班级", "班长", "团支书", "优秀学生", "优秀学生干部", "三好学生", "奖学金", "实验室", "大学生创新创业训练计划", "大创") {
		return "校内"
	}
	if kind == "experience" && containsAny(lowered, "项目", "科研", "研究") {
		return "校内"
	}
	return "校外"
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func clampInt(value int, minValue int, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func clampFloat(value float64, minValue float64, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
