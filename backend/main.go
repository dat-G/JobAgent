package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxUploadBytes = 32 << 20

type Server struct {
	frontendDir string
	prestoURL   *url.URL
	jobs        *JobStore
}

type DiagnosisRequest struct {
	Files []SourceFile `json:"files,omitempty"`
}

type SourceFile struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type Diagnosis struct {
	GeneratedAt           string               `json:"generated_at"`
	Mode                  string               `json:"mode"`
	InputSources          []SourceFile         `json:"input_sources"`
	AbilityProfile        AbilityProfile       `json:"ability_profile"`
	PathPlan              PathPlan             `json:"path_plan"`
	MatchingResult        MatchingResult       `json:"matching_result"`
	BackendRequirements   []BackendRequirement `json:"backend_requirements"`
	ProductionLimitations []string             `json:"production_limitations"`
}

type AbilityProfile struct {
	BasicInfo           BasicInfo        `json:"basic_info"`
	Education           []EducationItem  `json:"education"`
	RadarData           []ScoreDimension `json:"radar_data"`
	RadarSeries         []RadarSeries    `json:"radar_series,omitempty"`
	EvidenceSummary     []EvidenceItem   `json:"evidence_summary"`
	AwardsStatus        string           `json:"awards_status"`
	Awards              []AwardItem      `json:"awards"`
	ExperiencesStatus   string           `json:"experiences_status"`
	Experiences         []ExperienceItem `json:"experiences"`
	BenchmarkStatus     string           `json:"benchmark_status"`
	MajorBaselineStatus string           `json:"major_baseline_status"`
	MajorBaseline       MajorBaseline    `json:"major_baseline"`
	TopJobs             []MatchedJob     `json:"top5_matching_jobs"`
}

type BasicInfo struct {
	Name           string `json:"name"`
	Sex            string `json:"sex"`
	BirthYear      string `json:"birth_year"`
	School         string `json:"school"`
	Major          string `json:"major"`
	Degree         string `json:"degree"`
	GraduationYear string `json:"graduation_year"`
	TargetRole     string `json:"target_role"`
	ResumeStatus   string `json:"resume_status"`
	TranscriptUse  string `json:"transcript_use"`
}

type EducationItem struct {
	School             string `json:"school"`
	Degree             string `json:"degree"`
	Department         string `json:"department"`
	Major              string `json:"major"`
	Is985              bool   `json:"is_985"`
	Is211              bool   `json:"is_211"`
	IsDoubleFirstClass bool   `json:"is_double_first_class"`
	RuankeRank         int    `json:"ruanke_rank"`
	SchoolKind         string `json:"school_kind,omitempty"`
	ParentSchool       string `json:"parent_school,omitempty"`
}

type ScoreDimension struct {
	Name     string `json:"name"`
	Score    int    `json:"score"`
	MaxScore int    `json:"max_score"`
	Level    string `json:"level"`
	Reason   string `json:"reason"`
}

type RadarSeries struct {
	Key    string           `json:"key"`
	Label  string           `json:"label"`
	Count  int              `json:"count"`
	Source string           `json:"source,omitempty"`
	Scores []ScoreDimension `json:"scores"`
}

type EvidenceItem struct {
	Category string `json:"category"`
	Summary  string `json:"summary"`
	Signal   string `json:"signal"`
}

type AwardItem struct {
	Name                string    `json:"name"`
	Result              string    `json:"result"`
	EvidenceScope       string    `json:"evidence_scope"`
	Score               int       `json:"score,omitempty"`
	Level               float64   `json:"level,omitempty"`
	ImpactFactor        *float64  `json:"impact_factor,omitempty"`
	BenchmarkDimensions []string  `json:"benchmark_dimensions,omitempty"`
	BenchmarkScores     []float64 `json:"benchmark_scores,omitempty"`
	Signal              string    `json:"signal,omitempty"`
	Reason              string    `json:"reason"`
	DataSource          string    `json:"data_source"`
	ScoreSource         string    `json:"score_source"`
}

type ExperienceItem struct {
	Type                string    `json:"type"`
	Role                string    `json:"role"`
	Contribution        string    `json:"contribution"`
	EvidenceScope       string    `json:"evidence_scope"`
	Level               int       `json:"level"`
	Score               int       `json:"score,omitempty"`
	ImpactFactor        *float64  `json:"impact_factor,omitempty"`
	BenchmarkDimensions []string  `json:"benchmark_dimensions,omitempty"`
	BenchmarkScores     []float64 `json:"benchmark_scores,omitempty"`
	Signal              string    `json:"signal"`
	Reason              string    `json:"reason"`
	DataSource          string    `json:"data_source"`
	ScoreSource         string    `json:"score_source"`
	HybridStatus        string    `json:"hybrid_status,omitempty"`
}

type MajorBaseline struct {
	MajorName   string   `json:"major_name"`
	MajorFamily string   `json:"major_family"`
	BaseScore   int      `json:"base_score"`
	Dimensions  []string `json:"dimensions,omitempty"`
	Scores      []int    `json:"scores,omitempty"`
	Rationale   string   `json:"rationale,omitempty"`
	Confidence  float64  `json:"confidence,omitempty"`
	Source      string   `json:"source,omitempty"`
}

type MatchedJob struct {
	Rank                int              `json:"rank"`
	Title               string           `json:"title"`
	Category            string           `json:"category,omitempty"`
	Match               int              `json:"match"`
	AbilityMatch        int              `json:"ability_match,omitempty"`
	ExperienceMatch     int              `json:"experience_match,omitempty"`
	EducationGate       string           `json:"education_gate,omitempty"`
	EducationGateStatus string           `json:"education_gate_status,omitempty"`
	EvidenceStrength    string           `json:"evidence_strength,omitempty"`
	FitSummary          string           `json:"fit_summary,omitempty"`
	Risk                string           `json:"risk,omitempty"`
	RequirementRadar    []ScoreDimension `json:"requirement_radar,omitempty"`
	Reasons             []string         `json:"reasons"`
	ProofGaps           []string         `json:"proof_gaps,omitempty"`
	NextProof           string           `json:"next_proof"`
}

type PathPlan struct {
	ExportFormats []string    `json:"export_formats"`
	Stages        []PlanStage `json:"stages"`
}

type PlanStage struct {
	Stage       string       `json:"stage"`
	Goal        string       `json:"goal"`
	Weeks       []WeeklyTask `json:"weeks"`
	Resources   []Resource   `json:"resources"`
	Standards   []string     `json:"standards"`
	Deliverable string       `json:"deliverable"`
}

type WeeklyTask struct {
	Week     string `json:"week"`
	Task     string `json:"task"`
	Metric   string `json:"metric"`
	Priority string `json:"priority"`
}

type Resource struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type MatchingResult struct {
	TargetRole         string              `json:"target_role"`
	OverallMatch       int                 `json:"overall_match"`
	MatchLevel         string              `json:"match_level"`
	Source             string              `json:"source,omitempty"`
	MethodSummary      string              `json:"method_summary,omitempty"`
	FitSummary         string              `json:"fit_summary,omitempty"`
	SelectedJob        MatchedJob          `json:"selected_job,omitempty"`
	StudentRadar       []ScoreDimension    `json:"student_radar,omitempty"`
	TargetRadar        []ScoreDimension    `json:"target_radar,omitempty"`
	ReportSections     []ReportRow         `json:"report_sections"`
	GapDetails         []GapDetail         `json:"gap_details"`
	DevelopmentActions []DevelopmentAction `json:"development_actions,omitempty"`
	Recommendations    []string            `json:"recommendations"`
	Reasons            []string            `json:"recommended_reasons"`
	AgentNotes         []string            `json:"agent_notes,omitempty"`
}

