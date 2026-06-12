document.documentElement.classList.add("js-ready");

const deck = document.querySelector("#deck");
const form = document.querySelector("#diagnosisForm");
const resumeInput = document.querySelector("#resumeInput");
const transcriptInput = document.querySelector("#transcriptInput");
const otherInput = document.querySelector("#otherInput");
const runButton = document.querySelector("#runButton");
const uploadMessage = document.querySelector("#uploadMessage");
const lockState = document.querySelector("#lockState");
const unlockHint = document.querySelector("#unlockHint");
const toast = document.querySelector("#toast");
const scrollGradient = document.querySelector("#scrollGradient");
const assistant = document.querySelector("#llmAssistant");
const assistantToggle = document.querySelector("#assistantToggle");
const assistantPanel = document.querySelector("#assistantPanel");
const assistantTitle = document.querySelector("#assistantTitle");
const assistantHistoryBack = document.querySelector("#assistantHistoryBack");
const assistantClose = document.querySelector("#assistantClose");
const assistantNewSession = document.querySelector("#assistantNewSession");
const assistantArchiveSession = document.querySelector("#assistantArchiveSession");
const assistantArchive = document.querySelector("#assistantArchive");
const assistantArchiveList = document.querySelector("#assistantArchiveList");
const assistantArchiveCount = document.querySelector("#assistantArchiveCount");
const assistantMessages = document.querySelector("#assistantMessages");
const assistantSuggestions = document.querySelector("#assistantSuggestions");
const assistantContext = document.querySelector("#assistantContext");
const assistantForm = document.querySelector("#assistantForm");
const assistantEvidenceTray = document.querySelector("#assistantEvidenceTray");
const assistantInput = document.querySelector("#assistantInput");
const assistantInputMeta = document.querySelector("#assistantInputMeta");
const assistantSend = document.querySelector("#assistantSend");
const assistantRailStatus = document.querySelector("#assistantRailStatus");

const assistantStorageKey = "jobagent.llmAssistant.v1";
const assistantMaxMessages = 80;
const assistantMaxSessions = 24;
const assistantPromptLimit = 1200;
const assistantRequestTimeoutMs = 150000;
const assistantStreamTickMs = 52;
const assistantEditableTargets = {
  basic: {
    label: "基础信息",
    roots: ["/ability_profile/basic_info"],
    description: "姓名、性别、出生年份、学校、学院、专业、学历和材料来源。",
    schema: {
      type: "object",
      fields: {
        name: "string",
        sex: "string",
        birth_year: "string|number",
        school: "string",
        department: "string",
        major: "string",
        degree: "string",
        transcript_use: "string"
      }
    }
  },
  education: {
    label: "教育经历",
    roots: ["/ability_profile/education"],
    description: "多段学历、学校层次、排名和专业信息。",
    schema: {
      type: "array",
      item: {
        school: "string",
        department: "string",
        major: "string",
        degree: "string",
        school_level: "string",
        school_tags: "string[]",
        ruanke_rank: "number|string",
        inference: "string"
      }
    }
  },
  awards: {
    label: "奖项与证书",
    roots: ["/ability_profile/awards"],
    description: "没有长描述的奖项、比赛和证书证据。",
    schema: {
      type: "array",
      item: {
        name: "string",
        result: "string",
        evidence_scope: "校内|校外|string",
        level: "number",
        impact_factor: "number",
        benchmark_scores: "number[6]",
        reason: "string"
      }
    }
  },
  experiences: {
    label: "经历证据",
    roots: ["/ability_profile/experiences"],
    description: "有贡献描述的项目、实习、比赛、任职等经历。",
    schema: {
      type: "array",
      item: {
        type: "string",
        role: "string",
        contribution: "string",
        evidence_scope: "校内|校外|string",
        level: "number",
        impact_factor: "number",
        benchmark_scores: "number[6]",
        reason: "string"
      }
    }
  },
  profile_radar: {
    label: "能力画像雷达",
    roots: ["/ability_profile/radar_data", "/ability_profile/radar_series"],
    description: "六维能力分与校内、校外、综合雷达序列。",
    schema: {
      type: "object",
      fields: {
        radar_data: "{name:string, score:number}[]",
        radar_series: "{label:string, scope:string, scores:number[], count:number}[]"
      }
    }
  },
  matching: {
    label: "岗位匹配",
    roots: ["/matching_result"],
    description: "首选岗位、匹配度、目标雷达、差距、行动建议和推荐理由。",
    schema: {
      type: "object",
      fields: {
        target_role: "string",
        overall_match: "number",
        match_level: "string",
        selected_job: "object",
        student_radar: "{name:string, score:number}[]",
        target_radar: "{name:string, score:number}[]",
        gap_details: "{capability:string,current:string,expected:string,action:string,severity:string}[]",
        development_actions: "{priority:string,scope:string,description:string}[]",
        recommendations: "string[]",
        recommended_reasons: "string[]"
      }
    }
  },
  path_plan: {
    label: "路径规划",
    roots: ["/path_plan"],
    description: "阶段目标、周任务、达标标准、资源链接和导出格式。",
    schema: {
      type: "object",
      fields: {
        summary: "string",
        export_formats: "string[]",
        stages: "{stage:string,goal:string,deliverable:string,weeks:{week:string,task:string,metric:string,priority:string}[],standards:string[],resources:{label:string,url:string}[]}[]"
      }
    }
  },
  top_jobs: {
    label: "TOP5 岗位",
    roots: ["/ability_profile/top5_matching_jobs"],
    description: "画像区域和输出区域共用的推荐岗位列表。",
    schema: {
      type: "array",
      item: {
        title: "string",
        match: "number",
        category: "string",
        fit_summary: "string",
        reasons: "string[]",
        proof_gaps: "string[]",
        next_proof: "string",
        education_gate: "string"
      }
    }
  },
  job_recommendations: {
    label: "岗位推荐",
    roots: ["/matching_result", "/ability_profile/top5_matching_jobs"],
    description: "首选岗位匹配和推荐岗位队列，用于根据用户偏好的新方向重写推荐。",
    schema: {
      type: "object",
      fields: {
        matching_result: {
          target_role: "string",
          overall_match: "number",
          match_level: "string",
          fit_summary: "string",
          selected_job: "object",
          gap_details: "{capability:string,current:string,expected:string,action:string,severity:string}[]",
          development_actions: "{priority:string,scope:string,description:string}[]",
          recommendations: "string[]",
          recommended_reasons: "string[]"
        },
        top_jobs: "{title:string,match:number,category:string,fit_summary:string,reasons:string[],proof_gaps:string[],next_proof:string,education_gate:string}[]"
      }
    }
  }
};

let diagnosis = null;
let resumeReady = false;
let toastTimer = 0;
let runDetailTimer = 0;
let runDetailLastUpdate = 0;
let runDetailPendingText = "";
let diagnosisEvents = null;
let currentJobId = "";
let benchmarkRequestInFlight = false;
let matchingRequestInFlight = false;
let pathRequestInFlight = false;
let baseJobDone = false;
let failedRunStep = "";
let firstResultRevealed = false;
let activeStep = "upload";
let scrollSyncFrame = 0;
let radarAnimationFrame = 0;
let radarRenderState = null;

const benchmarkDimensions = ["逻辑", "语言", "专业", "领导", "抗压", "成长"];
const radarVisualGamma = 0.86;
const radarGridScores = [25, 50, 75, 100];
let assistantState = loadAssistantState();
let assistantBusy = false;
let assistantAbort = null;
let assistantFocusedEvidence = null;
let assistantFocusedContext = null;
let assistantFocusedContexts = [];
let assistantAgentStreamMessageId = "";
let assistantAgentStreamActive = false;
let assistantAgentStreamAutoOpened = false;
let assistantAgentTypewriterTimers = new Map();
let assistantStateSaveTimer = 0;
let assistantStreamRenderFrame = 0;

const agentSteps = ["resume_agent", "transcript_agent", "profile", "matching", "path", "outputs"];
const moduleLocks = {
  profile: false,
  matching: false,
  path: false,
  outputs: false
};

const runStepDetails = {
  resume_agent: "简历解析 Agent 正在读取材料并抽取基础信息。",
  transcript_agent: "成绩单解析 Agent 正在整理课程和成绩证据。",
  profile: "画像 Agent 正在合并简历与成绩单证据。",
  matching: "岗位匹配 Agent 正在计算推荐岗位和匹配度。",
  path: "路径规划 Agent 正在生成阶段目标和周任务。",
  outputs: "导出 Agent 正在整理结构化输出。"
};

function setRunDetail(message, options = {}) {
  const detail = document.querySelector("#runDetail");
  if (!detail) return;
  const text = formatAgentDisplayText(message || "正在生成诊断结果。");
  const immediate = Boolean(options.immediate);
  const minInterval = Number(options.minInterval || 900);
  if (immediate) {
    window.clearTimeout(runDetailTimer);
    runDetailTimer = 0;
    runDetailPendingText = "";
    runDetailLastUpdate = Date.now();
    if (detail.textContent !== text) detail.textContent = text;
    return;
  }
  if (detail.textContent === text || runDetailPendingText === text) return;
  window.clearTimeout(runDetailTimer);
  runDetailTimer = 0;
  const wait = Math.max(0, minInterval - (Date.now() - runDetailLastUpdate));
  runDetailPendingText = text;
  runDetailTimer = window.setTimeout(() => {
    runDetailLastUpdate = Date.now();
    if (runDetailPendingText && detail.textContent !== runDetailPendingText) {
      detail.textContent = runDetailPendingText;
    }
    runDetailPendingText = "";
    runDetailTimer = 0;
  }, wait);
}

function runDetailForDiagnosisEvent(event, data = {}) {
  if (data.agent_team_event) return agentTeamDockSummary(data.agent_team_event);
  return event.message || runStepDetails[event.step] || "正在生成诊断结果。";
}

function agentTeamDockSummary(rawEvent) {
  const event = normalizeAgentTeamEvent(rawEvent);
  const group = agentStreamGroupForEvent(event);
  const config = agentStreamPhaseConfig(group, event.workflow);
  if (event.status === "failed") {
    const agent = event.agent && event.agent !== "Agent" ? event.agent : config.label;
    return `${agent} 分析失败，详细事件已收纳到 AI 助手。`;
  }
  if (group === "planning") {
    if (event.status === "done") return `${config.label} 已完成任务拆解，正在启动多视角 Agent Team。`;
    return `${config.label} 正在拆解任务复杂度，详细事件已收纳到 AI 助手。`;
  }
  if (group === "synthesis") {
    if (event.status === "done") return `${config.label} 已完成综合裁决，正在整理结果。`;
    return `${config.label} 正在综合多视角结论，详细事件已收纳到 AI 助手。`;
  }
  const total = event.agentTotal || event.agentCount || 0;
  const index = event.agentIndex && total ? `（${event.agentIndex}/${total}）` : "";
  if (event.status === "done") return `${event.agent || "Agent"}${index} 已完成，Agent Team 仍在并行分析。`;
  return `Agent Team 正在并行分析${total ? ` ${total} 个视角` : ""}，详细日志已收纳到 AI 助手。`;
}

function createDiagnosisShell() {
  return {
    generated_at: "",
    mode: "legato-required",
    input_sources: [],
    ability_profile: {
      basic_info: {},
      education: [],
      radar_data: [],
      evidence_summary: [],
      awards_status: "waiting",
      awards: [],
      experiences_status: "waiting",
      experiences: [],
      benchmark_status: "waiting",
      major_baseline_status: "waiting",
      major_baseline: {},
      top5_matching_jobs: []
    },
    path_plan: { export_formats: [], stages: [] },
    matching_result: {
      selected_job: {},
      student_radar: [],
      target_radar: [],
      report_sections: [],
      gap_details: [],
      development_actions: [],
      recommendations: [],
      recommended_reasons: []
    },
    backend_requirements: [],
    production_limitations: []
  };
}

init();

async function init() {
  setupRevealObserver();
  setupStepObserver();
  setupScrollGradient();
  setupNavIndicator();
  setupUploads();
  setupRunStepRetries();
  setupExports();
  setupAssistant();
  resetResultModules();
}

function setupRevealObserver() {
  const observer = new IntersectionObserver((entries) => {
    entries.forEach((entry) => {
      if (entry.isIntersecting) entry.target.classList.add("is-visible");
    });
  }, { root: deck, threshold: 0.16 });

  document.querySelectorAll(".reveal").forEach((element) => observer.observe(element));
}

function setupStepObserver() {
  const observer = new IntersectionObserver((entries) => {
    const visible = entries.filter((entry) => entry.isIntersecting).sort((a, b) => b.intersectionRatio - a.intersectionRatio)[0];
    if (!visible) return;
    const step = visible.target.dataset.step;
    setActiveStep(step || activeStep);
  }, { root: deck, threshold: [0.45, 0.65] });

  document.querySelectorAll("[data-step]").forEach((section) => observer.observe(section));
}

function setupNavIndicator() {
  updateNavIndicator();
  window.addEventListener("resize", updateNavIndicator);
}

function setupUploads() {
  document.querySelector(".step-nav").addEventListener("click", (event) => {
    const link = event.target.closest("a[href]");
    if (!link) return;
    const target = link.getAttribute("href").replace("#", "");
    event.preventDefault();
    if (!resumeReady && target !== "upload") {
      showToast("请先上传简历。");
      return;
    }
    if (moduleLocks[target] === false) {
      showToast("该模块还在生成中。");
      return;
    }
    scrollToModule(target);
  });

  resumeInput.addEventListener("change", () => {
    updateFileState(resumeInput, "resumeState", "resumeDrop", "必传");
    resumeReady = resumeInput.files.length > 0;
    if (resumeReady) unlockDeck();
  });
  transcriptInput.addEventListener("change", () => updateFileState(transcriptInput, "transcriptState", "transcriptDrop", "可选"));
  otherInput.addEventListener("change", () => updateFileState(otherInput, "otherState", "otherDrop", "可选"));
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    if (!resumeReady) {
      showToast("请先上传简历。");
      return;
    }
    await runDiagnosis();
  });
}

function setupRunStepRetries() {
  const runSteps = document.querySelector("#runSteps");
  runSteps.addEventListener("click", (event) => {
    const item = event.target.closest("[data-run-step]");
    if (!item?.classList.contains("is-retryable")) return;
    retryFromFailedStep(item.dataset.runStep);
  });
  runSteps.addEventListener("keydown", (event) => {
    if (event.key !== "Enter" && event.key !== " ") return;
    const item = event.target.closest("[data-run-step]");
    if (!item?.classList.contains("is-retryable")) return;
    event.preventDefault();
    retryFromFailedStep(item.dataset.runStep);
  });
}

function setupScrollGradient() {
  deck.addEventListener("scroll", handleDeckScroll, { passive: true });
  window.addEventListener("resize", () => {
    syncActiveStepFromScroll();
    updateScrollGradient();
    updateNavIndicator();
  });
  updateScrollGradient();
}

function handleDeckScroll() {
  if (scrollSyncFrame) return;
  scrollSyncFrame = window.requestAnimationFrame(() => {
    scrollSyncFrame = 0;
    syncActiveStepFromScroll();
    updateScrollGradient();
  });
}

function updateFileState(input, stateId, dropId, fallback) {
  const state = document.querySelector(`#${stateId}`);
  const drop = document.querySelector(`#${dropId}`);
  const count = input.files.length;
  drop.classList.toggle("is-ready", count > 0);
  state.textContent = count === 0 ? fallback : count === 1 ? input.files[0].name : `${count} 个文件`;
}

function unlockDeck() {
  deck.classList.remove("is-locked");
  document.body.classList.add("can-scroll");
  unlockHint.classList.add("is-unlocked");
  lockState.classList.add("is-unlocked");
  lockState.textContent = "简历已就绪";
  runButton.disabled = false;
  uploadMessage.textContent = "简历已上传，可以生成诊断。";
  setRunDetail("材料已就绪，点击生成诊断后会显示实时进度。", { immediate: true });
  updateScrollGradient();
}

function updateScrollGradient() {
  if (!scrollGradient) return;
  const canScroll = !deck.classList.contains("is-locked") && deck.scrollHeight > deck.clientHeight + 12;
  const hasMoreBelow = deck.scrollTop + deck.clientHeight < deck.scrollHeight - 48;
  scrollGradient.classList.toggle("is-visible", canScroll && hasMoreBelow);
}

function syncActiveStepFromScroll() {
  const marker = deck.scrollTop + deck.clientHeight * 0.48;
  let current = activeStep;
  document.querySelectorAll("[data-step]").forEach((section) => {
    if (section.offsetTop <= marker) current = section.dataset.step || current;
  });
  setActiveStep(current);
}

function setActiveStep(step) {
  if (!step) return;
  const changed = activeStep !== step;
  activeStep = step;
  document.querySelectorAll("[data-step-link]").forEach((link) => {
    link.classList.toggle("is-active", link.dataset.stepLink === step);
  });
  updateNavIndicator({ scrollIntoView: changed });
}

function updateNavIndicator(options = {}) {
  const nav = document.querySelector(".step-nav");
  const activeLink = document.querySelector(`[data-step-link="${activeStep}"]`);
  if (!nav || !activeLink) return;
  const navRect = nav.getBoundingClientRect();
  const linkRect = activeLink.getBoundingClientRect();
  nav.style.setProperty("--nav-indicator-x", `${linkRect.left - navRect.left + nav.scrollLeft}px`);
  nav.style.setProperty("--nav-indicator-w", `${linkRect.width}px`);
  if (options.scrollIntoView) {
    activeLink.scrollIntoView({ block: "nearest", inline: "nearest" });
  }
}

async function runDiagnosis() {
  runButton.disabled = true;
  runButton.textContent = "生成中";
  document.querySelector(".generation-dock").classList.add("is-running");
  resetRunSteps();
  resetResultModules();
  const diagnosisSession = startAssistantDiagnosisSession();

  const payload = new FormData(form);

  try {
    const response = await fetch("/api/diagnosis", { method: "POST", body: payload });
    if (!response.ok) throw new Error("diagnosis request failed");
    const job = await response.json();
    bindAssistantDiagnosisJob(diagnosisSession, job);
    if (job.events_url) {
      currentJobId = job.job_id || "";
      connectDiagnosisEvents(job.events_url);
      return;
    }
    if (job.ability_profile) {
      diagnosis = job;
      renderDiagnosis(diagnosis);
      reconcileRunStepsFromDiagnosis(diagnosis);
      if (canMarkOutputsDone()) {
        markAgentStep("outputs", "done");
        unlockModule("outputs");
      }
      setRunDone();
      return;
    }
    throw new Error("diagnosis job missing events_url");
  } catch {
    setRunFailed("无法连接 Legato 诊断服务。");
    markAssistantAgentTeamFailed("无法连接 Legato 诊断服务。");
    showToast("Legato 诊断服务不可用，未生成诊断。");
  }
}

function closeDiagnosisEvents() {
  if (diagnosisEvents) {
    diagnosisEvents.close();
    diagnosisEvents = null;
  }
}

function resetRunSteps() {
  closeDiagnosisEvents();
  baseJobDone = false;
  failedRunStep = "";
  assistantAgentStreamMessageId = "";
  assistantAgentStreamActive = false;
  assistantAgentStreamAutoOpened = false;
  stopAgentTypewriters();
  document.querySelectorAll("[data-run-step]").forEach((item) => {
    item.classList.remove("is-done", "is-running", "is-failed");
    setRunStepRetryable(item.dataset.runStep, false);
  });
  document.querySelector("#runStatus").textContent = "生成中";
  setRunDetail("正在创建诊断任务并启动 Agent。", { immediate: true });
  setRunProgress(0);
  syncAssistantAvailability();
}

function startAssistantDiagnosisSession() {
  pruneEmptyActiveAssistantSession();
  const session = createAssistantSession(false, {
    title: buildDiagnosisSessionTitle(),
    diagnosisJobId: "",
    diagnosisFileName: resumeInput.files?.[0]?.name || ""
  });
  assistantArchive.open = false;
  assistantAgentStreamActive = true;
  assistantAgentStreamAutoOpened = true;
  const { message } = ensureAgentTeamStreamMessage(session);
  message.content = agentTeamStreamFallback(message.agentStream);
  message.updatedAt = new Date().toISOString();
  session.updatedAt = message.updatedAt;
  saveAssistantState();
  setAssistantExpanded(true, { silent: true, force: true });
  renderAssistant();
  return session;
}

function buildDiagnosisSessionTitle() {
  const fileName = resumeInput.files?.[0]?.name || "";
  const cleanName = fileName.replace(/\.[^.]+$/, "").trim();
  if (cleanName) return `诊断 · ${cleanName}`.slice(0, 80);
  return `诊断 · ${formatTime(new Date().toISOString())}`;
}

function bindAssistantDiagnosisJob(session, job) {
  if (!session || !job) return;
  session.diagnosisJobId = String(job.job_id || session.diagnosisJobId || "");
  session.updatedAt = new Date().toISOString();
  saveAssistantState();
  renderAssistantArchive();
}

function markAssistantAgentTeamFailed(messageText) {
  const { session, message } = findActiveAgentStreamMessage();
  if (!session || !message) return;
  const now = new Date().toISOString();
  message.status = "error";
  message.updatedAt = now;
  message.content = messageText;
  message.agentStream = message.agentStream || createAgentTeamStreamState();
  message.agentStream.status = "failed";
  message.agentStream.summary = messageText;
  message.agentStream.updatedAt = now;
  session.updatedAt = now;
  assistantAgentStreamActive = false;
  saveAssistantState();
  renderAssistant();
}

function setRunDone() {
  if (!canMarkRunDone()) {
    keepRunPendingUntilAllStepsDone();
    return;
  }
  failedRunStep = "";
  matchingRequestInFlight = false;
  pathRequestInFlight = false;
  document.querySelector("#runStatus").textContent = "诊断已生成";
  setRunDetail("诊断完成，可以查看和导出结果。", { immediate: true });
  document.querySelector(".generation-dock").classList.remove("is-running");
  setRunProgress(100);
  runButton.disabled = false;
  runButton.textContent = "重新生成";
  updateAssistantContext();
  renderAssistantSuggestions();
  syncAssistantAvailability();
}

function canMarkRunDone() {
  if (benchmarkRequestInFlight || matchingRequestInFlight || pathRequestInFlight || failedRunStep) return false;
  const allStepsDone = agentSteps.every((step) => document.querySelector(`[data-run-step="${step}"]`)?.classList.contains("is-done"));
  return allStepsDone && moduleLocks.profile && moduleLocks.matching && moduleLocks.path && moduleLocks.outputs;
}

function canMarkOutputsDone() {
  const allPriorStepsDone = agentSteps
    .filter((step) => step !== "outputs")
    .every((step) => document.querySelector(`[data-run-step="${step}"]`)?.classList.contains("is-done"));
  return allPriorStepsDone && moduleLocks.profile && moduleLocks.matching && moduleLocks.path;
}

function hasMeaningfulValue(value) {
  if (value === null || value === undefined) return false;
  if (Array.isArray(value)) return hasMeaningfulArray(value);
  if (typeof value === "object") return Object.values(value).some((entry) => hasMeaningfulValue(entry));
  return cleanDisplayText(value).length > 0;
}

function hasMeaningfulArray(value) {
  return Array.isArray(value) && value.some((item) => hasMeaningfulValue(item));
}

function hasMeaningfulMatchingResult(match) {
  if (!match || typeof match !== "object") return false;
  const selected = match.selected_job || {};
  return Boolean(
    cleanDisplayText(match.source)
    || cleanDisplayText(match.target_role)
    || cleanDisplayText(match.fit_summary)
    || cleanDisplayText(selected.title)
    || cleanDisplayText(selected.fit_summary)
    || Number(match.overall_match) > 0
    || hasMeaningfulArray(match.student_radar)
    || hasMeaningfulArray(match.target_radar)
    || hasMeaningfulArray(match.report_sections)
    || hasMeaningfulArray(match.gap_details)
    || hasMeaningfulArray(match.development_actions)
    || hasMeaningfulArray(match.recommendations)
    || hasMeaningfulArray(match.recommended_reasons)
  );
}

function hasMeaningfulPathPlan(plan) {
  if (!plan || typeof plan !== "object") return false;
  const stages = Array.isArray(plan.stages) ? plan.stages : [];
  return Boolean(
    cleanDisplayText(plan.summary)
    || cleanDisplayText(plan.path_plan_summary)
    || stages.some((stage) => {
      if (!stage || typeof stage !== "object") return false;
      return cleanDisplayText(stage.stage)
        || cleanDisplayText(stage.title)
        || cleanDisplayText(stage.goal)
        || cleanDisplayText(stage.deliverable)
        || hasMeaningfulArray(stage.weekly_tasks)
        || hasMeaningfulArray(stage.acceptance)
        || hasMeaningfulArray(stage.resources);
    })
  );
}

function hasReadyPathPlan(plan, match) {
  return hasMeaningfulPathPlan(plan) && hasMeaningfulMatchingResult(match);
}

function isProfileStepReady(profile) {
  return Boolean(profile) && isMatchingGateOpen(profile);
}

function keepRunPendingUntilAllStepsDone() {
  document.querySelector("#runStatus").textContent = "生成中";
  setRunDetail(nextIncompleteRunStepDetail(), { immediate: true });
  document.querySelector(".generation-dock").classList.add("is-running");
  runButton.disabled = true;
  runButton.textContent = "生成中";
  syncAssistantAvailability();
}

function nextIncompleteRunStepDetail() {
  const running = agentSteps.find((step) => document.querySelector(`[data-run-step="${step}"]`)?.classList.contains("is-running"));
  if (running) return runStepDetails[running] || `${stepLabel(running)}正在生成。`;
  const pending = agentSteps.find((step) => !document.querySelector(`[data-run-step="${step}"]`)?.classList.contains("is-done"));
  if (pending) return runStepDetails[pending] || `等待${stepLabel(pending)}完成。`;
  if (!moduleLocks.profile) return "等待能力画像模块解锁。";
  if (!moduleLocks.matching) return "等待岗位匹配模块解锁。";
  if (!moduleLocks.path) return "等待路径规划模块解锁。";
  if (!moduleLocks.outputs) return "等待结构化输出模块解锁。";
  return "等待最后阶段状态同步。";
}

function setRunFailed(message) {
  pathRequestInFlight = false;
  document.querySelector("#runStatus").textContent = "诊断失败";
  setRunDetail(message || "Legato 必需解析失败，请检查材料或后端服务。", { immediate: true });
  document.querySelector(".generation-dock").classList.remove("is-running");
  runButton.disabled = false;
  runButton.textContent = "重新生成";
  updateAssistantContext();
  syncAssistantAvailability();
}

function setRunWaitingForBenchmark() {
  document.querySelector("#runStatus").textContent = "生成中";
  setRunDetail(baseJobDone
    ? "基础流程已完成，等待 Item Benchmark 返回六维分布。"
    : "Item Benchmark 正在评估 Impact 和六维分布。", { immediate: true });
  document.querySelector(".generation-dock").classList.add("is-running");
  runButton.disabled = true;
  runButton.textContent = "生成中";
  syncAssistantAvailability();
}

function setRunProgress(value) {
  const progress = safeScore(value);
  document.querySelector("#runProgressBar").style.width = `${progress}%`;
  document.querySelector("#runPercent").textContent = `${progress}%`;
}

function connectDiagnosisEvents(eventsUrl) {
  diagnosisEvents = new EventSource(eventsUrl);
  diagnosisEvents.addEventListener("step.update", (event) => {
    handleDiagnosisEvent(JSON.parse(event.data));
  });
  diagnosisEvents.addEventListener("job.started", (event) => {
    const payload = JSON.parse(event.data);
    document.querySelector("#runStatus").textContent = "生成中";
    setRunDetail(payload.message || "异步诊断已开始。", { immediate: true });
  });
  diagnosisEvents.addEventListener("job.done", (event) => {
    const payload = JSON.parse(event.data);
    baseJobDone = true;
    if (payload.data && payload.data.diagnosis) {
      diagnosis = payload.data.diagnosis;
      if (!moduleLocks.profile || !moduleLocks.matching || !moduleLocks.path) {
        renderDiagnosis(diagnosis);
      } else {
        renderRequirements(diagnosis.backend_requirements || []);
        renderLimitations(diagnosis.production_limitations || []);
      }
      reconcileRunStepsFromDiagnosis(diagnosis);
    }
    const keepEventsForBenchmark = benchmarkRequestInFlight || diagnosis?.ability_profile?.benchmark_status === "benchmarking";
    if (keepEventsForBenchmark) {
      setRunWaitingForBenchmark();
    } else if (diagnosis?.ability_profile?.benchmark_status === "failed") {
      setBenchmarkRunFailed();
    } else {
      if (canMarkOutputsDone()) {
        markAgentStep("outputs", "done");
        unlockModule("outputs");
      }
      setRunDone();
    }
    if (!keepEventsForBenchmark) {
      closeDiagnosisEvents();
      updateAssistantContext();
    }
  });
  diagnosisEvents.addEventListener("job.failed", (event) => {
    const payload = JSON.parse(event.data);
    const errors = Array.isArray(payload.data?.errors) ? payload.data.errors : [];
    const detail = errors.filter(Boolean).join("；");
    const message = detail ? `${payload.message || "诊断失败"}：${detail}` : payload.message;
    setRunFailed(message);
    showToast(formatAgentDisplayText(message || "简历解析失败，请检查材料或后端服务。"));
    closeDiagnosisEvents();
  });
  diagnosisEvents.onerror = () => {
    closeDiagnosisEvents();
    setRunFailed("诊断事件流中断，请重新生成。");
    showToast("诊断事件流中断，请重新生成。");
  };
}

function reconcileRunStepsFromDiagnosis(result) {
  if (!result) return;
  if (result.ability_profile && !failedRunStep) {
    completeRunStepsThrough("transcript_agent");
    unlockModule("profile");
    if (isProfileStepReady(result.ability_profile)) {
      completeRunStepsThrough("profile");
    }
  }
  if (hasMeaningfulMatchingResult(result.matching_result) && !failedRunStep) {
    completeRunStepsThrough("matching");
    unlockModule("matching");
  }
  if (hasReadyPathPlan(result.path_plan, result.matching_result) && !failedRunStep) {
    completeRunStepsThrough("path");
    unlockModule("path");
  }
}

function reconcileRunStepsFromEventData(data = {}) {
  const eventDiagnosis = data.diagnosis || {};
  const profile = data.ability_profile || eventDiagnosis.ability_profile || diagnosis?.ability_profile;
  if (profile && isProfileStepReady(profile) && !failedRunStep) {
    completeRunStepsThrough("profile");
    unlockModule("profile");
  }
  const match = data.matching_result || eventDiagnosis.matching_result || diagnosis?.matching_result;
  if (hasMeaningfulMatchingResult(match) && !failedRunStep) {
    completeRunStepsThrough("matching");
    unlockModule("matching");
  }
  const plan = data.path_plan || eventDiagnosis.path_plan || diagnosis?.path_plan;
  if (hasReadyPathPlan(plan, match) && !failedRunStep) {
    completeRunStepsThrough("path");
    unlockModule("path");
  }
}

function handleDiagnosisEvent(event) {
  const data = event.data || {};
  reconcileRunStepsFromEventData(data);
  const runStepStatus = normalizedRunStepStatus(event, data);
  markAgentStep(event.step, runStepStatus);
  if (event.step === "path" && (runStepStatus === "running" || runStepStatus === "pending")) {
    lockModule("path");
  }
  if (event.step === "outputs" && (runStepStatus === "running" || runStepStatus === "pending")) {
    lockModule("outputs");
  }
  const hasAgentTeamEvent = Boolean(data.agent_team_event);
  const statusText = runStepStatus === "running"
    ? "生成中"
    : runStepStatus === "failed"
      ? `失败：${stepLabel(event.step)}`
      : runStepStatus === "done"
        ? `已完成：${stepLabel(event.step)}`
        : "生成中";
  document.querySelector("#runStatus").textContent = statusText;
  setRunDetail(runDetailForDiagnosisEvent(event, data), {
    immediate: event.status !== "running" || !hasAgentTeamEvent,
    minInterval: hasAgentTeamEvent ? 1200 : 900
  });

  if (data.agent_team_event) {
    handleAgentTeamChatEvent(data.agent_team_event);
  }
  if (data.ability_profile) {
    diagnosis = diagnosis || createDiagnosisShell();
    diagnosis.ability_profile = data.ability_profile;
    renderBasicInfo(diagnosis);
    renderAbilityRadar(data.ability_profile);
    renderResumeEvidence(data.ability_profile);
    if (shouldAutoRequestItemBenchmark(data.ability_profile, event)) {
      maybeRequestItemBenchmark(data.ability_profile);
    }
    if (event.step === "profile" && event.status === "failed" && data.ability_profile.benchmark_status === "failed") {
      setBenchmarkRunFailed(formatAgentDisplayText(data.error || event.message));
    }
    unlockModule("profile");
  }
  if (data.production_limitations && diagnosis) {
    diagnosis.production_limitations = data.production_limitations;
    renderLimitations(data.production_limitations);
  }
  if (event.step === "matching" && event.status === "failed") {
    setMatchingRunFailed(formatAgentDisplayText(data.matching_error || data.error || event.message));
  }
  if (event.step === "path" && event.status === "failed") {
    setPathRunFailed(formatAgentDisplayText(data.path_error || data.error || event.message));
  }
  if (hasMeaningfulMatchingResult(data.matching_result)) {
    diagnosis = diagnosis || createDiagnosisShell();
    diagnosis.matching_result = data.matching_result;
    renderMatching(data.matching_result);
    if (event.step === "matching" && event.status === "done") {
      finalizeAgentTeamStreamFromMatchingPayload(data);
    }
  }
  if (data.top_jobs) {
    diagnosis = diagnosis || createDiagnosisShell();
    diagnosis.ability_profile.top5_matching_jobs = data.top_jobs;
    renderTopJobs(data.top_jobs);
  }
  if (hasReadyPathPlan(data.path_plan, data.matching_result || diagnosis?.matching_result)) {
    diagnosis = diagnosis || createDiagnosisShell();
    diagnosis.path_plan = data.path_plan;
    renderPath(data.path_plan);
    if (event.step === "path" && event.status === "done") {
      finalizePathPlanningStreamFromPathPayload(data);
    }
  } else if (data.path_plan && event.step === "path" && event.status === "done") {
    lockModule("path");
  }
  if (data.diagnosis) {
    diagnosis = data.diagnosis;
    renderDiagnosis(diagnosis);
    reconcileRunStepsFromDiagnosis(diagnosis);
  }
  if (data.backend_requirements) {
    diagnosis = diagnosis || createDiagnosisShell();
    diagnosis.backend_requirements = data.backend_requirements;
    renderRequirements(data.backend_requirements);
  }
  if (event.status === "done") {
    if (event.step === "profile") unlockModule("profile");
    if (event.step === "matching" && hasMeaningfulMatchingResult(diagnosis?.matching_result)) unlockModule("matching");
    if (event.step === "path" && hasReadyPathPlan(diagnosis?.path_plan, diagnosis?.matching_result)) unlockModule("path");
    if (event.step === "outputs" && canMarkOutputsDone()) unlockModule("outputs");
    if (baseJobDone) setRunDone();
  }
  updateAssistantContext();
  renderAssistantSuggestions();
}