type DevelopmentAction struct {
	Priority    string `json:"priority"`
	Scope       string `json:"scope"`
	Description string `json:"description"`
}

type ReportRow struct {
	Name       string `json:"name"`
	Student    int    `json:"student"`
	RoleNeed   int    `json:"role_need"`
	Difference int    `json:"difference"`
	Status     string `json:"status,omitempty"`
	Note       string `json:"note,omitempty"`
}

type GapDetail struct {
	Capability string `json:"capability"`
	Current    string `json:"current"`
	Expected   string `json:"expected"`
	Action     string `json:"action"`
	Severity   string `json:"severity"`
}

type BackendRequirement struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Status   string   `json:"status"`
	Priority string   `json:"priority"`
	Details  []string `json:"details"`
}

func main() {
	addr := envDefault("JOBAGENT_ADDR", "127.0.0.1:8090")
	server := Server{
		frontendDir: frontendDir(),
		prestoURL:   parseOptionalURL(os.Getenv("PRESTO_URL")),
		jobs:        NewJobStore(),
	}

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           server.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("jobagent frontend/backend listening on http://%s", addr)
	log.Printf("serving frontend from %s", server.frontendDir)
	if server.prestoURL == nil {
		log.Printf("PRESTO_URL is not set, /api/presto/* returns 503")
	}
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server failed: %v", err)
	}
}

func (s Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/healthz", s.handleHealthz)
	mux.HandleFunc("/api/diagnosis/mock", s.handleMockDiagnosis)
	mux.HandleFunc("/api/diagnosis", s.handleDiagnosis)
	mux.HandleFunc("/api/diagnosis/", s.handleDiagnosisJob)
	mux.HandleFunc("/api/chat", s.handleChat)
	mux.HandleFunc("/api/export/ability-profile.json", s.handleAbilityProfileJSON)
	mux.HandleFunc("/api/export/ability-profile.xlsx", s.handleAbilityProfileXLSX)
	mux.HandleFunc("/api/export/path-plan.doc", s.handlePathPlanDoc)
	mux.HandleFunc("/api/export/backend-requirements.json", s.handleBackendRequirementsJSON)
	mux.HandleFunc("/api/presto/", s.handlePrestoProxy)
	mux.HandleFunc("/", s.handleStatic)
	return secureHeaders(mux)
}

func (s Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"mode":   "legato-required",
	})
}

func (s Server) handleMockDiagnosis(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	writeJSON(w, http.StatusOK, mockDiagnosis(DiagnosisRequest{}))
}

func (s Server) handleDiagnosis(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	s.startDiagnosisJob(w, r)
}

func (s Server) handleAbilityProfileJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	data, err := json.MarshalIndent(mockDiagnosis(DiagnosisRequest{}).AbilityProfile, "", "  ")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "JSON 导出失败")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="ability-profile.json"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s Server) handleAbilityProfileXLSX(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	data, err := buildAbilityProfileXLSX(mockDiagnosis(DiagnosisRequest{}).AbilityProfile)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Excel 导出失败")
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="ability-profile.xlsx"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s Server) handlePathPlanDoc(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	doc := buildPathPlanDoc(mockDiagnosis(DiagnosisRequest{}).PathPlan)
	w.Header().Set("Content-Type", "application/msword; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="growth-path-plan.doc"`)
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, doc)
}

func (s Server) handleBackendRequirementsJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requirements": backendRequirements()})
}

func (s Server) handlePrestoProxy(w http.ResponseWriter, r *http.Request) {
	if s.prestoURL == nil {
		writeError(w, http.StatusServiceUnavailable, "PRESTO_URL 未配置，当前诊断要求 Legato 后端解析")
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(s.prestoURL)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.URL.Path = "/" + strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/api/presto"), "/")
		if req.URL.Path == "/" {
			req.URL.Path = "/healthz"
		}
		req.Host = s.prestoURL.Host
	}
	proxy.ServeHTTP(w, r)
}

func (s Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, http.MethodGet, http.MethodHead)
		return
	}

	cleanPath := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	if cleanPath == "." {
		cleanPath = "index.html"
	}
	if !safeStaticPath(cleanPath) {
		writeError(w, http.StatusBadRequest, "invalid static path")
		return
	}
	target := filepath.Join(s.frontendDir, cleanPath)
	if info, err := os.Stat(target); err == nil && !info.IsDir() {
		http.ServeFile(w, r, target)
		return
	}
	http.ServeFile(w, r, filepath.Join(s.frontendDir, "index.html"))
}

func filesFromForm(r *http.Request, formKey string, kind string) []SourceFile {
	if r.MultipartForm == nil || r.MultipartForm.File == nil {
		return nil
	}
	headers := r.MultipartForm.File[formKey]
	files := make([]SourceFile, 0, len(headers))
	for _, header := range headers {
		files = append(files, SourceFile{
			Kind: kind,
			Name: header.Filename,
			Size: header.Size,
		})
	}
	return files
}

func benchmarkDimensionNames() []string {
	return []string{"逻辑", "语言", "专业", "领导", "抗压", "成长"}
}

func floatPtr(value float64) *float64 {
	return &value
}