function normalizedRunStepStatus(event, data = {}) {
  const eventDiagnosis = data.diagnosis || {};
  if ((event.status === "running" || event.status === "done") && !arePriorRunStepsDone(event.step)) {
    return "pending";
  }
  if (event.status !== "done") return event.status;
  if (event.step === "matching" && !hasMeaningfulMatchingResult(data.matching_result || eventDiagnosis.matching_result || diagnosis?.matching_result)) {
    return "pending";
  }
  if (event.step === "path" && !hasReadyPathPlan(data.path_plan || eventDiagnosis.path_plan || diagnosis?.path_plan, data.matching_result || eventDiagnosis.matching_result || diagnosis?.matching_result)) {
    return "pending";
  }
  if (event.step === "outputs" && !canMarkOutputsDone()) {
    return "pending";
  }
  return event.status;
}

function handleAgentTeamChatEvent(event) {
  const normalized = normalizeAgentTeamEvent(event);
  const terminalStatus = normalized.status === "done" || normalized.status === "failed";
  const streamTerminal = terminalStatus
    && (normalized.agentKey === "team" || normalized.agentKey === "synthesis_arbiter" || normalized.agentKey === "path_synthesis_arbiter" || normalized.phase === "final_synthesis");
  const isTokenDelta = normalized.tokenChannel === "content" && Boolean(normalized.tokenDelta);
  assistantAgentStreamActive = !streamTerminal;
  const session = ensureAssistantSession();
  const now = new Date().toISOString();
  const { message, created } = ensureAgentTeamStreamMessage(session, normalized);
  message.agentStream = updateAgentTeamStreamState(message.agentStream, normalized);
  message.content = agentTeamStreamFallback(message.agentStream);
  message.status = message.agentStream.status === "failed"
    ? "error"
    : message.agentStream.status === "done"
      ? "done"
      : "loading";
  message.updatedAt = now;
  session.updatedAt = now;
  if (isTokenDelta) {
    scheduleAssistantStateSave();
  } else {
    saveAssistantState();
  }
  if (normalized.agentKey && normalized.agentKey !== "team") {
    syncAgentTypewriterForEvent(message.id, normalized.agentKey);
  }
  if (created && !assistantAgentStreamAutoOpened && !assistantState.expanded) {
    assistantAgentStreamAutoOpened = true;
  }
  if (isTokenDelta && patchAgentStreamText(message.id, normalized.agentKey)) {
    return;
  }
  if (isTokenDelta && !isAgentStreamExpanded(message.agentStream, normalized.agentKey)) return;
  renderAssistant();
  if (streamTerminal && baseJobDone && !benchmarkRequestInFlight && !matchingRequestInFlight) {
    if (normalized.status === "done" && !failedRunStep) setRunDone();
    closeDiagnosisEvents();
  }
}

function ensureAgentTeamStreamMessage(session, event = null) {
  const phaseGroup = agentStreamGroupForEvent(event);
  const workflow = agentStreamWorkflowForEvent(event);
  let message = session.messages.find((item) => {
    if (item.streamType !== "agent_team") return false;
    const streamGroup = item.agentStream?.phaseGroup || "team";
    const streamWorkflow = item.agentStream?.workflow || "resume/job_matching";
    return streamGroup === phaseGroup && streamWorkflow === workflow;
  });
  if (message) {
    if (!message.agentStream) message.agentStream = createAgentTeamStreamState(phaseGroup, workflow);
    if (!message.agentStream.phaseGroup) message.agentStream.phaseGroup = phaseGroup;
    if (!message.agentStream.workflow) message.agentStream.workflow = workflow;
    assistantAgentStreamMessageId = message.id;
    return { message, created: false };
  }
  const now = new Date().toISOString();
  const stream = createAgentTeamStreamState(phaseGroup, workflow);
  message = {
    id: uniqueId("msg"),
    role: "assistant",
    content: agentTeamStreamFallback(stream),
    createdAt: now,
    updatedAt: now,
    status: "loading",
    retryPrompt: "",
    streamType: "agent_team",
    agentStream: stream
  };
  assistantAgentStreamMessageId = message.id;
  session.messages.push(message);
  return { message, created: true };
}

function createAgentTeamStreamState(phaseGroup = "planning", workflow = "resume/job_matching") {
  const config = agentStreamPhaseConfig(phaseGroup, workflow);
  return {
    title: config.title,
    workflow: config.workflow,
    phaseGroup: config.group,
    stageOrder: config.stageOrder,
    status: "running",
    complexity: "",
    agentCount: config.agentCount,
    phase: config.phase,
    summary: config.summary,
    order: [],
    agents: {},
    expandedAgents: {},
    logs: [],
    updatedAt: new Date().toISOString()
  };
}

function updateAgentTeamStreamState(stream, event) {
  const phaseGroup = stream?.phaseGroup || agentStreamGroupForEvent(event);
  const workflow = stream?.workflow || agentStreamWorkflowForEvent(event);
  stream = stream || createAgentTeamStreamState(phaseGroup, workflow);
  const isTokenDelta = event.tokenChannel === "content" && Boolean(event.tokenDelta);
  stream.phaseGroup = phaseGroup;
  stream.workflow = workflow;
  const config = agentStreamPhaseConfig(phaseGroup, workflow);
  stream.title = config.title;
  stream.stageOrder = config.stageOrder;
  stream.complexity = event.complexity || stream.complexity;
  if (phaseGroup === "team") {
    stream.agentCount = event.agentCount || event.agentTotal || stream.agentCount;
  } else {
    stream.agentCount = 1;
  }
  stream.phase = event.phase || stream.phase;
  stream.updatedAt = event.time;
  if (event.message) stream.summary = event.message;
  if (event.agentKey && event.agentKey !== "team") {
    upsertAgentStreamAgent(stream, event);
  }
  if (phaseGroup === "synthesis" && event.agentKey === "team" && event.status === "done") {
    completeSynthesisAgentFromTeamEvent(stream, event);
  }
  stream.status = resolveAgentStreamStatus(stream, event);
  if (!isTokenDelta) {
    stream.logs.push({
      time: event.time,
      agentKey: event.agentKey,
      agent: event.agent,
      status: event.status,
      message: event.message,
      runID: event.runID
    });
    if (stream.logs.length > 20) stream.logs = stream.logs.slice(-20);
  }
  return stream;
}

function completeSynthesisAgentFromTeamEvent(stream, event) {
  const synthesisKey = stream.workflow === "resume/path_planning" ? "path_synthesis_arbiter" : "synthesis_arbiter";
  const existing = stream.agents[synthesisKey];
  const message = existing?.message && existing.status === "done"
    ? existing.message
    : stream.workflow === "resume/path_planning"
      ? "Path Synthesis Arbiter 已返回结构化路径规划结果。"
      : "Synthesis Arbiter 已返回结构化岗位匹配结果。";
  upsertAgentStreamAgent(stream, {
    ...event,
    agentKey: synthesisKey,
    agent: stream.workflow === "resume/path_planning" ? "Path Synthesis Arbiter" : "Synthesis Arbiter",
    status: "done",
    phase: "final_synthesis",
    perspective: existing?.perspective || "multi_view_decision",
    reasoningEffort: existing?.reasoningEffort || "",
    focus: existing?.focus || (stream.workflow === "resume/path_planning" ? "综合所有视角结果，输出路径规划。" : "综合所有视角结果，输出岗位匹配报告。"),
    agentIndex: 1,
    agentTotal: 1,
    runID: existing?.runID || event.runID || "",
    message,
    outputPreview: existing?.outputPreview || event.outputPreview || ""
  });
  const item = stream.agents[synthesisKey];
  if (item?.outputPreview && !item.typedOutput) {
    item.typedOutput = item.outputPreview;
    item.typingDone = true;
  }
}

function agentStreamPhaseConfig(phaseGroup = "planning", workflow = "resume/job_matching") {
  const group = ["planning", "team", "synthesis"].includes(phaseGroup) ? phaseGroup : "team";
  const isPathPlanning = workflow === "resume/path_planning";
  const configs = isPathPlanning ? {
    planning: {
      group: "planning",
      workflow,
      title: "路径规划 · Planner",
      label: "Path Planner",
      phase: "planning",
      stageOrder: 1,
      agentCount: 1,
      summary: "Path Planner 正在把岗位匹配结果拆解成路径规划 Agent 队列。",
      empty: "等待 Path Planner 返回路径规划方案。"
    },
    team: {
      group: "team",
      workflow,
      title: "路径规划 · Agent Team",
      label: "路径设计",
      phase: "orchestration",
      stageOrder: 2,
      agentCount: 0,
      summary: "路径规划 Agent 将从阶段目标、周任务、证据交付和达标标准并发设计。",
      empty: "等待 Planner 派生的路径规划 Agent 队列。"
    },
    synthesis: {
      group: "synthesis",
      workflow,
      title: "路径规划 · Synthesis",
      label: "路径综合",
      phase: "final_synthesis",
      stageOrder: 3,
      agentCount: 1,
      summary: "Path Synthesis Arbiter 将综合 Agent 结果并输出可执行路径规划。",
      empty: "等待 Path Synthesis Arbiter 启动。"
    }
  } : {
    planning: {
      group: "planning",
      workflow,
      title: "规划阶段",
      label: "Adaptive Planner",
      phase: "planning",
      stageOrder: 1,
      agentCount: 1,
      summary: "Planner 正在判断简历复杂度并派生多视角 Agent。",
      empty: "等待 Adaptive Planner 返回派生方案。"
    },
    team: {
      group: "team",
      workflow,
      title: "多视角 Agent Team",
      label: "并行分析",
      phase: "orchestration",
      stageOrder: 2,
      agentCount: 0,
      summary: "多视角 Agent 将从能力、证据、教育背景、岗位族等角度并发分析。",
      empty: "等待 Planner 派生的 Agent 队列。"
    },
    synthesis: {
      group: "synthesis",
      workflow,
      title: "Synthesis Arbiter",
      label: "综合裁决",
      phase: "final_synthesis",
      stageOrder: 3,
      agentCount: 1,
      summary: "Synthesis Arbiter 将综合所有视角结果并输出可渲染的岗位匹配报告。",
      empty: "等待 Synthesis Arbiter 启动。"
    }
  };
  return configs[group];
}

function agentStreamGroupForEvent(event = null) {
  if (!event) return "planning";
  const agentKey = String(event.agentKey || "");
  const phase = String(event.phase || "");
  if (agentKey === "synthesis_arbiter" || agentKey === "path_synthesis_arbiter" || phase === "final_synthesis") return "synthesis";
  if (agentKey === "adaptive_planner" || agentKey === "path_planner" || phase === "planning") return "planning";
  if (agentKey === "team" && event.status === "done") return "synthesis";
  if (agentKey === "team" && phase === "orchestration" && !event.agentCount && event.status !== "failed") return "planning";
  return "team";
}

function agentStreamWorkflowForEvent(event = null) {
  const workflow = String(event?.workflow || "");
  return workflow === "resume/path_planning" ? workflow : "resume/job_matching";
}

function resolveAgentStreamStatus(stream, event) {
  if (event.status === "failed") return "failed";
  const planningKey = stream.workflow === "resume/path_planning" ? "path_planner" : "adaptive_planner";
  const synthesisKey = stream.workflow === "resume/path_planning" ? "path_synthesis_arbiter" : "synthesis_arbiter";
  if (stream.phaseGroup === "planning") {
    if (stream.agents[planningKey]?.status === "failed") return "failed";
    return stream.agents[planningKey]?.status === "done" ? "done" : "running";
  }
  if (stream.phaseGroup === "synthesis") {
    if (event.agentKey === "team" && event.status === "done") return "done";
    if (stream.agents[synthesisKey]?.status === "failed") return "failed";
    return stream.agents[synthesisKey]?.status === "done" ? "done" : "running";
  }
  const total = stream.agentCount || stream.order.length;
  const completed = stream.order.filter((key) => stream.agents[key]?.status === "done").length;
  const failed = stream.order.filter((key) => stream.agents[key]?.status === "failed").length;
  if (failed > 0) return "failed";
  if (total > 0 && completed >= total) return "done";
  return "running";
}

function upsertAgentStreamAgent(stream, event) {
  const key = event.agentKey || event.agent || "agent";
  if (!stream.agents[key]) {
    stream.order.push(key);
    stream.agents[key] = {
      key,
      agent: event.agent || key,
      status: "queued",
      phase: event.phase || "",
      perspective: event.perspective || "",
      reasoningEffort: event.reasoningEffort || "",
      focus: event.focus || "",
      agentIndex: event.agentIndex || 0,
      agentTotal: event.agentTotal || 0,
      runID: event.runID || "",
      message: "",
      outputPreview: "",
      typedOutput: "",
      typingDone: false,
      tokenStreamed: false,
      updatedAt: event.time
    };
  }
  const item = stream.agents[key];
  item.agent = event.agent || item.agent;
  item.status = normalizeAgentStreamStatus(event.status || item.status);
  if (item.tokenStreamed && item.status === "done") {
    item.typingDone = true;
  }
  item.phase = event.phase || item.phase;
  item.perspective = event.perspective || item.perspective;
  item.reasoningEffort = event.reasoningEffort || item.reasoningEffort;
  item.focus = event.focus || item.focus;
  item.agentIndex = event.agentIndex || item.agentIndex;
  item.agentTotal = event.agentTotal || item.agentTotal;
  item.runID = event.runID || item.runID;
  item.message = event.message || item.message;
  if (event.tokenDelta && event.tokenChannel === "content") {
    item.outputPreview = `${item.outputPreview || ""}${event.tokenDelta}`.slice(0, 3600);
    item.typedOutput = item.outputPreview;
    item.typingDone = false;
    item.tokenStreamed = true;
  }
  if (event.outputPreview) {
    const outputPreview = event.outputPreview.slice(0, 3600);
    if (!item.tokenStreamed && outputPreview !== item.outputPreview) {
      const previousTyped = item.typedOutput || "";
      item.outputPreview = outputPreview;
      item.typedOutput = outputPreview.startsWith(previousTyped) ? previousTyped : "";
      item.typingDone = item.typedOutput.length >= item.outputPreview.length;
    }
  }
  item.updatedAt = event.time;
  stream.order.sort((a, b) => {
    const left = stream.agents[a]?.agentIndex || 999;
    const right = stream.agents[b]?.agentIndex || 999;
    return left - right;
  });
}

function normalizeAgentTeamEvent(event) {
  return {
    team: String(event.team || ""),
    workflow: String(event.workflow || "resume/job_matching"),
    agentKey: String(event.agent_key || ""),
    agent: String(event.agent || event.agent_key || "Agent"),
    status: normalizeAgentStreamStatus(event.status || "running"),
    phase: String(event.phase || ""),
    perspective: String(event.perspective || ""),
    reasoningEffort: String(event.reasoning_effort || ""),
    focus: String(event.focus || ""),
    complexity: String(event.complexity || ""),
    agentCount: Number(event.agent_count || 0),
    agentIndex: Number(event.agent_index || 0),
    agentTotal: Number(event.agent_total || 0),
    runID: String(event.presto_run_id || ""),
    prestoEventType: String(event.presto_event_type || ""),
    message: String(event.message || ""),
    tokenChannel: String(event.token_channel || ""),
    tokenDelta: String(event.token_delta || ""),
    outputPreview: String(event.output_preview || ""),
    time: new Date().toISOString()
  };
}

function normalizeAgentStreamStatus(status) {
  if (status === "done" || status === "failed" || status === "streaming" || status === "queued") return status;
  return status === "running" ? "running" : "running";
}

function agentTeamStreamFallback(stream) {
  const config = agentStreamPhaseConfig(stream.phaseGroup, stream.workflow);
  const completed = stream.order.filter((key) => stream.agents[key]?.status === "done").length;
  const total = stream.phaseGroup === "team" ? stream.agentCount || stream.order.length : 1;
  const complexity = stream.complexity ? `复杂度 ${stream.complexity}，` : "";
  const progress = stream.phaseGroup === "team" ? `${completed}/${total || "--"} 个视角 Agent 已完成` : config.label;
  return formatAgentDisplayText(`${config.title}：${complexity}${progress}。${stream.summary || ""}`);
}

function agentTeamChatLine(event) {
  const agent = event.agent || event.agent_key || "Agent";
  const message = event.message || "";
  const runID = event.presto_run_id ? ` · ${shortRunId(event.presto_run_id)}` : "";
  const statusIcon = event.status === "failed" ? "失败" : event.status === "done" ? "完成" : "运行";
  return formatAgentDisplayText(`${statusIcon} ${agent}${runID}：${message}`.trim());
}

function shortRunId(value) {
  const text = String(value || "");
  return text.length > 12 ? text.slice(0, 12) : text;
}

function runStepItem(step) {
  return document.querySelector(`[data-run-step="${step}"]`);
}

function isRunStepDone(step) {
  return Boolean(runStepItem(step)?.classList.contains("is-done"));
}

function updateRunProgressFromSteps() {
  const doneCount = agentSteps.filter((name) => isRunStepDone(name)).length;
  const runningCount = agentSteps.filter((name) => runStepItem(name)?.classList.contains("is-running")).length;
  const perceivedProgress = doneCount + runningCount * 0.35;
  setRunProgress(Math.round((perceivedProgress / agentSteps.length) * 100));
}

function arePriorRunStepsDone(step) {
  const index = agentSteps.indexOf(step);
  if (index <= 0) return true;
  return agentSteps.slice(0, index).every((name) => isRunStepDone(name));
}

function forceRunStepDone(step) {
  if (!agentSteps.includes(step)) return;
  const item = runStepItem(step);
  if (!item) return;
  item.classList.add("is-done");
  item.classList.remove("is-running", "is-failed");
  setRunStepRetryable(step, false);
}

function completeRunStepsThrough(step) {
  const index = agentSteps.indexOf(step);
  if (index < 0) return;
  agentSteps.slice(0, index + 1).forEach((name) => forceRunStepDone(name));
  updateRunProgressFromSteps();
}

function markAgentStep(step, status) {
  if (!agentSteps.includes(step)) return;
  const item = runStepItem(step);
  if (!item) return;
  if ((status === "running" || status === "done") && !arePriorRunStepsDone(step)) {
    status = "pending";
  }
  const alreadyDone = item.classList.contains("is-done");
  if (status === "done") {
    item.classList.add("is-done");
    item.classList.remove("is-running", "is-failed");
    setRunStepRetryable(step, false);
  } else if (status === "failed") {
    item.classList.add("is-failed");
    item.classList.remove("is-running", "is-done");
  } else if (status === "running" && !alreadyDone) {
    item.classList.add("is-running");
    item.classList.remove("is-failed");
    setRunStepRetryable(step, false);
  } else if (status === "pending") {
    item.classList.remove("is-done", "is-running", "is-failed");
    setRunStepRetryable(step, false);
  }
  updateRunProgressFromSteps();
}

function setRunStepRetryable(step, enabled) {
  if (!agentSteps.includes(step)) return;
  const item = document.querySelector(`[data-run-step="${step}"]`);
  if (!item) return;
  item.classList.toggle("is-retryable", Boolean(enabled));
  if (enabled) {
    item.tabIndex = 0;
    item.setAttribute("role", "button");
    item.setAttribute("aria-label", `${stepLabel(step)}失败，点击从此处继续`);
    item.title = "点击从失败处继续";
  } else {
    item.removeAttribute("tabindex");
    item.removeAttribute("role");
    item.removeAttribute("aria-label");
    item.removeAttribute("title");
  }
}

function retryFromFailedStep(step) {
  if (step === "profile" && diagnosis?.ability_profile?.benchmark_status === "failed") {
    retryItemBenchmark();
    return;
  }
  if (step === "matching") {
    retryJobMatching();
    return;
  }
  if (step === "path") {
    retryPathPlanning();
    return;
  }
  showToast("该失败阶段暂不支持局部继续，请重新生成诊断。");
}

function resetResultModules() {
  diagnosis = null;
  currentJobId = "";
  benchmarkRequestInFlight = false;
  matchingRequestInFlight = false;
  pathRequestInFlight = false;
  firstResultRevealed = false;
  Object.keys(moduleLocks).forEach((module) => {
    moduleLocks[module] = false;
    const section = document.querySelector(`#${module}`);
    if (section) section.classList.add("is-module-locked");
  });
  document.querySelector("#basicInfo").innerHTML = "";
  const basicInfoState = document.querySelector("#basicInfoDataState");
  basicInfoState.hidden = false;
  basicInfoState.textContent = "等待结构化数据";
  basicInfoState.className = "status-pill is-warning";
  document.querySelector("#sourceList").innerHTML = "";
  renderResumeEvidence(createDiagnosisShell().ability_profile);
  renderAbilityRadar(createDiagnosisShell().ability_profile);
  document.querySelector("#overallMatch").textContent = "--";
  document.querySelector("#matchLevel").textContent = "等待 Job Matching";
  document.querySelector("#overallMeter").style.width = "0%";
  document.querySelector("#selectedJobTitle").textContent = "等待推荐";
  document.querySelector("#matchNarrative").textContent = "Benchmark 完成后，Legato 会返回首选岗位、推荐理由和需要补齐的证据。";
  document.querySelector("#matchAgentNotes").innerHTML = "";
  document.querySelector("#matchPrimaryReason").textContent = "等待证据";
  document.querySelector("#matchMainGap").textContent = "等待差距分析";
  document.querySelector("#matchNextProof").textContent = "等待补证据建议";
  document.querySelector("#matchEducationGate").textContent = "等待门槛判断";
  document.querySelector("#matchActionList").innerHTML = "";
  renderMatchingRadar({});
  document.querySelector("#reportRows").innerHTML = "";
  document.querySelector("#gapCards").innerHTML = "";
  document.querySelector("#pathStages").innerHTML = "";
  document.querySelector("#matchingJobs").innerHTML = "";
  const topJobs = document.querySelector("#topJobs");
  if (topJobs) topJobs.innerHTML = "";
  const requirementsList = document.querySelector("#requirementsList");
  if (requirementsList) requirementsList.innerHTML = "";
  const limitationsList = document.querySelector("#limitationsList");
  if (limitationsList) limitationsList.innerHTML = "";
}

function lockModule(module) {
  moduleLocks[module] = false;
  const section = document.querySelector(`#${module}`);
  if (!section) return;
  section.classList.remove("is-unlocking");
  section.classList.add("is-module-locked");
}

function unlockModule(module) {
  moduleLocks[module] = true;
  const section = document.querySelector(`#${module}`);
  if (section && section.classList.contains("is-module-locked")) {
    section.classList.add("is-unlocking");
    section.classList.remove("is-module-locked");
    window.setTimeout(() => section.classList.remove("is-unlocking"), 360);
  }
  if (module === "profile" && !firstResultRevealed) {
    firstResultRevealed = true;
    window.setTimeout(() => scrollToModule("profile"), 80);
  }
}

function scrollToModule(module) {
  const target = document.querySelector(`#${module}`);
  if (!target) return;
  const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  setActiveStep(module);
  deck.scrollTo({ top: target.offsetTop, behavior: reducedMotion ? "auto" : "smooth" });
}

function stepLabel(step) {
  return {
    resume_agent: "简历解析",
    transcript_agent: "成绩单解析",
    profile: "能力画像",
    matching: "岗位匹配",
    path: "路径规划",
    outputs: "结构化输出"
  }[step] || "诊断阶段";
}

function renderDiagnosis(data) {
  renderBasicInfo(data);
  renderAbilityRadar(data.ability_profile);
  renderResumeEvidence(data.ability_profile);
  if (shouldAutoRequestItemBenchmark(data.ability_profile)) {
    maybeRequestItemBenchmark(data.ability_profile);
  }
  renderMatching(data.matching_result);
  renderPath(hasReadyPathPlan(data.path_plan, data.matching_result) ? data.path_plan : {});
  renderTopJobs(data.ability_profile.top5_matching_jobs);
  renderRequirements(data.backend_requirements || []);
  renderLimitations(data.production_limitations || []);
}

function renderBasicInfo(data) {
  const info = data.ability_profile.basic_info;
  const basicInfo = document.querySelector("#basicInfo");
  const education = normalizedEducation(data.ability_profile);
  const identityItems = [
    ...(info.name ? [["姓名", info.name]] : []),
    ...(info.sex ? [["性别", info.sex]] : []),
    ...(info.birth_year ? [["出生年份", info.birth_year]] : [])
  ];
  const hasIdentity = identityItems.length > 0;

  basicInfo.innerHTML = `
    ${hasIdentity ? `<div class="identity-row">
      ${identityItems.map(([label, value]) => `
        <div class="identity-field">
          <span>${escapeHTML(label)}</span>
          <strong>${escapeHTML(value)}</strong>
        </div>
      `).join("")}
    </div>` : renderIdentityLoading()}
    <div class="education-stack">
      ${education.length ? education.map((item) => renderEducationCard(item)).join("") : renderEducationLoading()}
    </div>
  `;

  const parsedCount = [
    info.name,
    info.sex,
    info.birth_year,
    ...education.flatMap((item) => [item.school, item.department, item.major])
  ].filter((value) => String(value || "").trim()).length;
  const state = document.querySelector("#basicInfoDataState");
  if (education.length > 0 && info.name) {
    state.hidden = true;
    state.textContent = "";
  } else if (parsedCount > 0) {
    state.hidden = false;
    state.textContent = "部分字段待解析";
    state.className = "status-pill is-warning";
  } else {
    state.hidden = false;
    state.textContent = "等待结构化数据";
    state.className = "status-pill is-warning";
  }

  const sourceList = document.querySelector("#sourceList");
  sourceList.innerHTML = (data.input_sources || []).map((file) => {
    const size = file.size ? `${Math.round(file.size / 1024)} KB` : "已记录";
    const parts = [`${file.kind}：${file.name}`, size];
    if (file.kind === "resume" || file.kind === "transcript") parts.push("Legato 解析");
    return `<span>${parts.map(escapeHTML).join("，")}</span>`;
  }).join("");
}

function renderIdentityLoading() {
  return `
    <div class="identity-loading" role="status" aria-label="正在等待身份信息返回">
      <span class="loading-dot"></span>
      <span class="loading-line is-wide"></span>
      <span class="loading-line"></span>
      <span class="loading-line is-short"></span>
    </div>
  `;
}

function renderEducationLoading() {
  return `
    <article class="education-loading" aria-hidden="true">
      <span class="loading-line is-wide"></span>
      <span class="loading-line"></span>
      <span class="loading-line is-short"></span>
    </article>
  `;
}

function normalizedEducation(profile) {
  const items = Array.isArray(profile.education) ? profile.education.filter(Boolean) : [];
  if (items.length > 0) return items;
  const info = profile.basic_info || {};
  if (!info.school && !info.major && !info.degree) return [];
  return [{
    school: info.school,
    major: info.major,
    degree: info.degree,
    department: "",
    is_985: false,
    is_211: false,
    is_double_first_class: false,
    ruanke_rank: 0
  }];
}

function renderEducationCard(item) {
  const tags = [];
  if (item.is_985) tags.push(`<span class="school-tag is-985">985</span>`);
  if (item.is_211) tags.push(`<span class="school-tag is-211">211</span>`);
  if (Number(item.ruanke_rank) > 0) tags.push(`<span class="school-tag is-rank">#${escapeHTML(item.ruanke_rank)}</span>`);
  const school = cleanDisplayText(item.school);
  const detailItems = [
    ["学院", item.department],
    ["专业", item.major]
  ].filter(([, value]) => String(value || "").trim());
  return `
    <article class="education-card">
      ${school || tags.length ? `<div class="education-head">
        ${school ? `<strong>${escapeHTML(school)}</strong>` : ""}
        ${tags.length ? `<div class="school-tags">${tags.join("")}</div>` : ""}
      </div>` : ""}
      ${detailItems.length ? `<div class="education-meta">
        ${detailItems.map(([label, value]) => `
          <span><b>${escapeHTML(label)}</b>${escapeHTML(value)}</span>
        `).join("")}
      </div>` : ""}
    </article>
  `;
}

function renderAbilityRadar(profile) {
  profile = profile || {};
  const svg = document.querySelector("#radarChart");
  const text = document.querySelector("#radarText");
  const status = document.querySelector("#radarDataState");
  const wrap = document.querySelector(".radar-wrap");
  const series = normalizedBackendRadarSeries(profile);
  const hasRadarData = series.length > 0;
  const benchmarkStatus = profile.benchmark_status || "waiting";
  const isFailed = benchmarkStatus === "failed";
  const isLoading = isBenchmarkLoadingStatus(benchmarkStatus) && !hasRadarData;
  wrap.classList.toggle("is-loading", isLoading);
  wrap.classList.toggle("is-failed", isFailed);
  if (status) {
    status.textContent = isFailed ? "Benchmark 失败" : isLoading ? "等待 Benchmark" : hasRadarData ? "Legato Benchmark" : "等待证据";
    status.className = `status-pill ${isFailed ? "is-danger" : hasRadarData ? "is-real" : "is-warning"}`;
  }
  if (isFailed && !hasRadarData) {
    clearRadarAnimationState();
    renderRadarFailed(svg);
    if (text) text.textContent = "";
    return;
  }
  if (isLoading || !hasRadarData) {
    clearRadarAnimationState();
    renderRadarLoading(svg);
    if (text) text.textContent = "";
    return;
  }
  const items = benchmarkDimensions.map((name) => ({ name }));
  const center = { x: 180, y: 158 };
  const radius = 104;
  const startSeries = radarStartSeries(series);
  const maxPolygon = (score) => items.map((_, index) => {
    const point = pointFor(index, items.length, radius * radarVisualRatio(score), center);
    return `${point.x.toFixed(2)},${point.y.toFixed(2)}`;
  }).join(" ");

  svg.innerHTML = `
    <title id="radarChartTitle">六维能力雷达图</title>
    <desc id="radarChartDesc">${escapeHTML(series.map((entry) => `${entry.label}${entry.scores.map((score, index) => `${benchmarkDimensions[index]}${score}分`).join("，")}`).join("；"))}</desc>
    ${radarGridScores.map((score) => `<polygon class="radar-grid" points="${maxPolygon(score)}"></polygon>`).join("")}
    ${items.map((item, index) => {
      const outer = pointFor(index, items.length, radius, center);
      const label = pointFor(index, items.length, radius + 28, center);
      return `
        <line class="radar-axis" x1="${center.x}" y1="${center.y}" x2="${outer.x.toFixed(2)}" y2="${outer.y.toFixed(2)}"></line>
        <text class="radar-label" x="${label.x.toFixed(2)}" y="${label.y.toFixed(2)}" text-anchor="middle">${escapeHTML(item.name)}</text>
      `;
    }).join("")}
    ${startSeries.map((entry, seriesIndex) => {
      const points = entry.scores.map((score, index) => pointFor(index, entry.scores.length, radius * radarVisualRatio(score), center));
      const area = points.map((point) => `${point.x.toFixed(2)},${point.y.toFixed(2)}`).join(" ");
      return `
        <polygon class="radar-area radar-area-${entry.key}" data-radar-series="${escapeAttribute(entry.key)}" style="--radar-delay:${seriesIndex * 80}ms" points="${area}"></polygon>
        ${points.map((point, index) => `<circle class="radar-dot radar-dot-${entry.key}" data-radar-dot="${escapeAttribute(entry.key)}" data-radar-index="${index}" cx="${point.x.toFixed(2)}" cy="${point.y.toFixed(2)}" r="${entry.key === "overall" ? 4 : 3}"></circle>`).join("")}
      `;
    }).join("")}
    ${radarValueHoverLayer(items, series, center, radius)}
  `;

  const legend = document.querySelector("#radarLegend");
  if (legend) {
    legend.innerHTML = series.map((entry) => `
      <span class="radar-legend-item radar-legend-${entry.key}" tabindex="0" data-series="${entry.key}">
        <i></i>${escapeHTML(entry.label)}<b>${entry.count}</b>
      </span>
    `).join("");
  }
  if (text) text.textContent = "";
  animateRadarToSeries(svg, startSeries, series, {
    center,
    radius
  });
}

function normalizedProfileRadarData(profile) {
  return scoreDimensionArrayToScores(profile?.radar_data);
}

function normalizedBackendRadarSeries(profile) {
  const rawSeries = Array.isArray(profile?.radar_series) ? profile.radar_series : [];
  const series = rawSeries.map((entry) => {
    const scores = scoreDimensionArrayToScores(entry?.scores);
    if (scores.length !== benchmarkDimensions.length) return null;
    return {
      key: cleanDisplayText(entry?.key) || "overall",
      label: cleanDisplayText(entry?.label) || "综合",
      count: Number.isFinite(Number(entry?.count)) ? Number(entry.count) : "",
      scores
    };
  }).filter(Boolean);
  if (series.length) return ensureCampusRadarSeries(profile, series);
  const profileRadar = normalizedProfileRadarData(profile);
  if (profileRadar.length !== benchmarkDimensions.length) return [];
  return ensureCampusRadarSeries(profile, [{ key: "overall", label: "综合", count: "", scores: profileRadar }]);
}

function ensureCampusRadarSeries(profile, series) {
  if (!Array.isArray(series) || series.some((entry) => entry.key === "campus")) return series;
  const scores = normalizedMajorBaselineScores(profile);
  if (scores.length !== benchmarkDimensions.length) return series;
  const campusSeries = { key: "campus", label: "校内", count: 0, scores };
  const overallIndex = series.findIndex((entry) => entry.key === "overall");
  if (overallIndex < 0) return [campusSeries, ...series];
  return [
    ...series.slice(0, overallIndex + 1),
    campusSeries,
    ...series.slice(overallIndex + 1)
  ];
}

function normalizedMajorBaselineScores(profile) {
  const baseline = profile?.major_baseline || {};
  const rawScores = Array.isArray(baseline.scores) ? baseline.scores : [];
  if (rawScores.length === benchmarkDimensions.length && rawScores.every((score) => Number.isFinite(Number(score)))) {
    return rawScores.map((score) => Math.max(0, Math.min(100, Number(score))));
  }
  return scoreDimensionArrayToScores(rawScores);
}

function scoreDimensionArrayToScores(items) {
  const raw = Array.isArray(items) ? items : [];
  const byName = new Map(raw.map((item) => [cleanDisplayText(item?.name), Number(item?.score)]));
  const scores = benchmarkDimensions.map((name) => {
    const value = byName.has(name) ? byName.get(name) : NaN;
    return Number.isFinite(value) ? Math.max(0, Math.min(100, value)) : NaN;
  });
  return scores.every(Number.isFinite) ? scores : [];
}

function radarValueHoverLayer(items, series, center, radius) {
  const cardHeight = 88;
  return items.map((item, index) => {
    const outer = pointFor(index, items.length, radius + 8, center);
    const card = radarValueCardPosition(index, items.length, radius, center);
    const aria = `${item.name}分值：${series.map((entry) => `${entry.label}${Math.round(Number(entry.scores[index]) || 0)}分`).join("，")}`;
    return `
      <g class="radar-dimension-hover" tabindex="0" aria-label="${escapeAttribute(aria)}">
        <line class="radar-hover-zone" x1="${center.x}" y1="${center.y}" x2="${outer.x.toFixed(2)}" y2="${outer.y.toFixed(2)}"></line>
        <circle class="radar-hover-point" cx="${outer.x.toFixed(2)}" cy="${outer.y.toFixed(2)}" r="7"></circle>
        <g class="radar-value-card" transform="translate(${card.x}, ${card.y})" aria-hidden="true">
          <rect width="108" height="${cardHeight}" rx="8"></rect>
          <text class="radar-value-title" x="12" y="20">${escapeHTML(item.name)}</text>
          ${series.map((entry, rowIndex) => `
            <circle class="radar-value-swatch radar-value-swatch-${entry.key}" cx="15" cy="${36 + rowIndex * 16}" r="4"></circle>
            <text class="radar-value-row" x="25" y="${39 + rowIndex * 16}">${escapeHTML(entry.label)} ${Math.round(Number(entry.scores[index]) || 0)}分</text>
          `).join("")}
        </g>
      </g>
    `;
  }).join("");
}

function radarValueCardPosition(index, count, radius, center) {
  const anchor = pointFor(index, count, radius + 30, center);
  const x = Math.max(8, Math.min(244, Math.round(anchor.x - 54)));
  const y = Math.max(10, Math.min(222, Math.round(anchor.y - 42)));
  return { x, y };
}

function radarStartSeries(series) {
  const previous = radarRenderState?.currentSeries || radarRenderState?.targetSeries || [];
  return series.map((entry) => {
    const matched = previous.find((item) => item.key === entry.key && item.scores.length === entry.scores.length);
    return {
      ...entry,
      scores: matched ? matched.scores.map(Number) : entry.scores.map(() => 0)
    };
  });
}

function animateRadarToSeries(svg, startSeries, targetSeries, context) {
  if (radarAnimationFrame) {
    cancelAnimationFrame(radarAnimationFrame);
    radarAnimationFrame = 0;
  }
  const targetSnapshot = cloneRadarSeries(targetSeries);
  radarRenderState = {
    currentSeries: cloneRadarSeries(startSeries),
    targetSeries: targetSnapshot
  };
  const shouldReduceMotion = window.matchMedia?.("(prefers-reduced-motion: reduce)")?.matches;
  if (shouldReduceMotion || radarSeriesNearlyEqual(startSeries, targetSnapshot)) {
    applyRadarSeriesToSvg(svg, targetSnapshot, context.center, context.radius);
    radarRenderState.currentSeries = cloneRadarSeries(targetSnapshot);
    return;
  }

  const duration = 680;
  const startedAt = performance.now();
  const step = (now) => {
    const progress = Math.min(1, (now - startedAt) / duration);
    const eased = 1 - Math.pow(1 - progress, 4);
    const currentSeries = interpolateRadarSeries(startSeries, targetSnapshot, eased);
    applyRadarSeriesToSvg(svg, currentSeries, context.center, context.radius);
    radarRenderState.currentSeries = cloneRadarSeries(currentSeries);
    if (progress < 1) {
      radarAnimationFrame = requestAnimationFrame(step);
    } else {
      radarAnimationFrame = 0;
      radarRenderState.currentSeries = cloneRadarSeries(targetSnapshot);
      radarRenderState.targetSeries = cloneRadarSeries(targetSnapshot);
    }
  };
  radarAnimationFrame = requestAnimationFrame(step);
}

function applyRadarSeriesToSvg(svg, series, center, radius) {
  series.forEach((entry) => {
    const area = svg.querySelector(`[data-radar-series="${entry.key}"]`);
    if (area) area.setAttribute("points", radarPolygonPoints(entry.scores, center, radius));
    const dots = svg.querySelectorAll(`[data-radar-dot="${entry.key}"]`);
    entry.scores.forEach((score, index) => {
      const dot = dots[index];
      if (!dot) return;
      const point = pointFor(index, entry.scores.length, radius * radarVisualRatio(score), center);
      dot.setAttribute("cx", point.x.toFixed(2));
      dot.setAttribute("cy", point.y.toFixed(2));
    });
  });
}

function radarPolygonPoints(scores, center, radius) {
  return scores
    .map((score, index) => {
      const point = pointFor(index, scores.length, radius * radarVisualRatio(score), center);
      return `${point.x.toFixed(2)},${point.y.toFixed(2)}`;
    })
    .join(" ");
}

function radarVisualRatio(score) {
  const normalized = safeScore(score) / 100;
  if (normalized <= 0) return 0;
  return Math.pow(normalized, radarVisualGamma);
}

function interpolateRadarSeries(startSeries, targetSeries, progress) {
  return targetSeries.map((target) => {
    const start = startSeries.find((entry) => entry.key === target.key && entry.scores.length === target.scores.length);
    return {
      ...target,
      scores: target.scores.map((score, index) => {
        const from = start ? Number(start.scores[index]) || 0 : 0;
        return from + (Number(score) - from) * progress;
      })
    };
  });
}

function cloneRadarSeries(series) {
  return series.map((entry) => ({
    key: entry.key,
    label: entry.label,
    count: entry.count,
    scores: entry.scores.map(Number)
  }));
}

function radarSeriesNearlyEqual(left, right) {
  if (left.length !== right.length) return false;
  return right.every((entry) => {
    const matched = left.find((item) => item.key === entry.key && item.scores.length === entry.scores.length);
    if (!matched || matched.count !== entry.count) return false;
    return entry.scores.every((score, index) => Math.abs(score - matched.scores[index]) < 0.2);
  });
}

function clearRadarAnimationState() {
  if (radarAnimationFrame) {
    cancelAnimationFrame(radarAnimationFrame);
    radarAnimationFrame = 0;
  }
  radarRenderState = null;
}

function renderRadarLoading(svg) {
  const center = { x: 180, y: 158 };
  const radius = 104;
  const items = benchmarkDimensions.map((name) => ({ name }));
  const maxPolygon = (score) => items.map((_, index) => {
    const point = pointFor(index, items.length, radius * radarVisualRatio(score), center);
    return `${point.x.toFixed(2)},${point.y.toFixed(2)}`;
  }).join(" ");
  svg.innerHTML = `
    <title id="radarChartTitle">六维能力雷达图加载中</title>
    <desc id="radarChartDesc">Item Benchmark 正在返回六维分布。</desc>
    ${radarGridScores.map((score) => `<polygon class="radar-grid" points="${maxPolygon(score)}"></polygon>`).join("")}
    ${items.map((item, index) => {
      const outer = pointFor(index, items.length, radius, center);
      const label = pointFor(index, items.length, radius + 28, center);
      return `
        <line class="radar-axis" x1="${center.x}" y1="${center.y}" x2="${outer.x.toFixed(2)}" y2="${outer.y.toFixed(2)}"></line>
        <text class="radar-label" x="${label.x.toFixed(2)}" y="${label.y.toFixed(2)}" text-anchor="middle">${escapeHTML(item.name)}</text>
      `;
    }).join("")}
    <polygon class="radar-loading-area" points="${maxPolygon(42)}"></polygon>
  `;
  const legend = document.querySelector("#radarLegend");
  if (legend) {
    legend.innerHTML = ["综合", "校内", "校外"].map((label, index) => `
      <span class="radar-legend-item is-loading radar-legend-${["overall", "campus", "external"][index]}">
        <i></i>${label}<b></b>
      </span>
    `).join("");
  }
}

function renderRadarFailed(svg) {
  const center = { x: 180, y: 158 };
  const radius = 104;
  const items = benchmarkDimensions.map((name) => ({ name }));
  const maxPolygon = (score) => items.map((_, index) => {
    const point = pointFor(index, items.length, radius * radarVisualRatio(score), center);
    return `${point.x.toFixed(2)},${point.y.toFixed(2)}`;
  }).join(" ");
  svg.innerHTML = `
    <title id="radarChartTitle">六维能力雷达图生成失败</title>
    <desc id="radarChartDesc">Item Benchmark 未返回六维分布。</desc>
    ${radarGridScores.map((score) => `<polygon class="radar-grid" points="${maxPolygon(score)}"></polygon>`).join("")}
    ${items.map((item, index) => {
      const outer = pointFor(index, items.length, radius, center);
      const label = pointFor(index, items.length, radius + 28, center);
      return `
        <line class="radar-axis" x1="${center.x}" y1="${center.y}" x2="${outer.x.toFixed(2)}" y2="${outer.y.toFixed(2)}"></line>
        <text class="radar-label" x="${label.x.toFixed(2)}" y="${label.y.toFixed(2)}" text-anchor="middle">${escapeHTML(item.name)}</text>
      `;
    }).join("")}
    <polygon class="radar-failed-area" points="${maxPolygon(38)}"></polygon>
  `;
  const legend = document.querySelector("#radarLegend");
  if (legend) {
    legend.innerHTML = `<span class="radar-legend-item is-failed"><i></i>Benchmark 失败<b>!</b></span>`;
  }
}

function benchmarkedEvidenceItems(profile) {
  return [
    ...(Array.isArray(profile.awards) ? profile.awards : []),
    ...(Array.isArray(profile.experiences) ? profile.experiences : [])
  ].filter((item) => normalizedBenchmarkScores(item).length === 6 && hasImpactFactor(item));
}

function benchmarkableEvidenceItems(profile) {
  return [
    ...(Array.isArray(profile?.awards) ? profile.awards : []),
    ...(Array.isArray(profile?.experiences) ? profile.experiences : [])
  ].filter((item) => {
    const text = cleanDisplayText([item?.name, item?.result, item?.role, item?.contribution].filter(Boolean).join(" "));
    return Boolean(text);
  });
}

function unbenchmarkedEvidenceItems(profile) {
  return benchmarkableEvidenceItems(profile).filter((item) => !hasItemBenchmarkResult(item));
}

function maybeRequestItemBenchmark(profile, options = {}) {
  if (!currentJobId || benchmarkRequestInFlight) return;
  if (!profile) return;
  if (!options.force && diagnosis?.matching_result?.source) return;
  if (!options.force && profile.benchmark_status === "ready") return;
  if (!isBenchmarkGateOpen(profile)) return;

  const missingItems = buildBenchmarkItems(profile, { missingOnly: true });
  const hasMissingItems = missingItems.length > 0;
  if (profile.benchmark_status === "ready" && !hasMissingItems) return;

  const blockedStatuses = options.force
    ? ["mock", "benchmarking", "unavailable"]
    : ["mock", "benchmarking", "failed", "unavailable"];
  if (blockedStatuses.includes(profile.benchmark_status)) return;

  const items = profile.benchmark_status === "ready" ? missingItems : buildBenchmarkItems(profile);
  if (items.length === 0) {
    showToast("没有可用于 Item Benchmark 的证据条目。");
    return;
  }

  benchmarkRequestInFlight = true;
  failedRunStep = "";
  profile.benchmark_status = "benchmarking";
  setRunStepRetryable("profile", false);
  document.querySelector(`[data-run-step="profile"]`)?.classList.remove("is-done", "is-failed");
  markAgentStep("profile", "running");
  setRunWaitingForBenchmark();
  renderResumeEvidence(profile);
  renderAbilityRadar(profile);

  fetch(`/api/diagnosis/${encodeURIComponent(currentJobId)}/benchmark`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ items })
  })
    .then((response) => {
      if (!response.ok) {
        const error = new Error("Item Benchmark request failed");
        error.status = response.status;
        throw error;
      }
      return response.json();
    })
    .then((payload) => {
      if (!payload.ability_profile) return;
      diagnosis = diagnosis || createDiagnosisShell();
      diagnosis.ability_profile = payload.ability_profile;
      renderBasicInfo(diagnosis);
      renderAbilityRadar(payload.ability_profile);
      renderResumeEvidence(payload.ability_profile);
      if (hasMeaningfulMatchingResult(payload.matching_result)) {
        diagnosis.matching_result = payload.matching_result;
        renderMatching(payload.matching_result);
        finalizeAgentTeamStreamFromMatchingPayload(payload);
      }
      if (payload.top_jobs) {
        diagnosis.ability_profile.top5_matching_jobs = payload.top_jobs;
        renderTopJobs(payload.top_jobs);
      }
      if (payload.production_limitations) {
        diagnosis.production_limitations = payload.production_limitations;
        renderLimitations(payload.production_limitations);
      }
      reconcileRunStepsFromEventData(payload);
      if (payload.ability_profile.benchmark_status === "failed" || payload.error) {
        setBenchmarkRunFailed(payload.error);
        return;
      }
      completeRunStepsThrough("profile");
      if (payload.matching_error) {
        setMatchingRunFailed(payload.matching_error);
        return;
      }
      if (hasMeaningfulMatchingResult(diagnosis.matching_result)) {
        completeRunStepsThrough("matching");
        unlockModule("matching");
      }
      if (payload.path_error) {
        setPathRunFailed(payload.path_error);
        return;
      }
      if (hasReadyPathPlan(payload.path_plan, diagnosis.matching_result)) {
        diagnosis.path_plan = payload.path_plan;
        renderPath(payload.path_plan);
        finalizePathPlanningStreamFromPathPayload(payload);
        completeRunStepsThrough("path");
        unlockModule("path");
      } else if (payload.path_plan || payload.match_generated || payload.matching_result) {
        lockModule("path");
      }
      if (baseJobDone) setRunDone();
    })
    .catch((error) => {
      if (!diagnosis?.ability_profile) return;
      diagnosis.ability_profile.benchmark_status = error?.status === 404 ? "unavailable" : "failed";
      renderResumeEvidence(diagnosis.ability_profile);
      renderAbilityRadar(diagnosis.ability_profile);
      setBenchmarkRunFailed();
    })
    .finally(() => {
      benchmarkRequestInFlight = false;
      if (baseJobDone && !failedRunStep) setRunDone();
      if (baseJobDone && !assistantAgentStreamActive) closeDiagnosisEvents();
      updateAssistantContext();
    });
}

function shouldAutoRequestItemBenchmark(profile, event = {}) {
  if (!profile || benchmarkRequestInFlight) return false;
  if (!isBenchmarkGateOpen(profile)) return false;
  const status = profile.benchmark_status || "";
  if (isBenchmarkActiveStatus(status)) return false;
  if (["ready", "failed", "mock", "unavailable"].includes(status)) return false;
  if (event.step === "profile" && event.status === "running" && profile.major_baseline_status === "benchmarking") return false;
  return buildBenchmarkItems(profile).length > 0;
}

function finalizeAgentTeamStreamFromMatchingPayload(payload = {}) {
  if (!payload.matching_result && !payload.match_generated) return;
  const session = activeAssistantSession();
  if (!session) return;
  const hasAgentTeamStream = session.messages.some((item) => item.streamType === "agent_team");
  if (!hasAgentTeamStream) return;
  const selected = payload.matching_result?.selected_job || {};
  const preview = payload.matching_result?.fit_summary || selected.fit_summary || selected.title || "";
  const event = normalizeAgentTeamEvent({
    agent_key: "synthesis_arbiter",
    agent: "Synthesis Arbiter",
    status: "done",
    phase: "final_synthesis",
    message: "Synthesis Arbiter 已返回结构化岗位匹配结果。",
    output_preview: preview
  });
  const { message } = ensureAgentTeamStreamMessage(session, event);
  message.agentStream = updateAgentTeamStreamState(message.agentStream, event);
  message.content = agentTeamStreamFallback(message.agentStream);
  message.status = "done";
  message.updatedAt = event.time;
  finalizeLegacySynthesisStreams(session, event);
  session.updatedAt = event.time;
  assistantAgentStreamActive = false;
  saveAssistantState();
  renderAssistant();
}

function finalizeLegacySynthesisStreams(session, event) {
  session.messages.forEach((message) => {
    const stream = message.agentStream;
    if (message.streamType !== "agent_team" || !stream || !stream.agents) return;
    if (stream.workflow === "resume/path_planning") return;
    const hasSynthesisAgent = Boolean(stream.agents.synthesis_arbiter);
    if (!hasSynthesisAgent && stream.phaseGroup !== "synthesis") return;
    upsertAgentStreamAgent(stream, event);
    const item = stream.agents.synthesis_arbiter;
    if (!item) return;
    item.status = "done";
    item.message = event.message || "Synthesis Arbiter 已返回结构化岗位匹配结果。";
    item.typingDone = true;
    if (event.outputPreview) item.outputPreview = event.outputPreview;
    if (item.outputPreview && !item.typedOutput) item.typedOutput = item.outputPreview;
    if (stream.phaseGroup === "synthesis") {
      stream.status = "done";
      stream.summary = item.message;
      message.status = "done";
      message.content = agentTeamStreamFallback(stream);
    }
    message.updatedAt = event.time;
  });
}

function finalizePathPlanningStreamFromPathPayload(payload = {}) {
  if (!payload.path_plan) return;
  const session = activeAssistantSession();
  if (!session) return;
  const hasPathStream = session.messages.some((item) => item.streamType === "agent_team" && item.agentStream?.workflow === "resume/path_planning");
  if (!hasPathStream) return;
  const preview = pathPlanAgentPreview(payload.path_plan);
  const event = normalizeAgentTeamEvent({
    workflow: "resume/path_planning",
    agent_key: "path_synthesis_arbiter",
    agent: "Path Synthesis Arbiter",
    status: "done",
    phase: "final_synthesis",
    perspective: "path_plan_synthesis",
    message: "Path Synthesis Arbiter 已返回结构化路径规划结果。",
    output_preview: preview
  });
  const { message } = ensureAgentTeamStreamMessage(session, event);
  message.agentStream = updateAgentTeamStreamState(message.agentStream, event);
  message.content = agentTeamStreamFallback(message.agentStream);
  message.status = "done";
  message.updatedAt = event.time;
  finalizePathPlanningStreams(session, event);
  session.updatedAt = event.time;
  assistantAgentStreamActive = false;
  saveAssistantState();
  renderAssistant();
}

function finalizePathPlanningStreams(session, event) {
  session.messages.forEach((message) => {
    const stream = message.agentStream;
    if (message.streamType !== "agent_team" || !stream || !stream.agents) return;
    if (stream.workflow !== "resume/path_planning") return;
    upsertAgentStreamAgent(stream, event);
    const item = stream.agents.path_synthesis_arbiter;
    if (!item) return;
    item.status = "done";
    item.message = event.message || "Path Synthesis Arbiter 已返回结构化路径规划结果。";
    item.typingDone = true;
    if (event.outputPreview) item.outputPreview = event.outputPreview;
    if (item.outputPreview && !item.typedOutput) item.typedOutput = item.outputPreview;
    if (stream.phaseGroup === "synthesis") {
      stream.status = "done";
      stream.summary = item.message;
      message.status = "done";
      message.content = agentTeamStreamFallback(stream);
    }
    message.updatedAt = event.time;
  });
}

function pathPlanAgentPreview(plan = {}) {
  const stages = Array.isArray(plan.stages) ? plan.stages : [];
  const summary = plan.summary || plan.method_summary || "";
  const payload = {
    path_plan_summary: summary || "Path Planning Team 已生成路径规划。",
    stage_count: stages.length,
    stages: stages.slice(0, 4).map((stage) => ({
      title: stage.title || stage.name || "",
      goal: stage.goal || stage.objective || "",
      deliverable: stage.deliverable || "",
      standards: Array.isArray(stage.standards) ? stage.standards.slice(0, 3) : []
    }))
  };
  return JSON.stringify(payload);
}

function setBenchmarkRunFailed(errorMessage = "") {
  failedRunStep = "profile";
  benchmarkRequestInFlight = false;
  markAgentStep("profile", "failed");
  setRunStepRetryable("profile", true);
  document.querySelector("#runStatus").textContent = "失败：能力画像";
  setRunDetail(errorMessage
    ? `Item Benchmark 失败：${errorMessage}。点击下方红色“画像”继续。`
    : "Item Benchmark 失败，点击下方红色“画像”从失败处继续。", { immediate: true });
  document.querySelector(".generation-dock").classList.remove("is-running");
  runButton.disabled = false;
  runButton.textContent = "重新生成";
  setAssistantExpanded(false, { silent: true });
  syncAssistantAvailability();
}

function setMatchingRunFailed(errorMessage = "") {
  failedRunStep = "matching";
  matchingRequestInFlight = false;
  assistantAgentStreamActive = false;
  const assistantFailure = errorMessage
    ? `Job Matching 失败：${errorMessage}`
    : "Job Matching 失败，岗位匹配模块未生成。";
  markAgentStep("matching", "failed");
  setRunStepRetryable("matching", true);
  document.querySelector("#runStatus").textContent = "失败：岗位匹配";
  setRunDetail(errorMessage
    ? `Job Matching 失败：${errorMessage}。点击下方红色“匹配”继续。`
    : "Job Matching 失败，点击下方红色“匹配”从失败处继续。", { immediate: true });
  markAssistantMatchingStreamsFailed(assistantFailure);
  document.querySelector(".generation-dock").classList.remove("is-running");
  runButton.disabled = false;
  runButton.textContent = "重新生成";
  syncAssistantAvailability();
}

function setPathRunFailed(errorMessage = "") {
  failedRunStep = "path";
  pathRequestInFlight = false;
  assistantAgentStreamActive = false;
  markAgentStep("path", "failed");
  setRunStepRetryable("path", true);
  document.querySelector("#runStatus").textContent = "失败：路径规划";
  setRunDetail(errorMessage
    ? `Path Planning 失败：${errorMessage}。岗位匹配结果已保留，点击下方红色“路径”继续。`
    : "Path Planning 失败，路径规划模块未生成，岗位匹配结果已保留。点击下方红色“路径”从失败处继续。", { immediate: true });
  markAssistantPathPlanningStreamsFailed(errorMessage
    ? `Path Planning 失败：${errorMessage}`
    : "Path Planning 失败，路径规划模块未生成。");
  document.querySelector(".generation-dock").classList.remove("is-running");
  runButton.disabled = false;
  runButton.textContent = "重新生成";
  syncAssistantAvailability();
}

function markAssistantPathPlanningStreamsFailed(messageText = "") {
  const session = activeAssistantSession();
  if (!session) return;
  const now = new Date().toISOString();
  const summary = formatAgentDisplayText(messageText || "Path Planning 失败，路径规划模块未生成。");
  const failureEvent = normalizeAgentTeamEvent({
    workflow: "resume/path_planning",
    agent_key: "path_synthesis_arbiter",
    agent: "Path Synthesis Arbiter",
    status: "failed",
    phase: "final_synthesis",
    message: summary
  });
  failureEvent.time = now;
  let changed = false;
  stopAgentTypewriters();
  session.messages.forEach((sessionMessage) => {
    const stream = sessionMessage.agentStream;
    if (sessionMessage.streamType !== "agent_team" || !stream?.agents) return;
    if (stream.workflow !== "resume/path_planning") return;
    if (stream.phaseGroup === "synthesis" || stream.agents.path_synthesis_arbiter) {
      upsertAgentStreamAgent(stream, failureEvent);
    }
    Object.keys(stream.agents).forEach((key) => {
      const agent = stream.agents[key];
      if (!agent || agent.status === "done") return;
      agent.status = "failed";
      agent.message = summary;
      agent.typingDone = true;
    });
    stream.status = stream.status === "done" ? "done" : "failed";
    stream.summary = summary;
    stream.updatedAt = now;
    sessionMessage.status = stream.status === "done" ? "done" : "error";
    sessionMessage.content = agentTeamStreamFallback(stream);
    sessionMessage.updatedAt = now;
    changed = true;
  });
  if (!changed) return;
  session.updatedAt = now;
  saveAssistantState();
  renderAssistant();
}

function markAssistantMatchingStreamsFailed(messageText = "") {
  const session = activeAssistantSession();
  if (!session) return;
  const now = new Date().toISOString();
  const summary = formatAgentDisplayText(messageText || "Job Matching 失败，岗位匹配模块未生成。");
  const failureEvent = normalizeAgentTeamEvent({
    agent_key: "synthesis_arbiter",
    agent: "Synthesis Arbiter",
    status: "failed",
    phase: "final_synthesis",
    message: summary
  });
  failureEvent.time = now;
  let changed = false;
  let sawAgentStream = false;
  let sawSynthesisStream = false;
  stopAgentTypewriters();
  session.messages.forEach((sessionMessage) => {
    const stream = sessionMessage.agentStream;
    if (sessionMessage.streamType !== "agent_team" || !stream?.agents) return;
    if (stream.workflow === "resume/path_planning") return;
    sawAgentStream = true;
    if (stream.phaseGroup === "synthesis" || stream.agents.synthesis_arbiter) sawSynthesisStream = true;
    if (stream.status === "done" && stream.phaseGroup !== "synthesis") return;
    if (stream.phaseGroup === "synthesis" || stream.agents.synthesis_arbiter) {
      upsertAgentStreamAgent(stream, failureEvent);
    }
    Object.keys(stream.agents).forEach((key) => {
      const agent = stream.agents[key];
      if (!agent || agent.status === "done") return;
      agent.status = "failed";
      agent.message = summary;
      agent.typingDone = true;
      agent.updatedAt = now;
    });
    stream.status = "failed";
    stream.summary = summary;
    stream.updatedAt = now;
    sessionMessage.status = "error";
    sessionMessage.content = agentTeamStreamFallback(stream);
    sessionMessage.updatedAt = now;
    changed = true;
  });
  if (sawAgentStream && !sawSynthesisStream) {
    const { message } = ensureAgentTeamStreamMessage(session, failureEvent);
    message.agentStream = updateAgentTeamStreamState(message.agentStream, failureEvent);
    message.content = agentTeamStreamFallback(message.agentStream);
    message.status = "error";
    message.updatedAt = now;
    changed = true;
  }
  if (!changed) return;
  session.updatedAt = now;
  assistantAgentStreamActive = false;
  saveAssistantState();
  renderAssistant();
}

function retryItemBenchmark() {
  if (!currentJobId || !diagnosis?.ability_profile) {
    showToast("没有可继续的诊断任务，请重新生成。");
    return;
  }
  diagnosis.ability_profile.benchmark_status = "waiting";
  setRunStepRetryable("profile", false);
  showToast("正在从 Item Benchmark 继续。");
  maybeRequestItemBenchmark(diagnosis.ability_profile, { force: true });
}

function retryJobMatching() {
  if (!currentJobId || !diagnosis?.ability_profile) {
    showToast("没有可继续的诊断任务，请重新生成。");
    return;
  }
  if (!isMatchingGateOpen(diagnosis.ability_profile)) {
    showToast("能力画像尚未完成 Benchmark，暂不能启动岗位匹配。");
    return;
  }
  if (matchingRequestInFlight) return;
  matchingRequestInFlight = true;
  pathRequestInFlight = false;
  failedRunStep = "";
  assistantAgentStreamActive = false;
  setRunStepRetryable("matching", false);
  document.querySelector(`[data-run-step="matching"]`)?.classList.remove("is-done", "is-failed");
  document.querySelector(`[data-run-step="path"]`)?.classList.remove("is-done", "is-failed");
  document.querySelector(`[data-run-step="outputs"]`)?.classList.remove("is-done", "is-failed");
  markAgentStep("matching", "running");
  lockModule("matching");
  lockModule("path");
  lockModule("outputs");
  document.querySelector("#runStatus").textContent = "生成中";
  setRunDetail("正在从岗位匹配继续，Legato Job Matching Team 正在重新生成推荐。", { immediate: true });
  document.querySelector(".generation-dock").classList.add("is-running");
  runButton.disabled = true;
  runButton.textContent = "生成中";

  fetch(`/api/diagnosis/${encodeURIComponent(currentJobId)}/matching`, {
    method: "POST"
  })
    .then((response) => {
      if (!response.ok) {
        const error = new Error("Job Matching request failed");
        error.status = response.status;
        throw error;
      }
      return response.json();
    })
    .then((payload) => {
      diagnosis = diagnosis || createDiagnosisShell();
      if (payload.ability_profile) {
        diagnosis.ability_profile = payload.ability_profile;
        renderBasicInfo(diagnosis);
        renderAbilityRadar(payload.ability_profile);
        renderResumeEvidence(payload.ability_profile);
      }
      if (payload.production_limitations) {
        diagnosis.production_limitations = payload.production_limitations;
        renderLimitations(payload.production_limitations);
      }
      if (payload.matching_error || payload.error) {
        setMatchingRunFailed(payload.matching_error || payload.error);
        return;
      }
      if (payload.matching_result) {
        diagnosis.matching_result = payload.matching_result;
        renderMatching(payload.matching_result);
        finalizeAgentTeamStreamFromMatchingPayload(payload);
      }
      if (payload.top_jobs) {
        diagnosis.ability_profile.top5_matching_jobs = payload.top_jobs;
        renderTopJobs(payload.top_jobs);
      }
      reconcileRunStepsFromEventData(payload);
      if (hasMeaningfulMatchingResult(diagnosis.matching_result)) {
        completeRunStepsThrough("matching");
        unlockModule("matching");
      } else {
        setMatchingRunFailed("Job Matching 未返回有效匹配结果。");
        return;
      }
      if (payload.path_error || payload.error) {
        setPathRunFailed(payload.path_error || payload.error);
        return;
      }
      if (hasReadyPathPlan(payload.path_plan, diagnosis.matching_result)) {
        diagnosis.path_plan = payload.path_plan;
        renderPath(payload.path_plan);
        finalizePathPlanningStreamFromPathPayload(payload);
        completeRunStepsThrough("path");
        unlockModule("path");
      } else {
        lockModule("path");
      }
      if (baseJobDone) setRunDone();
    })
    .catch((error) => {
      setMatchingRunFailed(error?.status === 409 ? "Benchmark 仍在运行，暂不能重跑岗位匹配" : "");
    })
    .finally(() => {
      matchingRequestInFlight = false;
      if (baseJobDone && !failedRunStep) setRunDone();
      updateAssistantContext();
      renderAssistantSuggestions();
    });
}

function retryPathPlanning() {
  if (!currentJobId || !diagnosis?.matching_result) {
    showToast("没有可继续的岗位匹配结果，请先完成岗位匹配。");
    return;
  }
  if (!hasMeaningfulMatchingResult(diagnosis.matching_result)) {
    showToast("岗位匹配结果尚未完成，暂不能启动路径规划。");
    return;
  }
  if (pathRequestInFlight) return;
  pathRequestInFlight = true;
  failedRunStep = "";
  assistantAgentStreamActive = false;
  setRunStepRetryable("path", false);
  document.querySelector(`[data-run-step="path"]`)?.classList.remove("is-done", "is-failed");
  document.querySelector(`[data-run-step="outputs"]`)?.classList.remove("is-done", "is-failed");
  markAgentStep("path", "running");
  unlockModule("matching");
  lockModule("path");
  lockModule("outputs");
  document.querySelector("#runStatus").textContent = "生成中";
  setRunDetail("正在从路径规划继续，Legato Path Planning Team 正在重新生成阶段目标和周任务。", { immediate: true });
  document.querySelector(".generation-dock").classList.add("is-running");
  runButton.disabled = true;
  runButton.textContent = "生成中";

  fetch(`/api/diagnosis/${encodeURIComponent(currentJobId)}/path`, {
    method: "POST"
  })
    .then((response) => {
      if (!response.ok) {
        const error = new Error("Path Planning request failed");
        error.status = response.status;
        throw error;
      }
      return response.json();
    })
    .then((payload) => {
      diagnosis = diagnosis || createDiagnosisShell();
      if (payload.ability_profile) {
        diagnosis.ability_profile = payload.ability_profile;
        renderBasicInfo(diagnosis);
        renderAbilityRadar(payload.ability_profile);
        renderResumeEvidence(payload.ability_profile);
      }
      if (payload.matching_result) {
        diagnosis.matching_result = payload.matching_result;
        renderMatching(payload.matching_result);
      }
      if (payload.top_jobs) {
        diagnosis.ability_profile.top5_matching_jobs = payload.top_jobs;
        renderTopJobs(payload.top_jobs);
      }
      if (payload.production_limitations) {
        diagnosis.production_limitations = payload.production_limitations;
        renderLimitations(payload.production_limitations);
      }
      if (payload.path_error || payload.error) {
        setPathRunFailed(payload.path_error || payload.error);
        return;
      }
      if (!hasReadyPathPlan(payload.path_plan, diagnosis.matching_result)) {
        setPathRunFailed("Path Planning 未返回有效阶段目标。");
        return;
      }
      diagnosis.path_plan = payload.path_plan;
      renderPath(payload.path_plan);
      finalizePathPlanningStreamFromPathPayload(payload);
      completeRunStepsThrough("path");
      unlockModule("path");
      markAgentStep("outputs", "done");
      unlockModule("outputs");
      setRunDone();
    })
    .catch((error) => {
      setPathRunFailed(error?.status === 409 ? "岗位匹配尚未完成，暂不能重跑路径规划" : "");
    })
    .finally(() => {
      pathRequestInFlight = false;
      if (!failedRunStep && canMarkRunDone()) setRunDone();
      updateAssistantContext();
      renderAssistantSuggestions();
    });
}

function isBenchmarkGateOpen(profile) {
  if (!profile) return false;
  const awardStatus = profile.awards_status || (Array.isArray(profile.awards) && profile.awards.length ? "ready" : "waiting");
  const experienceStatus = profile.experiences_status || (Array.isArray(profile.experiences) && profile.experiences.length ? "ready" : "waiting");
  return isResumeProfileReady(profile) && isBenchmarkEvidenceTerminal(awardStatus) && isBenchmarkEvidenceTerminal(experienceStatus);
}

function isMatchingGateOpen(profile) {
  return isBenchmarkGateOpen(profile)
    && profile?.benchmark_status === "ready"
    && profile?.major_baseline_status === "ready";
}