func mockDiagnosis(req DiagnosisRequest) Diagnosis {
	recommendedRole := "前端工程师"
	files := req.Files
	if len(files) == 0 {
		files = []SourceFile{
			{Kind: "resume", Name: "resume-demo.pdf", Size: 418_320},
			{Kind: "transcript", Name: "transcript-demo.pdf", Size: 712_904},
		}
	}

	return Diagnosis{
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		Mode:         "mock",
		InputSources: files,
		AbilityProfile: AbilityProfile{
			BasicInfo: BasicInfo{
				Name:           "陈曦",
				Sex:            "男",
				BirthYear:      "2002",
				School:         "东北农业大学",
				Major:          "计算机科学与技术",
				Degree:         "本科",
				GraduationYear: "2026",
				TargetRole:     recommendedRole,
				ResumeStatus:   "模拟数据：未调用 Legato 解析",
				TranscriptUse:  "可选材料，当前用于补充课程和 GPA 线索",
			},
			Education: []EducationItem{
				{
					School:             "东北农业大学",
					Degree:             "本科",
					Department:         "电气与信息学院",
					Major:              "计算机科学与技术",
					Is985:              false,
					Is211:              true,
					IsDoubleFirstClass: true,
					RuankeRank:         120,
				},
			},
			RadarData: []ScoreDimension{
				{Name: "逻辑", Score: 74, MaxScore: 100, Level: "良好", Reason: "建模、问题拆解和工程分析证据较完整"},
				{Name: "语言", Score: 64, MaxScore: 100, Level: "中等", Reason: "英语证书和项目表达有基础，岗位化表达仍需补强"},
				{Name: "专业", Score: 80, MaxScore: 100, Level: "强", Reason: "计算机专业背景和前端项目证据支撑专业维度"},
				{Name: "领导", Score: 62, MaxScore: 100, Level: "中等", Reason: "有组织协作线索，但缺少明确 ownership 和团队结果"},
				{Name: "抗压", Score: 68, MaxScore: 100, Level: "良好", Reason: "竞赛和交付经历能体现约束下完成任务"},
				{Name: "成长", Score: 76, MaxScore: 100, Level: "良好", Reason: "项目迭代和跨任务学习证据较强"},
			},
			EvidenceSummary: []EvidenceItem{
				{Category: "教育背景", Summary: "计算机科学与技术本科，学校具备 211 与双一流标签", Signal: "岗位基础匹配"},
				{Category: "项目经历", Summary: "视频云平台监控前台系统开发，能支撑前端工程方向", Signal: "专业维度强"},
				{Category: "竞赛证书", Summary: "英语六级、数学建模竞赛奖项可补充学习能力证据", Signal: "综合能力良好"},
				{Category: "待补证据", Summary: "性能优化、测试覆盖、组件化设计、线上问题处理证据不足", Signal: "工程深度待补"},
			},
			AwardsStatus:        "mock",
			BenchmarkStatus:     "mock",
			MajorBaselineStatus: "mock",
			MajorBaseline: MajorBaseline{
				MajorName:   "计算机科学与技术",
				MajorFamily: "工科类",
				BaseScore:   51,
				Dimensions:  benchmarkDimensionNames(),
				Scores:      []int{56, 46, 59, 42, 49, 53},
				Rationale:   "按工科类专业、211/双一流/软科#120学校层次给出能力prior。",
				Confidence:  0.68,
				Source:      "mock_major_baseline",
			},
			Awards: []AwardItem{
				{
					Name:                "2023 年全国大学生数学建模竞赛黑龙江赛区",
					Result:              "一等奖",
					EvidenceScope:       "校外",
					Level:               7,
					ImpactFactor:        floatPtr(6.8),
					BenchmarkDimensions: benchmarkDimensionNames(),
					BenchmarkScores:     []float64{0.28, 0.08, 0.22, 0.12, 0.18, 0.12},
					Signal:              "较强证据",
					Reason:              "省级正式竞赛一等奖，能补充建模、协作和问题拆解能力，但仍缺少个人贡献描述。",
					DataSource:          "模拟数据",
					ScoreSource:         "模拟评分",
				},
				{
					Name:                "全国大学英语六级考试",
					Result:              "567 分",
					EvidenceScope:       "校外",
					Level:               3,
					ImpactFactor:        floatPtr(2.8),
					BenchmarkDimensions: benchmarkDimensionNames(),
					BenchmarkScores:     []float64{0.06, 0.58, 0.04, 0.04, 0.10, 0.18},
					Signal:              "基础证据",
					Reason:              "基础证书可证明英语能力，按 Legato 评分约定不作为高价值能力证据。",
					DataSource:          "模拟数据",
					ScoreSource:         "模拟评分",
				},
				{
					Name:                "校级优秀学生干部",
					Result:              "荣誉称号",
					EvidenceScope:       "校内",
					Level:               2,
					ImpactFactor:        floatPtr(2.0),
					BenchmarkDimensions: benchmarkDimensionNames(),
					BenchmarkScores:     []float64{0.10, 0.15, 0.05, 0.30, 0.15, 0.25},
					Signal:              "弱证据",
					Reason:              "泛荣誉可作综合素质补充，但缺少具体任务、动作和结果。",
					DataSource:          "模拟数据",
					ScoreSource:         "模拟评分",
				},
			},
			ExperiencesStatus: "mock",
			Experiences: []ExperienceItem{
				{
					Type:                "实习",
					Role:                "前端开发实习生",
					Contribution:        "参与视频云平台监控前台系统开发，负责页面模块实现和联调。",
					EvidenceScope:       "校外",
					Level:               7,
					ImpactFactor:        floatPtr(6.2),
					BenchmarkDimensions: benchmarkDimensionNames(),
					BenchmarkScores:     []float64{0.16, 0.09, 0.34, 0.10, 0.19, 0.12},
					Signal:              "强证据",
					Reason:              "有岗位相关技术贡献和交付场景，仍需要补充量化结果与个人 ownership。",
					DataSource:          "模拟数据",
					ScoreSource:         "模拟评分",
				},
				{
					Type:                "项目",
					Role:                "前端负责人",
					Contribution:        "完成作品集项目的组件拆分、状态管理和部署说明。",
					EvidenceScope:       "校内",
					Level:               6,
					ImpactFactor:        floatPtr(5.6),
					BenchmarkDimensions: benchmarkDimensionNames(),
					BenchmarkScores:     []float64{0.20, 0.08, 0.36, 0.12, 0.12, 0.12},
					Signal:              "有效证据",
					Reason:              "项目相关性较高，若补齐线上地址、测试和性能数据，可提升证据强度。",
					DataSource:          "模拟数据",
					ScoreSource:         "模拟评分",
				},
				{
					Type:                "校园",
					Role:                "社团活动组织者",
					Contribution:        "组织校园活动并协调团队分工。",
					EvidenceScope:       "校内",
					Level:               4,
					ImpactFactor:        floatPtr(3.2),
					BenchmarkDimensions: benchmarkDimensionNames(),
					BenchmarkScores:     []float64{0.08, 0.22, 0.05, 0.34, 0.13, 0.18},
					Signal:              "普通经历",
					Reason:              "体现协作和执行，但与推荐岗位能力要求的直接关系较弱。",
					DataSource:          "模拟数据",
					ScoreSource:         "模拟评分",
				},
			},
			TopJobs: []MatchedJob{
				{Rank: 1, Title: "前端工程师", Category: "本专业相关", Match: 82, AbilityMatch: 84, ExperienceMatch: 78, EducationGate: "通过", FitSummary: "专业和项目都能支撑前端方向，工程化证据需要补齐。", RequirementRadar: []ScoreDimension{{Name: "逻辑", Score: 78}, {Name: "语言", Score: 60}, {Name: "专业", Score: 86}, {Name: "领导", Score: 62}, {Name: "抗压", Score: 72}, {Name: "成长", Score: 78}}, Reasons: []string{"项目经验直接相关", "专业背景匹配", "可通过作品集快速补强"}, NextProof: "补充组件库、性能优化和测试覆盖案例"},
				{Rank: 2, Title: "Web 全栈开发", Category: "本专业扩展", Match: 76, AbilityMatch: 77, ExperienceMatch: 70, EducationGate: "通过", FitSummary: "前端能力可迁移，但后端接口和数据库证据偏少。", RequirementRadar: []ScoreDimension{{Name: "逻辑", Score: 80}, {Name: "语言", Score: 58}, {Name: "专业", Score: 82}, {Name: "领导", Score: 60}, {Name: "抗压", Score: 74}, {Name: "成长", Score: 76}}, Reasons: []string{"前端基础较强", "需要后端接口与数据库证据"}, NextProof: "完成一个含鉴权、接口、部署的全栈项目"},
				{Rank: 3, Title: "低代码平台开发", Category: "本专业扩展", Match: 73, AbilityMatch: 75, ExperienceMatch: 68, EducationGate: "通过", FitSummary: "组件抽象和交互经验相关，配置化产品证据不足。", RequirementRadar: []ScoreDimension{{Name: "逻辑", Score: 76}, {Name: "语言", Score: 62}, {Name: "专业", Score: 80}, {Name: "领导", Score: 66}, {Name: "抗压", Score: 70}, {Name: "成长", Score: 76}}, Reasons: []string{"前端交互经验相关", "需要配置化和组件抽象证据"}, NextProof: "做一个表单编排或流程设计器 Demo"},
				{Rank: 4, Title: "测试开发工程师", Category: "跨方向可迁移", Match: 68, AbilityMatch: 70, ExperienceMatch: 58, EducationGate: "通过", FitSummary: "工程基础可迁移，测试工具链和质量体系证据不足。", RequirementRadar: []ScoreDimension{{Name: "逻辑", Score: 82}, {Name: "语言", Score: 56}, {Name: "专业", Score: 76}, {Name: "领导", Score: 54}, {Name: "抗压", Score: 76}, {Name: "成长", Score: 72}}, Reasons: []string{"工程基础可迁移", "测试工具链证据不足"}, NextProof: "补充 Playwright、接口测试和 CI 报告"},
				{Rank: 5, Title: "数据可视化工程师", Category: "跨方向可迁移", Match: 66, AbilityMatch: 68, ExperienceMatch: 56, EducationGate: "通过", FitSummary: "前端方向可延展，但图表与数据处理经历不足。", RequirementRadar: []ScoreDimension{{Name: "逻辑", Score: 80}, {Name: "语言", Score: 60}, {Name: "专业", Score: 78}, {Name: "领导", Score: 55}, {Name: "抗压", Score: 70}, {Name: "成长", Score: 74}}, Reasons: []string{"前端方向可延展", "图表与数据处理经历不足"}, NextProof: "构建一个含 ECharts/D3 的指标分析项目"},
			},
		},
		PathPlan: PathPlan{
			ExportFormats: []string{"PDF", "Word"},
			Stages: []PlanStage{
				{
					Stage: "第 1 阶段，0 到 30 天",
					Goal:  "把简历证据转成可验证作品集，补齐前端岗位基础题和项目表达",
					Weeks: []WeeklyTask{
						{Week: "第 1 周", Task: "梳理推荐岗位能力要求，抽取 20 条高频能力项并映射到个人经历", Metric: "形成岗位能力矩阵 1 份", Priority: "高"},
						{Week: "第 2 周", Task: "重构一个已有前端项目，补充 README、架构图、核心截图和部署地址", Metric: "作品集项目上线", Priority: "高"},
						{Week: "第 3 周", Task: "完成 HTML/CSS/JS/浏览器网络基础专项复盘", Metric: "完成 60 道岗位题", Priority: "中"},
						{Week: "第 4 周", Task: "按 STAR 结构重写项目经历和实习经历", Metric: "简历一页版定稿", Priority: "高"},
					},
					Resources: []Resource{
						{Label: "MDN Web Docs", URL: "https://developer.mozilla.org/"},
						{Label: "web.dev 性能指南", URL: "https://web.dev/learn/performance/"},
					},
					Standards:   []string{"简历项目描述包含背景、动作、结果和技术取舍", "作品集能在 3 分钟内被面试官理解", "基础题正确率达到 80%"},
					Deliverable: "岗位能力矩阵、作品集链接、一页简历",
				},
				{
					Stage: "第 2 阶段，31 到 60 天",
					Goal:  "强化工程交付能力，补充测试、性能和组件化证据",
					Weeks: []WeeklyTask{
						{Week: "第 5 周", Task: "为作品集增加单元测试和关键交互端到端测试", Metric: "核心路径 E2E 覆盖 3 条", Priority: "高"},
						{Week: "第 6 周", Task: "做一次性能分析并优化首屏、资源体积和交互延迟", Metric: "Lighthouse Performance 达到 90+", Priority: "高"},
						{Week: "第 7 周", Task: "抽象 6 个可复用组件并补充状态说明", Metric: "组件文档 1 份", Priority: "中"},
						{Week: "第 8 周", Task: "模拟一轮技术面试并复盘薄弱题型", Metric: "输出复盘清单 1 份", Priority: "中"},
					},
					Resources: []Resource{
						{Label: "Playwright 文档", URL: "https://playwright.dev/"},
						{Label: "Chrome DevTools 性能文档", URL: "https://developer.chrome.com/docs/devtools/performance/"},
					},
					Standards:   []string{"能解释性能瓶颈和优化依据", "测试报告可展示", "组件状态覆盖 default、hover、disabled、error"},
					Deliverable: "测试报告、性能对比、组件说明",
				},
				{
					Stage: "第 3 阶段，61 到 90 天",
					Goal:  "进入投递节奏，围绕目标公司和岗位持续迭代材料",
					Weeks: []WeeklyTask{
						{Week: "第 9 周", Task: "建立 30 家公司投递列表，按岗位要求分层", Metric: "A/B/C 岗位池完成", Priority: "高"},
						{Week: "第 10 周", Task: "针对 10 个岗位改写简历关键词和项目排序", Metric: "定制简历 10 份", Priority: "高"},
						{Week: "第 11 周", Task: "进行 2 次技术模拟面试和 1 次 HR 模拟面试", Metric: "面试反馈记录 3 份", Priority: "中"},
						{Week: "第 12 周", Task: "根据反馈回补项目细节、题库和投递策略", Metric: "进入稳定投递节奏", Priority: "高"},
					},
					Resources: []Resource{
						{Label: "LeetCode", URL: "https://leetcode.cn/"},
						{Label: "牛客求职题库", URL: "https://www.nowcoder.com/"},
					},
					Standards:   []string{"每个岗位有对应推荐理由和风险点", "投递记录可追踪", "面试问题有复盘闭环"},
					Deliverable: "岗位池、定制简历、面试复盘",
				},
			},
		},
		MatchingResult: MatchingResult{
			TargetRole:    recommendedRole,
			OverallMatch:  82,
			MatchLevel:    "高潜力匹配",
			Source:        "mock_job_matching",
			MethodSummary: "模拟 Agent Team 按六维能力、经历相关性和学历门槛进行排序。",
			FitSummary:    "前端工程师与学生现有专业背景和项目证据最贴近，短板集中在工程化测试、性能优化和岗位表达。",
			SelectedJob: MatchedJob{
				Rank:             1,
				Title:            recommendedRole,
				Category:         "本专业相关",
				Match:            82,
				AbilityMatch:     84,
				ExperienceMatch:  78,
				EducationGate:    "通过",
				FitSummary:       "专业和项目都能支撑前端方向，工程化证据需要补齐。",
				RequirementRadar: []ScoreDimension{{Name: "逻辑", Score: 78}, {Name: "语言", Score: 60}, {Name: "专业", Score: 86}, {Name: "领导", Score: 62}, {Name: "抗压", Score: 72}, {Name: "成长", Score: 78}},
				Reasons:          []string{"项目经验直接相关", "专业背景匹配", "可通过作品集快速补强"},
				NextProof:        "补充组件库、性能优化和测试覆盖案例",
			},
			StudentRadar: []ScoreDimension{
				{Name: "逻辑", Score: 74},
				{Name: "语言", Score: 64},
				{Name: "专业", Score: 80},
				{Name: "领导", Score: 62},
				{Name: "抗压", Score: 68},
				{Name: "成长", Score: 76},
			},
			TargetRadar: []ScoreDimension{{Name: "逻辑", Score: 78}, {Name: "语言", Score: 60}, {Name: "专业", Score: 86}, {Name: "领导", Score: 62}, {Name: "抗压", Score: 72}, {Name: "成长", Score: 78}},
			ReportSections: []ReportRow{
				{Name: "逻辑", Student: 74, RoleNeed: 78, Difference: -4},
				{Name: "语言", Student: 64, RoleNeed: 60, Difference: 4},
				{Name: "专业", Student: 80, RoleNeed: 86, Difference: -6},
				{Name: "领导", Student: 62, RoleNeed: 62, Difference: 0},
				{Name: "抗压", Student: 68, RoleNeed: 72, Difference: -4},
				{Name: "成长", Student: 76, RoleNeed: 78, Difference: -2},
			},
			GapDetails: []GapDetail{
				{Capability: "工程化测试", Current: "简历未体现测试覆盖", Expected: "能展示单测、E2E 或 CI 证据", Action: "为作品集补 Playwright 测试和截图报告", Severity: "高"},
				{Capability: "性能优化", Current: "缺少量化指标", Expected: "能说明首屏、包体积、交互延迟优化", Action: "完成一次 Lighthouse 优化对比", Severity: "高"},
				{Capability: "岗位表达", Current: "项目贡献较泛", Expected: "能对应推荐岗位要求说明负责模块和结果", Action: "按 STAR 改写项目经历", Severity: "中"},
				{Capability: "算法基础", Current: "证据偏弱", Expected: "通过高频题训练支撑校招面试", Action: "完成数组、字符串、树、动态规划专项", Severity: "中"},
			},
			Recommendations: []string{
				"优先投递前端工程师和低代码平台开发岗位，把作品集作为核心证据。",
				"暂缓投递算法、数据工程等强算法岗位，除非先补充题库训练和项目证据。",
				"简历第一屏应突出项目上线、性能优化、组件化和实习贡献。",
			},
			DevelopmentActions: []DevelopmentAction{
				{Priority: "高", Scope: "校外", Description: "将作品集项目整理为 GitHub 仓库，补充性能优化、测试覆盖和部署说明。"},
				{Priority: "高", Scope: "校外", Description: "补充一个可运行的组件库或低代码 Demo，输出在线预览和技术复盘。"},
				{Priority: "中", Scope: "校内", Description: "在课程项目或实验室项目中主动承担模块 owner，量化分工、进度和交付结果。"},
				{Priority: "中", Scope: "校外", Description: "按目标岗位改写简历项目描述，突出问题、行动、结果和可验证指标。"},
			},
			Reasons: []string{
				"专业背景与推荐岗位基础要求一致。",
				"项目和实习经历能支撑前端方向。",
				"短板集中在可通过 60 到 90 天任务补齐的工程证据。",
			},
			AgentNotes: []string{"六维能力优先", "经历证据第二", "学历作为门槛而非单点排序依据"},
		},
		BackendRequirements: backendRequirements(),
		ProductionLimitations: []string{
			"当前 /api/diagnosis/mock 返回的是样例数据，正式诊断不使用该 mock 回退。",
			"当前文件上传只记录文件元信息，未解析真实简历和成绩单内容。",
			"PDF 导出由前端打印样式完成，生产环境建议改为后端模板化 PDF 服务。",
		},
	}
}