function isResumeProfileReady(profile) {
  const info = profile?.basic_info || {};
  const status = cleanDisplayText(info.resume_status || "");
  if (!status || status.includes("等待")) return false;
  return Boolean(
    cleanDisplayText(info.name || "") ||
    cleanDisplayText(info.school || "") ||
    cleanDisplayText(info.major || "") ||
    (Array.isArray(profile?.education) && profile.education.length > 0)
  );
}

function isBenchmarkEvidenceTerminal(status) {
  return ["ready", "empty"].includes(status || "");
}

function buildBenchmarkItems(profile, options = {}) {
  const awards = Array.isArray(profile.awards) ? profile.awards : [];
  const experiences = Array.isArray(profile.experiences) ? profile.experiences : [];
  const awardItems = awards.map((item, index) => ({
    source: item,
    payload: {
      kind: "award",
      key: `award:${index}`,
      name: item.name || "",
      result: item.result || "",
      evidence_scope: normalizedEvidenceScope(item),
      level: numericMetric(item.level)
    }
  }));
  const experienceItems = experiences.map((item, index) => ({
    source: item,
    payload: {
      kind: "experience",
      key: `experience:${index}`,
      name: item.role || item.contribution || item.type || "",
      result: "",
      experience_type: item.type || "",
      role: item.role || "",
      contribution: item.contribution || "",
      evidence_scope: normalizedEvidenceScope(item),
      level: numericMetric(item.level)
    }
  }));
  return [...awardItems, ...experienceItems]
    .filter((entry) => !options.missingOnly || !hasItemBenchmarkResult(entry.source))
    .map((entry) => entry.payload)
    .filter((item) => item.name || item.result || item.role || item.contribution);
}

function renderResumeEvidence(profile) {
  profile = profile || {};
  const awards = profile.awards || [];
  const experiences = profile.experiences || [];
  const awardStatus = profile.awards_status || (awards.length ? "ready" : "waiting");
  const experienceStatus = profile.experiences_status || (experiences.length ? "ready" : "waiting");
  const benchmarkStatus = profile.benchmark_status || "waiting";
  const isAwardLoading = isEvidenceLoadingStatus(awardStatus);
  const isExperienceRefining = experienceStatus === "refining";
  const hasBenchmarkableEvidence = benchmarkableEvidenceItems(profile).length > 0;
  const hasUnbenchmarkedEvidence = unbenchmarkedEvidenceItems(profile).length > 0;
  const isBenchmarkQueued = hasBenchmarkableEvidence
    && hasUnbenchmarkedEvidence
    && !["failed", "mock", "unavailable"].includes(benchmarkStatus);
  const isBenchmarkLoading = (isBenchmarkActiveStatus(benchmarkStatus) || isBenchmarkQueued) && hasBenchmarkableEvidence;
  const isBenchmarkFailed = benchmarkStatus === "failed";

  setEvidencePill("#awardDataState", awardStatus, {
    waiting: "等待奖项 Agent",
    loading: "奖项解析中",
    refining: "奖项解析中",
    ready: "",
    empty: "未识别到奖项",
    failed: "奖项解析失败",
    mock: "模拟数据"
  });
  setEvidencePill("#experienceDataState", experienceStatus, {
    waiting: "等待经历 Agent",
    ready: "",
    refining: "hybrid 分析中",
    empty: "未识别到经历",
    failed: "经历解析失败",
    mock: "模拟数据"
  });

  const overallStatus = evidenceOverallStatus(awardStatus, experienceStatus, awards.length, experiences.length);
  const evidenceState = isBenchmarkLoading
    ? "benchmarking"
    : benchmarkStatus === "ready"
      ? "benchmark_ready"
      : benchmarkStatus === "unavailable"
        ? "benchmark_unavailable"
      : benchmarkStatus === "failed"
        ? "failed"
        : overallStatus;
  setEvidencePill("#resumeEvidenceState", evidenceState, {
    waiting: "等待结构化数据",
    benchmarking: "Benchmark 评分中",
    benchmark_ready: "Benchmark 已完成",
    benchmark_unavailable: "Benchmark 未启用",
    ready: "已有结构化证据",
    empty: "无可用证据",
    failed: "部分解析失败",
    mock: "模拟数据"
  });

  const awardList = document.querySelector("#awardList");
  awardList.classList.toggle("is-loading", isAwardLoading);
  const awardSkeleton = isAwardLoading && awards.length === 0 ? renderAwardLoadingSkeleton(false) : "";
  awardList.innerHTML = awards.length
    ? awards.map((item, index) => renderAwardItem(item, isAwardLoading, isBenchmarkLoading, isBenchmarkFailed, index)).join("")
    : awardSkeleton || renderEvidenceEmpty(awardStatus, "等待 Resume workflow 返回奖项与证书。", "Legato 未识别到奖项或证书。", "奖项 Agent 未返回可用结果。");

  const experienceList = document.querySelector("#experienceList");
  experienceList.classList.toggle("is-refining", isExperienceRefining);
  const hybridSkeleton = isExperienceRefining && experiences.length === 0 ? renderExperienceHybridSkeleton(false) : "";
  experienceList.innerHTML = experiences.length
    ? experiences.map((item, index) => renderExperienceItem(item, isExperienceItemRefining(item, isExperienceRefining), isBenchmarkLoading, isBenchmarkFailed, index)).join("")
    : hybridSkeleton || renderEvidenceEmpty(experienceStatus, "等待 Resume workflow 返回经历评分。", "Legato 未识别到项目、实习或活动经历。", "经历 Agent 未返回可用结果。");
  syncAssistantAvailability();
}

function renderAwardItem(item, loading = false, benchmarkLoading = false, benchmarkFailed = false, index = 0) {
  const title = cleanDisplayText(item.name);
  const result = cleanDisplayText(item.result);
  return `
    <article class="evidence-item${loading ? " is-loading" : ""}${benchmarkLoading && !hasImpactFactor(item) ? " is-benchmarking" : ""}${benchmarkFailed && !hasImpactFactor(item) ? " is-benchmark-failed" : ""}">
      <header>
        <div>
          ${title ? `<strong>${escapeHTML(title)}</strong>` : ""}
          ${result && result !== title ? `<span>${escapeHTML(result)}</span>` : ""}
          ${renderEvidenceScopeTag(item)}
        </div>
        ${renderEvidenceScorePair(item, benchmarkLoading, loading, benchmarkFailed)}
      </header>
      ${renderDimensionMeter(item, benchmarkLoading, benchmarkFailed)}
      ${renderEvidenceReason(item.signal, item.reason)}
      ${renderEvidenceChatButton("award", index, title || result || "奖项")}
    </article>
  `;
}

function renderExperienceItem(item, refining = false, benchmarkLoading = false, benchmarkFailed = false, index = 0) {
  const type = cleanDisplayText(item.type);
  const role = cleanDisplayText(item.role);
  const contribution = cleanDisplayText(item.contribution);
  const title = [type, role].filter(Boolean).join(" · ") || contribution;
  const subtitle = title && contribution && contribution !== title && contribution !== role ? contribution : "";
  return `
    <article class="evidence-item${refining ? " is-refining" : ""}${benchmarkLoading && !hasImpactFactor(item) ? " is-benchmarking" : ""}${benchmarkFailed && !hasImpactFactor(item) ? " is-benchmark-failed" : ""}">
      <header>
        <div>
          ${title ? `<strong>${escapeHTML(title)}</strong>` : ""}
          ${subtitle ? `<span>${escapeHTML(subtitle)}</span>` : ""}
          ${renderEvidenceScopeTag(item)}
        </div>
        ${renderEvidenceScorePair(item, benchmarkLoading, refining, benchmarkFailed)}
      </header>
      ${renderDimensionMeter(item, benchmarkLoading, benchmarkFailed)}
      ${renderEvidenceReason("", item.reason)}
      ${renderEvidenceChatButton("experience", index, title || "经历")}
    </article>
  `;
}

function renderEvidenceChatButton(kind, index, label) {
  const available = isAssistantInspectable();
  const aria = `添加到聊天：${kind === "award" ? "奖项" : "经历"} ${label}`;
  return `
    <button
      type="button"
      class="evidence-chat-button"
      data-evidence-chat="${escapeAttribute(kind)}"
      data-evidence-index="${index}"
      data-evidence-key="${escapeAttribute(`${kind}:${index}`)}"
      aria-label="${escapeAttribute(aria)}"
      aria-pressed="false"
      title="${available ? "添加到聊天" : "生成诊断后可添加到聊天"}"
      ${available ? "" : "disabled"}
    >
      <span aria-hidden="true">+</span>
      <strong>添加到聊天</strong>
    </button>
  `;
}

function renderResultChatButton(kind, index = "", label = "生成结果", subindex = "") {
  const available = isAssistantInspectable();
  const key = resultContextKey(kind, index, subindex);
  const selected = isFocusedContextSelected(key);
  const variantClass = kind === "path_task" ? " is-path-task-chat" : "";
  return `
    <button
      type="button"
      class="evidence-chat-button context-chat-button${variantClass}${selected ? " is-added" : ""}"
      data-result-chat="${escapeAttribute(kind)}"
      data-result-index="${escapeAttribute(index)}"
      data-result-subindex="${escapeAttribute(subindex)}"
      data-context-key="${escapeAttribute(key)}"
      aria-label="添加到聊天：${escapeAttribute(label)}"
      aria-pressed="${selected ? "true" : "false"}"
      title="${available ? selected ? "已加入聊天上下文" : "添加到聊天" : "生成诊断后可添加到聊天"}"
      ${available ? "" : "disabled"}
    >
      <span aria-hidden="true">+</span>
      <strong>添加到聊天</strong>
    </button>
  `;
}

function resultContextKey(kind, index = "", subindex = "") {
  return [kind, index, subindex].filter((value) => value !== "" && value !== undefined && value !== null).join(":");
}

function isExperienceItemRefining(item, listRefining) {
  if (!listRefining) return false;
  const status = item?.hybrid_status || "pending";
  return !["ready", "failed"].includes(status);
}

function renderEvidenceReason(signal, reason) {
  const cleanSignal = cleanDisplayText(signal);
  const cleanReason = cleanDisplayText(reason);
  if (cleanSignal && cleanReason) {
    return `<p><strong>${escapeHTML(cleanSignal)}：</strong>${escapeHTML(cleanReason)}</p>`;
  }
  if (cleanReason) return `<p>${escapeHTML(cleanReason)}</p>`;
  if (cleanSignal) return `<p><strong>${escapeHTML(cleanSignal)}</strong></p>`;
  return "";
}

function renderEvidenceScopeTag(item) {
  return `
    <div class="evidence-scope-row">
      <span class="${scopeBadgeClass(item)}">${escapeHTML(normalizedEvidenceScope(item))}</span>
    </div>
  `;
}

function renderEvidenceScorePair(item, benchmarkLoading, levelLoading = false, benchmarkFailed = false) {
  const level = numericMetric(item.level);
  const impact = numericMetric(item.impact_factor);
  return `
    <div class="evidence-score-pair" aria-label="证据双评分">
      ${renderMetricChip("Level", level, levelLoading)}
      ${renderMetricChip("Impact", impact, benchmarkLoading, benchmarkFailed && !hasMetricValue(impact))}
    </div>
  `;
}

function renderMetricChip(label, value, loading, failed = false) {
  if (failed && !hasMetricValue(value)) {
    return `
      <div class="metric-chip is-failed" aria-label="${escapeAttribute(label)} 评分失败">
        <span>${escapeHTML(label)}</span>
        <b>失败</b>
      </div>
    `;
  }
  if (loading && !hasMetricValue(value)) {
    return `
      <div class="metric-chip is-loading" aria-label="${escapeAttribute(label)} 正在评分">
        <span>${escapeHTML(label)}</span>
        <b></b>
      </div>
    `;
  }
  if (!hasMetricValue(value)) {
    return `
      <div class="metric-chip is-empty" aria-label="${escapeAttribute(label)} 未返回评分">
        <span>${escapeHTML(label)}</span>
        <b>未返回</b>
      </div>
    `;
  }
  return `
    <div class="metric-chip">
      <span>${escapeHTML(label)}</span>
      <b>${formatMetric(value)}</b>
      <small>/10</small>
    </div>
  `;
}

function renderDimensionMeter(item, benchmarkLoading, benchmarkFailed = false) {
  const scores = normalizedBenchmarkScores(item);
  if (scores.length === 0) {
    if (benchmarkFailed) {
      return `
        <div class="dimension-meter is-failed" aria-label="六维 Benchmark 评分失败">
          <span class="dimension-failed"></span>
        </div>
      `;
    }
    if (!benchmarkLoading) {
      return `
        <div class="dimension-meter is-empty" aria-label="六维 Benchmark 未返回评分">
          <span class="dimension-empty"></span>
        </div>
      `;
    }
    return `
      <div class="dimension-meter is-loading" aria-label="六维 Benchmark 正在评分">
        <span class="dimension-skeleton"></span>
      </div>
    `;
  }
  const dimensions = Array.isArray(item.benchmark_dimensions) && item.benchmark_dimensions.length === scores.length
    ? item.benchmark_dimensions
    : benchmarkDimensions;
  const visualShares = visualBenchmarkShares(scores);
  return `
    <div class="dimension-meter" aria-label="${escapeAttribute(dimensions.map((name, index) => `${name}${Math.round(scores[index] * 100)}%`).join("，"))}">
      ${scores.map((score, index) => {
        const tooltip = dimensionTooltip(dimensions[index] || benchmarkDimensions[index] || "维度", score);
        return `
        <span
          class="dimension-segment dim-${index}"
          style="--share:${(visualShares[index] * 100).toFixed(4)}%; --segment-delay:${index * 22}ms"
          tabindex="0"
          role="img"
          aria-label="${escapeAttribute(tooltip)}"
          data-tooltip="${escapeAttribute(tooltip)}"
        ></span>
      `;
      }).join("")}
    </div>
  `;
}

function dimensionTooltip(name, score) {
  return `${name} ${Math.round(score * 100)}%`;
}

function normalizedBenchmarkScores(item) {
  const raw = Array.isArray(item?.benchmark_scores) ? item.benchmark_scores : Array.isArray(item?.scores) ? item.scores : [];
  const values = raw.slice(0, 6).map((value) => {
    const number = Number(value);
    return Number.isFinite(number) && number > 0 ? Math.min(number, 1) : 0;
  });
  const total = values.reduce((sum, value) => sum + value, 0);
  if (values.length !== 6 || total <= 0) return [];
  return values.map((value) => value / total);
}

function visualBenchmarkShares(scores) {
  const floor = 0.018;
  const remaining = Math.max(0, 1 - floor * scores.length);
  const shares = scores.map((score) => floor + score * remaining);
  const total = shares.reduce((sum, value) => sum + value, 0);
  if (total <= 0) return scores.map(() => 1 / scores.length);
  return shares.map((value) => value / total);
}

function hasImpactFactor(item) {
  return hasMetricValue(numericMetric(item?.impact_factor));
}

function hasBenchmarkScores(item) {
  return normalizedBenchmarkScores(item).length === 6;
}

function hasItemBenchmarkResult(item) {
  return hasImpactFactor(item) && hasBenchmarkScores(item);
}

function hasMetricValue(value) {
  return Number.isFinite(value) && value >= 0;
}

function numericMetric(value) {
  if (value === null || value === undefined || value === "") return NaN;
  const number = Number(value);
  return Number.isFinite(number) ? number : NaN;
}

function formatMetric(value) {
  const rounded = Math.round(value * 10) / 10;
  return Number.isInteger(rounded) ? String(rounded) : rounded.toFixed(1);
}

function renderAwardLoadingSkeleton(subtle) {
  return `
    <div class="evidence-loading award-loading ${subtle ? "is-subtle" : ""}" role="status" aria-label="奖项与证书正在结构化解析">
      <span class="loading-line is-wide"></span>
      <span class="loading-line"></span>
      <span class="loading-line is-short"></span>
    </div>
  `;
}

function renderExperienceHybridSkeleton(subtle) {
  return `
    <div class="evidence-loading hybrid-loading ${subtle ? "is-subtle" : ""}" role="status" aria-label="experience_hybrid 正在进行高精度经历分析">
      <span class="loading-line is-wide"></span>
      <span class="loading-line"></span>
      <span class="loading-line is-short"></span>
    </div>
  `;
}

function renderEvidenceEmpty(status, waitingText, emptyText, failedText) {
  if (isEvidenceLoadingStatus(status)) {
    return `
      <div class="evidence-loading" role="status" aria-label="${escapeHTML(waitingText)}">
        <span class="loading-line is-wide"></span>
        <span class="loading-line"></span>
        <span class="loading-line is-short"></span>
      </div>
    `;
  }
  const text = status === "failed" ? failedText : emptyText;
  return `<div class="evidence-empty">${escapeHTML(text)}</div>`;
}

function isEvidenceLoadingStatus(status) {
  return ["waiting", "loading", "running", "refining"].includes(status || "waiting");
}

function isBenchmarkLoadingStatus(status) {
  return ["waiting", "loading", "running", "benchmarking"].includes(status || "waiting");
}

function isBenchmarkActiveStatus(status) {
  return ["loading", "running", "benchmarking"].includes(status || "");
}

function evidenceOverallStatus(awardStatus, experienceStatus, awardCount, experienceCount) {
  if (awardStatus === "mock" || experienceStatus === "mock") return "mock";
  if (awardCount > 0 || experienceCount > 0) return "ready";
  if (awardStatus === "failed" || experienceStatus === "failed") return "failed";
  if ((awardStatus === "empty" || awardStatus === "failed") && (experienceStatus === "empty" || experienceStatus === "failed")) return "empty";
  return "waiting";
}

function setEvidencePill(selector, status, labels) {
  const element = document.querySelector(selector);
  if (!element) return;
  const normalized = status || "waiting";
  const label = Object.prototype.hasOwnProperty.call(labels, normalized) ? labels[normalized] : labels.waiting;
  if (label === "") {
    element.hidden = true;
    element.textContent = "";
    return;
  }
  element.hidden = false;
  element.textContent = label;
  element.className = `status-pill ${statusPillClass(normalized)}`;
}

function statusPillClass(status) {
  if (status === "ready" || status === "benchmark_ready") return "is-real";
  if (status === "mock") return "is-simulated";
  if (status === "failed") return "is-danger";
  return "is-warning";
}

function normalizedEvidenceScope(item) {
  const explicit = String(item?.evidence_scope || "").trim();
  if (explicit === "校内" || explicit === "校外") return explicit;
  const text = [
    item?.name,
    item?.result,
    item?.type,
    item?.role,
    item?.contribution
  ].join("");
  const lower = text.toLowerCase();
  if (containsAnyText(lower, ["校外", "全国", "国家级", "省级", "省", "市级", "市", "区域", "赛区", "国际", "企业", "公司", "集团", "实习", "英语四级", "英语六级", "cet", "计算机等级", "蓝桥", "acm", "icpc", "ctf", "挑战杯", "互联网+"])) {
    return "校外";
  }
  if (containsAnyText(text, ["校内", "校级", "院级", "学院", "学校", "学生会", "社团", "协会", "班级", "班长", "团支书", "优秀学生", "优秀学生干部", "三好学生", "奖学金", "实验室", "大创"])) {
    return "校内";
  }
  if (containsAnyText(text, ["项目", "科研", "研究"])) return "校内";
  return "校外";
}

function scopeBadgeClass(item) {
  return normalizedEvidenceScope(item) === "校内" ? "scope-badge is-campus" : "scope-badge is-external";
}

function containsAnyText(text, needles) {
  return needles.some((needle) => needle && text.includes(needle));
}

function pointFor(index, total, radius, center) {
  const angle = -Math.PI / 2 + (Math.PI * 2 * index) / total;
  return {
    x: center.x + Math.cos(angle) * radius,
    y: center.y + Math.sin(angle) * radius
  };
}

function renderMatching(match) {
  match = match || {};
  const selected = match.selected_job || {};
  const overall = Number.isFinite(Number(match.overall_match)) ? safeScore(match.overall_match) : 0;
  const reasons = matchingReasonList(match, selected);
  const reportRows = normalizedMatchingReportRows(match);
  const mainGap = primaryMatchingGap(match, reportRows);
  const radarSummary = matchingRadarSummary(reportRows);
  const nextProof = selected.next_proof || mainGap?.action || firstProofGap(selected) || "补充更贴近目标岗位的项目证据";
  document.querySelector("#overallMatch").textContent = overall ? `${Math.round(overall)}%` : "--";
  document.querySelector("#matchLevel").textContent = match.match_level || "等待 Job Matching";
  document.querySelector("#overallMeter").style.width = `${overall}%`;
  document.querySelector("#selectedJobTitle").textContent = selected.title || match.target_role || "等待推荐";
  document.querySelector("#matchDecisionBadges").innerHTML = renderMatchDecisionBadges(match, selected, radarSummary);
  document.querySelector("#matchNarrative").textContent = compactMatchingText(match.fit_summary || selected.fit_summary || "Benchmark 完成后，Legato 会返回首选岗位、推荐理由和需要补齐的证据。", 190);
  document.querySelector("#matchAgentNotes").innerHTML = (match.agent_notes || [])
    .slice(0, 4)
    .map((note) => `<span>${escapeHTML(compactMatchingText(note, 22))}</span>`)
    .join("");
  document.querySelector("#matchPrimaryReason").innerHTML = renderFocusReasonList(reasons, selected);
  document.querySelector("#matchMainGap").innerHTML = renderFocusGap(mainGap, radarSummary);
  document.querySelector("#matchNextProof").innerHTML = renderFocusText(nextProof);
  document.querySelector("#matchEducationGate").innerHTML = renderEducationGateChip(selected.education_gate || match.education_gate || "未返回门槛限制", selected.education_gate_status || match.education_gate_status);
  document.querySelector("#matchActionList").innerHTML = renderMatchActionList(match, selected, mainGap, radarSummary);
  renderMatchingRadar(match, reportRows);

  document.querySelector("#reportRows").innerHTML = reportRows.length ? reportRows.map((row, index) => `
    <div class="report-row is-${escapeAttribute(matchingRowStatus(row))}" style="--match-stagger:${index * 32}ms">
      <strong>${escapeHTML(row.name)}</strong>
      <div class="report-bars">
        <div class="report-bar" aria-label="学生分值 ${row.student}"><span style="width:${safeScore(row.student)}%"></span></div>
        <div class="need-bar" aria-label="岗位要求 ${row.role_need}"><span style="width:${safeScore(row.role_need)}%"></span></div>
      </div>
      <span class="report-status ${escapeAttribute(matchingRowStatus(row))}">${escapeHTML(matchingStatusLabel(matchingRowStatus(row)))}</span>
      <span class="delta">${row.difference > 0 ? "+" : ""}${row.difference}</span>
      <small>${escapeHTML(compactMatchingText(row.note || matchingRowNote(row), 46))}</small>
    </div>
  `).join("") : `<div class="matching-empty">等待六维差距报表。</div>`;

  const gaps = Array.isArray(match.gap_details) ? match.gap_details : [];
  document.querySelector("#gapCards").innerHTML = gaps.length
    ? gaps.map((gap, index) => renderGapCard(gap, index)).join("")
    : `<div class="matching-empty">等待差距明细。</div>`;
}

function matchingReasonList(match, selected) {
  const sources = [
    selected?.reasons,
    match?.reasons,
    match?.agent_notes,
    selected?.fit_summary,
    match?.fit_summary
  ];
  const values = sources.flatMap((source) => {
    if (Array.isArray(source)) return source;
    return source ? String(source).split(/[；;。]/) : [];
  });
  const seen = new Set();
  return values
    .map((item) => normalizeMatchingFocusText(item))
    .filter((item) => {
      const key = item.toLowerCase();
      if (!item || seen.has(key)) return false;
      seen.add(key);
      return true;
    })
    .slice(0, 5);
}

function primaryMatchingGap(match, reportRows = []) {
  const gaps = Array.isArray(match?.gap_details) ? match.gap_details : [];
  if (gaps.length) {
    return [...gaps].sort((left, right) => severityWeight(right.severity) - severityWeight(left.severity))[0];
  }
  const weakest = matchingRadarSummary(reportRows).weakest;
  if (!weakest) return null;
  return {
    capability: weakest.name,
    current: `${weakest.student} / ${weakest.role_need}`,
    expected: `岗位要求 ${weakest.role_need}`,
    action: weakest.note || matchingRowNote(weakest),
    severity: weakest.status === "gap" ? "高" : weakest.status === "limited" ? "中" : "低"
  };
}

function normalizedMatchingReportRows(match) {
  const explicitRows = Array.isArray(match?.report_sections) ? match.report_sections : [];
  const profileStudent = currentProfileStudentRadar();
  const target = matchingTargetRadar(match);
  if (profileStudent.length === benchmarkDimensions.length && target.length === benchmarkDimensions.length) {
    const explicitByName = new Map(explicitRows.map((row) => [row?.name || row?.capability, row]));
    return benchmarkDimensions.map((name, index) => {
      const explicit = explicitByName.get(name) || {};
      const student = profileStudent[index].score;
      const roleNeed = target[index].score;
      const difference = student - roleNeed;
      const status = matchingRowStatus({ ...explicit, difference });
      return {
        name,
        student,
        role_need: roleNeed,
        difference,
        status,
        note: explicit.note || matchingRowNote({ ...explicit, name, student, role_need: roleNeed, difference, status })
      };
    });
  }
  const rows = explicitRows.length ? explicitRows : reportRowsFromMatchingRadar(match);
  return rows.map((row) => {
    const student = Math.round(safeScore(row?.student));
    const roleNeed = Math.round(safeScore(row?.role_need ?? row?.target));
    const difference = Number.isFinite(Number(row?.difference)) ? Math.round(Number(row.difference)) : student - roleNeed;
    const status = matchingRowStatus({ ...row, difference });
    return {
      name: row?.name || row?.capability || "能力项",
      student,
      role_need: roleNeed,
      difference,
      status,
      note: row?.note || matchingRowNote({ ...row, status, difference, name: row?.name || row?.capability || "能力项", student, role_need: roleNeed })
    };
  }).filter((row) => row.name);
}

function reportRowsFromMatchingRadar(match) {
  const student = matchingStudentRadar(match);
  const target = matchingTargetRadar(match);
  if (student.length !== benchmarkDimensions.length || target.length !== benchmarkDimensions.length) return [];
  return benchmarkDimensions.map((name, index) => ({
    name,
    student: student[index].score,
    role_need: target[index].score,
    difference: student[index].score - target[index].score
  }));
}

function currentProfileStudentRadar() {
  const profileRadar = normalizedProfileRadarData(diagnosis?.ability_profile);
  return profileRadar.length === benchmarkDimensions.length ? radarScoresToMatchingDimensions(profileRadar) : [];
}

function radarScoresToMatchingDimensions(scores) {
  return benchmarkDimensions.map((name, index) => ({
    name,
    score: Math.round(safeScore(scores[index])),
    max_score: 100
  }));
}

function matchingStudentRadar(match) {
  const profileStudent = currentProfileStudentRadar();
  if (profileStudent.length === benchmarkDimensions.length) return profileStudent;
  return normalizeMatchingRadar(match?.student_radar);
}

function matchingTargetRadar(match) {
  return normalizeMatchingRadar(match?.target_radar || match?.selected_job?.requirement_radar);
}

function matchingRadarSummary(rows = []) {
  const normalized = rows.map((row) => ({ ...row, status: matchingRowStatus(row) }));
  const weakRows = normalized.filter((row) => row.status === "gap" || row.status === "limited");
  const advantageRows = normalized.filter((row) => row.status === "advantage");
  const weakest = [...normalized].sort((left, right) => Number(left.difference || 0) - Number(right.difference || 0))[0] || null;
  const strongest = [...normalized].sort((left, right) => Number(right.difference || 0) - Number(left.difference || 0))[0] || null;
  return { rows: normalized, weakRows, advantageRows, weakest, strongest };
}

function matchingRowStatus(row) {
  const value = String(row?.status || "").toLowerCase();
  if (["advantage", "fit", "limited", "gap"].includes(value)) return value;
  const text = `${row?.status || ""}${row?.note || ""}`;
  if (text.includes("优势")) return "advantage";
  if (text.includes("有限")) return "limited";
  if (text.includes("短板") || text.includes("缺口")) return "gap";
  const difference = Number(row?.difference || 0);
  if (difference >= 6) return "advantage";
  if (difference >= -3) return "fit";
  if (difference >= -12) return "limited";
  return "gap";
}

function matchingStatusLabel(status) {
  return {
    advantage: "优势",
    fit: "达标",
    limited: "有限",
    gap: "短板"
  }[status] || "待判";
}

function matchingRowNote(row) {
  const name = row?.name || "能力项";
  const difference = Number(row?.difference || 0);
  const status = matchingRowStatus(row);
  if (status === "advantage") return `${name}高于岗位要求${difference}分，可作为面试支撑点。`;
  if (status === "fit") return `${name}基本达到岗位要求。`;
  if (status === "limited") return `${name}低于岗位要求${Math.abs(difference)}分，需要补证据。`;
  if (status === "gap") return `${name}是当前主要短板，差距${Math.abs(difference)}分。`;
  return "";
}

function renderMatchDecisionBadges(match, selected, radarSummary) {
  const badges = [
    renderEducationGateChip(selected?.education_gate || match?.education_gate || "门槛待判", selected?.education_gate_status || match?.education_gate_status),
    renderEvidenceStrengthChip(selected?.evidence_strength, selected),
  ];
  if (radarSummary?.weakest) {
    badges.push(`<span class="decision-chip is-${escapeAttribute(matchingRowStatus(radarSummary.weakest))}">${escapeHTML(radarSummary.weakest.name)} ${escapeHTML(matchingStatusLabel(matchingRowStatus(radarSummary.weakest)))}</span>`);
  }
  return badges.filter(Boolean).join("");
}

function renderEducationGateChip(label, status) {
  const meta = educationGateMeta(label, status);
  return `<span class="decision-chip gate-chip is-${escapeAttribute(meta.status)}"><b>${escapeHTML(meta.label)}</b>${escapeHTML(meta.text)}</span>`;
}

function educationGateMeta(label, status) {
  const text = compactMatchingText(label || "门槛待判", 28);
  const combined = `${status || ""} ${label || ""}`.toLowerCase();
  if (combined.includes("blocked") || combined.includes("不建议") || combined.includes("不通过")) return { status: "blocked", label: "门槛", text };
  if (combined.includes("stretch") || combined.includes("突破") || combined.includes("高impact")) return { status: "stretch", label: "门槛", text };
  if (combined.includes("risk") || combined.includes("风险")) return { status: "risk", label: "门槛", text };
  if (combined.includes("pass") || combined.includes("通过") || combined.includes("达标")) return { status: "pass", label: "门槛", text };
  return { status: "unknown", label: "门槛", text };
}

function renderEvidenceStrengthChip(value, selected) {
  const strength = evidenceStrengthMeta(value, selected);
  return `<span class="decision-chip evidence-chip is-${escapeAttribute(strength.status)}"><b>证据</b>${escapeHTML(strength.label)}</span>`;
}

function evidenceStrengthMeta(value, selected = {}) {
  const normalized = String(value || "").toLowerCase();
  if (normalized.includes("strong") || normalized.includes("强")) return { status: "strong", label: "强支撑" };
  if (normalized.includes("weak") || normalized.includes("弱") || normalized.includes("有限")) return { status: "weak", label: "需补强" };
  const score = Number(selected.experience_match || selected.match || 0);
  if (score >= 78) return { status: "strong", label: "强支撑" };
  if (score >= 64) return { status: "medium", label: "中等支撑" };
  return { status: "weak", label: "需补强" };
}

function renderFocusReasonList(reasons, selected) {
  const sourceItems = reasons.length ? reasons : [selected?.fit_summary || "等待证据"];
  const items = sourceItems
    .map((item) => normalizeMatchingFocusText(item))
    .filter(Boolean)
    .slice(0, 3);
  const safeItems = items.length ? items : ["等待证据"];
  return `<ul class="focus-evidence-list">${safeItems.map((item) => `<li>${escapeHTML(item)}</li>`).join("")}</ul>`;
}

function renderFocusGap(mainGap, radarSummary) {
  const status = mainGap ? severityClass(mainGap.severity) : matchingRowStatus(radarSummary?.weakest);
  const capability = normalizeMatchingFocusText(mainGap?.capability || radarSummary?.weakest?.name || "等待差距分析");
  const text = mainGap ? (mainGap.current || mainGap.expected || mainGap.action || "") : (radarSummary?.weakest?.note || "");
  return `
    <span class="focus-status is-${escapeAttribute(status || "pending")}">${escapeHTML(status === "high" ? "高风险" : status === "medium" ? "需关注" : status === "gap" ? "短板" : status === "limited" ? "有限" : "跟踪")}</span>
    <strong>${escapeHTML(capability)}</strong>
    ${text ? `<small>${escapeHTML(normalizeMatchingFocusText(text))}</small>` : ""}
  `;
}

function renderFocusText(text) {
  return `<strong>${escapeHTML(normalizeMatchingFocusText(text))}</strong>`;
}

function normalizeMatchingFocusText(value) {
  return cleanDisplayText(value).replace(/\s+/g, " ");
}

function renderMatchActionList(match, selected, mainGap, radarSummary = {}) {
  const actions = normalizedDevelopmentActions(match, selected, mainGap, radarSummary);
  if (!actions.length) return "";
  return `
    <table class="match-action-table" aria-label="可提升点">
      <colgroup>
        <col class="col-rank">
        <col class="col-priority">
        <col class="col-scope">
        <col class="col-description">
      </colgroup>
      <thead>
        <tr>
          <th scope="col">序号</th>
          <th scope="col">优先级</th>
          <th scope="col">校内/校外</th>
          <th scope="col">描述</th>
        </tr>
      </thead>
      <tbody>
        ${actions.map((item, index) => `
          <tr style="--match-stagger:${index * 34}ms">
            <td class="action-rank">${index + 1}</td>
            <td><span class="match-action-priority ${escapeAttribute(developmentPriorityClass(item.priority))}">${escapeHTML(item.priority)}</span></td>
            <td class="action-scope">${escapeHTML(item.scope)}</td>
            <td class="action-description">${escapeHTML(item.description)}</td>
          </tr>
        `).join("")}
      </tbody>
    </table>
  `;
}

function normalizedDevelopmentActions(match, selected, mainGap, radarSummary = {}) {
  const lowest = radarSummary.weakest;
  const structured = structuredDevelopmentActions(match);
  const recommendationActions = normalizedMatchRecommendations(match).map((value) => developmentActionFromRecommendation(value));
  const fallbackActions = [
    {
      priority: "高",
      scope: developmentScope(selected?.next_proof || mainGap?.action || firstProofGap(selected) || ""),
      description: selected?.next_proof || mainGap?.action || firstProofGap(selected) || "补充目标岗位相关项目、代码或作品集"
    },
    {
      priority: lowest?.difference < -12 ? "高" : "中",
      scope: "校内",
      description: lowest?.name ? `${lowest.name}${lowest.difference > 0 ? "保持优势，沉淀可验证案例。" : "优先补齐，选择课程项目、实验室任务或作品集任务补证据。"}` : "等待六维差距返回后补充提升动作。"
    },
    {
      priority: "低",
      scope: "校外",
      description: selected?.category || match?.match_level || "基于六维画像和证据强度筛选投递方向。"
    }
  ];
  return uniqueDevelopmentActions([...structured, ...recommendationActions, ...fallbackActions]);
}

function structuredDevelopmentActions(match) {
  if (!Array.isArray(match?.development_actions)) return [];
  return match.development_actions.map((item) => {
    const description = cleanDisplayText(item?.description || item?.action || item?.task);
    if (!description) return null;
    return {
      priority: normalizeDevelopmentPriority(item?.priority),
      scope: developmentScope(description, item?.scope),
      description: compactMatchingText(description, 180)
    };
  }).filter(Boolean);
}

function developmentActionFromRecommendation(value) {
  const parsed = parseRecommendationActionMeta(value);
  return {
    priority: normalizeDevelopmentPriority(parsed.priority),
    scope: developmentScope(parsed.text, parsed.scope),
    description: compactMatchingText(parsed.text, 180)
  };
}

function parseRecommendationActionMeta(value) {
  const raw = compactMatchingText(value, 160);
  const match = raw.match(/^【([^】]+)】\s*/);
  if (!match) return { text: raw, scope: "", priority: "" };
  const tokens = match[1].split(/[·/｜|,，\s]+/).map((item) => item.trim()).filter(Boolean);
  const scope = tokens.find((item) => item.includes("校内") || item.includes("校外") || item.includes("证据")) || "";
  const priority = tokens.find((item) => item.includes("优先级") || item.includes("优先") || item.includes("关注")) || "";
  return {
    text: raw.slice(match[0].length).trim() || raw,
    scope,
    priority
  };
}

function uniqueDevelopmentActions(actions) {
  const seen = new Set();
  return actions
    .map((item) => ({
      priority: normalizeDevelopmentPriority(item?.priority),
      scope: developmentScope(item?.description, item?.scope),
      description: compactMatchingText(item?.description, 180)
    }))
    .filter((item) => {
      if (!item.description) return false;
      const key = `${item.priority}|${item.scope}|${item.description}`.toLowerCase();
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    })
    .sort((left, right) => developmentPriorityWeight(right.priority) - developmentPriorityWeight(left.priority))
    .slice(0, 12);
}

function normalizedMatchRecommendations(match) {
  if (!Array.isArray(match?.recommendations)) return [];
  const seen = new Set();
  return match.recommendations
    .map((item) => compactMatchingText(item, 160))
    .filter((item) => {
      const key = item.toLowerCase();
      if (!item || seen.has(key)) return false;
      seen.add(key);
      return true;
    })
    .slice(0, 3);
}

function normalizeDevelopmentPriority(value) {
  const text = String(value || "");
  if (text.includes("高") || text.toLowerCase().includes("high")) return "高";
  if (text.includes("低") || text.toLowerCase().includes("low")) return "低";
  return "中";
}

function developmentPriorityWeight(priority) {
  return { 高: 3, 中: 2, 低: 1 }[normalizeDevelopmentPriority(priority)] || 2;
}

function developmentPriorityClass(priority) {
  return {
    高: "is-high",
    中: "is-medium",
    低: "is-low"
  }[normalizeDevelopmentPriority(priority)] || "is-medium";
}

function developmentScope(description, explicit = "") {
  const scope = String(explicit || "");
  if (scope.includes("校内")) return "校内";
  if (scope.includes("校外")) return "校外";
  const text = String(description || "");
  if (text.includes("校内") || text.includes("课程") || text.includes("实验室") || text.includes("社团") || text.includes("导师") || text.includes("大创")) return "校内";
  return "校外";
}

function firstProofGap(job) {
  return Array.isArray(job?.proof_gaps) ? job.proof_gaps.find(Boolean) : "";
}

function renderGapCard(gap, index) {
  const severity = gap?.severity || "待判断";
  const current = formatAgentDisplayText(gap?.current || "暂无明确证据");
  const expected = formatAgentDisplayText(gap?.expected || "等待岗位要求");
  const action = formatAgentDisplayText(gap?.action || "补充可验证成果");
  return `
    <article class="gap-card context-chat-host is-${escapeAttribute(severityClass(severity))}" style="--match-stagger:${index * 36}ms">
      <header>
        <strong>${escapeHTML(gap?.capability || "能力项")}</strong>
        <span class="severity ${severityClass(severity)}">${escapeHTML(severity)}</span>
      </header>
      <div class="gap-card-body">
        <p><span>当前</span>${escapeHTML(current)}</p>
        <p><span>目标</span>${escapeHTML(expected)}</p>
      </div>
      <div class="gap-action">
        <span>建议</span>
        <strong>${escapeHTML(action)}</strong>
      </div>
      ${renderResultChatButton("gap", index, gap?.capability || "差距明细")}
    </article>
  `;
}

function severityClass(severity) {
  const text = String(severity || "");
  if (text.includes("高")) return "high";
  if (text.includes("中")) return "medium";
  if (text.includes("低")) return "low";
  return "pending";
}

function severityWeight(severity) {
  return { high: 3, medium: 2, low: 1, pending: 0 }[severityClass(severity)] || 0;
}

function compactMatchingText(text, maxLength = 72) {
  const value = formatAgentDisplayText(text || "").replace(/\s+/g, " ").trim();
  if (value.length <= maxLength) return value;
  return `${value.slice(0, maxLength - 1).trim()}…`;
}

function renderMatchingRadar(match, reportRows = []) {
  const svg = document.querySelector("#matchingRadarChart");
  const text = document.querySelector("#matchingRadarText");
  const legend = document.querySelector("#matchingRadarLegend");
  const stats = document.querySelector("#matchingRadarStats");
  const insights = document.querySelector("#matchingRadarInsights");
  if (!svg) return;
  const student = matchingStudentRadar(match);
  const target = matchingTargetRadar(match);
  if (student.length !== benchmarkDimensions.length || target.length !== benchmarkDimensions.length) {
    renderMatchingRadarWaiting(svg);
    if (legend) legend.innerHTML = "";
    if (stats) stats.innerHTML = "";
    if (text) text.textContent = "等待首选岗位目标能力雷达。";
    if (insights) insights.innerHTML = "";
    return;
  }
  const rows = reportRows.length ? reportRows : normalizedMatchingReportRows(match);
  const summary = matchingRadarSummary(rows);
  const rowByName = new Map(rows.map((row) => [row.name, row]));
  const center = { x: 180, y: 178 };
  const radius = 106;
  const ring = (score) => benchmarkDimensions.map((_, index) => {
    const point = pointFor(index, benchmarkDimensions.length, radius * radarVisualRatio(score), center);
    return `${point.x.toFixed(2)},${point.y.toFixed(2)}`;
  }).join(" ");
  const studentPoints = radarPolygonPoints(student.map((item) => item.score), center, radius);
  const targetPoints = radarPolygonPoints(target.map((item) => item.score), center, radius);
  svg.innerHTML = `
    <title>个人画像与岗位目标雷达对比</title>
    <desc>${escapeHTML(benchmarkDimensions.map((name, index) => `${name}个人${student[index].score}分，岗位${target[index].score}分`).join("；"))}</desc>
    ${radarGridScores.map((score) => `<polygon class="radar-grid" points="${ring(score)}"></polygon>`).join("")}
    ${benchmarkDimensions.map((name, index) => {
      const outer = pointFor(index, benchmarkDimensions.length, radius, center);
      const label = pointFor(index, benchmarkDimensions.length, radius + 28, center);
      const status = matchingRowStatus(rowByName.get(name));
      const statusPoint = pointFor(index, benchmarkDimensions.length, radius + 50, center);
      return `
        <line class="radar-axis matching-radar-axis is-${escapeAttribute(status)}" x1="${center.x}" y1="${center.y}" x2="${outer.x.toFixed(2)}" y2="${outer.y.toFixed(2)}"></line>
        <text class="radar-label" x="${label.x.toFixed(2)}" y="${label.y.toFixed(2)}" text-anchor="middle">${escapeHTML(name)}</text>
        ${status !== "fit" ? `<text class="matching-radar-axis-status is-${escapeAttribute(status)}" x="${statusPoint.x.toFixed(2)}" y="${statusPoint.y.toFixed(2)}" text-anchor="middle">${escapeHTML(matchingStatusLabel(status))}</text>` : ""}
      `;
    }).join("")}
    <polygon class="matching-radar-area is-target" points="${targetPoints}"></polygon>
    <polygon class="matching-radar-area is-student" points="${studentPoints}"></polygon>
    ${matchingRadarDimensionLayer(student, target, rowByName, center, radius)}
  `;
  if (legend) {
    const studentAvg = averageMatchingRadarScore(student);
    const targetAvg = averageMatchingRadarScore(target);
    legend.innerHTML = `
      <span class="radar-legend-item matching-radar-legend-student" tabindex="0">
        <i></i>个人画像<b>${escapeHTML(studentAvg)}分</b>
      </span>
      <span class="radar-legend-item matching-radar-legend-target" tabindex="0">
        <i></i>岗位目标<b>${escapeHTML(targetAvg)}分</b>
      </span>
    `;
  }
  if (stats) {
    stats.innerHTML = renderMatchingRadarStats(rows, summary);
  }
  if (text) {
    const weakest = summary.weakest;
    const strongest = summary.strongest;
    const weakestDelta = Number(weakest?.difference || 0);
    const strongestDelta = Number(strongest?.difference || 0);
    const shortboardText = weakest && weakestDelta < 0
      ? `最大短板为${weakest.name}${formatSignedDelta(weakest.difference)}分`
      : "暂无明显短板";
    const advantageText = strongest && strongestDelta > 0
      ? `当前相对优势为${strongest.name}${formatSignedDelta(strongest.difference)}分`
      : "暂无明显优势";
    text.textContent = `${match?.target_role || match?.selected_job?.title || "首选岗位"}要求下，${shortboardText}，${advantageText}。`;
  }
  if (insights) {
    insights.innerHTML = renderMatchingRadarInsights(rows);
  }
}

function matchingRadarDimensionLayer(student, target, rowByName, center, radius) {
  return benchmarkDimensions.map((name, index) => {
    const studentScore = safeScore(student[index]?.score);
    const targetScore = safeScore(target[index]?.score);
    const row = rowByName.get(name) || {
      name,
      student: Math.round(studentScore),
      role_need: Math.round(targetScore),
      difference: Math.round(studentScore - targetScore)
    };
    const status = matchingRowStatus(row);
    const delta = Math.round(Number(row.difference ?? studentScore - targetScore) || 0);
    const studentPoint = pointFor(index, benchmarkDimensions.length, radius * radarVisualRatio(studentScore), center);
    const targetPoint = pointFor(index, benchmarkDimensions.length, radius * radarVisualRatio(targetScore), center);
    const outer = pointFor(index, benchmarkDimensions.length, radius + 8, center);
    const card = matchingRadarValueCardPosition(index, benchmarkDimensions.length, radius, center);
    const delay = 160 + index * 34;
    const aria = `${name}：个人${Math.round(studentScore)}分，岗位${Math.round(targetScore)}分，差距${formatSignedDelta(delta)}分，${matchingStatusLabel(status)}`;
    return `
      <g class="radar-dimension-hover matching-radar-dimension is-${escapeAttribute(status)}" tabindex="0" aria-label="${escapeAttribute(aria)}" style="--radar-delay:${delay}ms">
        <line class="radar-hover-zone" x1="${center.x}" y1="${center.y}" x2="${outer.x.toFixed(2)}" y2="${outer.y.toFixed(2)}"></line>
        <line class="matching-radar-delta-line is-${escapeAttribute(status)}" x1="${targetPoint.x.toFixed(2)}" y1="${targetPoint.y.toFixed(2)}" x2="${studentPoint.x.toFixed(2)}" y2="${studentPoint.y.toFixed(2)}"></line>
        ${status === "gap" || status === "limited" ? `<circle class="matching-radar-highlight is-${escapeAttribute(status)}" cx="${studentPoint.x.toFixed(2)}" cy="${studentPoint.y.toFixed(2)}" r="9"></circle>` : ""}
        <circle class="matching-radar-dot is-target" cx="${targetPoint.x.toFixed(2)}" cy="${targetPoint.y.toFixed(2)}" r="3.2"></circle>
        <circle class="matching-radar-dot is-student" cx="${studentPoint.x.toFixed(2)}" cy="${studentPoint.y.toFixed(2)}" r="3.8"></circle>
        <circle class="radar-hover-point matching-radar-hover-point" cx="${outer.x.toFixed(2)}" cy="${outer.y.toFixed(2)}" r="7"></circle>
        <g class="radar-value-card matching-radar-value-card" transform="translate(${card.x}, ${card.y})" aria-hidden="true">
          <rect width="132" height="94" rx="8"></rect>
          <text class="radar-value-title" x="12" y="20">${escapeHTML(name)}</text>
          <circle class="matching-radar-swatch-student" cx="15" cy="37" r="4"></circle>
          <text class="matching-radar-value-row" x="25" y="40">个人 ${Math.round(studentScore)}分</text>
          <circle class="matching-radar-swatch-target" cx="15" cy="54" r="4"></circle>
          <text class="matching-radar-value-row" x="25" y="57">岗位 ${Math.round(targetScore)}分</text>
          <text class="matching-radar-value-delta is-${escapeAttribute(status)}" x="12" y="74">差距 ${escapeHTML(formatSignedDelta(delta))}分 · ${escapeHTML(matchingStatusLabel(status))}</text>
        </g>
      </g>
    `;
  }).join("");
}

function matchingRadarValueCardPosition(index, count, radius, center) {
  const anchor = pointFor(index, count, radius + 36, center);
  const x = Math.max(8, Math.min(220, Math.round(anchor.x - 66)));
  const y = Math.max(12, Math.min(254, Math.round(anchor.y - 47)));
  return { x, y };
}

function averageMatchingRadarScore(items = []) {
  const scores = items.map((item) => Number(item?.score)).filter(Number.isFinite);
  if (!scores.length) return "--";
  return Math.round(scores.reduce((sum, score) => sum + score, 0) / scores.length);
}

function renderMatchingRadarStats(rows = [], summary = matchingRadarSummary(rows)) {
  if (!rows.length) return "";
  const averageDelta = Math.round(rows.reduce((sum, row) => sum + Number(row.difference || 0), 0) / rows.length);
  const weakest = summary.weakest;
  const strongest = summary.strongest;
  const weakestDelta = Number(weakest?.difference || 0);
  const strongestDelta = Number(strongest?.difference || 0);
  const hasShortboard = weakest && weakestDelta < 0;
  const hasAdvantage = strongest && strongestDelta > 0;
  return `
    <span class="matching-radar-stat ${averageDelta < -3 ? "is-gap" : averageDelta > 5 ? "is-advantage" : ""}">
      <span>平均差</span><b>${escapeHTML(formatSignedDelta(averageDelta))}</b>
    </span>
    <span class="matching-radar-stat ${hasShortboard ? "is-gap" : ""}">
      <span>最大短板</span><b>${hasShortboard ? `${escapeHTML(weakest.name)} ${escapeHTML(formatSignedDelta(weakest.difference))}` : "暂无明显短板"}</b>
    </span>
    <span class="matching-radar-stat ${hasAdvantage ? "is-advantage" : ""}">
      <span>优势项</span><b>${hasAdvantage ? `${escapeHTML(strongest.name)} ${escapeHTML(formatSignedDelta(strongest.difference))}` : "暂无明显优势"}</b>
    </span>
  `;
}

function formatSignedDelta(value) {
  const number = Math.round(Number(value) || 0);
  return `${number > 0 ? "+" : ""}${number}`;
}

function renderMatchingRadarInsights(rows = []) {
  const summary = matchingRadarSummary(rows);
  const priorityRows = [
    ...summary.weakRows.sort((left, right) => Number(left.difference || 0) - Number(right.difference || 0)),
    ...summary.advantageRows.sort((left, right) => Number(right.difference || 0) - Number(left.difference || 0)),
  ].slice(0, 4);
  if (!priorityRows.length) {
    return `<span class="radar-insight is-fit"><b>达标</b>六维差距暂无明显短板</span>`;
  }
  return priorityRows.map((row) => {
    const status = matchingRowStatus(row);
    return `
      <span class="radar-insight is-${escapeAttribute(status)}">
        <b>${escapeHTML(row.name)} · ${escapeHTML(matchingStatusLabel(status))}</b>
        ${escapeHTML(compactMatchingText(row.note || matchingRowNote(row), 52))}
      </span>
    `;
  }).join("");
}

function renderMatchingRadarWaiting(svg) {
  const center = { x: 180, y: 178 };
  const radius = 106;
  const ring = (score) => benchmarkDimensions.map((_, index) => {
    const point = pointFor(index, benchmarkDimensions.length, radius * radarVisualRatio(score), center);
    return `${point.x.toFixed(2)},${point.y.toFixed(2)}`;
  }).join(" ");
  svg.innerHTML = `
    <title>个人画像与岗位目标雷达等待中</title>
    <desc>等待 Job Matching 返回岗位目标雷达。</desc>
    ${radarGridScores.map((score) => `<polygon class="radar-grid" points="${ring(score)}"></polygon>`).join("")}
    ${benchmarkDimensions.map((name, index) => {
      const outer = pointFor(index, benchmarkDimensions.length, radius, center);
      const label = pointFor(index, benchmarkDimensions.length, radius + 28, center);
      return `
        <line class="radar-axis" x1="${center.x}" y1="${center.y}" x2="${outer.x.toFixed(2)}" y2="${outer.y.toFixed(2)}"></line>
        <text class="radar-label" x="${label.x.toFixed(2)}" y="${label.y.toFixed(2)}" text-anchor="middle">${escapeHTML(name)}</text>
      `;
    }).join("")}
    <polygon class="radar-loading-area" points="${ring(42)}"></polygon>
  `;
}

function normalizeMatchingRadar(items) {
  if (!Array.isArray(items)) return [];
  const lookup = new Map(items.map((item) => [String(item?.name || item?.dimension || ""), safeScore(item?.score)]));
  return benchmarkDimensions.map((name) => ({
    name,
    score: lookup.has(name) ? Math.round(lookup.get(name)) : NaN
  })).filter((item) => Number.isFinite(item.score));
}

function renderPath(plan) {
  const target = document.querySelector("#pathStages");
  if (!target) return;
  const stages = Array.isArray(plan?.stages) ? plan.stages : [];
  if (!stages.length) {
    target.innerHTML = `
      <article class="path-empty">
        <strong>等待 Path Planning Team</strong>
        <span>岗位匹配完成后，Legato 会生成阶段目标、周任务和达标标准。</span>
      </article>
    `;
    return;
  }
  target.innerHTML = stages.map((stage, index) => {
    const weeks = Array.isArray(stage.weeks) ? stage.weeks : [];
    const standards = Array.isArray(stage.standards) ? stage.standards : [];
    const resources = Array.isArray(stage.resources) ? stage.resources : [];
    const taskRows = weeks.length ? weeks : [{ week: "待生成", task: "等待周任务", metric: "Path Planning Team 正在整理可执行任务", priority: "中" }];
    const highCount = taskRows.filter((week) => pathPriorityClass(week.priority) === "high").length;
    const resourceRows = resources.length ? resources : [];
    return `
      <article class="stage-panel context-chat-host reveal is-visible" style="--path-stagger:${index * 54}ms">
        <div class="stage-heading">
          <span class="path-stage-index">${String(index + 1).padStart(2, "0")}</span>
          <div>
            <h3>${escapeHTML(stage.stage || `第 ${index + 1} 阶段`)}</h3>
            <p class="stage-goal">${escapeHTML(stage.goal || "等待阶段目标")}</p>
            <div class="stage-meta-chips" aria-label="阶段任务摘要">
              <span>${escapeHTML(`${taskRows.length} 个周任务`)}</span>
              ${highCount ? `<span class="is-high">${escapeHTML(`${highCount} 个高优先`)}</span>` : ""}
              <span>${escapeHTML(resources.length ? `${resources.length} 个资源` : "资源待补充")}</span>
            </div>
          </div>
        </div>
        <div class="path-deliverable">
          <span>阶段交付物</span>
          <strong>${escapeHTML(stage.deliverable || "阶段作品、复盘文档和可验证证据")}</strong>
        </div>
        <ol class="task-list">
          ${taskRows.map((week, weekIndex) => {
            const priority = normalizePathPriority(week.priority);
            const priorityClass = pathPriorityClass(priority);
            const isOpen = priorityClass === "high" || weekIndex === 0;
            return `
            <li class="is-${escapeAttribute(priorityClass)}" style="--task-stagger:${weekIndex * 34}ms">
              <details class="path-task" ${isOpen ? "open" : ""}>
                <summary>
                  <span class="task-week">${escapeHTML(week.week || "本周")}</span>
                  <strong>${escapeHTML(week.task || "等待任务")}</strong>
                  <b>${escapeHTML(priority)}</b>
                </summary>
                <div class="task-detail">
                  <span>达标指标</span>
                  <p>${escapeHTML(week.metric || "等待达标指标")}</p>
                </div>
              </details>
              ${renderResultChatButton("path_task", index, week.task || "周任务", weekIndex)}
            </li>
            `;
          }).join("")}
        </ol>
        <div class="path-standard-block">
          <strong>达标标准</strong>
          <ul class="acceptance-list">
            ${standards.map((standard) => `<li>${escapeHTML(standard)}</li>`).join("")}
          </ul>
        </div>
        <div class="resource-list${resourceRows.length ? "" : " is-empty"}" aria-label="阶段资源">
          ${resourceRows.length
            ? resourceRows.map((resource) => `<a href="${safeURLAttribute(resource.url)}" target="_blank" rel="noreferrer">${escapeHTML(resource.label)}</a>`).join("")
            : `<span>暂无资源链接</span>`}
        </div>
        ${renderResultChatButton("path_stage", index, stage.stage || "路径阶段")}
      </article>
    `;
  }).join("");
}

function normalizePathPriority(priority) {
  const value = cleanDisplayText(priority);
  if (value.includes("高")) return "高";
  if (value.includes("低")) return "低";
  return "中";
}

function pathPriorityClass(priority) {
  priority = normalizePathPriority(priority);
  if (priority === "高") return "high";
  if (priority === "低") return "low";
  return "medium";
}

function renderTopJobs(jobs) {
  jobs = jobs || [];
  const matchingList = document.querySelector("#matchingJobs");
  const outputList = document.querySelector("#topJobs");
  if (matchingList) {
    matchingList.innerHTML = jobs.length
      ? jobs.map((job, index) => renderMatchingJobCard(job, index)).join("")
      : `<div class="matching-empty">等待 Job Matching 返回岗位队列。</div>`;
  }
  if (outputList) {
    outputList.innerHTML = jobs.map((job, index) => renderOutputJobCard(job, index)).join("");
  }
}