func backendRequirements() []BackendRequirement {
	return []BackendRequirement{
		{ID: "BR-01", Title: "材料上传与对象存储", Status: "not_started", Priority: "P0", Details: []string{"支持简历必传、成绩单可选、其他材料可选", "返回文件 ID、解析状态、可追踪错误", "限制文件类型、大小和病毒扫描策略"}},
		{ID: "BR-02", Title: "简历结构化解析 API", Status: "partial_in_legato_cli", Priority: "P0", Details: []string{"把 Legato 简历 workflow 暴露为 HTTP API", "输出基础信息、教育、证书奖项、经历和置信度", "支持 PDF、DOCX、Markdown、图片 OCR 回退"}},
		{ID: "BR-03", Title: "成绩单解析与课程能力映射", Status: "partial_ocr_blocked", Priority: "P0", Details: []string{"解析课程、学期、成绩、GPA 和专业课程分类", "将课程映射到岗位能力维度", "扫描版成绩单需要稳定 OCR 服务"}},
		{ID: "BR-04", Title: "岗位能力模型与 JD 解析", Status: "partial_in_legato_presto_team", Priority: "P0", Details: []string{"已新增 Adaptive Planner 动态派生多视角 Presto Agent", "后端会校验 Planner 输出并限制 3 到 500 个 Agent、最多 500 个并发 run", "每个 Presto run 的事件流会转发到前端 chat 状态卡", "当前基于简历证据、六维能力和学历门槛推断岗位", "后续仍需接入真实岗位库、JD 数据源和地区过滤", "后续可扩展为粘贴 JD 的定向分析模式"}},
		{ID: "BR-05", Title: "能力评分与雷达数据引擎", Status: "ready_in_backend", Priority: "P0", Details: []string{"Item Benchmark 生成证据级六维分布和 Impact", "Major Baseline 生成专业六维 prior", "Go 后端统一聚合学生最终六维画像与雷达 series", "Job Matching 使用同一画像生成岗位目标雷达和差距"}},
		{ID: "BR-06", Title: "成长路径规划生成", Status: "partial_in_presto_team", Priority: "P1", Details: []string{"Job Matching 完成后启动 Legato Path Planning Team", "Path Planner 动态派生路径规划 Agent", "输出阶段目标、周任务、资源链接和达标标准", "根据学生短板和岗位权重调整优先级", "后续支持任务完成状态和再规划"}},
		{ID: "BR-07", Title: "结构化导出服务", Status: "mock_in_gateway", Priority: "P1", Details: []string{"能力画像导出 JSON 和 Excel", "路径规划导出 PDF 和 Word", "匹配结果导出可视化报表"}},
		{ID: "BR-08", Title: "异步任务与 SSE 事件契约", Status: "partial_in_presto", Priority: "P1", Details: []string{"统一上传、解析、评分、生成报告的 run 状态", "提供可恢复的事件流和错误码", "支持长任务超时与重试"}},
		{ID: "BR-09", Title: "用户数据安全与权限", Status: "not_started", Priority: "P0", Details: []string{"学生材料包含隐私数据，需要认证、授权和脱敏日志", "支持文件删除、数据保留周期和审计记录", "导出文件需要访问控制"}},
		{ID: "BR-10", Title: "岗位推荐数据源", Status: "not_started", Priority: "P2", Details: []string{"接入岗位库或招聘平台数据", "按地区、学历、技能和意向过滤", "推荐理由需要可解释"}},
	}
}