function renderMatchingJobCard(job, index = 0) {
  const reasons = Array.isArray(job.reasons) ? job.reasons : [];
  const proofGaps = Array.isArray(job.proof_gaps) ? job.proof_gaps : [];
  const category = job.category || "推荐岗位";
  const educationGate = job.education_gate || "";
  const rank = Number(job.rank || index + 1);
  const isTop = rank === 1;
  const sizeClass = isTop ? " is-top is-large" : " is-small";
  return `
    <article class="matching-job-card context-chat-host${sizeClass}" data-job-card-size="${isTop ? "large" : "small"}" style="--match-stagger:${index * 42}ms">
      <header>
        <div>
          <span class="job-rank">${rank ? `#${rank}` : ""}</span>
          <h3>${escapeHTML(job.title || "")}</h3>
        </div>
        <strong>${safeScore(job.match)}%</strong>
      </header>
      <div class="job-tags">
        <span>${escapeHTML(category)}</span>
        ${educationGate ? renderEducationGateChip(educationGate, job.education_gate_status) : ""}
        ${renderEvidenceStrengthChip(job.evidence_strength, job)}
      </div>
      <div class="job-progress-group">
        ${renderJobProgress("综合", job.match)}
        ${renderJobProgress("六维", job.ability_match || job.match)}
        ${renderJobProgress("经历", job.experience_match)}
      </div>
      ${job.fit_summary ? `<p class="job-fit-summary">${escapeHTML(compactMatchingText(job.fit_summary, isTop ? 190 : 118))}</p>` : ""}
      ${reasons.length ? `<ul class="job-reason-list">${reasons.slice(0, isTop ? 4 : 2).map((item) => `<li>${escapeHTML(compactMatchingText(item, isTop ? 82 : 58))}</li>`).join("")}</ul>` : ""}
      ${proofGaps.length ? `<div class="job-proof-gaps"><span>缺口</span>${proofGaps.slice(0, 3).map((item) => `<b>${escapeHTML(compactMatchingText(item, 46))}</b>`).join("")}</div>` : ""}
      ${job.next_proof ? `<small class="job-next-proof"><span>下一步</span>${escapeHTML(compactMatchingText(job.next_proof, isTop ? 120 : 74))}</small>` : ""}
      ${renderResultChatButton("job", index, job.title || "推荐岗位")}
    </article>
  `;
}

function renderOutputJobCard(job, index = 0) {
  const reasons = Array.isArray(job.reasons) ? job.reasons : [];
  const mockBadge = isMockMatchingResult() ? `<span class="sim-badge">模拟数据</span>` : "";
  return `
    <article class="job-item context-chat-host">
      <div class="job-head">
        <strong>${job.rank}. ${escapeHTML(job.title || "")}</strong>
        ${mockBadge}
        <span class="match">${safeScore(job.match)}%</span>
      </div>
      <div class="job-match-bar" aria-label="${escapeHTML(job.title || "")}能力匹配度 ${safeScore(job.match)}%">
        <span style="width:${safeScore(job.match)}%"></span>
      </div>
      ${job.category ? `<p>${escapeHTML(formatAgentDisplayText(job.category))}${job.education_gate ? `，${escapeHTML(formatAgentDisplayText(job.education_gate))}` : ""}</p>` : ""}
      ${reasons.length ? `<p>${escapeHTML(formatAgentDisplayText(reasons.join("；")))}</p>` : ""}
      ${job.next_proof ? `<p>${escapeHTML(formatAgentDisplayText(job.next_proof))}</p>` : ""}
      ${renderResultChatButton("output_job", index, job.title || "TOP5 岗位")}
    </article>
  `;
}

function renderJobProgress(label, value) {
  const score = Math.round(safeScore(value));
  return `
    <div class="job-progress">
      <span>${escapeHTML(label)}</span>
      <div class="job-progress-bar" aria-hidden="true"><i style="width:${score}%"></i></div>
      <strong>${score}%</strong>
    </div>
  `;
}

function isMockMatchingResult() {
  const source = String(diagnosis?.matching_result?.source || diagnosis?.mode || "");
  return source.toLowerCase().includes("mock");
}

function renderRequirements(requirements) {
  const target = document.querySelector("#requirementsList");
  if (!target) return;
  target.innerHTML = requirements.map((item) => `
    <article class="requirement">
      <header>
        <div>
          <strong>${escapeHTML(item.title)}</strong>
          <small>${escapeHTML(item.id)} · ${escapeHTML(item.status)}</small>
        </div>
        <span class="status-pill">${escapeHTML(item.priority)}</span>
      </header>
      <ul>
        ${item.details.map((detail) => `<li>${escapeHTML(detail)}</li>`).join("")}
      </ul>
    </article>
  `).join("");
}

function renderLimitations(limitations) {
  const list = document.querySelector("#limitationsList");
  if (!list) return;
  list.innerHTML = (limitations || []).map((item) => `<li>${escapeHTML(item)}</li>`).join("");
}

function setupExports() {
  document.addEventListener("click", (event) => {
    const button = event.target.closest("[data-export]");
    if (!button) return;
    const type = button.dataset.export;
    if (!diagnosis) return;

    if (type === "profile-json") {
      downloadJSON("ability-profile.json", diagnosis.ability_profile);
    }
    if (type === "profile-xlsx") {
      window.location.href = diagnosisExportURL("ability-profile.xlsx", "/api/export/ability-profile.xlsx");
    }
    if (type === "profile-pdf") {
      printSectionAsPDF("profile", "能力画像");
    }
    if (type === "match-json") {
      downloadJSON("matching-result.json", diagnosis.matching_result);
    }
    if (type === "match-pdf") {
      printSectionAsPDF("matching", "匹配结果");
    }
    if (type === "path-json") {
      downloadJSON("path-plan.json", diagnosis.path_plan);
    }
    if (type === "path-doc") {
      window.location.href = diagnosisExportURL("path-plan.doc", "/api/export/path-plan.doc");
    }
    if (type === "path-pdf") {
      printSectionAsPDF("path", "路径规划");
    }
  });
}

function diagnosisExportURL(filename, fallback) {
  if (!currentJobId) return fallback;
  return `/api/diagnosis/${encodeURIComponent(currentJobId)}/export/${encodeURIComponent(filename)}`;
}

function setupAssistant() {
  ensureAssistantSession();
  renderAssistant();

  assistantToggle.addEventListener("click", () => {
    if (!isAssistantInspectable()) {
      setAssistantExpanded(false, { silent: true });
      showToast("生成诊断后可查看 AI 助手。");
      return;
    }
    setAssistantExpanded(assistant.classList.contains("is-collapsed"));
  });
  assistantClose.addEventListener("click", () => setAssistantExpanded(false));
  assistantHistoryBack.addEventListener("click", closeAssistantHistory);
  assistantNewSession.addEventListener("click", () => {
    if (!isAssistantReady()) {
      showToast("全部诊断结果生成后才能追问。");
      return;
    }
    assistantArchive.open = false;
    syncAssistantHistoryView();
    createAssistantSession();
    setAssistantExpanded(true);
    renderAssistant();
    assistantInput.focus();
  });
  assistantArchiveSession.addEventListener("click", () => {
    if (!isAssistantReady()) {
      showToast("全部诊断结果生成后才能追问。");
      return;
    }
    toggleAssistantHistory();
  });
  assistantArchive.addEventListener("toggle", () => {
    syncAssistantHistoryView();
  });
  assistantArchiveList.addEventListener("click", (event) => {
    const deleteButton = event.target.closest("[data-delete-session]");
    if (deleteButton) {
      deleteAssistantSession(deleteButton.dataset.deleteSession);
      return;
    }
    const button = event.target.closest("[data-restore-session]");
    if (!button) return;
    if (!isAssistantReady()) {
      showToast("全部诊断结果生成后才能追问。");
      return;
    }
    restoreAssistantSession(button.dataset.restoreSession);
  });
  document.addEventListener("click", (event) => {
    const button = event.target.closest("[data-evidence-chat]");
    if (!button) return;
    event.preventDefault();
    event.stopPropagation();
    addEvidenceToAssistant(button.dataset.evidenceChat, Number(button.dataset.evidenceIndex));
  });
  document.addEventListener("click", (event) => {
    const button = event.target.closest("[data-result-chat]");
    if (!button) return;
    event.preventDefault();
    event.stopPropagation();
    addResultToAssistant(button.dataset.resultChat, button.dataset.resultIndex || "", button.dataset.resultSubindex || "");
  });
  assistantSuggestions.addEventListener("click", (event) => {
    const button = event.target.closest("[data-suggestion]");
    if (!button || assistantBusy || !isAssistantReady()) return;
    assistantInput.value = button.dataset.suggestion;
    updateAssistantInputMeta();
    assistantInput.focus();
  });
  assistantMessages.addEventListener("click", (event) => {
    if (event.target.closest(".agent-stream-detail")) return;
    const agentButton = event.target.closest("[data-agent-stream-toggle]");
    if (agentButton) {
      toggleAgentStreamDetail(agentButton.dataset.agentStreamToggle, agentButton.dataset.agentStreamMessage);
      return;
    }
    const button = event.target.closest("[data-retry-message]");
    if (!button || assistantBusy || !isAssistantReady()) return;
    retryAssistantMessage(button.dataset.retryMessage);
  });
  assistantEvidenceTray.addEventListener("click", (event) => {
    const button = event.target.closest("[data-clear-evidence-context]");
    if (!button || assistantBusy) return;
    clearAssistantFocusedEvidence(button.dataset.clearEvidenceContext || "");
  });
  setupAssistantScrollbar(assistantMessages);
  setupAssistantScrollbar(assistantArchiveList);
  setupAssistantScrollbar(assistantSuggestions);
  assistantInput.addEventListener("input", updateAssistantInputMeta);
  assistantInput.addEventListener("keydown", (event) => {
    if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) {
      event.preventDefault();
      assistantForm.requestSubmit();
    }
  });
  assistantForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    await sendAssistantMessage();
  });
  window.addEventListener("beforeunload", () => {
    if (assistantAbort) assistantAbort.abort();
    if (assistantStateSaveTimer) saveAssistantState();
  });
}

function setupAssistantScrollbar(element) {
  let timer = 0;
  element.addEventListener("scroll", () => {
    element.classList.add("is-scrolling");
    window.clearTimeout(timer);
    timer = window.setTimeout(() => element.classList.remove("is-scrolling"), 720);
  }, { passive: true });
}

function loadAssistantState() {
  try {
    const raw = window.localStorage.getItem(assistantStorageKey);
    if (!raw) return defaultAssistantState();
    const parsed = JSON.parse(raw);
    if (!parsed || !Array.isArray(parsed.sessions)) return defaultAssistantState();
    const sessions = parsed.sessions
      .filter((session) => session && typeof session.id === "string")
      .slice(0, assistantMaxSessions)
      .map((session) => ({
        id: session.id,
        title: String(session.title || "新会话").slice(0, 80),
        prestoSessionId: typeof session.prestoSessionId === "string" ? session.prestoSessionId : "",
        diagnosisJobId: typeof session.diagnosisJobId === "string" ? session.diagnosisJobId : "",
        diagnosisFileName: typeof session.diagnosisFileName === "string" ? session.diagnosisFileName : "",
        focusedEvidence: normalizeFocusedEvidence(session.focusedEvidence),
        focusedContext: normalizeFocusedContext(session.focusedContext),
        focusedContexts: normalizeFocusedContexts(session.focusedContexts || session.focusedContext),
        archived: Boolean(session.archived),
        createdAt: session.createdAt || new Date().toISOString(),
        updatedAt: session.updatedAt || new Date().toISOString(),
        messages: Array.isArray(session.messages)
          ? normalizeAssistantMessages(session.messages.slice(-assistantMaxMessages))
          : []
      }));
    return {
      version: 1,
      activeId: typeof parsed.activeId === "string" ? parsed.activeId : "",
      expanded: false,
      sessions
    };
  } catch {
    window.localStorage.removeItem(assistantStorageKey);
    return defaultAssistantState();
  }
}

function normalizeAssistantMessages(messages) {
  const normalized = [];
  messages.forEach((message) => {
    const item = normalizeAssistantMessage(message);
    if (!item) return;
    if (shouldSplitLegacyAgentStreamMessage(message, item)) {
      normalized.push(...splitLegacyAgentStreamMessage(item));
      return;
    }
    normalized.push(item);
  });
  return normalized.slice(-assistantMaxMessages);
}

function normalizeAssistantMessage(message) {
  if (!message || typeof message.content !== "string") return null;
  const role = ["user", "assistant", "system"].includes(message.role) ? message.role : "assistant";
  return {
    id: typeof message.id === "string" ? message.id : uniqueId("msg"),
    role,
    content: message.content.slice(0, 8000),
    createdAt: message.createdAt || new Date().toISOString(),
    updatedAt: message.updatedAt || message.createdAt || new Date().toISOString(),
    status: ["loading", "error", "done"].includes(message.status) ? message.status : "done",
    retryPrompt: typeof message.retryPrompt === "string" ? message.retryPrompt : "",
    streamType: ["agent_team", "evidence_context"].includes(message.streamType) ? message.streamType : "",
    focusEvidenceKey: typeof message.focusEvidenceKey === "string" ? message.focusEvidenceKey : "",
    focusedContexts: normalizeFocusedContexts(message.focusedContexts),
    agentStream: message.streamType === "agent_team" ? normalizeAgentStreamState(message.agentStream) : null
  };
}

function shouldSplitLegacyAgentStreamMessage(rawMessage, normalizedMessage) {
  const rawStream = rawMessage?.agentStream;
  const stream = normalizedMessage?.agentStream;
  if (normalizedMessage?.streamType !== "agent_team" || !stream) return false;
  if (stream.workflow === "resume/path_planning") return false;
  const keys = new Set(stream.order || []);
  const mixedLegacyTeam = keys.has("adaptive_planner") && keys.has("synthesis_arbiter") && stream.order.length > 2;
  return mixedLegacyTeam && (!rawStream?.phaseGroup || rawStream.phaseGroup === "team");
}

function splitLegacyAgentStreamMessage(message) {
  const source = message.agentStream;
  const workflow = source.workflow || "resume/job_matching";
  const groups = {
    planning: source.order.filter((key) => key === "adaptive_planner"),
    team: source.order.filter((key) => key !== "adaptive_planner" && key !== "synthesis_arbiter"),
    synthesis: source.order.filter((key) => key === "synthesis_arbiter")
  };
  return ["planning", "team", "synthesis"]
    .filter((group) => groups[group].length > 0)
    .map((group) => {
      const stream = createAgentTeamStreamState(group, workflow);
      stream.complexity = source.complexity || "";
      stream.order = groups[group];
      stream.agents = {};
      stream.expandedAgents = {};
      stream.order.forEach((key) => {
        stream.agents[key] = source.agents[key];
        if (source.expandedAgents?.[key]) stream.expandedAgents[key] = true;
      });
      stream.logs = Array.isArray(source.logs)
        ? source.logs.filter((item) => {
          if (group === "planning") return item.agentKey === "adaptive_planner" || item.agentKey === "team";
          if (group === "synthesis") return item.agentKey === "synthesis_arbiter" || (item.agentKey === "team" && item.status === "done");
          return stream.order.includes(item.agentKey);
        })
        : [];
      stream.agentCount = group === "team" ? source.agentCount || stream.order.length : 1;
      stream.status = legacyAgentStreamGroupStatus(stream, group);
      stream.summary = legacyAgentStreamGroupSummary(stream, source.summary);
      stream.updatedAt = source.updatedAt || message.updatedAt;
      return {
        ...message,
        id: `${message.id}-${group}`,
        status: stream.status === "failed" ? "error" : stream.status === "done" ? "done" : "loading",
        content: agentTeamStreamFallback(stream),
        agentStream: stream
      };
    });
}

function legacyAgentStreamGroupStatus(stream, group) {
  if (stream.order.some((key) => stream.agents[key]?.status === "failed")) return "failed";
  if (group !== "team") {
    return stream.order.every((key) => stream.agents[key]?.status === "done") ? "done" : "running";
  }
  const total = stream.agentCount || stream.order.length;
  const completed = stream.order.filter((key) => stream.agents[key]?.status === "done").length;
  return total > 0 && completed >= total ? "done" : "running";
}

function legacyAgentStreamGroupSummary(stream, fallback) {
  const latestAgent = [...stream.order].reverse()
    .map((key) => stream.agents[key])
    .find((agent) => agent?.message);
  return latestAgent?.message || fallback || stream.summary;
}

function normalizeAgentStreamState(stream) {
  if (!stream || typeof stream !== "object") return createAgentTeamStreamState();
  const phaseGroup = ["planning", "team", "synthesis"].includes(stream.phaseGroup) ? stream.phaseGroup : "team";
  const workflow = stream.workflow === "resume/path_planning" ? "resume/path_planning" : "resume/job_matching";
  const defaults = createAgentTeamStreamState(phaseGroup, workflow);
  const agents = {};
  const order = Array.isArray(stream.order) ? stream.order.slice(0, 8).map(String) : [];
  const sourceAgents = stream.agents && typeof stream.agents === "object" ? stream.agents : {};
  order.forEach((key) => {
    const item = sourceAgents[key];
    if (!item || typeof item !== "object") return;
    agents[key] = {
      key,
      agent: String(item.agent || key).slice(0, 80),
      status: normalizeAgentStreamStatus(item.status || "queued"),
      phase: String(item.phase || "").slice(0, 40),
      perspective: String(item.perspective || "").slice(0, 140),
      reasoningEffort: String(item.reasoningEffort || "").slice(0, 12),
      focus: String(item.focus || "").slice(0, 220),
      agentIndex: Number(item.agentIndex || 0),
      agentTotal: Number(item.agentTotal || 0),
      runID: String(item.runID || "").slice(0, 80),
      message: String(item.message || "").slice(0, 260),
      outputPreview: String(item.outputPreview || "").slice(0, 3600),
      typedOutput: String(item.typedOutput || "").slice(0, 3600),
      typingDone: Boolean(item.typingDone),
      tokenStreamed: Boolean(item.tokenStreamed),
      updatedAt: item.updatedAt || new Date().toISOString()
    };
  });
  const expandedAgents = {};
  const sourceExpanded = stream.expandedAgents && typeof stream.expandedAgents === "object" ? stream.expandedAgents : {};
  Object.keys(sourceExpanded).forEach((key) => {
    if (agents[key] && sourceExpanded[key]) expandedAgents[key] = true;
  });
  const normalized = {
    title: String(stream.title || defaults.title).slice(0, 80),
    workflow,
    phaseGroup,
    stageOrder: Number(stream.stageOrder || defaults.stageOrder),
    status: normalizeAgentStreamStatus(stream.status || "running"),
    complexity: String(stream.complexity || "").slice(0, 32),
    agentCount: Number(stream.agentCount || defaults.agentCount),
    phase: String(stream.phase || defaults.phase).slice(0, 40),
    summary: String(stream.summary || defaults.summary).slice(0, 260),
    order: order.filter((key) => agents[key]),
    agents,
    expandedAgents,
    logs: Array.isArray(stream.logs) ? stream.logs.slice(-20).map((item) => ({
      time: item.time || new Date().toISOString(),
      agentKey: String(item.agentKey || ""),
      agent: String(item.agent || "").slice(0, 80),
      status: normalizeAgentStreamStatus(item.status || "running"),
      message: String(item.message || "").slice(0, 260),
      runID: String(item.runID || "").slice(0, 80)
    })) : [],
    updatedAt: stream.updatedAt || new Date().toISOString()
  };
  return repairStaleSynthesisStream(normalized);
}

function repairStaleSynthesisStream(stream) {
  if (stream.phaseGroup !== "synthesis" || stream.status !== "running") return stream;
  const synthesisKey = stream.workflow === "resume/path_planning" ? "path_synthesis_arbiter" : "synthesis_arbiter";
  const item = stream.agents[synthesisKey];
  if (!item || item.status === "done" || item.status === "failed") return stream;
  const updatedAt = Date.parse(item.updatedAt || stream.updatedAt || "");
  if (!Number.isFinite(updatedAt) || Date.now() - updatedAt < 2 * 60 * 1000) return stream;
  item.status = "failed";
  item.message = stream.workflow === "resume/path_planning"
    ? "Path Synthesis Arbiter 未收到结束事件，请查看路径规划阶段状态。"
    : "Synthesis Arbiter 未收到结束事件，请查看匹配阶段状态或重试。";
  item.typingDone = true;
  if (item.outputPreview && !item.typedOutput) item.typedOutput = item.outputPreview;
  stream.status = "failed";
  stream.summary = item.message;
  return stream;
}

function defaultAssistantState() {
  return { version: 1, activeId: "", expanded: false, sessions: [] };
}

function saveAssistantState() {
  if (assistantStateSaveTimer) {
    window.clearTimeout(assistantStateSaveTimer);
    assistantStateSaveTimer = 0;
  }
  assistantState.sessions = assistantState.sessions.slice(0, assistantMaxSessions).map((session) => {
    const focusedContexts = normalizeFocusedContexts(session.focusedContexts || session.focusedContext);
    return {
      ...session,
      focusedContexts,
      focusedContext: focusedContexts.at(-1) || null,
      messages: session.messages.slice(-assistantMaxMessages)
    };
  });
  try {
    window.localStorage.setItem(assistantStorageKey, JSON.stringify(assistantState));
  } catch {
    const active = activeAssistantSession();
    assistantState.sessions = active ? [active] : [];
    try {
      window.localStorage.setItem(assistantStorageKey, JSON.stringify(assistantState));
      showToast("会话存储空间不足，已仅保留当前会话。");
    } catch {
      showToast("浏览器无法保存会话，本次刷新后记录可能丢失。");
    }
  }
}

function scheduleAssistantStateSave(delay = 600) {
  if (assistantStateSaveTimer) window.clearTimeout(assistantStateSaveTimer);
  assistantStateSaveTimer = window.setTimeout(() => {
    assistantStateSaveTimer = 0;
    saveAssistantState();
  }, delay);
}

function ensureAssistantSession() {
  const active = activeAssistantSession();
  if (active && !active.archived) return active;
  return createAssistantSession(false);
}

function createAssistantSession(announce = true, options = {}) {
  const now = new Date().toISOString();
  assistantFocusedEvidence = null;
  assistantFocusedContext = null;
  assistantFocusedContexts = [];
  const session = {
    id: uniqueId("chat"),
    title: String(options.title || "新诊断对话").slice(0, 80),
    prestoSessionId: "",
    diagnosisJobId: String(options.diagnosisJobId || ""),
    diagnosisFileName: String(options.diagnosisFileName || ""),
    focusedEvidence: null,
    focusedContext: null,
    focusedContexts: [],
    archived: false,
    createdAt: now,
    updatedAt: now,
    messages: []
  };
  assistantState.sessions.unshift(session);
  assistantState.activeId = session.id;
  saveAssistantState();
  if (announce) showToast("已创建新对话。");
  return session;
}

function isEmptyAssistantSession(session) {
  return Boolean(
    session &&
    !session.diagnosisJobId &&
    !session.diagnosisFileName &&
    !session.focusedEvidence &&
    !session.focusedContext &&
    (!Array.isArray(session.focusedContexts) || session.focusedContexts.length === 0) &&
    (!Array.isArray(session.messages) || session.messages.length === 0)
  );
}

function pruneEmptyActiveAssistantSession() {
  const active = activeAssistantSession();
  if (!isEmptyAssistantSession(active)) return;
  assistantState.sessions = assistantState.sessions.filter((session) => session.id !== active.id);
  assistantState.activeId = "";
}

function activeAssistantSession() {
  return assistantState.sessions.find((session) => session.id === assistantState.activeId) || null;
}

function toggleAssistantHistory() {
  assistantArchive.open = !assistantArchive.open;
  syncAssistantHistoryView();
}

function closeAssistantHistory() {
  assistantArchive.open = false;
  syncAssistantHistoryView();
}

function restoreAssistantSession(sessionID) {
  const session = assistantState.sessions.find((item) => item.id === sessionID);
  if (!session) return;
  session.archived = false;
  session.updatedAt = new Date().toISOString();
  assistantState.activeId = session.id;
  assistantFocusedEvidence = normalizeFocusedEvidence(session.focusedEvidence);
  assistantFocusedContexts = normalizeFocusedContexts(session.focusedContexts || session.focusedContext);
  assistantFocusedContext = assistantFocusedContexts.at(-1) || normalizeFocusedContext(session.focusedContext);
  assistantArchive.open = false;
  saveAssistantState();
  renderAssistant();
  setAssistantExpanded(true);
  showToast("已打开历史对话。");
}

function deleteAssistantSession(sessionID) {
  const before = assistantState.sessions.length;
  assistantState.sessions = assistantState.sessions.filter((session) => session.id !== sessionID);
  if (assistantState.sessions.length === before) return;
  if (assistantState.activeId === sessionID) {
    assistantState.activeId = "";
    ensureAssistantSession();
  }
  saveAssistantState();
  renderAssistant();
  showToast("历史对话已删除。");
}

function addEvidenceToAssistant(kind, index) {
  if (!isAssistantInspectable()) {
    showToast("生成诊断后可添加到聊天。");
    return;
  }
  const evidence = evidenceContextByKey(kind, index);
  if (!evidence) {
    showToast("未找到这条证据，请重新生成诊断。");
    return;
  }
  const context = focusedContextFromEvidence(evidence);
  assistantFocusedEvidence = evidence;
  const session = ensureAssistantSession();
  appendFocusedContext(context, { evidence });
  session.focusedEvidence = evidence;
  assistantInput.value = context.prompt || assistantInput.value;
  updateAssistantInputMeta();
  saveAssistantState();
  setAssistantExpanded(true);
  renderAssistant();
  assistantInput.focus();
  showToast("已加入聊天上下文。");
}

function addResultToAssistant(kind, index = "", subindex = "") {
  if (!isAssistantInspectable()) {
    showToast("生成诊断后可添加到聊天。");
    return;
  }
  const context = resultContextByKey(kind, index, subindex);
  if (!context) {
    showToast("这部分结果还未生成。");
    return;
  }
  appendFocusedContext(context);
  assistantInput.value = context.prompt || assistantInput.value;
  updateAssistantInputMeta();
  saveAssistantState();
  setAssistantExpanded(true);
  renderAssistant();
  assistantInput.focus();
  showToast("已加入聊天上下文。");
}

function clearAssistantFocusedEvidence(key = "") {
  if (key) {
    removeFocusedContext(key);
    saveAssistantState();
    renderAssistant();
    showToast("已移除聊天上下文。");
    return;
  }
  assistantFocusedEvidence = null;
  assistantFocusedContext = null;
  assistantFocusedContexts = [];
  const session = activeAssistantSession();
  if (session) {
    session.focusedEvidence = null;
    session.focusedContext = null;
    session.focusedContexts = [];
    session.updatedAt = new Date().toISOString();
  }
  saveAssistantState();
  renderAssistant();
  showToast("已清空聊天上下文。");
}

function evidenceContextByKey(kind, index) {
  const profile = diagnosis?.ability_profile || {};
  const list = kind === "award"
    ? Array.isArray(profile.awards) ? profile.awards : []
    : kind === "experience"
      ? Array.isArray(profile.experiences) ? profile.experiences : []
      : [];
  const item = list[index];
  if (!item) return null;
  const context = kind === "award" ? assistantAwardContext(item) : assistantExperienceContext(item);
  const title = kind === "award"
    ? cleanDisplayText(item.name || item.result)
    : cleanDisplayText(item.role || item.contribution || item.type);
  return {
    ...context,
    key: `${kind}:${index}`,
    kind,
    type: cleanDisplayText(item.type || context.type || ""),
    title: title || (kind === "award" ? "奖项与证书" : "经历"),
    score_summary: evidenceScoreSummary(item),
    dimension_summary: evidenceDimensionSummary(item)
  };
}

function resultContextByKey(kind, index = "", subindex = "") {
  const profile = diagnosis?.ability_profile || {};
  const match = diagnosis?.matching_result || {};
  const pathPlan = diagnosis?.path_plan || {};
  const jobs = Array.isArray(profile.top5_matching_jobs) ? profile.top5_matching_jobs : [];
  const reportRows = normalizedMatchingReportRows(match);
  const gaps = Array.isArray(match.gap_details) ? match.gap_details : [];
  const stages = Array.isArray(pathPlan.stages) ? pathPlan.stages : [];
  const numericIndex = Number(index);
  const numericSubindex = Number(subindex);
  switch (kind) {
    case "basic":
      return makeFocusedContext({
        key: "basic",
        category: "能力画像",
        type: "基础信息",
        title: profile.basic_info?.name ? `${profile.basic_info.name}的基础信息` : "基础信息",
        summary: "基础信息、学校、学院、专业和上传材料来源。",
        data: {
          basic_info: profile.basic_info || {},
          education: normalizedEducation(profile),
          input_sources: diagnosis?.input_sources || []
        }
      });
    case "profile_radar":
      return makeFocusedContext({
        key: "profile_radar",
        category: "能力画像",
        type: "六维雷达",
        title: "六维能力画像",
        summary: "学生当前能力分布、Benchmark 状态和雷达序列。",
        data: {
          benchmark_status: profile.benchmark_status || "",
          radar_series: normalizedBackendRadarSeries(profile),
          six_dim_scores: radarScoresToMatchingDimensions(normalizedProfileRadarData(profile))
        }
      });
    case "match_summary":
      return makeFocusedContext({
        key: "match_summary",
        category: "岗位匹配",
        type: "首选岗位",
        title: match.selected_job?.title || match.target_role || "首选岗位匹配",
        summary: match.fit_summary || match.selected_job?.fit_summary || "首选岗位匹配度、推荐理由和下一步证据。",
        data: {
          selected_job: match.selected_job || {},
          overall_match: match.overall_match || "",
          match_level: match.match_level || "",
          reasons: matchingReasonList(match, match.selected_job || {}),
          main_gap: primaryMatchingGap(match, reportRows),
          development_actions: structuredDevelopmentActions(match)
        }
      });
    case "matching_radar":
      return makeFocusedContext({
        key: "matching_radar",
        category: "岗位匹配",
        type: "目标雷达",
        title: "个人画像与岗位目标雷达",
        summary: "个人能力与岗位目标能力的六维差距。",
        data: {
          target_role: match.target_role || match.selected_job?.title || "",
          student_radar: matchingStudentRadar(match),
          target_radar: matchingTargetRadar(match),
          report_rows: reportRows,
          summary: matchingRadarSummary(reportRows)
        }
      });
    case "recommendation_queue":
      return makeFocusedContext({
        key: "recommendation_queue",
        category: "岗位匹配",
        type: "推荐队列",
        title: "推荐岗位队列",
        summary: "TOP 推荐岗位及每个岗位的匹配度、理由和证据缺口。",
        data: { jobs: jobs.slice(0, 5) }
      });
    case "job":
    case "output_job": {
      const job = jobs[numericIndex];
      if (!job) return null;
      return makeFocusedContext({
        key: `${kind}:${numericIndex}`,
        category: kind === "output_job" ? "结构化输出" : "岗位匹配",
        type: "推荐岗位",
        title: job.title || `推荐岗位 ${numericIndex + 1}`,
        summary: job.fit_summary || (Array.isArray(job.reasons) ? job.reasons.join("；") : ""),
        data: job
      });
    }
    case "report":
      return makeFocusedContext({
        key: "report",
        category: "岗位匹配",
        type: "差距报表",
        title: "六维差距报表",
        summary: "每个能力维度的学生分值、岗位要求和差距。",
        data: { rows: reportRows }
      });
    case "report_row": {
      const row = reportRows[numericIndex];
      if (!row) return null;
      return makeFocusedContext({
        key: `report_row:${numericIndex}`,
        category: "岗位匹配",
        type: "能力维度",
        title: `${row.name}维度差距`,
        summary: row.note || matchingRowNote(row),
        data: row
      });
    }
    case "gap": {
      const gap = gaps[numericIndex];
      if (!gap) return null;
      return makeFocusedContext({
        key: `gap:${numericIndex}`,
        category: "岗位匹配",
        type: "差距明细",
        title: gap.capability || `差距 ${numericIndex + 1}`,
        summary: gap.action || gap.expected || gap.current || "",
        data: gap
      });
    }
    case "gap_summary":
      return makeFocusedContext({
        key: "gap_summary",
        category: "岗位匹配",
        type: "差距明细",
        title: "差距明细",
        summary: "岗位能力差距、当前证据、目标要求和补强建议。",
        data: { gaps }
      });
    case "path_stage": {
      const stage = stages[numericIndex];
      if (!stage) return null;
      return makeFocusedContext({
        key: `path_stage:${numericIndex}`,
        category: "路径规划",
        type: "阶段目标",
        title: stage.stage || `第 ${numericIndex + 1} 阶段`,
        summary: stage.goal || stage.deliverable || "",
        data: stage
      });
    }
    case "path_task": {
      const stage = stages[numericIndex];
      const week = Array.isArray(stage?.weeks) ? stage.weeks[numericSubindex] : null;
      if (!stage || !week) return null;
      return makeFocusedContext({
        key: `path_task:${numericIndex}:${numericSubindex}`,
        category: "路径规划",
        type: "周任务",
        title: `${stage.stage || `第 ${numericIndex + 1} 阶段`}：${week.week || `任务 ${numericSubindex + 1}`}`,
        summary: week.task || week.metric || "",
        data: { stage: stage.stage || "", goal: stage.goal || "", task: week }
      });
    }
    case "top_jobs":
      return makeFocusedContext({
        key: "top_jobs",
        category: "结构化输出",
        type: "TOP5 岗位",
        title: "TOP5 匹配岗位",
        summary: "结构化输出中的 TOP5 岗位列表。",
        data: { jobs: jobs.slice(0, 5) }
      });
    default:
      return null;
  }
}

function makeFocusedContext({ key, category, type, title, summary, data }) {
  const cleanTitle = cleanDisplayText(title || type || category || "生成结果");
  const cleanType = cleanDisplayText(type || category || "结果");
  const cleanCategory = cleanDisplayText(category || "生成结果");
  const cleanSummary = cleanDisplayText(summary || "");
  return {
    key: String(key || cleanType).slice(0, 120),
    kind: "result",
    category: cleanCategory,
    type: cleanType,
    title: cleanTitle.slice(0, 120),
    summary: cleanSummary.slice(0, 500),
    data,
    prompt: `请围绕「${cleanTitle}」解释关键依据、风险和下一步该怎么做。`
  };
}

function focusedContextFromEvidence(evidence) {
  return makeFocusedContext({
    key: evidence.key,
    category: evidence.kind === "award" ? "奖项与证书" : "经历",
    type: assistantEvidenceTypeLabel(evidence),
    title: assistantEvidenceDisplayTitle(evidence, assistantEvidenceTypeLabel(evidence)),
    summary: evidence.reason || "",
    data: evidence
  });
}

function evidenceScoreSummary(item) {
  const level = numericMetric(item?.level);
  const impact = numericMetric(item?.impact_factor);
  return {
    level: hasMetricValue(level) ? formatMetric(level) : "",
    impact_factor: hasMetricValue(impact) ? formatMetric(impact) : ""
  };
}

function evidenceDimensionSummary(item) {
  const scores = normalizedBenchmarkScores(item);
  if (!scores.length) return [];
  const dimensions = Array.isArray(item?.benchmark_dimensions) && item.benchmark_dimensions.length === scores.length
    ? item.benchmark_dimensions
    : benchmarkDimensions;
  return scores.map((score, index) => ({
    name: dimensions[index] || benchmarkDimensions[index] || `维度${index + 1}`,
    weight: Math.round(score * 100)
  }));
}

function normalizeFocusedEvidence(evidence) {
  if (!evidence || typeof evidence !== "object") return null;
  const key = typeof evidence.key === "string" ? evidence.key.slice(0, 80) : "";
  if (!key) return null;
  const kind = evidence.kind === "award" || evidence.kind === "experience" ? evidence.kind : "experience";
  return {
    ...evidence,
    key,
    kind,
    type: cleanDisplayText(evidence.type || ""),
    title: cleanDisplayText(evidence.title || (kind === "award" ? evidence.name || evidence.result : evidence.role || evidence.contribution)).slice(0, 120) || (kind === "award" ? "奖项与证书" : "经历证据"),
    score_summary: evidence.score_summary && typeof evidence.score_summary === "object" ? evidence.score_summary : {},
    dimension_summary: Array.isArray(evidence.dimension_summary) ? evidence.dimension_summary.slice(0, 8) : []
  };
}

function normalizeFocusedContext(context) {
  if (!context || typeof context !== "object") return null;
  const key = typeof context.key === "string" ? context.key.slice(0, 120) : "";
  if (!key) return null;
  return {
    ...context,
    key,
    kind: cleanDisplayText(context.kind || "result"),
    category: cleanDisplayText(context.category || "生成结果"),
    type: cleanDisplayText(context.type || "结果"),
    title: cleanDisplayText(context.title || context.type || "生成结果").slice(0, 120),
    summary: cleanDisplayText(context.summary || "").slice(0, 500),
    prompt: cleanDisplayText(context.prompt || "")
  };
}

function normalizeFocusedContexts(value) {
  const source = Array.isArray(value) ? value : value ? [value] : [];
  const byKey = new Map();
  source.forEach((item) => {
    const context = normalizeFocusedContext(item);
    if (!context) return;
    byKey.delete(context.key);
    byKey.set(context.key, context);
  });
  return Array.from(byKey.values()).slice(-8);
}

function focusedContextKeySet() {
  return new Set(focusedContextsForAssistant({ persist: false }).map((context) => context.key));
}

function isFocusedContextSelected(key) {
  return Boolean(key && focusedContextKeySet().has(key));
}

function persistFocusedContexts(contexts, options = {}) {
  const normalized = normalizeFocusedContexts(contexts);
  const latestContext = normalized.at(-1) || null;
  assistantFocusedContexts = normalized;
  assistantFocusedContext = latestContext;
  if (options.evidence !== undefined) assistantFocusedEvidence = options.evidence;
  const session = ensureAssistantSession();
  session.focusedContexts = normalized;
  session.focusedContext = latestContext;
  if (options.evidence !== undefined) session.focusedEvidence = options.evidence;
  session.updatedAt = new Date().toISOString();
  return normalized;
}

function appendFocusedContext(context, options = {}) {
  const normalized = normalizeFocusedContext(context);
  if (!normalized) return [];
  const current = focusedContextsForAssistant({ persist: false });
  return persistFocusedContexts([...current.filter((item) => item.key !== normalized.key), normalized], options);
}

function removeFocusedContext(key) {
  const cleanKey = String(key || "").slice(0, 120);
  if (!cleanKey) return focusedContextsForAssistant({ persist: false });
  const current = focusedContextsForAssistant({ persist: false });
  const next = current.filter((context) => context.key !== cleanKey);
  const removedEvidence = assistantFocusedEvidence?.key === cleanKey;
  return persistFocusedContexts(next, { evidence: removedEvidence ? null : assistantFocusedEvidence });
}

function clearAssistantFocusedContextsSilently(session = activeAssistantSession()) {
  assistantFocusedEvidence = null;
  assistantFocusedContext = null;
  assistantFocusedContexts = [];
  if (session) {
    session.focusedEvidence = null;
    session.focusedContext = null;
    session.focusedContexts = [];
    session.updatedAt = new Date().toISOString();
  }
}

function focusedEvidenceForAssistant() {
  const session = activeAssistantSession();
  const focused = normalizeFocusedEvidence(session?.focusedEvidence) || normalizeFocusedEvidence(assistantFocusedEvidence);
  assistantFocusedEvidence = focused;
  if (session && focused && session.focusedEvidence !== focused) session.focusedEvidence = focused;
  return focused;
}

function focusedContextForAssistant() {
  const focused = focusedContextsForAssistant().at(-1);
  assistantFocusedContext = focused;
  return focused;
}

function focusedContextsForAssistant(options = {}) {
  const persist = options.persist !== false;
  const session = activeAssistantSession();
  const evidence = normalizeFocusedEvidence(session?.focusedEvidence) || normalizeFocusedEvidence(assistantFocusedEvidence);
  const evidenceContext = evidence ? focusedContextFromEvidence(evidence) : null;
  const merged = normalizeFocusedContexts([
    evidenceContext,
    ...(Array.isArray(session?.focusedContexts) ? session.focusedContexts : []),
    ...(Array.isArray(assistantFocusedContexts) ? assistantFocusedContexts : []),
    session?.focusedContext,
    assistantFocusedContext
  ]);
  assistantFocusedContexts = merged;
  assistantFocusedContext = merged.at(-1) || null;
  if (session && persist) {
    session.focusedContexts = merged;
    session.focusedContext = assistantFocusedContext;
  }
  return merged;
}

function renderAssistantEvidenceTray() {
  if (!assistantEvidenceTray) return;
  const contexts = focusedContextsForAssistant();
  assistantEvidenceTray.hidden = contexts.length === 0;
  if (contexts.length === 0) {
    assistantEvidenceTray.innerHTML = "";
    return;
  }
  assistantEvidenceTray.innerHTML = contexts.map((context) => {
    const typeLabel = context.type || "结果";
    const title = context.title || "生成结果";
    const category = context.category || "生成结果";
    return `
    <div class="assistant-evidence-chip" title="${escapeAttribute(`${typeLabel}：${title}`)}" data-context-chip="${escapeAttribute(context.key)}">
      <span class="assistant-evidence-type">${escapeHTML(typeLabel)}</span>
      <strong class="assistant-evidence-title">${escapeHTML(title)}</strong>
      <span class="assistant-evidence-category">${escapeHTML(category)}</span>
      <button type="button" data-clear-evidence-context="${escapeAttribute(context.key)}" aria-label="移除聊天上下文" title="移除聊天上下文">×</button>
    </div>
  `;
  }).join("");
}

function assistantEvidenceDisplayTitle(evidence, typeLabel) {
  let title = cleanDisplayText(evidence?.title || "");
  const type = cleanDisplayText(typeLabel || evidence?.type || "");
  if (type) {
    const escapedType = escapeRegExp(type);
    title = title.replace(new RegExp(`^${escapedType}\\s*[·•|｜:：/\\-—]+\\s*`), "");
  }
  title = title.replace(/^(项目|经历|实习|科研项目|课程项目|校园活动|社会实践|竞赛项目)\s*[·•|｜:：/\-—]+\s*/, "");
  return title || type || "重点证据";
}

function assistantEvidenceTypeLabel(evidence) {
  if (evidence?.kind === "award") {
    const text = cleanDisplayText(`${evidence?.type || ""} ${evidence?.title || ""}`);
    if (/证书|认证|certificate|certification/i.test(text)) return "证书";
    if (/竞赛|比赛|大赛|获奖|奖/i.test(text)) return "竞赛奖项";
    return "奖项证书";
  }
  const explicitType = cleanDisplayText(evidence?.type || "");
  const text = cleanDisplayText(`${explicitType} ${evidence?.role || ""} ${evidence?.title || ""}`);
  if (/科研|课题|论文|实验室|research/i.test(text)) return "科研项目";
  if (/课程|课设|course/i.test(text)) return "课程项目";
  if (/实习|intern/i.test(text)) return "实习经历";
  if (/竞赛|比赛|大赛|challenge|competition/i.test(text)) return "竞赛项目";
  if (/社团|学生会|校园|志愿|活动|实践/i.test(text)) return "校园实践";
  if (/项目|project/i.test(text)) return explicitType || "项目经历";
  return explicitType || "经历";
}

function assistantEvidenceCategoryLabel(evidence) {
  return evidence?.kind === "award" ? "奖项与证书" : "经历";
}

function assistantEvidenceContextMessage(evidence) {
  const typeLabel = evidence.kind === "award" ? "奖项与证书" : "经历证据";
  const scoreParts = [
    evidence.score_summary.level ? `Level ${evidence.score_summary.level}/10` : "",
    evidence.score_summary.impact_factor ? `Impact ${evidence.score_summary.impact_factor}/10` : ""
  ].filter(Boolean);
  const dimensionText = evidence.dimension_summary.length
    ? `六维权重：${evidence.dimension_summary.map((item) => `${item.name}${item.weight}%`).join("，")}`
    : "六维权重尚未返回。";
  const reason = cleanDisplayText(evidence.reason);
  return [
    `已加入重点上下文：${typeLabel}「${evidence.title}」`,
    scoreParts.length ? scoreParts.join("，") : "Level / Impact 仍在等待评分。",
    dimensionText,
    reason ? `评分依据：${reason}` : ""
  ].filter(Boolean).join("\n");
}

function isAssistantReady() {
  const allModulesReady = moduleLocks.profile && moduleLocks.matching && moduleLocks.path && moduleLocks.outputs;
  const benchmarkStatus = diagnosis?.ability_profile?.benchmark_status || "";
  return Boolean(
    diagnosis &&
    allModulesReady &&
    !diagnosisEvents &&
    !benchmarkRequestInFlight &&
    !matchingRequestInFlight &&
    !failedRunStep &&
    benchmarkStatus !== "benchmarking" &&
    benchmarkStatus !== "failed"
  );
}

function isAssistantInspectable() {
  return isAssistantReady() || assistantAgentStreamActive || Boolean(diagnosisEvents) || benchmarkRequestInFlight || matchingRequestInFlight || Boolean(diagnosis);
}

function syncAssistantAvailability() {
  const ready = isAssistantReady();
  const inspectable = isAssistantInspectable();
  const inspectOnly = inspectable && !ready;
  assistant.classList.toggle("is-locked", !inspectable);
  assistant.classList.toggle("is-inspect-only", inspectOnly);
  assistant.classList.toggle("is-streaming", assistantAgentStreamActive && !ready);
  assistantToggle.setAttribute("aria-disabled", String(!inspectable));
  assistantPanel.setAttribute("aria-disabled", "false");
  assistantInput.disabled = assistantBusy || !inspectable;
  assistantSend.disabled = assistantBusy || !ready;
  assistantNewSession.disabled = assistantBusy || !ready;
  assistantArchiveSession.disabled = assistantBusy || !ready;
  assistantSuggestions.querySelectorAll("button").forEach((button) => {
    button.disabled = assistantBusy || !ready;
  });
  const selectedContextKeys = focusedContextKeySet();
  document.querySelectorAll("[data-evidence-chat]").forEach((button) => {
    const selected = selectedContextKeys.has(button.dataset.evidenceKey);
    button.disabled = !inspectable;
    button.setAttribute("aria-disabled", String(!inspectable));
    button.setAttribute("aria-pressed", String(selected));
    button.classList.toggle("is-added", selected);
    button.title = selected ? "已加入聊天" : inspectable ? "添加到聊天" : "生成诊断后可添加到聊天";
  });
  document.querySelectorAll("[data-result-chat]").forEach((button) => {
    const selected = selectedContextKeys.has(button.dataset.contextKey);
    button.disabled = !inspectable;
    button.setAttribute("aria-disabled", String(!inspectable));
    button.setAttribute("aria-pressed", String(selected));
    button.classList.toggle("is-added", selected);
    button.title = selected ? "已加入聊天上下文" : inspectable ? "添加到聊天" : "生成诊断后可添加到聊天";
  });
  if (!inspectable && !assistant.classList.contains("is-collapsed")) {
    setAssistantExpanded(false, { silent: true });
  }
  if (ready) {
    updateAssistantRailStatus(assistantBusy ? "生成中" : "可追问");
  } else if (inspectOnly) {
    updateAssistantRailStatus("生成中可查看");
  } else {
    updateAssistantRailStatus(diagnosisEvents || benchmarkRequestInFlight || matchingRequestInFlight ? "结果生成中" : "结果完成后可追问");
  }
}

function renderAssistant() {
  ensureAssistantSession();
  applyAssistantExpandedState(assistantState.expanded);
  renderAssistantArchive();
  renderAssistantMessages();
  renderAssistantEvidenceTray();
  renderAssistantSuggestions();
  updateAssistantContext();
  updateAssistantInputMeta();
  syncAssistantAvailability();
  syncAssistantHistoryView();
}

function syncAssistantHistoryView() {
  const isHistory = Boolean(assistantArchive.open);
  assistant.classList.toggle("is-history-view", isHistory);
  assistantArchiveSession.setAttribute("aria-expanded", String(isHistory));
  assistantTitle.textContent = isHistory ? "历史对话" : "AI助手";
}

function renderAssistantArchive() {
  const history = assistantState.sessions.filter((session) => session.id !== assistantState.activeId && !isEmptyAssistantSession(session));
  assistantArchiveCount.textContent = String(history.length);
  assistantArchiveList.innerHTML = history.length
    ? history.map((session) => `
      <div class="assistant-archive-item">
        <span title="${escapeAttribute(session.title)}">${escapeHTML(session.title || "历史对话")}</span>
        <button type="button" data-restore-session="${escapeAttribute(session.id)}">打开</button>
        <button type="button" class="assistant-delete-session" data-delete-session="${escapeAttribute(session.id)}" aria-label="删除历史对话">删除</button>
      </div>
    `).join("")
    : `<div class="assistant-archive-empty">暂无历史对话。</div>`;
}

function renderAssistantMessages(options = {}) {
  const session = activeAssistantSession();
  const messages = assistantDisplayMessages(session?.messages || []);
  if (messages.length === 0) {
    assistantMessages.innerHTML = `<div class="assistant-empty">${isAssistantReady() ? "继续追问诊断结果。" : assistantAgentStreamActive ? "正在接收 Agent Team 流。" : "诊断完成后可追问。"}</div>`;
    return;
  }
  const stickToBottom = options.stickToBottom === true;
  const previousScrollTop = assistantMessages.scrollTop;
  const openLogs = new Set([...assistantMessages.querySelectorAll("[data-agent-stream-log][open]")]
    .map((node) => node.dataset.agentStreamLog));
  const typeoutState = captureAgentTypeoutState();
  assistantMessages.innerHTML = messages.map((message) => {
    const label = message.streamType === "evidence_context" ? "重点证据" : message.role === "user" ? "你" : message.role === "system" ? "系统" : "AI助手";
    const stateClass = [
      message.status === "loading" ? " is-loading" : message.status === "error" ? " is-error" : "",
      message.isStreamingAnswer ? " is-answer-streaming" : ""
    ].join("");
    const streamClass = message.streamType === "agent_team" ? " is-agent-stream" : message.streamType === "evidence_context" ? " is-evidence-context" : "";
    const content = message.role === "user" ? message.content : formatAgentDisplayText(message.content);
    const userRefs = message.role === "user" ? renderAssistantMessageRefs(message.focusedContexts) : "";
    const body = message.streamType === "agent_team" && message.agentStream
      ? renderAgentTeamStreamMessage(message.agentStream, message.id)
      : renderAssistantTextBubble(message, content);
    return `
      <article class="assistant-message is-${escapeAttribute(message.role)}${stateClass}${streamClass}" data-assistant-message-id="${escapeAttribute(message.id)}">
        <b>${escapeHTML(label)} · ${formatTime(message.createdAt)}</b>
        ${body}
        ${userRefs}
        ${message.status === "error" && message.retryPrompt ? `<button type="button" class="assistant-retry" data-retry-message="${escapeAttribute(message.id)}">重试这条问题</button>` : ""}
      </article>
    `;
  }).join("");
  openLogs.forEach((key) => {
    const detail = assistantMessages.querySelector(`[data-agent-stream-log="${CSS.escape(key)}"]`);
    if (detail) detail.open = true;
  });
  restoreAgentTypeoutState(typeoutState);
  if (stickToBottom) {
    assistantMessages.scrollTop = assistantMessages.scrollHeight;
  } else {
    const maxScrollTop = Math.max(0, assistantMessages.scrollHeight - assistantMessages.clientHeight);
    assistantMessages.scrollTop = Math.min(previousScrollTop, maxScrollTop);
  }
}

function renderAssistantTextBubble(message, content) {
  const cursor = message.role === "assistant"
    ? `<span class="assistant-stream-cursor" aria-hidden="true"></span>`
    : "";
  return `<div class="assistant-bubble"><span data-assistant-message-text>${escapeHTML(content)}</span>${cursor}</div>`;
}

function renderAssistantMessageRefs(contexts) {
  const refs = normalizeFocusedContexts(contexts);
  if (refs.length === 0) return "";
  const visibleRefs = refs.slice(0, 4);
  const rest = refs.length - visibleRefs.length;
  return `
    <div class="assistant-message-refs" aria-label="本条问题引用的上下文">
      <span>引用</span>
      ${visibleRefs.map((context) => `
        <em title="${escapeAttribute(`${context.category || "生成结果"}：${context.title || context.type || "上下文"}`)}">
          ${escapeHTML(context.type || context.category || "结果")} · ${escapeHTML(context.title || "上下文")}
        </em>
      `).join("")}
      ${rest > 0 ? `<em>+${rest}</em>` : ""}
    </div>
  `;
}

function captureAgentTypeoutState() {
  const state = new Map();
  assistantMessages.querySelectorAll("[data-agent-typeout]").forEach((node) => {
    const key = node.dataset.agentTypeout;
    const textNode = node.querySelector("[data-agent-typeout-text]");
    if (!key || !textNode) return;
    state.set(key, {
      text: textNode.textContent || "",
      scrollTop: node.scrollTop,
      stickToBottom: node.scrollHeight - node.scrollTop - node.clientHeight < 24
    });
  });
  return state;
}

function restoreAgentTypeoutState(state) {
  if (!state?.size) return;
  assistantMessages.querySelectorAll("[data-agent-typeout]").forEach((node) => {
    const key = node.dataset.agentTypeout;
    const previous = state.get(key);
    const textNode = node.querySelector("[data-agent-typeout-text]");
    if (!previous || !textNode) return;
    if (!textNode.textContent && previous.text) textNode.textContent = previous.text;
    window.requestAnimationFrame(() => {
      node.scrollTop = previous.stickToBottom ? node.scrollHeight : Math.min(previous.scrollTop, node.scrollHeight);
    });
  });
}

function assistantDisplayMessages(messages) {
  return messages.flatMap((message) => (
    shouldSplitRenderedAgentStreamMessage(message)
      ? splitLegacyAgentStreamMessage(message)
      : [message]
  ));
}

function shouldSplitRenderedAgentStreamMessage(message) {
  const stream = message?.agentStream;
  if (message?.streamType !== "agent_team" || !stream) return false;
  if (stream.workflow === "resume/path_planning") return false;
  const keys = new Set(stream.order || []);
  return keys.has("adaptive_planner") && keys.has("synthesis_arbiter") && stream.order.length > 2;
}

function isAgentStreamExpanded(stream, agentKey) {
  const key = String(agentKey || "");
  return Boolean(key && stream?.expandedAgents?.[key]);
}

function shouldStickAssistantToBottom() {
  const distance = assistantMessages.scrollHeight - assistantMessages.scrollTop - assistantMessages.clientHeight;
  return distance < 96;
}

function renderAgentTeamStreamMessage(stream, messageID = "") {
  const config = agentStreamPhaseConfig(stream.phaseGroup, stream.workflow);
  const workflowClass = stream.workflow === "resume/path_planning" ? " is-path-planning" : " is-job-matching";
  const completed = stream.order.filter((key) => stream.agents[key]?.status === "done").length;
  const failed = stream.order.filter((key) => stream.agents[key]?.status === "failed").length;
  const total = stream.phaseGroup === "team" ? stream.agentCount || stream.order.length : 1;
  const statusLabel = stream.status === "done" ? "完成" : stream.status === "failed" ? "失败" : "运行中";
  const progressLabel = stream.phaseGroup === "team"
    ? `${completed}/${total || "--"} 个视角${failed ? ` · ${failed} 个失败` : ""}`
    : config.label;
  return `
    <div class="assistant-bubble agent-stream-card is-${escapeAttribute(config.group)}${workflowClass}">
      <div class="agent-stream-head">
        <div class="agent-stream-title">
          <strong>${escapeHTML(stream.title || "Agent Team 正在匹配岗位")}</strong>
          <span>阶段 ${config.stageOrder}/3 · ${escapeHTML(statusLabel)} · ${escapeHTML(progressLabel)}</span>
        </div>
        ${stream.complexity ? `<span class="agent-stream-complexity">${escapeHTML(stream.complexity)}</span>` : ""}
      </div>
      ${stream.summary ? `<p class="agent-stream-summary">${escapeHTML(formatAgentDisplayText(stream.summary))}</p>` : ""}
      <div class="agent-stream-queue">
        ${stream.order.length ? stream.order.map((key) => renderAgentQueueItem(stream.agents[key], Boolean(stream.expandedAgents?.[key]), messageID)).join("") : `<div class="agent-stream-empty">${escapeHTML(config.empty)}</div>`}
      </div>
      ${renderAgentStreamLog(stream.logs, `${messageID}:${config.group}`)}
    </div>
  `;
}

function renderAgentStreamStage(label, active, done) {
  return `<span class="agent-stream-stage${active ? " is-active" : ""}${done ? " is-done" : ""}">${escapeHTML(label)}</span>`;
}

function renderAgentQueueItem(agent, expanded = false, messageID = "") {
  if (!agent) return "";
  const status = agent.status || "queued";
  const statusLabel = {
    queued: "等待",
    running: "运行",
    streaming: "推理",
    done: "完成",
    failed: "失败"
  }[status] || "运行";
  const index = agent.agentIndex && agent.agentTotal ? `${agent.agentIndex}/${agent.agentTotal}` : "";
  return `
    <button type="button" class="agent-stream-agent is-${escapeAttribute(status)}${expanded ? " is-expanded" : ""}" data-agent-stream-toggle="${escapeAttribute(agent.key)}" data-agent-stream-message="${escapeAttribute(messageID)}" aria-expanded="${expanded}">
      <span class="agent-stream-agent-head">
        <span class="agent-stream-state">${escapeHTML(statusLabel)}</span>
        <strong class="agent-stream-agent-name">${escapeHTML(formatAgentDisplayText(agent.agent || agent.key))}</strong>
        ${index ? `<span class="agent-stream-index">${escapeHTML(index)}</span>` : ""}
        <span class="agent-stream-expand" aria-hidden="true">${expanded ? "-" : "+"}</span>
      </span>
      <span class="agent-stream-meta">
        ${agent.perspective ? `<span>${escapeHTML(formatAgentDisplayText(agent.perspective))}</span>` : ""}
        ${agent.reasoningEffort ? `<span class="agent-stream-effort">${renderEffortBars(agent.reasoningEffort)}${escapeHTML(agent.reasoningEffort)}</span>` : ""}
        ${agent.runID ? `<span class="agent-stream-run-id">${escapeHTML(shortRunId(agent.runID))}</span>` : ""}
      </span>
      ${agent.focus ? `<span class="agent-stream-focus">${escapeHTML(formatAgentDisplayText(agent.focus))}</span>` : ""}
      ${agent.message ? `<span class="agent-stream-message">${escapeHTML(formatAgentDisplayText(agent.message))}</span>` : ""}
      ${expanded ? renderAgentStreamDetail(agent, messageID) : ""}
    </button>
  `;
}

function renderAgentStreamDetail(agent, messageID = "") {
  const hasOutput = Boolean(agent.outputPreview);
  const isComplete = agent.status === "done" || agent.status === "failed" || agent.typingDone;
  const text = hasOutput
    ? isComplete
      ? agent.outputPreview || agent.typedOutput || ""
      : agent.typedOutput || agent.outputPreview || ""
    : agent.message || agent.focus || "等待 Agent 输出。";
  const cursor = hasOutput && !isComplete ? `<span class="agent-stream-cursor" aria-hidden="true"></span>` : "";
  const liveState = agentStreamLiveState(agent, hasOutput);
  const typeoutKey = `${messageID}:${agent.key}`;
  return `
    <span class="agent-stream-detail">
      <span class="agent-output-board" data-agent-structured-output="${escapeAttribute(typeoutKey)}" data-agent-message="${escapeAttribute(messageID)}" data-agent-key="${escapeAttribute(agent.key)}">${renderAgentStructuredOutputContent(text, agent)}</span>
      <span class="agent-output-live">
        <span class="agent-output-live-head"><span>实时流</span><b>${liveState}</b></span>
        <span class="agent-stream-typeout" data-agent-typeout="${escapeAttribute(typeoutKey)}" data-agent-message="${escapeAttribute(messageID)}" data-agent-key="${escapeAttribute(agent.key)}"><span data-agent-typeout-text="${escapeAttribute(typeoutKey)}" data-agent-message="${escapeAttribute(messageID)}" data-agent-key="${escapeAttribute(agent.key)}">${escapeHTML(formatAgentDisplayText(text))}</span>${cursor}</span>
      </span>
    </span>
  `;
}

function renderAgentStructuredOutputContent(text, agent = {}) {
  const model = buildAgentOutputViewModel(text, agent);
  if (model.isEmpty) {
    return `
      <span class="agent-output-kicker"><i aria-hidden="true"></i><span>重点视图</span><b>等待</b></span>
      <span class="agent-output-skeleton">
        <span></span><span></span><span></span>
      </span>
    `;
  }
  const metricItems = layoutAgentOutputItems(model.metrics);
  const sectionItems = layoutAgentOutputItems(model.sections);
  return `
    <span class="agent-output-kicker"><i aria-hidden="true"></i><span>重点视图</span><b>${escapeHTML(model.sourceLabel)}</b></span>
    ${model.summary ? `<span class="agent-output-summary">${escapeHTML(model.summary)}</span>` : ""}
    ${metricItems.length ? `<span class="agent-output-metrics">${metricItems.map(({ item, index, span }) => `<span style="--agent-output-span:${span}; --agent-section-index:${index}"><b>${escapeHTML(item.label)}</b>${escapeHTML(item.value)}</span>`).join("")}</span>` : ""}
    ${sectionItems.length ? `<span class="agent-output-sections">
      ${sectionItems.map(({ item: section, index, span }) => `
        <span class="agent-output-section" style="--agent-output-span:${span}; --agent-section-index:${index}">
          <span class="agent-output-section-title">${escapeHTML(section.label)}</span>
          ${section.items.map((item) => `<span class="agent-output-point">${escapeHTML(item)}</span>`).join("")}
        </span>
      `).join("")}
    </span>` : ""}
  `;
}

function layoutAgentOutputItems(items, maxPerRow = 3) {
  const list = Array.isArray(items) ? items.filter(Boolean) : [];
  const rows = [];
  for (let index = 0; index < list.length; index += maxPerRow) {
    rows.push(list.slice(index, index + maxPerRow));
  }
  return rows.flatMap((row, rowIndex) => {
    const span = agentOutputSpanForRowCount(row.length);
    return row.map((item, rowItemIndex) => ({
      item,
      span,
      index: rowIndex * maxPerRow + rowItemIndex
    }));
  });
}

function agentOutputSpanForRowCount(count) {
  if (count <= 1) return 6;
  if (count === 2) return 3;
  return 2;
}

function agentStreamLiveState(agent, hasOutput = Boolean(agent?.outputPreview)) {
  if (agent?.status === "failed") return "已失败";
  if (agent?.status === "done" || agent?.typingDone) return "已完成";
  return hasOutput ? "正在组织" : "等待输出";
}

function buildAgentOutputViewModel(text, agent = {}) {
  const cleaned = stripAgentOutputText(text);
  const payload = parseAgentOutputPayload(cleaned);
  if (payload) {
    return buildStructuredAgentOutputView(payload, agent, cleaned);
  }
  const fallbackItems = extractReadablePoints(cleaned, 5);
  const summary = compactAgentText(fallbackItems[0] || agent.message || agent.focus || "", 120);
  const sections = fallbackItems.length
    ? [{ label: "关键内容", items: fallbackItems.slice(0, 4).map((item) => compactAgentText(item, 86)) }]
    : [];
  return {
    isEmpty: !summary && sections.length === 0,
    sourceLabel: "文本",
    summary,
    metrics: agent.reasoningEffort ? [{ label: "思考", value: agent.reasoningEffort }] : [],
    sections
  };
}

function buildStructuredAgentOutputView(payload, agent, rawText) {
  const summary = compactAgentText(firstAgentOutputValue(payload, [
    "summary",
    "conclusion",
    "rationale",
    "recommendation",
    "recommended_jobs_scope",
    "path_plan_summary",
    "method_summary",
    "target_role",
    "goal",
    "objective",
    "fit_summary",
    "final_recommendation",
    "decision"
  ]) || agent.message || agent.focus || "", 132);
  const sectionDefs = [
    { label: "推荐方向", keys: ["recommended_jobs", "recommended_jobs_scope", "job_families", "target_roles", "target_role", "best_fit_roles", "role_recommendations", "recommended_roles"] },
    { label: "路径阶段", keys: ["stages", "stage_plan", "milestones", "phases", "goal", "objective", "deliverable"] },
    { label: "周任务", keys: ["weeks", "weekly_tasks", "tasks", "learning_path", "development_plan", "roadmap"] },
    { label: "达标标准", keys: ["standards", "acceptance_criteria", "success_metrics", "metrics", "deliverables"] },
    { label: "判断依据", keys: ["rationale", "evidence", "supporting_evidence", "strengths", "signals", "fit_reasoning", "fit_summary", "method_summary", "positive_signals"] },
    { label: "风险盲点", keys: ["risks", "counterfactual_risks", "gaps", "gap_summary", "weaknesses", "concerns", "risk_summary"] },
    { label: "下一步", keys: ["actions", "next_steps", "recommendations", "learning_path", "development_plan", "roadmap", "mitigations"] }
  ];
  const sections = sectionDefs
    .map((section) => ({
      label: section.label,
      items: collectAgentOutputItems(payload, section.keys).slice(0, 3).map((item) => compactAgentText(item, 88))
    }))
    .filter((section) => section.items.length);
  if (sections.length === 0) {
    const readable = extractReadablePoints(rawText, 4);
    if (readable.length) sections.push({ label: "关键内容", items: readable.map((item) => compactAgentText(item, 88)) });
  }
  return {
    isEmpty: !summary && sections.length === 0,
    sourceLabel: "结构化",
    summary,
    metrics: collectAgentOutputMetrics(payload, agent).slice(0, 3),
    sections
  };
}

function stripAgentOutputText(text) {
  return formatAgentDisplayText(text)
    .replace(/```(?:json)?/gi, "")
    .replace(/```/g, "")
    .trim();
}

function parseAgentOutputPayload(text) {
  const jsonText = extractBalancedJSON(text);
  if (jsonText) {
    try {
      return JSON.parse(jsonText);
    } catch {
      // Truncated streaming previews still contain useful JSON key-value pairs.
    }
  }
  return parseLooseAgentOutputPayload(text);
}

function extractBalancedJSON(text) {
  const source = String(text || "");
  const start = source.search(/[\[{]/);
  if (start < 0) return "";
  const opener = source[start];
  const closer = opener === "{" ? "}" : "]";
  let depth = 0;
  let inString = false;
  let quote = "";
  let escaped = false;
  for (let index = start; index < source.length; index += 1) {
    const char = source[index];
    if (inString) {
      if (escaped) {
        escaped = false;
      } else if (char === "\\") {
        escaped = true;
      } else if (char === quote) {
        inString = false;
      }
      continue;
    }
    if (char === "\"" || char === "'") {
      inString = true;
      quote = char;
      continue;
    }
    if (char === opener) depth += 1;
    if (char === closer) depth -= 1;
    if (depth === 0) return source.slice(start, index + 1);
  }
  return "";
}

function parseLooseAgentOutputPayload(text) {
  const source = String(text || "");
  const values = {};
  const stringPair = /"([^"]+)"\s*:\s*"((?:\\.|[^"\\])*)"/g;
  let match = stringPair.exec(source);
  while (match) {
    values[match[1]] = decodeLooseJSONString(match[2]);
    match = stringPair.exec(source);
  }
  const numberPair = /"([^"]+)"\s*:\s*(-?\d+(?:\.\d+)?)/g;
  match = numberPair.exec(source);
  while (match) {
    if (values[match[1]] == null) values[match[1]] = match[2];
    match = numberPair.exec(source);
  }
  return Object.keys(values).length ? values : null;
}

function decodeLooseJSONString(value) {
  try {
    return JSON.parse(`"${value}"`);
  } catch {
    return String(value || "").replace(/\\"/g, "\"").replace(/\\n/g, " ");
  }
}

function firstAgentOutputValue(payload, keys) {
  const values = collectAgentOutputItems(payload, keys);
  return values[0] || "";
}

function collectAgentOutputItems(payload, keys) {
  const normalized = new Set(keys.map((key) => key.toLowerCase()));
  const values = [];
  walkAgentOutput(payload, (key, value) => {
    if (normalized.has(String(key || "").toLowerCase())) {
      values.push(...flattenAgentOutputValue(value));
    }
  });
  return dedupeAgentOutputItems(values);
}

function walkAgentOutput(value, visit, key = "") {
  if (!value || typeof value !== "object") return;
  if (Array.isArray(value)) {
    value.forEach((item) => walkAgentOutput(item, visit, key));
    return;
  }
  Object.entries(value).forEach(([entryKey, entryValue]) => {
    visit(entryKey, entryValue);
    if (entryValue && typeof entryValue === "object") walkAgentOutput(entryValue, visit, entryKey);
  });
}

function flattenAgentOutputValue(value) {
  if (value == null) return [];
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") {
    return extractReadablePoints(String(value), 4);
  }
  if (Array.isArray(value)) {
    return value.flatMap((item) => flattenAgentOutputValue(item)).slice(0, 8);
  }
  if (typeof value === "object") {
    const preferred = ["title", "name", "week", "task", "role", "job", "direction", "goal", "objective", "reason", "description", "evidence", "action", "deliverable"];
    const headline = preferred.map((key) => value[key]).find((item) => item != null && String(item).trim());
    if (headline) {
      const detail = ["goal", "objective", "task", "metric", "deliverable", "reason", "description", "evidence", "action"]
        .map((key) => value[key])
        .find((item) => item != null && String(item).trim() && String(item).trim() !== String(headline).trim());
      return [detail ? `${headline}：${detail}` : String(headline)];
    }
    return Object.entries(value)
      .filter(([, item]) => item == null || typeof item !== "object")
      .slice(0, 3)
      .map(([entryKey, item]) => `${agentOutputKeyLabel(entryKey)}：${item}`);
  }
  return [];
}

function collectAgentOutputMetrics(payload, agent = {}) {
  const metrics = [];
  if (agent.reasoningEffort) metrics.push({ label: "思考", value: agent.reasoningEffort });
  const metricKeys = ["complexity", "confidence", "match_score", "overall_match", "match_level", "reasoning_effort", "synthesis_effort"];
  walkAgentOutput(payload, (key, value) => {
    if (metrics.length >= 4) return;
    const normalized = String(key || "").toLowerCase();
    if (!metricKeys.includes(normalized)) return;
    if (value == null || typeof value === "object") return;
    const text = compactAgentText(String(value), 18);
    if (text) metrics.push({ label: agentOutputKeyLabel(key), value: text });
  });
  return dedupeAgentOutputMetrics(metrics);
}

function extractReadablePoints(text, limit = 4) {
  const source = String(text || "")
    .replace(/[{}[\]"]/g, " ")
    .replace(/\s+/g, " ")
    .trim();
  if (!source) return [];
  return source
    .split(/(?:\n+|[。；;]|(?<!\d),(?!\d)|，)/)
    .map((item) => item.replace(/^[\s:：、,\-.]+/, "").trim())
    .filter((item) => item.length >= 4)
    .slice(0, limit);
}

function compactAgentText(text, maxLength = 96) {
  const value = String(text || "").replace(/\s+/g, " ").trim();
  if (value.length <= maxLength) return value;
  return `${value.slice(0, maxLength - 1).trim()}…`;
}

function dedupeAgentOutputItems(items) {
  const seen = new Set();
  return items
    .map((item) => compactAgentText(item, 120))
    .filter((item) => {
      const key = item.toLowerCase();
      if (!item || seen.has(key)) return false;
      seen.add(key);
      return true;
    });
}

function dedupeAgentOutputMetrics(metrics) {
  const seen = new Set();
  return metrics.filter((item) => {
    const key = `${item.label}:${item.value}`.toLowerCase();
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

function agentOutputKeyLabel(key) {
  const labels = {
    complexity: "复杂度",
    confidence: "置信",
    match_score: "匹配",
    overall_match: "匹配",
    match_level: "评级",
    reasoning_effort: "思考",
    synthesis_effort: "综合",
    rationale: "依据",
    recommendation: "建议",
    risk_summary: "风险"
  };
  return labels[String(key || "").toLowerCase()] || String(key || "").replaceAll("_", " ");
}

function toggleAgentStreamDetail(agentKey, messageID = "") {
  const key = String(agentKey || "");
  if (!key) return;
  const { session, message } = findActiveAgentStreamMessage(messageID, key);
  const stream = message?.agentStream;
  if (!session || !message || !stream?.agents?.[key]) return;
  stream.expandedAgents = stream.expandedAgents || {};
  if (stream.expandedAgents[key]) {
    delete stream.expandedAgents[key];
  } else {
    stream.expandedAgents[key] = true;
  }
  const now = new Date().toISOString();
  message.updatedAt = now;
  session.updatedAt = now;
  saveAssistantState();
  renderAssistantMessages();
  if (stream.expandedAgents[key]) startAgentOutputTypewriter(message.id, key);
}

function patchAgentStreamText(messageID, agentKey) {
  const key = String(agentKey || "");
  if (!key) return false;
  const { message } = findActiveAgentStreamMessage(messageID);
  const stream = message?.agentStream;
  const agent = stream?.agents?.[key];
  if (!stream?.expandedAgents?.[key] || !agent) return false;
  const typeoutKey = `${messageID}:${key}`;
  const textNode = assistantMessages.querySelector(`[data-agent-typeout-text="${CSS.escape(typeoutKey)}"]`)
    || [...assistantMessages.querySelectorAll("[data-agent-typeout-text]")]
      .find((node) => node.dataset.agentKey === key || node.dataset.agentTypeoutText === key);
  if (!textNode) return false;
  const typeout = textNode.closest("[data-agent-typeout]");
  const stickToBottom = typeout ? typeout.scrollHeight - typeout.scrollTop - typeout.clientHeight < 24 : false;
  textNode.textContent = formatAgentDisplayText(agent.typedOutput || agent.outputPreview || "");
  if (typeout) {
    let cursor = typeout.querySelector(".agent-stream-cursor");
    const showCursor = !agent.typingDone && agent.status !== "done" && agent.status !== "failed";
    if (showCursor && !cursor) {
      cursor = document.createElement("span");
      cursor.className = "agent-stream-cursor";
      cursor.setAttribute("aria-hidden", "true");
      typeout.append(cursor);
    }
    if (cursor) cursor.hidden = !showCursor;
    const liveState = typeout.closest(".agent-output-live")?.querySelector(".agent-output-live-head b");
    if (liveState) liveState.textContent = agentStreamLiveState(agent);
    if (stickToBottom) {
      window.requestAnimationFrame(() => {
        typeout.scrollTop = typeout.scrollHeight;
      });
    }
  }
  const structured = assistantMessages.querySelector(`[data-agent-structured-output="${CSS.escape(typeoutKey)}"]`)
    || [...assistantMessages.querySelectorAll("[data-agent-structured-output]")]
      .find((node) => node.dataset.agentKey === key);
  if (structured) {
    const structuredText = stableAgentStructuredOutputText(agent);
    const signature = agentStructuredOutputSignature(structuredText, agent);
    if (structured.dataset.agentStructuredSignature !== signature) {
      structured.innerHTML = renderAgentStructuredOutputContent(structuredText, agent);
      structured.dataset.agentStructuredSignature = signature;
    }
  }
  return true;
}

function stableAgentStructuredOutputText(agent) {
  const text = agent?.status === "done" || agent?.status === "failed" || agent?.typingDone
    ? agent.outputPreview || agent.typedOutput || ""
    : agent?.typedOutput || agent?.outputPreview || "";
  if (!agent || !text) return text || "";
  if (agent.status === "done" || agent.status === "failed" || agent.typingDone) {
    agent.structuredOutputText = text;
    agent.structuredOutputUpdatedAt = Date.now();
    return text;
  }
  const previous = agent.structuredOutputText || "";
  const now = Date.now();
  const lastUpdate = Number(agent.structuredOutputUpdatedAt || 0);
  const hasBoundary = /[。；;.!?]\s*$/.test(text) || /[}\]]\s*$/.test(text);
  const shouldRefresh = !previous
    || text.length < previous.length
    || text.length - previous.length >= 140
    || (hasBoundary && now - lastUpdate >= 220)
    || now - lastUpdate >= 900;
  if (shouldRefresh) {
    agent.structuredOutputText = text;
    agent.structuredOutputUpdatedAt = now;
    return text;
  }
  return previous;
}

function agentStructuredOutputSignature(text, agent = {}) {
  return [
    agent.status || "",
    agent.reasoningEffort || "",
    agent.message || "",
    hashAgentOutputString(text)
  ].join(":");
}

function hashAgentOutputString(value) {
  const text = String(value || "");
  let hash = 0;
  for (let index = 0; index < text.length; index += 1) {
    hash = ((hash << 5) - hash + text.charCodeAt(index)) | 0;
  }
  return String(hash);
}

function syncAgentTypewriterForEvent(messageID, agentKey) {
  const { message } = findActiveAgentStreamMessage(messageID);
  const stream = message?.agentStream;
  const key = String(agentKey || "");
  if (!stream?.expandedAgents?.[key] || !stream.agents?.[key]?.outputPreview) return;
  if (stream.agents[key]?.tokenStreamed) return;
  startAgentOutputTypewriter(message.id, key);
}

function findActiveAgentStreamMessage(messageID = "", agentKey = "") {
  const session = activeAssistantSession();
  if (!session) return { session: null, message: null };
  let message = messageID
    ? session.messages.find((item) => item.id === messageID)
    : null;
  if (!message && agentKey) {
    message = [...session.messages].reverse().find((item) => item.streamType === "agent_team" && item.agentStream?.agents?.[agentKey]);
  }
  if (!message && assistantAgentStreamMessageId) {
    message = session.messages.find((item) => item.id === assistantAgentStreamMessageId);
  }
  if (!message) {
    message = [...session.messages].reverse().find((item) => item.streamType === "agent_team");
  }
  return { session, message: message || null };
}

function startAgentOutputTypewriter(messageID, agentKey) {
  const key = String(agentKey || "");
  const timerKey = `${messageID}:${key}`;
  if (!messageID || !key || assistantAgentTypewriterTimers.has(timerKey)) return;
  const reducedMotion = window.matchMedia?.("(prefers-reduced-motion: reduce)")?.matches;
  const tick = () => {
    const { session, message } = findActiveAgentStreamMessage(messageID);
    const stream = message?.agentStream;
    const agent = stream?.agents?.[key];
    if (!session || !message || !stream?.expandedAgents?.[key] || !agent?.outputPreview) {
      assistantAgentTypewriterTimers.delete(timerKey);
      return;
    }
    const target = agent.outputPreview;
    const current = agent.typedOutput || "";
    if (reducedMotion || current.length >= target.length) {
      agent.typedOutput = target;
      agent.typingDone = true;
      saveAssistantState();
      patchAgentStreamText(messageID, key);
      assistantAgentTypewriterTimers.delete(timerKey);
      return;
    }
    const step = target.length > 1200 ? 4 : target.length > 600 ? 2 : 1;
    agent.typedOutput = target.slice(0, current.length + step);
    agent.typingDone = agent.typedOutput.length >= target.length;
    message.updatedAt = new Date().toISOString();
    session.updatedAt = message.updatedAt;
    if (agent.typingDone || agent.typedOutput.length % 24 === 0) saveAssistantState();
    patchAgentStreamText(messageID, key);
    const delay = target.length > 1200 ? 14 : 18;
    assistantAgentTypewriterTimers.set(timerKey, window.setTimeout(tick, delay));
  };
  assistantAgentTypewriterTimers.set(timerKey, window.setTimeout(tick, 0));
}

function stopAgentTypewriters() {
  assistantAgentTypewriterTimers.forEach((timer) => window.clearTimeout(timer));
  assistantAgentTypewriterTimers.clear();
}

function renderEffortBars(effort) {
  const count = { low: 1, medium: 2, high: 3, xhigh: 4 }[String(effort || "").toLowerCase()] || 2;
  return `<i class="agent-effort-bars">${[0, 1, 2, 3].map((index) => `<em class="${index < count ? "is-on" : ""}"></em>`).join("")}</i>`;
}

function renderAgentStreamLog(logs, key = "agent-stream-log") {
  if (!Array.isArray(logs) || logs.length === 0) return "";
  const recent = logs.slice(-3);
  return `
    <details class="agent-stream-log" data-agent-stream-log="${escapeAttribute(key)}">
      <summary>事件日志</summary>
      ${recent.map((item) => `
        <span class="agent-stream-log-item">
          <b>${escapeHTML(formatTime(item.time))}</b>
          <em>${escapeHTML(formatAgentDisplayText(item.agent || item.agentKey || "Agent"))}</em>
          <i>${escapeHTML(formatAgentDisplayText(item.message || item.status))}</i>
        </span>
      `).join("")}
    </details>
  `;
}

function renderAssistantSuggestions() {
  const suggestions = isAssistantReady() ? [
    "优先补哪项能力？",
    "首位岗位为什么匹配？",
    "本周行动清单",
    "显示路径规划 schema",
    "把路径任务改得更偏前端求职"
  ] : [];
  assistantSuggestions.innerHTML = suggestions.map((text) => `
    <button type="button" data-suggestion="${escapeAttribute(text)}">${escapeHTML(text)}</button>
  `).join("");
}

function updateAssistantContext() {
  if (!assistantContext) return;
  const ready = isAssistantReady();
  const label = ready ? "可追问" : assistantAgentStreamActive ? "Agent Team 流式生成" : diagnosisEvents || benchmarkRequestInFlight || matchingRequestInFlight || diagnosis ? "诊断生成中" : "等待诊断";
  const pillClass = ready ? "is-real" : "is-warning";
  assistantContext.setAttribute("aria-label", label);
  assistantContext.innerHTML = `<span class="status-pill ${pillClass}">${escapeHTML(label)}</span>`;
  syncAssistantAvailability();
}

function updateAssistantInputMeta() {
  const length = assistantInput.value.length;
  const ready = isAssistantReady();
  const inspectable = isAssistantInspectable();
  assistantInput.placeholder = ready
    ? "追问、请求 schema 或修改当前结果"
    : inspectable ? "可先编辑问题，诊断完成后发送" : "诊断完成后可追问";
  assistantInputMeta.textContent = ready
    ? length ? `${length}/${assistantPromptLimit}` : `最多 ${assistantPromptLimit} 字`
    : inspectable ? "可编辑，等待全部结果后发送" : `最多 ${assistantPromptLimit} 字`;
  assistantInputMeta.style.color = length > assistantPromptLimit * 0.9 ? "var(--danger)" : "";
}

function setAssistantExpanded(expanded, options = {}) {
  if (expanded && !options.force && !isAssistantInspectable()) {
    if (!options.silent) showToast("生成诊断后可查看 AI 助手。");
    expanded = false;
  }
  const changed = assistantState.expanded !== expanded;
  assistantState.expanded = expanded;
  applyAssistantExpandedState(expanded);
  if (changed || !options.silent) saveAssistantState();
}

function applyAssistantExpandedState(expanded) {
  assistant.classList.toggle("is-collapsed", !expanded);
  document.body.classList.toggle("assistant-expanded", expanded);
  assistantToggle.setAttribute("aria-expanded", String(expanded));
}

async function sendAssistantMessage(promptOverride = "", focusedContextsOverride = null) {
  if (!isAssistantReady()) {
    showToast("诊断仍在生成。你可以查看 Agent Team 进度，结果完成后再追问。");
    if (!isAssistantInspectable()) setAssistantExpanded(false, { silent: true });
    return;
  }
  const session = ensureAssistantSession();
  const prompt = String(promptOverride || assistantInput.value).trim();
  if (!prompt) {
    showToast("请输入要询问的问题。");
    assistantInput.focus();
    return;
  }
  if (prompt.length > assistantPromptLimit) {
    showToast("问题超过 1200 字，请先压缩。");
    return;
  }
  if (assistantBusy) return;

  assistantBusy = true;
  assistantSend.disabled = true;
  assistantInput.disabled = true;
  updateAssistantRailStatus("生成中");
  const now = new Date().toISOString();
  const focusedContexts = Array.isArray(focusedContextsOverride)
    ? normalizeFocusedContexts(focusedContextsOverride)
    : focusedContextsForAssistant();
  session.messages.push({ id: uniqueId("msg"), role: "user", content: prompt, createdAt: now, status: "done", focusedContexts });
  const loadingID = uniqueId("msg");
  session.messages.push({ id: loadingID, role: "assistant", content: "正在通过 Legato Chat workflow 生成回答。", createdAt: now, status: "loading", retryPrompt: prompt, focusedContexts });
  session.title = titleForPrompt(prompt);
  session.updatedAt = now;
  assistantInput.value = "";
  clearAssistantFocusedContextsSilently(session);
  saveAssistantState();
  renderAssistant();

  const answerStreamer = createAssistantAnswerStreamer(session, loadingID);
  try {
    const response = await requestAssistantAnswer(session, prompt, focusedContexts, {
      onStatus(message) {
        answerStreamer.status(message);
      },
      onDelta(delta) {
        answerStreamer.push(delta);
      }
    });
    await answerStreamer.drain();
    const uiResult = applyAssistantUIIntent(response.chat);
    const loading = session.messages.find((message) => message.id === loadingID);
    if (loading) {
      loading.content = assistantAnswerWithUIResult(response.answer, uiResult) || "模型没有返回可用内容。";
      loading.status = "done";
      loading.retryPrompt = "";
      loading.uiResult = uiResult;
      loading.isStreamingAnswer = false;
    }
  } catch (error) {
    answerStreamer.stop();
    const loading = session.messages.find((message) => message.id === loadingID);
    if (loading) {
      loading.content = assistantErrorMessage(error);
      loading.status = "error";
      loading.retryPrompt = prompt;
      loading.isStreamingAnswer = false;
    }
  } finally {
    session.updatedAt = new Date().toISOString();
    assistantBusy = false;
    assistantSend.disabled = false;
    assistantInput.disabled = false;
    saveAssistantState();
    renderAssistant();
  }
}

async function retryAssistantMessage(messageID) {
  const session = activeAssistantSession();
  const message = session?.messages.find((item) => item.id === messageID);
  if (!message?.retryPrompt) return;
  const retryContexts = normalizeFocusedContexts(message.focusedContexts);
  session.messages = session.messages.filter((item) => item.id !== messageID);
  saveAssistantState();
  await sendAssistantMessage(message.retryPrompt, retryContexts);
}

async function requestAssistantAnswer(session, prompt, focusedContexts = null, streamHandlers = {}) {
  assistantAbort = new AbortController();
  const timeout = window.setTimeout(() => assistantAbort.abort(), assistantRequestTimeoutMs);
  try {
    const response = await fetch("/api/chat?stream=1", {
      method: "POST",
      headers: { "Content-Type": "application/json", "Accept": "text/event-stream" },
      body: JSON.stringify({
        question: prompt,
        diagnosis: assistantContextForModel({ focusedContexts }),
        ui_schema_catalog: assistantUISchemaCatalog(prompt),
        history: assistantHistoryForModel(session, prompt)
      }),
      signal: assistantAbort.signal
    });
    if (!response.ok) {
      const text = await response.text();
      let data = {};
      try {
        data = text ? JSON.parse(text) : {};
      } catch {
        data = {};
      }
      const error = new Error(data.error || data.message || `http_${response.status}`);
      error.status = response.status;
      throw error;
    }
    const contentType = response.headers.get("Content-Type") || "";
    const payload = response.body && contentType.toLowerCase().includes("text/event-stream")
      ? await readAssistantChatStream(response.body, streamHandlers)
      : await response.json();
    return {
      answer: String(payload.answer || payload.chat?.answer || "").trim(),
      chat: payload.chat && typeof payload.chat === "object" ? payload.chat : {},
      raw: payload
    };
  } finally {
    window.clearTimeout(timeout);
    assistantAbort = null;
  }
}

async function readAssistantChatStream(body, handlers = {}) {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let donePayload = null;
  for (;;) {
    const { value, done } = await reader.read();
    buffer += decoder.decode(value || new Uint8Array(), { stream: !done });
    const events = buffer.split(/\n\n/);
    buffer = events.pop() || "";
    for (const rawEvent of events) {
      const payload = consumeAssistantChatStreamEvent(rawEvent, handlers);
      if (payload) donePayload = payload;
    }
    if (done) break;
  }
  if (buffer.trim()) {
    const payload = consumeAssistantChatStreamEvent(buffer, handlers);
    if (payload) donePayload = payload;
  }
  if (!donePayload) throw new Error("chat_stream_incomplete");
  return donePayload;
}

function consumeAssistantChatStreamEvent(rawEvent, handlers = {}) {
  const normalized = String(rawEvent || "").replace(/\r/g, "");
  if (!normalized.trim()) return null;
  let event = "message";
  const dataLines = [];
  normalized.split("\n").forEach((line) => {
    if (line.startsWith("event:")) event = line.slice(6).trim();
    if (line.startsWith("data:")) dataLines.push(line.slice(5).trimStart());
  });
  if (!dataLines.length) return null;
  let payload = {};
  try {
    payload = JSON.parse(dataLines.join("\n"));
  } catch {
    throw new Error("chat_stream_invalid_json");
  }
  if (event === "chat.status") {
    handlers.onStatus?.(String(payload.message || ""));
    return null;
  }
  if (event === "chat.chunk") {
    handlers.onDelta?.(String(payload.delta || ""));
    return null;
  }
  if (event === "chat.error") {
    const error = new Error(String(payload.error || "chat_stream_failed"));
    error.source = "chat_stream";
    throw error;
  }
  if (event === "chat.done") {
    return payload;
  }
  return null;
}

function scheduleAssistantStreamRender() {
  if (assistantStreamRenderFrame) return;
  assistantStreamRenderFrame = window.requestAnimationFrame(() => {
    assistantStreamRenderFrame = 0;
    renderAssistantMessages({ stickToBottom: true });
  });
}

function updateAssistantMessageBubble(messageID, content, options = {}) {
  const selector = `[data-assistant-message-id="${CSS.escape(messageID)}"]`;
  const item = assistantMessages.querySelector(selector);
  const textNode = item?.querySelector("[data-assistant-message-text]");
  if (!item || !textNode) {
    renderAssistantMessages({ stickToBottom: true });
    return;
  }
  textNode.textContent = formatAgentDisplayText(content);
  item.classList.toggle("is-loading", options.status === "loading");
  item.classList.toggle("is-error", options.status === "error");
  item.classList.toggle("is-answer-streaming", Boolean(options.streaming));
  if (options.stickToBottom !== false) assistantMessages.scrollTop = assistantMessages.scrollHeight;
}

function createAssistantAnswerStreamer(session, messageID) {
  const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  let visible = "";
  let pending = "";
  let timer = 0;
  let drainResolve = null;

  const message = () => session.messages.find((item) => item.id === messageID);
  const finishDrain = () => {
    if (!pending && !timer && drainResolve) {
      const resolve = drainResolve;
      drainResolve = null;
      resolve();
    }
  };
  const render = (content, streaming) => updateAssistantMessageBubble(messageID, content, {
    status: "loading",
    streaming,
    stickToBottom: true
  });
  const writeVisible = () => {
    const loading = message();
    if (!loading) return false;
    loading.content = visible || "正在生成回答。";
    loading.status = "loading";
    loading.isStreamingAnswer = Boolean(visible);
    render(loading.content, Boolean(visible));
    return true;
  };
  const step = () => {
    timer = 0;
    if (!pending) {
      finishDrain();
      return;
    }
    const batchSize = pending.length > 96 ? 3 : pending.length > 36 ? 2 : 1;
    const emitted = pending.slice(0, batchSize);
    visible += emitted;
    pending = pending.slice(batchSize);
    if (!writeVisible()) {
      pending = "";
      finishDrain();
      return;
    }
    if (pending) {
      timer = window.setTimeout(step, assistantStreamDelayFor(emitted, pending));
    } else {
      finishDrain();
    }
  };
  const start = () => {
    if (!timer && pending) timer = window.setTimeout(step, assistantStreamTickMs);
  };

  return {
    status(text) {
      if (visible || pending) return;
      const loading = message();
      if (!loading) return;
      loading.content = text || "Legato Chat workflow 正在生成回答。";
      loading.status = "loading";
      loading.isStreamingAnswer = false;
      render(loading.content, false);
    },
    push(delta) {
      const text = String(delta || "");
      if (!text) return;
      if (reducedMotion) {
        visible += text;
        pending = "";
        writeVisible();
        finishDrain();
        return;
      }
      pending += text;
      start();
    },
    drain() {
      if (reducedMotion || (!pending && !timer)) return Promise.resolve();
      return new Promise((resolve) => {
        drainResolve = resolve;
        start();
      });
    },
    stop() {
      if (timer) {
        window.clearTimeout(timer);
        timer = 0;
      }
      pending = "";
      finishDrain();
    }
  };
}

function assistantStreamDelayFor(emitted, pending) {
  const text = String(emitted || "");
  if (!pending) return 0;
  if (/[。！？!?]\s*$/.test(text)) return 190;
  if (/[，、；：,;:]\s*$/.test(text)) return 105;
  return assistantStreamTickMs;
}

function assistantUISchemaCatalog(prompt = "") {
  const targets = assistantSchemaTargetsForPrompt(prompt);
  return Object.fromEntries(targets.map((target) => {
    const config = assistantEditableTargets[target];
    return [target, {
      target,
      label: config.label,
      roots: config.roots,
      description: config.description,
      schema: config.schema,
      current_value: assistantEditableCurrentValue(target)
    }];
  }));
}

function assistantSchemaTargetsForPrompt(prompt = "") {
  const text = cleanDisplayText(prompt).toLowerCase();
  if (isAssistantJobPreferencePrompt(text)) {
    return ["job_recommendations", "matching", "top_jobs"];
  }
  return Object.keys(assistantEditableTargets);
}

function isAssistantJobPreferencePrompt(text) {
  if (!text) return false;
  const hasPreference = /不喜欢|不想|不要|换|改|转|想干|想做|希望|更想|倾向|推荐/.test(text);
  const hasRoleSignal = /岗位|职位|方向|职业|工程师|研究员|产品|运营|算法|开发|安全|渗透|测试|数据|前端|后端|web|ai|llm|java|python/.test(text);
  return hasPreference && hasRoleSignal;
}

function assistantEditableCurrentValue(target) {
  const profile = diagnosis?.ability_profile || {};
  switch (target) {
    case "basic":
      return deepCloneJSON(profile.basic_info || {});
    case "education":
      return deepCloneJSON(Array.isArray(profile.education) ? profile.education : []);
    case "awards":
      return deepCloneJSON(Array.isArray(profile.awards) ? profile.awards.slice(0, 24) : []);
    case "experiences":
      return deepCloneJSON(Array.isArray(profile.experiences) ? profile.experiences.slice(0, 24) : []);
    case "profile_radar":
      return deepCloneJSON({
        radar_data: profile.radar_data || [],
        radar_series: profile.radar_series || []
      });
    case "matching":
      return deepCloneJSON(diagnosis?.matching_result || {});
    case "path_plan":
      return deepCloneJSON(diagnosis?.path_plan || {});
    case "top_jobs":
      return deepCloneJSON(Array.isArray(profile.top5_matching_jobs) ? profile.top5_matching_jobs : []);
    case "job_recommendations":
      return deepCloneJSON({
        matching_result: diagnosis?.matching_result || {},
        top_jobs: Array.isArray(profile.top5_matching_jobs) ? profile.top5_matching_jobs : []
      });
    default:
      return null;
  }
}

function applyAssistantUIIntent(chat) {
  const intent = normalizeAssistantUIIntent(chat?.ui_intent);
  if (!intent || intent.mode === "none") return { status: "none", applied: 0, message: "" };
  const targetConfig = assistantEditableTargets[intent.target];
  if (!targetConfig) {
    return { status: "blocked", applied: 0, message: "未识别可编辑区域。" };
  }
  if (intent.mode === "show_schema") {
    return {
      status: "schema",
      applied: 0,
      message: `${targetConfig.label} schema 已返回。`,
      target: intent.target
    };
  }
  if (intent.mode !== "update_result") return { status: "none", applied: 0, message: "" };
  if (!diagnosis) {
    return { status: "blocked", applied: 0, message: "当前没有可修改的诊断结果。" };
  }
  const patches = intent.patches.filter((patch) => isAssistantPatchAllowed(intent.target, patch));
  if (!patches.length) {
    return { status: "blocked", applied: 0, message: "模型没有返回可安全应用的修改。" };
  }
  const draft = deepCloneJSON(diagnosis);
  try {
    patches.forEach((patch) => applyJSONPatchOperation(draft, patch));
  } catch (error) {
    return { status: "blocked", applied: 0, message: `修改未应用：${error.message || "patch 失败"}` };
  }
  diagnosis = normalizeAssistantPatchedDiagnosis(ensureDiagnosisShape(draft), intent.target);
  renderDiagnosis(diagnosis);
  updateAssistantContext();
  renderAssistantSuggestions();
  const message = assistantUIIntentToastMessage(intent, targetConfig, patches);
  showToast(message);
  return {
    status: "applied",
    applied: patches.length,
    message,
    target: intent.target
  };
}

function assistantUIIntentToastMessage(intent, targetConfig, patches) {
  if (intent.target === "job_recommendations" || intent.target === "top_jobs") return "推荐已更新";
  if (intent.target === "matching") return "匹配已更新";
  if (intent.target === "path_plan") return "路径已更新";
  return `${targetConfig.label}已更新`;
}

function normalizeAssistantUIIntent(value) {
  if (!value || typeof value !== "object") return null;
  const mode = cleanDisplayText(value.mode || "none");
  const target = cleanDisplayText(value.target || "none");
  if (!["none", "show_schema", "update_result"].includes(mode)) return null;
  if (target !== "none" && !assistantEditableTargets[target]) return null;
  const patches = Array.isArray(value.patches)
    ? value.patches.slice(0, 40).map((patch) => normalizeAssistantPatch(patch, target)).filter(Boolean)
    : [];
  const schema = value.schema && typeof value.schema === "object" && !Array.isArray(value.schema) ? value.schema : {};
  return {
    mode,
    target,
    patches,
    schema,
    summary: cleanDisplayText(value.summary || "").slice(0, 500)
  };
}

function normalizeAssistantPatch(value, target = "") {
  if (!value || typeof value !== "object") return null;
  const op = cleanDisplayText(value.op);
  const path = normalizeAssistantPatchPath(target, cleanDisplayText(value.path));
  if (!["add", "replace", "remove"].includes(op) || !path.startsWith("/") || path.length > 240) return null;
  if (op !== "remove" && !Object.prototype.hasOwnProperty.call(value, "value")) return null;
  const patch = { op, path };
  if (op !== "remove" && Object.prototype.hasOwnProperty.call(value, "value")) {
    patch.value = deepCloneJSON(value.value);
  }
  return patch;
}

function normalizeAssistantPatchPath(target, path) {
  if (!path) return "";
  let cleanPath = path.startsWith("/") ? path : `/${path}`;
  if (cleanPath.startsWith("/diagnosis/")) cleanPath = cleanPath.slice("/diagnosis".length);
  cleanPath = normalizeAssistantPatchPathAlias(target, cleanPath);
  const config = assistantEditableTargets[target];
  if (!config || target === "none") return cleanPath;
  if (config.roots.some((root) => cleanPath === root || cleanPath.startsWith(`${root}/`))) {
    return cleanPath;
  }
  const root = config.roots[0];
  if (!root) return cleanPath;
  if (cleanPath === "/") return root;
  return `${root}${cleanPath}`;
}

function normalizeAssistantPatchPathAlias(target, path) {
  if (["top_jobs", "job_recommendations"].includes(target)) {
    if (path === "/top_jobs") return "/ability_profile/top5_matching_jobs";
    if (path.startsWith("/top_jobs/")) return `/ability_profile/top5_matching_jobs${path.slice("/top_jobs".length)}`;
    if (path === "/ability_profile/top_jobs") return "/ability_profile/top5_matching_jobs";
    if (path.startsWith("/ability_profile/top_jobs/")) return `/ability_profile/top5_matching_jobs${path.slice("/ability_profile/top_jobs".length)}`;
  }
  if (["matching", "job_recommendations"].includes(target)) {
    if (path === "/matching" || path === "/job_matching") return "/matching_result";
    if (path.startsWith("/matching/")) return `/matching_result${path.slice("/matching".length)}`;
    if (path.startsWith("/job_matching/")) return `/matching_result${path.slice("/job_matching".length)}`;
  }
  return path;
}

function isAssistantPatchAllowed(target, patch) {
  const config = assistantEditableTargets[target];
  if (!config || !patch) return false;
  const parts = parseJSONPointer(patch.path);
  if (!parts || parts.some((part) => ["__proto__", "prototype", "constructor"].includes(part))) return false;
  return config.roots.some((root) => patch.path === root || patch.path.startsWith(`${root}/`));
}

function applyJSONPatchOperation(root, patch) {
  const parts = parseJSONPointer(patch.path);
  if (!parts || parts.length === 0) throw new Error("patch path 无效");
  const key = parts.at(-1);
  const parent = resolveJSONPointerParent(root, parts);
  if (Array.isArray(parent)) {
    applyArrayPatch(parent, key, patch);
    return;
  }
  if (!parent || typeof parent !== "object") throw new Error("patch 父节点不存在");
  if (patch.op === "remove") {
    if (!Object.prototype.hasOwnProperty.call(parent, key)) throw new Error("remove 路径不存在");
    delete parent[key];
    return;
  }
  parent[key] = deepCloneJSON(patch.value);
}

function resolveJSONPointerParent(root, parts) {
  let cursor = root;
  for (const part of parts.slice(0, -1)) {
    if (Array.isArray(cursor)) {
      const index = Number(part);
      if (!Number.isInteger(index) || index < 0 || index >= cursor.length) throw new Error("数组路径不存在");
      cursor = cursor[index];
    } else if (cursor && typeof cursor === "object") {
      if (!Object.prototype.hasOwnProperty.call(cursor, part)) throw new Error("对象路径不存在");
      cursor = cursor[part];
    } else {
      throw new Error("patch 路径不存在");
    }
  }
  return cursor;
}

function applyArrayPatch(parent, key, patch) {
  const index = key === "-" ? parent.length : Number(key);
  if (!Number.isInteger(index) || index < 0 || index > parent.length) throw new Error("数组索引无效");
  if (patch.op === "add") {
    parent.splice(index, 0, deepCloneJSON(patch.value));
    return;
  }
  if (index >= parent.length) throw new Error("数组索引不存在");
  if (patch.op === "remove") {
    parent.splice(index, 1);
    return;
  }
  parent[index] = deepCloneJSON(patch.value);
}

function parseJSONPointer(path) {
  if (typeof path !== "string" || !path.startsWith("/")) return null;
  return path.slice(1).split("/").map((part) => part.replace(/~1/g, "/").replace(/~0/g, "~"));
}

function ensureDiagnosisShape(value) {
  const shell = createDiagnosisShell();
  const next = value && typeof value === "object" ? value : {};
  return {
    ...shell,
    ...next,
    ability_profile: { ...shell.ability_profile, ...(next.ability_profile || {}) },
    matching_result: { ...shell.matching_result, ...(next.matching_result || {}) },
    path_plan: { ...shell.path_plan, ...(next.path_plan || {}) },
    backend_requirements: Array.isArray(next.backend_requirements) ? next.backend_requirements : shell.backend_requirements,
    production_limitations: Array.isArray(next.production_limitations) ? next.production_limitations : shell.production_limitations
  };
}

function normalizeAssistantPatchedDiagnosis(next, target = "") {
  if (!next || !["top_jobs", "job_recommendations"].includes(target)) return next;
  const profile = next.ability_profile || {};
  const jobs = Array.isArray(profile.top5_matching_jobs) ? profile.top5_matching_jobs : [];
  profile.top5_matching_jobs = jobs.slice(0, 5).map((job, index) => normalizeAssistantJobRecommendation(job, index));
  next.ability_profile = profile;
  if (profile.top5_matching_jobs.length) {
    const first = profile.top5_matching_jobs[0];
    const match = next.matching_result || {};
    const selected = match.selected_job && typeof match.selected_job === "object" ? match.selected_job : {};
    match.target_role = first.title || match.target_role || "";
    match.overall_match = Number.isFinite(Number(first.match)) ? Number(first.match) : match.overall_match;
    match.fit_summary = first.fit_summary || match.fit_summary || selected.fit_summary || "";
    match.selected_job = {
      ...selected,
      ...first,
      title: first.title || selected.title || match.target_role || ""
    };
    if (Array.isArray(first.reasons) && first.reasons.length) match.recommended_reasons = first.reasons;
    next.matching_result = match;
  }
  return next;
}

function normalizeAssistantJobRecommendation(job, index = 0) {
  const source = job && typeof job === "object" ? job : {};
  return {
    ...source,
    rank: Number(source.rank || index + 1),
    title: cleanDisplayText(source.title || source.target_role || `推荐岗位 ${index + 1}`),
    category: cleanDisplayText(source.category || "用户偏好方向"),
    match: safeScore(source.match ?? source.overall_match ?? 0),
    fit_summary: cleanDisplayText(source.fit_summary || source.summary || ""),
    reasons: Array.isArray(source.reasons) ? source.reasons.map(cleanDisplayText).filter(Boolean).slice(0, 4) : [],
    proof_gaps: Array.isArray(source.proof_gaps) ? source.proof_gaps.map(cleanDisplayText).filter(Boolean).slice(0, 4) : [],
    next_proof: cleanDisplayText(source.next_proof || source.next_step || ""),
    education_gate: cleanDisplayText(source.education_gate || ""),
    education_gate_status: cleanDisplayText(source.education_gate_status || ""),
    evidence_strength: cleanDisplayText(source.evidence_strength || "")
  };
}

function assistantAnswerWithUIResult(answer, result) {
  const cleanAnswer = cleanDisplayText(answer);
  if (!result || result.status === "none") return cleanAnswer;
  if (result.status === "applied") {
    return [cleanAnswer, `界面已更新：${result.message}`].filter(Boolean).join(" ");
  }
  if (result.status === "blocked") {
    return [cleanAnswer, `界面未更新：${result.message}`].filter(Boolean).join(" ");
  }
  return cleanAnswer;
}

function deepCloneJSON(value) {
  if (typeof structuredClone === "function") return structuredClone(value);
  return JSON.parse(JSON.stringify(value ?? null));
}

async function fetchJSON(url, options) {
  const response = await fetch(url, options);
  const text = await response.text();
  let data = {};
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      throw new Error("invalid_json");
    }
  }
  if (!response.ok) {
    const error = new Error(data.error || data.message || `http_${response.status}`);
    error.status = response.status;
    throw error;
  }
  return data;
}

function assistantContextForModel(options = {}) {
  if (!diagnosis) return { status: "no_diagnosis", message: "尚未生成诊断。用户可能仍在材料上传阶段。" };
  const profile = diagnosis.ability_profile || {};
  const info = profile.basic_info || {};
  const match = diagnosis.matching_result || {};
  const topJobs = Array.isArray(profile.top5_matching_jobs) ? profile.top5_matching_jobs.slice(0, 5) : [];
  const profileRadar = normalizedProfileRadarData(profile);
  const sixDimScores = profileRadar.length
    ? radarScoresToMatchingDimensions(profileRadar)
    : Array.isArray(match.student_radar) ? match.student_radar : [];
  const radarSeries = normalizedBackendRadarSeries(profile);
  const gaps = Array.isArray(match.gap_details) ? match.gap_details.slice(0, 5) : [];
  const developmentActions = Array.isArray(match.development_actions) ? match.development_actions.slice(0, 12) : [];
  const stages = Array.isArray(diagnosis.path_plan?.stages) ? diagnosis.path_plan.stages.slice(0, 3) : [];
  const focusedContexts = Array.isArray(options.focusedContexts)
    ? normalizeFocusedContexts(options.focusedContexts)
    : focusedContextsForAssistant();
  return {
    status: "ready",
    basic_info: {
      name: info.name || "",
      school: info.school || "",
      major: info.major || "",
      degree: info.degree || "",
      target_role: info.target_role || match.target_role || "",
      transcript_use: info.transcript_use || ""
    },
    education: Array.isArray(profile.education) ? profile.education : [],
    awards_status: profile.awards_status || "",
    awards: Array.isArray(profile.awards) ? profile.awards.slice(0, 24).map(assistantAwardContext) : [],
    experiences_status: profile.experiences_status || "",
    experiences: Array.isArray(profile.experiences) ? profile.experiences.slice(0, 24).map(assistantExperienceContext) : [],
    benchmark_status: profile.benchmark_status || "",
    major_baseline_status: profile.major_baseline_status || "",
    major_baseline: profile.major_baseline || {},
    six_dim_scores: sixDimScores,
    radar_series: radarSeries,
    top_jobs: topJobs,
    matching: {
      target_role: match.target_role || "",
      overall_match: match.overall_match || "",
      match_level: match.match_level || "",
      development_actions: developmentActions,
      gaps
    },
    path_stages: stages.map((stage) => ({
      stage: stage.stage,
      goal: stage.goal,
      deliverable: stage.deliverable
    })),
    focused_contexts: focusedContexts,
    focused_context: focusedContexts.at(-1) || null,
    focused_evidence: focusedEvidenceForAssistant(),
    production_limitations: diagnosis.production_limitations || []
  };
}

function assistantAwardContext(item) {
  return {
    name: item?.name || "",
    result: item?.result || "",
    evidence_scope: item?.evidence_scope || "",
    level: item?.level ?? "",
    impact_factor: item?.impact_factor ?? "",
    benchmark_scores: Array.isArray(item?.benchmark_scores) ? item.benchmark_scores : [],
    reason: item?.reason || ""
  };
}

function assistantExperienceContext(item) {
  return {
    type: item?.type || "",
    role: item?.role || "",
    contribution: item?.contribution || "",
    evidence_scope: item?.evidence_scope || "",
    level: item?.level ?? "",
    impact_factor: item?.impact_factor ?? "",
    benchmark_scores: Array.isArray(item?.benchmark_scores) ? item.benchmark_scores : [],
    reason: item?.reason || ""
  };
}

function assistantHistoryForModel(session, currentPrompt) {
  const messages = (session?.messages || [])
    .filter((message) => message.status === "done" && !["agent_team", "evidence_context"].includes(message.streamType) && ["user", "assistant", "system"].includes(message.role) && message.content)
    .map((message) => ({ role: message.role, content: message.content }));
  if (messages.length > 0) {
    const last = messages[messages.length - 1];
    if (last.role === "user" && last.content === currentPrompt) {
      messages.pop();
    }
  }
  return messages.slice(-12);
}

function assistantContextSummary() {
  const profile = diagnosis?.ability_profile || {};
  const info = profile.basic_info || {};
  const match = diagnosis?.matching_result || {};
  const role = match.target_role || info.target_role || profile.top5_matching_jobs?.[0]?.title || "推荐岗位";
  const score = match.overall_match ? `${match.overall_match}%` : "待评分";
  const name = info.name ? `${info.name}，` : "";
  return `${name}${role}匹配度 ${score}。可追问能力短板、岗位理由和阶段任务。`;
}

function assistantErrorMessage(error) {
  if (error?.name === "AbortError") return "模型响应超时，请稍后重试。";
  if (error?.status === 503) return "Legato Chat 或 Presto 服务不可用，请检查后端代理。";
  if (error?.status === 400) return "请求内容无法被模型服务识别，请缩短问题后重试。";
  if (error?.status === 429) return "模型请求过于频繁，请稍后重试。";
  if (error?.status >= 500) return "Legato Chat 服务暂时不可用，请稍后重试。";
  if (error?.source === "chat_stream") return `AI助手生成失败：${cleanDisplayText(error.message || "模型返回错误")}`;
  return "无法连接 LLM 服务，请检查网络或后端状态。";
}

function titleForPrompt(prompt) {
  const compact = prompt.replace(/\s+/g, " ").trim();
  return compact.length > 24 ? `${compact.slice(0, 24)}...` : compact || "新诊断对话";
}

function formatTime(value) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "刚刚";
  return new Intl.DateTimeFormat("zh-CN", { hour: "2-digit", minute: "2-digit" }).format(date);
}

function uniqueId(prefix) {
  if (window.crypto?.randomUUID) return `${prefix}_${window.crypto.randomUUID()}`;
  return `${prefix}_${Date.now()}_${Math.random().toString(16).slice(2)}`;
}

function updateAssistantRailStatus(status) {
  assistantRailStatus.textContent = status;
  const tooltip = `AI助手 · ${status}`;
  assistantToggle.dataset.tooltip = tooltip;
  assistantToggle.title = tooltip;
}

function downloadJSON(filename, value) {
  const blob = new Blob([JSON.stringify(value, null, 2)], { type: "application/json;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  document.body.append(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

function printSectionAsPDF(target, label) {
  const printTarget = ["profile", "matching", "path"].includes(target) ? target : "";
  if (!printTarget) {
    showToast("暂不支持该模块导出 PDF。");
    return;
  }
  const targetSlide = document.querySelector(`#${printTarget}`);
  const restoredDetails = [];
  targetSlide?.querySelectorAll("details").forEach((detail) => {
    if (!detail.open) {
      detail.open = true;
      restoredDetails.push(detail);
    }
  });
  document.body.dataset.printTarget = printTarget;
  document.body.classList.add("is-printing-export");
  showToast(`即将打开打印面板，请选择另存为 PDF：${label}`);
  const cleanup = () => {
    delete document.body.dataset.printTarget;
    document.body.classList.remove("is-printing-export");
    restoredDetails.forEach((detail) => {
      detail.open = false;
    });
    window.removeEventListener("afterprint", cleanup);
  };
  window.addEventListener("afterprint", cleanup);
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      window.print();
      window.setTimeout(cleanup, 1400);
    });
  });
}

function showToast(message) {
  window.clearTimeout(toastTimer);
  toast.textContent = formatAgentDisplayText(message);
  toast.classList.add("is-visible");
  toastTimer = window.setTimeout(() => toast.classList.remove("is-visible"), 2600);
}

function cleanDisplayText(value) {
  return String(value ?? "").trim();
}

function escapeRegExp(value) {
  return String(value ?? "").replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function formatAgentDisplayText(value) {
  return String(value ?? "")
    .replace(/\bagent team\b/g, "Agent Team")
    .replace(/\bAgent team\b/g, "Agent Team")
    .replace(/\bagent\b/g, "Agent");
}

function safeScore(value) {
  const score = Number(value);
  if (!Number.isFinite(score)) return 0;
  return Math.max(0, Math.min(100, score));
}

function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

function escapeAttribute(value) {
  return escapeHTML(value);
}

function safeURLAttribute(value) {
  const text = String(value ?? "");
  try {
    const parsed = new URL(text);
    if (parsed.protocol === "http:" || parsed.protocol === "https:") {
      return escapeHTML(parsed.href);
    }
  } catch {
    return "#";
  }
  return "#";
}