func buildAbilityProfileXLSX(profile AbilityProfile) ([]byte, error) {
	title := "能力画像"
	if profile.BasicInfo.Name != "" {
		title += " · " + profile.BasicInfo.Name
	}
	if profile.BasicInfo.TargetRole != "" {
		title += " · 推荐岗位：" + profile.BasicInfo.TargetRole
	}
	rows := [][]string{
		{title},
		{"姓名", profile.BasicInfo.Name},
		{"性别", profile.BasicInfo.Sex},
		{"出生年份", profile.BasicInfo.BirthYear},
		{"学校", profile.BasicInfo.School},
		{"专业", profile.BasicInfo.Major},
		{"学历", profile.BasicInfo.Degree},
		{"毕业年份", profile.BasicInfo.GraduationYear},
		{"推荐首选岗位", profile.BasicInfo.TargetRole},
		{},
		{"教育经历", "学院", "专业", "学历", "985", "211", "双一流", "软科排名", "学校类型"},
	}
	for _, item := range profile.Education {
		rows = append(rows, []string{
			item.School,
			item.Department,
			item.Major,
			item.Degree,
			boolLabel(item.Is985),
			boolLabel(item.Is211),
			boolLabel(item.IsDoubleFirstClass),
			rankLabel(item.RuankeRank),
			schoolKindLabel(item),
		})
	}
	if len(profile.MajorBaseline.Scores) > 0 {
		rows = append(rows,
			[]string{},
			[]string{"专业六维基线", "专业族群", "基础分", "六维基线", "说明", "来源"},
			[]string{
				profile.MajorBaseline.MajorName,
				profile.MajorBaseline.MajorFamily,
				fmt.Sprintf("%d", profile.MajorBaseline.BaseScore),
				formatIntDistribution(profile.MajorBaseline.Dimensions, profile.MajorBaseline.Scores),
				profile.MajorBaseline.Rationale,
				profile.MajorBaseline.Source,
			},
		)
	}
	rows = append(rows, []string{}, []string{"六维指标", "分数", "等级", "说明"})
	for _, item := range profile.RadarData {
		rows = append(rows, []string{item.Name, fmt.Sprintf("%d/%d", item.Score, item.MaxScore), item.Level, item.Reason})
	}
	rows = append(rows, []string{}, []string{"奖项与证书", "结果", "分类", "Level", "Impact", "六维分布", "评分说明", "来源"})
	for _, item := range profile.Awards {
		rows = append(rows, []string{
			item.Name,
			item.Result,
			item.EvidenceScope,
			formatScore10(item.Level),
			formatOptionalScore10(item.ImpactFactor),
			formatBenchmarkDistribution(item.BenchmarkDimensions, item.BenchmarkScores),
			item.Reason,
			item.DataSource + "；" + item.ScoreSource,
		})
	}
	rows = append(rows, []string{}, []string{"经历", "角色", "贡献", "分类", "Level", "Impact", "六维分布", "证据强度", "评分说明", "来源"})
	for _, item := range profile.Experiences {
		label := item.Type
		if label == "" {
			label = "经历"
		}
		rows = append(rows, []string{
			label,
			item.Role,
			item.Contribution,
			item.EvidenceScope,
			formatScore10(float64(item.Level)),
			formatOptionalScore10(item.ImpactFactor),
			formatBenchmarkDistribution(item.BenchmarkDimensions, item.BenchmarkScores),
			item.Signal,
			item.Reason,
			item.DataSource + "；" + item.ScoreSource,
		})
	}
	rows = append(rows, []string{}, []string{"TOP5 匹配岗位", "匹配度", "推荐理由", "下一步证据"})
	for _, job := range profile.TopJobs {
		rows = append(rows, []string{fmt.Sprintf("%d. %s", job.Rank, job.Title), fmt.Sprintf("%d%%", job.Match), strings.Join(job.Reasons, "；"), job.NextProof})
	}

	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	maxCols := maxRowColumns(rows)
	if maxCols < 10 {
		maxCols = 10
	}
	files := map[string]string{
		"[Content_Types].xml":        contentTypesXML(),
		"_rels/.rels":                rootRelsXML(),
		"xl/workbook.xml":            workbookXML(maxCols, len(rows)),
		"xl/_rels/workbook.xml.rels": workbookRelsXML(),
		"xl/worksheets/sheet1.xml":   worksheetXML(rows),
		"xl/styles.xml":              workbookStylesXML(),
		"docProps/core.xml":          corePropsXML(),
		"docProps/app.xml":           appPropsXML(),
	}
	for name, content := range files {
		writer, err := archive.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := io.WriteString(writer, content); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func worksheetXML(rows [][]string) string {
	var builder strings.Builder
	builder.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	maxCols := maxRowColumns(rows)
	if maxCols < 10 {
		maxCols = 10
	}
	builder.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	builder.WriteString(`<sheetPr><pageSetUpPr fitToPage="1"/></sheetPr>`)
	builder.WriteString(fmt.Sprintf(`<dimension ref="A1:%s%d"/>`, columnName(maxCols), len(rows)))
	builder.WriteString(`<sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>`)
	builder.WriteString(`<sheetFormatPr defaultRowHeight="18"/>`)
	builder.WriteString(worksheetColumnsXML())
	builder.WriteString(`<sheetData>`)
	for rowIndex, row := range rows {
		height := excelRowHeight(row)
		if height > 0 {
			builder.WriteString(fmt.Sprintf(`<row r="%d" ht="%.1f" customHeight="1">`, rowIndex+1, height))
		} else {
			builder.WriteString(fmt.Sprintf(`<row r="%d">`, rowIndex+1))
		}
		for colIndex, value := range row {
			ref := fmt.Sprintf("%s%d", columnName(colIndex+1), rowIndex+1)
			styleID := excelCellStyle(rowIndex, row, colIndex, value)
			builder.WriteString(fmt.Sprintf(`<c r="%s" s="%d" t="inlineStr"><is><t>%s</t></is></c>`, ref, styleID, xmlText(value)))
		}
		builder.WriteString(`</row>`)
	}
	builder.WriteString(`</sheetData>`)
	builder.WriteString(fmt.Sprintf(`<mergeCells count="1"><mergeCell ref="A1:%s1"/></mergeCells>`, columnName(maxCols)))
	builder.WriteString(`<printOptions horizontalCentered="1"/>`)
	builder.WriteString(`<pageMargins left="0.25" right="0.25" top="0.45" bottom="0.45" header="0.20" footer="0.20"/>`)
	builder.WriteString(`<pageSetup paperSize="9" orientation="landscape" fitToWidth="1" fitToHeight="0"/>`)
	builder.WriteString(`</worksheet>`)
	return builder.String()
}

func worksheetColumnsXML() string {
	widths := []float64{16, 18, 20, 12, 11, 11, 28, 34, 28, 24}
	var builder strings.Builder
	builder.WriteString(`<cols>`)
	for index, width := range widths {
		col := index + 1
		builder.WriteString(fmt.Sprintf(`<col min="%d" max="%d" width="%.1f" customWidth="1"/>`, col, col, width))
	}
	builder.WriteString(`</cols>`)
	return builder.String()
}

func excelCellStyle(rowIndex int, row []string, colIndex int, value string) int {
	if rowIndex == 0 {
		return 1
	}
	if len(row) == 0 || isBlankRow(row) {
		return 0
	}
	if isExcelSectionHeader(row) {
		return 2
	}
	if rowIndex >= 1 && rowIndex <= 8 {
		if colIndex == 0 {
			return 3
		}
		return 5
	}
	if strings.Contains(value, "/10") || strings.HasSuffix(value, "%") || strings.HasPrefix(value, "#") || value == "是" || value == "否" {
		return 6
	}
	return 5
}

func excelRowHeight(row []string) float64 {
	if len(row) == 0 || isBlankRow(row) {
		return 8
	}
	maxLen := 0
	for _, value := range row {
		if length := len([]rune(value)); length > maxLen {
			maxLen = length
		}
	}
	switch {
	case maxLen > 90:
		return 60
	case maxLen > 58:
		return 46
	case maxLen > 34:
		return 34
	default:
		return 0
	}
}

func maxRowColumns(rows [][]string) int {
	maxCols := 0
	for _, row := range rows {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}
	return maxCols
}

func isBlankRow(row []string) bool {
	for _, value := range row {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}

func isExcelSectionHeader(row []string) bool {
	if len(row) == 0 {
		return false
	}
	switch strings.TrimSpace(row[0]) {
	case "教育经历", "专业六维基线", "六维指标", "奖项与证书", "经历", "TOP5 匹配岗位":
		return true
	default:
		return false
	}
}

func buildPathPlanDoc(plan PathPlan) string {
	stats := pathPlanDocStats(plan)
	var builder strings.Builder
	builder.WriteString(`<!doctype html><html><head><meta charset="utf-8"><title>成长路径规划</title>`)
	builder.WriteString(`<style>
@page{size:595.3pt 841.9pt;margin:34pt 38pt 36pt 38pt}
body{margin:0;font-family:Arial,"PingFang SC","Microsoft YaHei",sans-serif;font-size:10pt;line-height:1.38;color:#111827;background:#fff}
h1{margin:0 0 7pt;color:#0f172a;font-size:20pt;line-height:1.12}
h2{margin:0 0 5pt;color:#0f172a;font-size:13pt;line-height:1.22}
p{margin:0 0 6pt}.muted{color:#64748b}.small{font-size:8.5pt;color:#64748b}
.summary{width:100%;border-collapse:separate;border-spacing:5pt;margin:8pt 0 10pt}
.summary td{width:25%;border:1px solid #d8e0ea;background:#f8fafc;padding:6pt 7pt;vertical-align:top}
.summary b{display:block;margin-bottom:2pt;color:#475569;font-size:8.5pt}.summary strong{color:#0f172a;font-size:12pt}
.stage{margin:12pt 0 0;padding-top:8pt;border-top:2pt solid #2563eb;page-break-inside:avoid}
.goal{margin-bottom:6pt;color:#334155}
.deliverable{margin:6pt 0 8pt;padding:6pt 7pt;border:1px solid #d8e0ea;background:#f8fafc}
table.tasks{width:100%;border-collapse:collapse;table-layout:fixed;margin:6pt 0 8pt}
.tasks th,.tasks td{border:1px solid #cbd5e1;padding:4.5pt 5pt;text-align:left;vertical-align:top;font-size:9pt;line-height:1.32;word-break:break-word}
.tasks th{background:#eaf2ff;color:#1e3a8a;font-weight:bold}
.col-week{width:13%}.col-task{width:43%}.col-metric{width:34%}.col-priority{width:10%}
.priority{display:inline-block;min-width:18pt;text-align:center;padding:1.5pt 4pt;font-weight:bold}
.priority-high{color:#991b1b;background:#fee2e2}.priority-medium{color:#1d4ed8;background:#dbeafe}.priority-low{color:#047857;background:#d1fae5}
.two-col{width:100%;border-collapse:collapse;margin-top:6pt}.two-col td{width:50%;vertical-align:top;padding:0 6pt 0 0}
.block-title{display:block;margin:0 0 4pt;color:#334155;font-weight:bold}
ul.compact-list{margin:0 0 4pt 14pt;padding:0}ul.compact-list li{margin:0 0 3pt}
.resource-url{display:block;margin-top:1pt;color:#64748b;font-size:8pt}
.empty{color:#94a3b8}.footer{margin-top:12pt;padding-top:6pt;border-top:1px solid #e2e8f0;color:#64748b;font-size:8pt}
</style>`)
	builder.WriteString(`</head><body><h1>个性化成长路径规划</h1>`)
	builder.WriteString(`<p class="muted">围绕推荐岗位能力差距生成阶段目标、周任务、资源链接和达标标准。</p>`)
	builder.WriteString(`<table class="summary"><tr>`)
	builder.WriteString(`<td><b>阶段</b><strong>` + fmt.Sprintf("%d", stats.stageCount) + `</strong></td>`)
	builder.WriteString(`<td><b>周任务</b><strong>` + fmt.Sprintf("%d", stats.taskCount) + `</strong></td>`)
	builder.WriteString(`<td><b>高优先级</b><strong>` + fmt.Sprintf("%d", stats.highPriorityCount) + `</strong></td>`)
	builder.WriteString(`<td><b>导出格式</b><strong>` + html.EscapeString(pathPlanDocFormats(plan)) + `</strong></td>`)
	builder.WriteString(`</tr></table>`)
	if len(plan.Stages) == 0 {
		builder.WriteString(`<p class="empty">暂无路径规划结果。请等待 Path Planning Team 返回阶段目标后再导出。</p>`)
		builder.WriteString(`</body></html>`)
		return builder.String()
	}
	for _, stage := range plan.Stages {
		builder.WriteString(`<section class="stage">`)
		builder.WriteString(`<h2>` + html.EscapeString(defaultText(stage.Stage, "阶段目标")) + `</h2>`)
		builder.WriteString(`<p class="goal"><strong>目标：</strong>` + html.EscapeString(defaultText(stage.Goal, "待补充阶段目标")) + `</p>`)
		builder.WriteString(`<table class="tasks"><thead><tr><th class="col-week">周次</th><th class="col-task">任务</th><th class="col-metric">达标指标</th><th class="col-priority">优先级</th></tr></thead><tbody>`)
		for _, week := range stage.Weeks {
			priority := defaultText(week.Priority, "中")
			builder.WriteString(`<tr><td>` + html.EscapeString(defaultText(week.Week, "本周")) + `</td><td>` + html.EscapeString(defaultText(week.Task, "待补充任务")) + `</td><td>` + html.EscapeString(defaultText(week.Metric, "待补充达标指标")) + `</td><td><span class="priority ` + docPriorityClass(priority) + `">` + html.EscapeString(priority) + `</span></td></tr>`)
		}
		builder.WriteString(`</tbody></table>`)
		builder.WriteString(`<p class="deliverable"><strong>阶段交付物：</strong>` + html.EscapeString(defaultText(stage.Deliverable, "阶段作品、复盘文档和可验证证据")) + `</p>`)
		builder.WriteString(`<table class="two-col"><tr><td><span class="block-title">达标标准</span>` + pathPlanDocStandards(stage.Standards) + `</td><td><span class="block-title">资源链接</span>` + pathPlanDocResources(stage.Resources) + `</td></tr></table>`)
		builder.WriteString(`</section>`)
	}
	builder.WriteString(`<p class="footer">由 JobAgent 根据当前诊断结果生成。建议导出后按实际岗位 JD 和时间安排复核。</p>`)
	builder.WriteString(`</body></html>`)
	return builder.String()
}

type pathPlanDocStat struct {
	stageCount        int
	taskCount         int
	highPriorityCount int
	resourceCount     int
}

func pathPlanDocStats(plan PathPlan) pathPlanDocStat {
	stats := pathPlanDocStat{stageCount: len(plan.Stages)}
	for _, stage := range plan.Stages {
		stats.taskCount += len(stage.Weeks)
		stats.resourceCount += len(stage.Resources)
		for _, week := range stage.Weeks {
			if strings.Contains(week.Priority, "高") {
				stats.highPriorityCount++
			}
		}
	}
	return stats
}

func pathPlanDocFormats(plan PathPlan) string {
	if len(plan.ExportFormats) == 0 {
		return "PDF / Word"
	}
	return strings.Join(plan.ExportFormats, " / ")
}

func pathPlanDocStandards(items []string) string {
	if len(items) == 0 {
		return `<p class="empty">暂无达标标准</p>`
	}
	var builder strings.Builder
	builder.WriteString(`<ul class="compact-list">`)
	for _, item := range items {
		builder.WriteString(`<li>` + html.EscapeString(item) + `</li>`)
	}
	builder.WriteString(`</ul>`)
	return builder.String()
}

func pathPlanDocResources(items []Resource) string {
	if len(items) == 0 {
		return `<p class="empty">暂无资源链接</p>`
	}
	var builder strings.Builder
	builder.WriteString(`<ul class="compact-list">`)
	for _, item := range items {
		label := defaultText(item.Label, item.URL)
		url := strings.TrimSpace(item.URL)
		if url == "" {
			builder.WriteString(`<li>` + html.EscapeString(label) + `</li>`)
			continue
		}
		builder.WriteString(`<li><a href="` + html.EscapeString(url) + `">` + html.EscapeString(label) + `</a><span class="resource-url">` + html.EscapeString(url) + `</span></li>`)
	}
	builder.WriteString(`</ul>`)
	return builder.String()
}

func docPriorityClass(priority string) string {
	switch {
	case strings.Contains(priority, "高"):
		return "priority-high"
	case strings.Contains(priority, "低"):
		return "priority-low"
	default:
		return "priority-medium"
	}
}

func defaultText(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func contentTypesXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/><Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/><Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/></Types>`
}

func rootRelsXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/><Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/></Relationships>`
}

func workbookXML(maxCols int, maxRows int) string {
	printArea := fmt.Sprintf("'Ability Profile'!$A$1:$%s$%d", columnName(maxCols), maxRows)
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Ability Profile" sheetId="1" r:id="rId1"/></sheets><definedNames><definedName name="_xlnm.Print_Area" localSheetId="0">` + xmlText(printArea) + `</definedName></definedNames></workbook>`
}

func workbookRelsXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`
}

func workbookStylesXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <fonts count="4">
    <font><sz val="10"/><name val="Arial"/></font>
    <font><b/><sz val="16"/><color rgb="FFFFFFFF"/><name val="Arial"/></font>
    <font><b/><sz val="10"/><color rgb="FFFFFFFF"/><name val="Arial"/></font>
    <font><b/><sz val="10"/><color rgb="FF334155"/><name val="Arial"/></font>
  </fonts>
  <fills count="5">
    <fill><patternFill patternType="none"/></fill>
    <fill><patternFill patternType="gray125"/></fill>
    <fill><patternFill patternType="solid"><fgColor rgb="FF2563EB"/><bgColor indexed="64"/></patternFill></fill>
    <fill><patternFill patternType="solid"><fgColor rgb="FF334155"/><bgColor indexed="64"/></patternFill></fill>
    <fill><patternFill patternType="solid"><fgColor rgb="FFEFF6FF"/><bgColor indexed="64"/></patternFill></fill>
  </fills>
  <borders count="2">
    <border><left/><right/><top/><bottom/><diagonal/></border>
    <border><left style="thin"><color rgb="FFD8E0EA"/></left><right style="thin"><color rgb="FFD8E0EA"/></right><top style="thin"><color rgb="FFD8E0EA"/></top><bottom style="thin"><color rgb="FFD8E0EA"/></bottom><diagonal/></border>
  </borders>
  <cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>
  <cellXfs count="7">
    <xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>
    <xf numFmtId="0" fontId="1" fillId="2" borderId="1" xfId="0" applyFill="1" applyFont="1" applyBorder="1" applyAlignment="1"><alignment horizontal="left" vertical="center"/></xf>
    <xf numFmtId="0" fontId="2" fillId="3" borderId="1" xfId="0" applyFill="1" applyFont="1" applyBorder="1" applyAlignment="1"><alignment horizontal="left" vertical="center" wrapText="1"/></xf>
    <xf numFmtId="0" fontId="3" fillId="4" borderId="1" xfId="0" applyFill="1" applyFont="1" applyBorder="1" applyAlignment="1"><alignment horizontal="left" vertical="top" wrapText="1"/></xf>
    <xf numFmtId="0" fontId="3" fillId="4" borderId="1" xfId="0" applyFill="1" applyFont="1" applyBorder="1" applyAlignment="1"><alignment horizontal="center" vertical="center" wrapText="1"/></xf>
    <xf numFmtId="0" fontId="0" fillId="0" borderId="1" xfId="0" applyBorder="1" applyAlignment="1"><alignment horizontal="left" vertical="top" wrapText="1"/></xf>
    <xf numFmtId="0" fontId="0" fillId="0" borderId="1" xfId="0" applyBorder="1" applyAlignment="1"><alignment horizontal="center" vertical="top" wrapText="1"/></xf>
  </cellXfs>
  <cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>
</styleSheet>`
}

func corePropsXML() string {
	now := time.Now().UTC().Format(time.RFC3339)
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:dcmitype="http://purl.org/dc/dcmitype/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"><dc:title>Ability Profile</dc:title><dc:creator>JobAgent</dc:creator><dcterms:created xsi:type="dcterms:W3CDTF">` + now + `</dcterms:created></cp:coreProperties>`
}

func appPropsXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"><Application>JobAgent</Application></Properties>`
}

func columnName(index int) string {
	var name string
	for index > 0 {
		index--
		name = string(rune('A'+index%26)) + name
		index /= 26
	}
	return name
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func methodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

func frontendDir() string {
	if value := strings.TrimSpace(os.Getenv("FRONTEND_DIR")); value != "" {
		return value
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "../frontend"
	}
	candidates := []string{
		filepath.Join(cwd, "frontend"),
		filepath.Join(cwd, "..", "frontend"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(filepath.Join(candidate, "index.html")); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return filepath.Join(cwd, "..", "frontend")
}

func safeStaticPath(path string) bool {
	if path == "" || filepath.IsAbs(path) {
		return false
	}
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		if part == ".." {
			return false
		}
	}
	return true
}

func parseOptionalURL(value string) *url.URL {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		log.Printf("ignoring invalid PRESTO_URL %q", value)
		return nil
	}
	return parsed
}

func envDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func boolLabel(value bool) string {
	if value {
		return "是"
	}
	return "否"
}

func rankLabel(rank int) string {
	if rank <= 0 {
		return ""
	}
	return fmt.Sprintf("#%d", rank)
}

func schoolKindLabel(item EducationItem) string {
	if item.SchoolKind == "independent_college" {
		if item.ParentSchool != "" {
			return "独立学院/原三本（母体：" + item.ParentSchool + "）"
		}
		return "独立学院/原三本"
	}
	return item.SchoolKind
}

func formatScore10(value float64) string {
	return fmt.Sprintf("%.1f/10", value)
}

func formatOptionalScore10(value *float64) string {
	if value == nil {
		return ""
	}
	return formatScore10(*value)
}

func formatBenchmarkDistribution(dimensions []string, scores []float64) string {
	if len(scores) == 0 {
		return ""
	}
	if len(dimensions) != len(scores) {
		dimensions = benchmarkDimensionNames()
	}
	parts := make([]string, 0, len(scores))
	for index, score := range scores {
		name := fmt.Sprintf("维度%d", index+1)
		if index < len(dimensions) && dimensions[index] != "" {
			name = dimensions[index]
		}
		parts = append(parts, fmt.Sprintf("%s %.0f%%", name, score*100))
	}
	return strings.Join(parts, "；")
}

func formatIntDistribution(dimensions []string, scores []int) string {
	if len(scores) == 0 {
		return ""
	}
	if len(dimensions) != len(scores) {
		dimensions = benchmarkDimensionNames()
	}
	parts := make([]string, 0, len(scores))
	for index, score := range scores {
		name := fmt.Sprintf("维度%d", index+1)
		if index < len(dimensions) && dimensions[index] != "" {
			name = dimensions[index]
		}
		parts = append(parts, fmt.Sprintf("%s %d", name, score))
	}
	return strings.Join(parts, "；")
}

func xmlText(value string) string {
	return html.EscapeString(value)
}
