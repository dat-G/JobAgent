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
const assistantInput = document.querySelector("#assistantInput");
const assistantInputMeta = document.querySelector("#assistantInputMeta");
const assistantSend = document.querySelector("#assistantSend");
const assistantRailStatus = document.querySelector("#assistantRailStatus");

const assistantStorageKey = "jobagent.llmAssistant.v1";
const assistantMaxMessages = 80;
const assistantMaxSessions = 24;
const assistantPromptLimit = 1200;

let diagnosis = null;
let resumeReady = false;
let toastTimer = 0;
let diagnosisEvents = null;
let currentJobId = "";
let benchmarkRequestInFlight = false;
let baseJobDone = false;
let failedRunStep = "";
let firstResultRevealed = false;
let activeStep = "upload";
let scrollSyncFrame = 0;
let radarAnimationFrame = 0;
let radarRenderState = null;

const benchmarkDimensions = ["逻辑", "语言", "专业", "领导", "抗压", "成长"];
const radarEvidenceGain = 1.0;
const radarEvidenceDiminishThreshold = 0.65;
const radarEvidenceTailThreshold = 0.7;
const radarEvidenceTailGain = 0.04;
const radarEvidenceSoftCap = 0.88;
const radarVisualGamma = 0.86;
const radarGridScores = [25, 50, 75, 100];
const cappedEvidenceBucketRules = {
  lowImpactAwardCertificate: { singleCap: 0.035, totalCap: 0.08 },
  campusAward: { singleCap: 0.045, totalCap: 0.1 },
  genericCampusRole: { singleCap: 0.045, totalCap: 0.1 },
  untitledProject: { singleCap: 0.06, totalCap: 0.15 },
  impactLow: { singleCap: 0.06, totalCap: 0.14 },
  impactMedium: { singleCap: 0.1, totalCap: 0.24 },
  impactHigh: { singleCap: 0.16, totalCap: 0.38 },
  impactExceptional: { singleCap: 0.24, totalCap: 0.52 }
};
const schoolTierConfigs = {
  T0: { label: "985/Top50", base: 68, noHighCap: 78, highCap: 92, exceptionalCap: 94, liftScale: 0.25 },
  T1: { label: "211/双一流/Top150", base: 62, noHighCap: 72, highCap: 86, exceptionalCap: 88, liftScale: 0.28 },
  T2: { label: "非双一流Top151-250", base: 52, noHighCap: 62, highCap: 76, exceptionalCap: 78, liftScale: 0.3 },
  T3: { label: "普通本科", base: 46, noHighCap: 56, highCap: 68, exceptionalCap: 72, liftScale: 0.28 },
  T4A: { label: "独立学院历史+较高学历", base: 45, noHighCap: 54, highCap: 66, exceptionalCap: 70, liftScale: 0.24 },
  T4: { label: "独立学院/原三本", base: 38, noHighCap: 52, highCap: 60, exceptionalCap: 62, liftScale: 0.22 },
  T5: { label: "专科", base: 34, noHighCap: 48, highCap: 56, exceptionalCap: 60, liftScale: 0.2 }
};
const academicPriorWeight = 0.28;
const academicPriorFloorRatio = 0.85;
let assistantState = loadAssistantState();
let assistantBusy = false;
let assistantAbort = null;
let assistantFocusedEvidence = null;
let assistantAgentStreamMessageId = "";
let assistantAgentStreamActive = false;
let assistantAgentStreamAutoOpened = false;
let assistantAgentTypewriterTimers = new Map();
let assistantStateSaveTimer = 0;

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
  document.querySelector("#runDetail").textContent = "材料已就绪，点击生成诊断后会显示实时进度。";
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
      unlockModule("profile");
      unlockModule("matching");
      unlockModule("path");
      unlockModule("outputs");
      setRunDone();
      runButton.disabled = false;
      runButton.textContent = "重新生成";
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
  document.querySelector("#runDetail").textContent = "正在创建诊断任务并启动 Agent。";
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
  document.querySelector("#runStatus").textContent = "诊断已生成";
  document.querySelector("#runDetail").textContent = "诊断完成，可以查看和导出结果。";
  document.querySelector(".generation-dock").classList.remove("is-running");
  setRunProgress(100);
  runButton.disabled = false;
  runButton.textContent = "重新生成";
  updateAssistantContext();
  renderAssistantSuggestions();
  syncAssistantAvailability();
}

function setRunFailed(message) {
  document.querySelector("#runStatus").textContent = "诊断失败";
  document.querySelector("#runDetail").textContent = formatAgentDisplayText(message || "Legato 必需解析失败，请检查材料或后端服务。");
  document.querySelector(".generation-dock").classList.remove("is-running");
  runButton.disabled = false;
  runButton.textContent = "重新生成";
  updateAssistantContext();
  syncAssistantAvailability();
}

function setRunWaitingForBenchmark() {
  document.querySelector("#runStatus").textContent = "生成中";
  document.querySelector("#runDetail").textContent = baseJobDone
    ? "基础流程已完成，等待 Item Benchmark 返回六维分布。"
    : "Item Benchmark 正在评估 Impact 和六维分布。";
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
    document.querySelector("#runDetail").textContent = formatAgentDisplayText(payload.message || "异步诊断已开始。");
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
    }
    unlockModule("outputs");
    const keepEventsForBenchmark = benchmarkRequestInFlight || diagnosis?.ability_profile?.benchmark_status === "benchmarking";
    if (keepEventsForBenchmark) {
      setRunWaitingForBenchmark();
    } else if (diagnosis?.ability_profile?.benchmark_status === "failed") {
      setBenchmarkRunFailed();
    } else {
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

function handleDiagnosisEvent(event) {
  markAgentStep(event.step, event.status);
  const statusText = event.status === "running"
    ? "生成中"
    : event.status === "failed"
      ? `失败：${stepLabel(event.step)}`
      : `已完成：${stepLabel(event.step)}`;
  document.querySelector("#runStatus").textContent = statusText;
  document.querySelector("#runDetail").textContent = formatAgentDisplayText(event.message || runStepDetails[event.step] || "正在生成诊断结果。");

  const data = event.data || {};
  if (data.agent_team_event) {
    handleAgentTeamChatEvent(data.agent_team_event);
  }
  if (data.ability_profile) {
    diagnosis = diagnosis || createDiagnosisShell();
    diagnosis.ability_profile = data.ability_profile;
    renderBasicInfo(diagnosis);
    renderAbilityRadar(data.ability_profile);
    renderResumeEvidence(data.ability_profile);
    maybeRequestItemBenchmark(data.ability_profile);
    if (event.step === "profile" && event.status === "failed" && data.ability_profile.benchmark_status === "failed") {
      setBenchmarkRunFailed(formatAgentDisplayText(data.error || event.message));
    }
    unlockModule("profile");
  }
  if (data.matching_result) {
    diagnosis = diagnosis || createDiagnosisShell();
    diagnosis.matching_result = data.matching_result;
    renderMatching(data.matching_result);
  }
  if (data.top_jobs) {
    diagnosis = diagnosis || createDiagnosisShell();
    diagnosis.ability_profile.top5_matching_jobs = data.top_jobs;
    renderTopJobs(data.top_jobs);
  }
  if (data.path_plan) {
    diagnosis = diagnosis || createDiagnosisShell();
    diagnosis.path_plan = data.path_plan;
    renderPath(data.path_plan);
  }
  if (data.diagnosis) {
    diagnosis = data.diagnosis;
    renderDiagnosis(diagnosis);
  }
  if (data.backend_requirements) {
    diagnosis = diagnosis || createDiagnosisShell();
    diagnosis.backend_requirements = data.backend_requirements;
    renderRequirements(data.backend_requirements);
  }
  if (data.production_limitations && diagnosis) {
    diagnosis.production_limitations = data.production_limitations;
    renderLimitations(data.production_limitations);
  }

  if (event.status === "done") {
    if (event.step === "profile") unlockModule("profile");
    if (event.step === "matching") unlockModule("matching");
    if (event.step === "path") unlockModule("path");
    if (event.step === "outputs") unlockModule("outputs");
  }
  updateAssistantContext();
  renderAssistantSuggestions();
}

function handleAgentTeamChatEvent(event) {
  const normalized = normalizeAgentTeamEvent(event);
  const teamTerminal = normalized.agentKey === "team" && (normalized.status === "done" || normalized.status === "failed");
  const isTokenDelta = normalized.tokenChannel === "content" && Boolean(normalized.tokenDelta);
  assistantAgentStreamActive = !teamTerminal;
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
  if (teamTerminal && baseJobDone && !benchmarkRequestInFlight) {
    if (normalized.status === "done" && !failedRunStep) setRunDone();
    closeDiagnosisEvents();
  }
}

function ensureAgentTeamStreamMessage(session, event = null) {
  const phaseGroup = agentStreamGroupForEvent(event);
  let message = session.messages.find((item) => {
    if (item.streamType !== "agent_team") return false;
    const streamGroup = item.agentStream?.phaseGroup || "team";
    return streamGroup === phaseGroup;
  });
  if (message) {
    if (!message.agentStream) message.agentStream = createAgentTeamStreamState(phaseGroup);
    if (!message.agentStream.phaseGroup) message.agentStream.phaseGroup = phaseGroup;
    assistantAgentStreamMessageId = message.id;
    return { message, created: false };
  }
  const now = new Date().toISOString();
  const stream = createAgentTeamStreamState(phaseGroup);
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

function createAgentTeamStreamState(phaseGroup = "planning") {
  const config = agentStreamPhaseConfig(phaseGroup);
  return {
    title: config.title,
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
  stream = stream || createAgentTeamStreamState(phaseGroup);
  const isTokenDelta = event.tokenChannel === "content" && Boolean(event.tokenDelta);
  stream.phaseGroup = phaseGroup;
  stream.stageOrder = agentStreamPhaseConfig(phaseGroup).stageOrder;
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
  const existing = stream.agents.synthesis_arbiter;
  const message = existing?.message && existing.status === "done"
    ? existing.message
    : "Synthesis Arbiter 已返回结构化岗位匹配结果。";
  upsertAgentStreamAgent(stream, {
    ...event,
    agentKey: "synthesis_arbiter",
    agent: "Synthesis Arbiter",
    status: "done",
    phase: "final_synthesis",
    perspective: existing?.perspective || "multi_view_decision",
    reasoningEffort: existing?.reasoningEffort || "",
    focus: existing?.focus || "综合所有视角结果，输出岗位匹配报告。",
    agentIndex: 1,
    agentTotal: 1,
    runID: existing?.runID || event.runID || "",
    message,
    outputPreview: existing?.outputPreview || event.outputPreview || ""
  });
  const item = stream.agents.synthesis_arbiter;
  if (item?.outputPreview && !item.typedOutput) {
    item.typedOutput = item.outputPreview;
    item.typingDone = true;
  }
}

function agentStreamPhaseConfig(phaseGroup = "planning") {
  const group = ["planning", "team", "synthesis"].includes(phaseGroup) ? phaseGroup : "team";
  const configs = {
    planning: {
      group: "planning",
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
  if (agentKey === "synthesis_arbiter" || phase === "final_synthesis") return "synthesis";
  if (agentKey === "adaptive_planner" || phase === "planning") return "planning";
  if (agentKey === "team" && event.status === "done") return "synthesis";
  if (agentKey === "team" && phase === "orchestration" && !event.agentCount && event.status !== "failed") return "planning";
  return "team";
}

function resolveAgentStreamStatus(stream, event) {
  if (event.status === "failed") return "failed";
  if (stream.phaseGroup === "planning") {
    return stream.agents.adaptive_planner?.status === "done" ? "done" : "running";
  }
  if (stream.phaseGroup === "synthesis") {
    if (event.agentKey === "team" && event.status === "done") return "done";
    return stream.agents.synthesis_arbiter?.status === "done" ? "done" : "running";
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
  const config = agentStreamPhaseConfig(stream.phaseGroup);
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

function markAgentStep(step, status) {
  if (!agentSteps.includes(step)) return;
  const item = document.querySelector(`[data-run-step="${step}"]`);
  if (!item) return;
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
  }
  const doneCount = agentSteps.filter((name) => document.querySelector(`[data-run-step="${name}"]`)?.classList.contains("is-done")).length;
  const runningCount = agentSteps.filter((name) => document.querySelector(`[data-run-step="${name}"]`)?.classList.contains("is-running")).length;
  const perceivedProgress = doneCount + runningCount * 0.35;
  setRunProgress(Math.round((perceivedProgress / agentSteps.length) * 100));
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
  showToast("该失败阶段暂不支持局部继续，请重新生成诊断。");
}

function resetResultModules() {
  diagnosis = null;
  currentJobId = "";
  benchmarkRequestInFlight = false;
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
  renderMatchingRadar({});
  document.querySelector("#reportRows").innerHTML = "";
  document.querySelector("#gapTable").innerHTML = "";
  document.querySelector("#pathStages").innerHTML = "";
  document.querySelector("#topJobs").innerHTML = "";
  document.querySelector("#matchingJobs").innerHTML = "";
  document.querySelector("#requirementsList").innerHTML = "";
  document.querySelector("#limitationsList").innerHTML = "";
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
  maybeRequestItemBenchmark(data.ability_profile);
  renderMatching(data.matching_result);
  renderPath(data.path_plan);
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
  const evidenceItems = benchmarkedEvidenceItems(profile);
  const benchmarkStatus = profile.benchmark_status || "waiting";
  const isFailed = benchmarkStatus === "failed";
  const isLoading = isBenchmarkLoadingStatus(benchmarkStatus) && evidenceItems.length === 0;
  wrap.classList.toggle("is-loading", isLoading);
  wrap.classList.toggle("is-failed", isFailed);
  if (status) {
    status.textContent = isFailed ? "Benchmark 失败" : isLoading ? "等待 Benchmark" : evidenceItems.length ? "Legato Benchmark" : "等待证据";
    status.className = `status-pill ${isFailed ? "is-danger" : evidenceItems.length ? "is-real" : "is-warning"}`;
  }
  if (isFailed && evidenceItems.length === 0) {
    clearRadarAnimationState();
    renderRadarFailed(svg);
    if (text) text.textContent = "";
    return;
  }
  if (isLoading || evidenceItems.length === 0) {
    clearRadarAnimationState();
    renderRadarLoading(svg);
    if (text) text.textContent = "";
    return;
  }
  const items = benchmarkDimensions.map((name) => ({ name }));
  const center = { x: 180, y: 158 };
  const radius = 104;
  const series = buildRadarSeries(profile, evidenceItems);
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

function radarValueHoverLayer(items, series, center, radius) {
  return items.map((item, index) => {
    const outer = pointFor(index, items.length, radius + 8, center);
    const card = radarValueCardPosition(index, items.length, radius, center);
    const aria = `${item.name}分值：${series.map((entry) => `${entry.label}${Math.round(Number(entry.scores[index]) || 0)}分`).join("，")}`;
    return `
      <g class="radar-dimension-hover" tabindex="0" aria-label="${escapeAttribute(aria)}">
        <line class="radar-hover-zone" x1="${center.x}" y1="${center.y}" x2="${outer.x.toFixed(2)}" y2="${outer.y.toFixed(2)}"></line>
        <circle class="radar-hover-point" cx="${outer.x.toFixed(2)}" cy="${outer.y.toFixed(2)}" r="7"></circle>
        <g class="radar-value-card" transform="translate(${card.x}, ${card.y})" aria-hidden="true">
          <rect width="108" height="76" rx="8"></rect>
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
  const y = Math.max(10, Math.min(234, Math.round(anchor.y - 36)));
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

function buildRadarSeries(profile, items) {
  const academicBaseline = academicBaselineVector(profile);
  const campusItems = items.filter((item) => normalizedEvidenceScope(item) === "校内");
  const externalItems = items.filter((item) => normalizedEvidenceScope(item) === "校外");
  const schoolTier = schoolTierProfile(profile);
  return [
    buildRadarSeriesEntry("overall", "综合", items, academicBaseline, schoolTier, { includeAcademicPrior: true }),
    buildRadarSeriesEntry("campus", "校内", campusItems, academicBaseline, schoolTier, { includeAcademicPrior: true }),
    buildRadarSeriesEntry("external", "校外", externalItems, academicBaseline, schoolTier, { includeAcademicPrior: false })
  ];
}

function buildRadarSeriesEntry(key, label, items, academicBaseline, schoolTier, options = {}) {
  const dimensionContributions = Array.from({ length: 6 }, emptyDimensionContributionBucket);
  const quality = evidenceQualitySummary(items);
  items.forEach((item) => {
    const distribution = normalizedBenchmarkScores(item);
    if (distribution.length !== 6) return;
    const strength = evidenceStrength(item);
    const bucketKey = cappedEvidenceBucketKey(item);
    distribution.forEach((share, index) => {
      const contribution = Math.max(0, Math.min(0.96, share * strength * radarEvidenceGain));
      const rule = bucketKey ? cappedEvidenceBucketRules[bucketKey] : null;
      if (rule) {
        dimensionContributions[index].capped[bucketKey].push(Math.min(contribution, rule.singleCap));
      } else {
        dimensionContributions[index].regular.push(contribution);
      }
    });
  });
  const scores = dimensionContributions.map(({ regular, capped }) => {
    const cappedContributions = Object.entries(capped)
      .map(([bucketKey, contributions]) => {
        const rule = cappedEvidenceBucketRules[bucketKey];
        return rule ? cappedEvidenceBucket(contributions, rule.totalCap) : 0;
      })
      .filter((contribution) => contribution > 0);
    return diminishingEvidenceScore([
      ...regular,
      ...cappedContributions
    ]);
  });
  const scaledScores = scores.map((score, index) => {
    const evidenceScore = Math.round(score * 100);
    return combineEvidenceWithSchoolTier(
      evidenceScore,
      academicBaseline,
      schoolTier,
      quality,
      items.length,
      index,
      options
    );
  });
  return {
    key,
    label,
    count: items.length,
    scores: scaledScores
  };
}

function emptyDimensionContributionBucket() {
  return {
    regular: [],
    capped: Object.fromEntries(Object.keys(cappedEvidenceBucketRules).map((bucketKey) => [bucketKey, []]))
  };
}

function cappedEvidenceBucket(contributions, cap) {
  if (!Array.isArray(contributions) || contributions.length === 0) return 0;
  return Math.min(diminishingEvidenceScore(contributions), cap);
}

function cappedEvidenceBucketKey(item) {
  if (isLowImpactAwardOrCertificateEvidence(item)) return "lowImpactAwardCertificate";
  if (isCampusAwardEvidence(item)) return "campusAward";
  if (isGenericCampusRoleEvidence(item)) return "genericCampusRole";
  if (isUntitledProfessionalProjectEvidence(item)) return "untitledProject";
  return impactEvidenceBucketKey(item);
}

function impactEvidenceBucketKey(item) {
  const impact = numericMetric(item?.impact_factor);
  const level = numericMetric(item?.level);
  const signal = Number.isFinite(impact) ? impact : level;
  if (!Number.isFinite(signal)) return "impactLow";
  if (signal >= 8.5) return "impactExceptional";
  if (signal >= 7) return "impactHigh";
  if (signal >= 5.5) return "impactMedium";
  return "impactLow";
}

function evidenceQualitySummary(items) {
  const summary = {
    highImpactCount: 0,
    exceptionalImpactCount: 0,
    cappedLowCount: 0
  };
  items.forEach((item) => {
    const bucketKey = cappedEvidenceBucketKey(item);
    const impact = numericMetric(item?.impact_factor);
    if (["lowImpactAwardCertificate", "campusAward", "genericCampusRole", "untitledProject", "impactLow"].includes(bucketKey)) {
      summary.cappedLowCount += 1;
    }
    if (Number.isFinite(impact) && impact >= 8.5 && bucketKey === "impactExceptional") {
      summary.exceptionalImpactCount += 1;
    } else if (Number.isFinite(impact) && impact >= 7 && (bucketKey === "impactHigh" || bucketKey === "impactExceptional")) {
      summary.highImpactCount += 1;
    }
  });
  return summary;
}

function isLowImpactAwardOrCertificateEvidence(item) {
  if (evidenceKind(item) !== "award") return false;
  const text = evidenceText(item);
  const level = numericMetric(item?.level);
  const impact = numericMetric(item?.impact_factor);
  return (
    (Number.isFinite(impact) && impact <= 3.5) ||
    (Number.isFinite(level) && level <= 3) ||
    containsAnyText(text, [
      "证书",
      "CET",
      "英语四级",
      "英语六级",
      "计算机二级",
      "NISP一级",
      "奖学金",
      "优秀学生",
      "优秀学生干部",
      "三好学生",
      "标兵"
    ])
  );
}

function isCampusAwardEvidence(item) {
  return evidenceKind(item) === "award" && normalizedEvidenceScope(item) === "校内";
}

function isGenericCampusRoleEvidence(item) {
  if (evidenceKind(item) !== "experience") return false;
  const text = evidenceText(item);
  return normalizedEvidenceScope(item) === "校内" && containsAnyText(text, [
    "学生会",
    "社团",
    "班长",
    "团支书",
    "部长",
    "主席",
    "干部",
    "优秀学生",
    "组织活动"
  ]);
}

function isUntitledProfessionalProjectEvidence(item) {
  const type = cleanDisplayText(item?.type || item?.experience_type);
  const role = cleanDisplayText(item?.role);
  const contribution = cleanDisplayText(item?.contribution);
  const text = `${type}${role}${contribution}`;
  if (!text) return false;
  if (containsAnyText(text, ["实习", "比赛", "竞赛", "任职", "社团", "学生会", "班长", "部长", "主席"])) return false;
  const isProfessionalProject = containsAnyText(text, [
    "项目",
    "科研",
    "研究",
    "课题",
    "实验",
    "系统",
    "平台",
    "模型",
    "算法",
    "开发",
    "漏洞",
    "测试",
    "数据"
  ]);
  return isProfessionalProject && !hasConcreteProjectTitle(role);
}

function hasConcreteProjectTitle(role) {
  const normalized = cleanDisplayText(role).replace(/\s+/g, "");
  if (!normalized || normalized.length < 4) return false;
  const genericTitles = [
    "项目",
    "科研项目",
    "研究项目",
    "项目经历",
    "科研经历",
    "参与者",
    "参与人",
    "成员",
    "队员",
    "负责人",
    "核心成员",
    "角色未解析",
    "未解析"
  ];
  return !genericTitles.includes(normalized);
}

function evidenceKind(item) {
  if (cleanDisplayText(item?.type || item?.role || item?.contribution || item?.experience_type)) {
    return "experience";
  }
  return "award";
}

function evidenceText(item) {
  return [
    item?.name,
    item?.result,
    item?.type,
    item?.experience_type,
    item?.role,
    item?.contribution,
    item?.evidence_scope
  ].map((value) => cleanDisplayText(value)).join("");
}

function diminishingEvidenceScore(contributions) {
  return contributions
    .filter((contribution) => Number.isFinite(contribution) && contribution > 0)
    .sort((left, right) => right - left)
    .reduce((score, contribution) => addDiminishingEvidence(score, contribution), 0);
}

function addDiminishingEvidence(score, contribution) {
  const rawDelta = (1 - score) * contribution;
  if (rawDelta <= 0) return score;
  if (score < radarEvidenceDiminishThreshold) {
    const roomBeforeThreshold = radarEvidenceDiminishThreshold - score;
    if (rawDelta <= roomBeforeThreshold) return Math.min(radarEvidenceSoftCap, score + rawDelta);
    const overflow = rawDelta - roomBeforeThreshold;
    const nextScore = radarEvidenceDiminishThreshold + overflow * evidenceTailGainAt(radarEvidenceDiminishThreshold);
    return Math.min(radarEvidenceSoftCap, nextScore);
  }
  const nextScore = score + rawDelta * evidenceTailGainAt(score);
  return Math.min(radarEvidenceSoftCap, nextScore);
}

function evidenceTailGainAt(score) {
  if (score <= radarEvidenceDiminishThreshold) return 0.15;
  if (score >= radarEvidenceTailThreshold) return radarEvidenceTailGain;
  const progress = (score - radarEvidenceDiminishThreshold) / (radarEvidenceTailThreshold - radarEvidenceDiminishThreshold);
  return radarEvidenceTailGain + (0.15 - radarEvidenceTailGain) * Math.pow(1 - progress, 2);
}

function combineEvidenceWithSchoolTier(evidenceScore, academicBaseline, schoolTier, quality, itemCount, dimensionIndex, options = {}) {
  const includeAcademicPrior = options.includeAcademicPrior !== false;
  const config = schoolTier?.config || schoolTierConfigs.T3;
  const cap = schoolTierScoreCap(config, quality);
  if (!includeAcademicPrior) {
    if (itemCount === 0) return 0;
    return Math.round(Math.min(cap, evidenceScore));
  }
  const prior = academicPriorForDimension(academicBaseline, schoolTier, dimensionIndex);
  if (itemCount === 0) return Math.round(Math.min(cap, prior));
  const lift = evidenceScore * config.liftScale;
  return Math.round(Math.min(cap, prior + lift));
}

function schoolTierScoreCap(config, quality) {
  if ((quality?.exceptionalImpactCount || 0) > 0) return config.exceptionalCap;
  if ((quality?.highImpactCount || 0) > 0) return config.highCap;
  return config.noHighCap;
}

function academicPriorForDimension(academicBaseline, schoolTier, dimensionIndex) {
  const config = schoolTier?.config || schoolTierConfigs.T3;
  const scores = Array.isArray(academicBaseline?.scores) && academicBaseline.scores.length === 6
    ? academicBaseline.scores.map(Number)
    : Array(6).fill(50);
  const validScores = scores.filter(Number.isFinite);
  const baselineMean = validScores.length
    ? validScores.reduce((sum, value) => sum + value, 0) / validScores.length
    : 50;
  const dimensionOffset = Number.isFinite(scores[dimensionIndex]) ? scores[dimensionIndex] - baselineMean : 0;
  const transcriptBase = Number(academicBaseline?.transcript_base);
  const transcriptOffset = Number.isFinite(transcriptBase) ? Math.max(-8, Math.min(12, transcriptBase - 50)) * 0.45 : 0;
  return Math.max(25, Math.min(85, config.base + transcriptOffset + dimensionOffset * 0.55));
}

function schoolTierProfile(profile) {
  const education = normalizedEducation(profile);
  const tiers = education.map(educationTierKey).filter(Boolean);
  const hasIndependent = education.some(isIndependentCollegeEducation);
  const bestNonIndependent = tiers
    .filter((tier) => tier !== "T4")
    .sort((left, right) => schoolTierRank(left) - schoolTierRank(right))[0] || "";
  let tierKey = tiers.sort((left, right) => schoolTierRank(left) - schoolTierRank(right))[0] || "T3";
  if (hasIndependent) {
    if (bestNonIndependent === "T0" || bestNonIndependent === "T1" || bestNonIndependent === "T2") {
      tierKey = "T4A";
    } else {
      tierKey = "T4";
    }
  }
  return {
    key: tierKey,
    config: schoolTierConfigs[tierKey] || schoolTierConfigs.T3,
    hasIndependent
  };
}

function schoolTierRank(tierKey) {
  return { T0: 0, T1: 1, T2: 2, T3: 3, T4A: 4, T4: 5, T5: 6 }[tierKey] ?? 3;
}

function educationTierKey(item) {
  if (isIndependentCollegeEducation(item)) return "T4";
  const degree = cleanDisplayText(item?.degree || item?.degree_level);
  if (degree.includes("专科")) return "T5";
  const rank = Number(item?.ruanke_rank);
  if (item?.is_985 || (Number.isFinite(rank) && rank > 0 && rank <= 50)) return "T0";
  if (item?.is_211 || item?.is_double_first_class || (Number.isFinite(rank) && rank > 0 && rank <= 150)) return "T1";
  if (Number.isFinite(rank) && rank > 0 && rank <= 250) return "T2";
  return "T3";
}

function isIndependentCollegeEducation(item) {
  const school = cleanDisplayText(item?.school);
  const kind = cleanDisplayText(item?.school_kind);
  return kind === "independent_college" || (/大学.+学院$/.test(school) && !school.includes("大学院"));
}

function combineEvidenceWithAcademicPrior(evidenceScore, baseline, itemCount) {
  if (!Number.isFinite(baseline)) return evidenceScore;
  const academicPrior = Math.round(Math.max(25, Math.min(85, baseline)));
  if (itemCount === 0) return academicPrior;
  const blended = evidenceScore * (1 - academicPriorWeight) + academicPrior * academicPriorWeight;
  const priorFloor = academicPrior * academicPriorFloorRatio;
  return Math.round(Math.max(priorFloor, blended));
}

function academicBaselineVector(profile) {
  const workflowBaseline = workflowAcademicBaseline(profile);
  if (workflowBaseline) return workflowBaseline;
  const base = academicBaseScore(profile);
  const majorText = [
    profile?.basic_info?.major,
    ...normalizedEducation(profile).map((item) => item.major)
  ].join("");
  const isStem = containsAnyText(majorText, ["计算机", "软件", "数据", "网络", "信息", "数学", "统计", "电子", "电气", "自动化", "工程", "物理", "化学", "生物"]);
  const hasMajor = majorText.trim().length > 0;
  const scores = [
    base + (isStem ? 3 : 0),
    base + (isStem ? -2 : 3),
    base + (hasMajor ? 5 : 0),
    base - 10,
    base - 4,
    base
  ].map((score) => Math.round(Math.max(25, Math.min(85, score))));
  return { base, transcript_base: base, scores, major_family: isStem ? "工科类" : "未知", source: "frontend_fallback" };
}

function workflowAcademicBaseline(profile) {
  const baseline = profile?.major_baseline;
  if (!baseline || !Array.isArray(baseline.scores) || baseline.scores.length !== 6) return null;
  const scores = baseline.scores.map((score) => {
    const value = Number(score);
    if (!Number.isFinite(value)) return 0;
    return Math.round(Math.max(25, Math.min(85, value)));
  });
  if (scores.some((score) => score <= 0)) return null;
  const base = Number(baseline.base_score);
  return {
    base: Number.isFinite(base) ? Math.round(Math.max(30, Math.min(85, base))) : 50,
    transcript_base: academicBaseScore(profile),
    scores,
    major_name: String(baseline.major_name || ""),
    major_family: String(baseline.major_family || ""),
    rationale: String(baseline.rationale || ""),
    source: String(baseline.source || "major_baseline")
  };
}

function academicBaseScore(profile) {
  const transcriptUse = String(profile?.basic_info?.transcript_use || "");
  const gpaMatch = transcriptUse.match(/GPA[:：]?\s*([0-9]+(?:\.[0-9]+)?)/i);
  if (!gpaMatch) {
    const averageMatch = transcriptUse.match(/(?:均分|平均分|平均成绩)[:：]?\s*([0-9]+(?:\.[0-9]+)?)/);
    return averageMatch ? academicAverageToPrior(Number(averageMatch[1])) : 50;
  }
  const raw = Number(gpaMatch[1]);
  if (!Number.isFinite(raw) || raw <= 0) return 50;
  if (raw <= 4.3) {
    return academicAverageToPrior(80 + (raw - 3) * 15);
  }
  if (raw <= 5) {
    return academicAverageToPrior(80 + (raw - 3.5) * 10);
  }
  return academicAverageToPrior(raw);
}

function academicAverageToPrior(average) {
  if (!Number.isFinite(average) || average <= 0) return 50;
  return Math.round(Math.max(35, Math.min(78, 50 + (average - 80) * 1.6)));
}

function evidenceStrength(item) {
  const rawLevel = numericMetric(item.level);
  const rawImpact = numericMetric(item.impact_factor);
  const hasLevel = Number.isFinite(rawLevel);
  const hasImpact = Number.isFinite(rawImpact);
  if (!hasLevel && !hasImpact) return 0;
  const level = hasLevel ? Math.max(0, Math.min(10, rawLevel)) / 10 : 0;
  const impact = hasImpact ? Math.max(0, Math.min(10, rawImpact)) / 10 : 0;
  if (!hasImpact) return level;
  if (!hasLevel) return impact;
  return Math.max(0, Math.min(1, level * 0.4 + impact * 0.6));
}

function maybeRequestItemBenchmark(profile, options = {}) {
  if (!currentJobId || benchmarkRequestInFlight) return;
  if (!profile) return;
  const blockedStatuses = options.force
    ? ["ready", "mock", "benchmarking", "unavailable"]
    : ["ready", "mock", "benchmarking", "failed", "unavailable"];
  if (blockedStatuses.includes(profile.benchmark_status)) return;
  if (!isHybridBenchmarkGateOpen(profile.experiences_status)) return;

  const items = buildBenchmarkItems(profile);
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
      if (payload.matching_result) {
        diagnosis.matching_result = payload.matching_result;
        renderMatching(payload.matching_result);
        finalizeAgentTeamStreamFromMatchingPayload(payload);
      }
      if (payload.top_jobs) {
        diagnosis.ability_profile.top5_matching_jobs = payload.top_jobs;
        renderTopJobs(payload.top_jobs);
      }
      if (payload.ability_profile.benchmark_status === "failed" || payload.error) {
        setBenchmarkRunFailed(payload.error);
        return;
      }
      markAgentStep("profile", "done");
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
      if (baseJobDone && !assistantAgentStreamActive) closeDiagnosisEvents();
      updateAssistantContext();
    });
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
    agent_key: "team",
    agent: "Legato Job Matching Team",
    status: "done",
    phase: "orchestration",
    message: "动态 Presto Agent Team 已完成岗位推荐和匹配报告。",
    output_preview: preview
  });
  const { message } = ensureAgentTeamStreamMessage(session, event);
  message.agentStream = updateAgentTeamStreamState(message.agentStream, event);
  message.content = agentTeamStreamFallback(message.agentStream);
  message.status = "done";
  message.updatedAt = event.time;
  session.updatedAt = event.time;
  assistantAgentStreamActive = false;
  saveAssistantState();
  renderAssistant();
}

function setBenchmarkRunFailed(errorMessage = "") {
  failedRunStep = "profile";
  benchmarkRequestInFlight = false;
  markAgentStep("profile", "failed");
  setRunStepRetryable("profile", true);
  document.querySelector("#runStatus").textContent = "失败：能力画像";
  document.querySelector("#runDetail").textContent = errorMessage
    ? `Item Benchmark 失败：${errorMessage}。点击下方红色“画像”继续。`
    : "Item Benchmark 失败，点击下方红色“画像”从失败处继续。";
  document.querySelector(".generation-dock").classList.remove("is-running");
  runButton.disabled = false;
  runButton.textContent = "重新生成";
  setAssistantExpanded(false, { silent: true });
  syncAssistantAvailability();
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

function isHybridBenchmarkGateOpen(status) {
  return ["ready", "empty", "failed", "mock"].includes(status || "");
}

function buildBenchmarkItems(profile) {
  const awards = Array.isArray(profile.awards) ? profile.awards : [];
  const experiences = Array.isArray(profile.experiences) ? profile.experiences : [];
  return [
    ...awards.map((item, index) => ({
      kind: "award",
      key: `award:${index}`,
      name: item.name || "",
      result: item.result || "",
      evidence_scope: normalizedEvidenceScope(item),
      level: numericMetric(item.level)
    })),
    ...experiences.map((item, index) => ({
      kind: "experience",
      key: `experience:${index}`,
      name: item.role || item.contribution || item.type || "",
      result: "",
      experience_type: item.type || "",
      role: item.role || "",
      contribution: item.contribution || "",
      evidence_scope: normalizedEvidenceScope(item),
      level: numericMetric(item.level)
    }))
  ].filter((item) => item.name || item.result || item.role || item.contribution);
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
  const isBenchmarkLoading = isBenchmarkLoadingStatus(benchmarkStatus) && (awards.length > 0 || experiences.length > 0);
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
  const ready = isAssistantReady();
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
      title="${ready ? "添加到聊天" : "全部结果生成后才能追问"}"
      ${ready ? "" : "disabled"}
    >
      <span aria-hidden="true">+</span>
      <strong>添加到聊天</strong>
    </button>
  `;
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
      ${renderMetricChip("Level", level, levelLoading || !hasMetricValue(level))}
      ${renderMetricChip("Impact", impact, benchmarkLoading || (!benchmarkFailed && !hasMetricValue(impact)), benchmarkFailed && !hasMetricValue(impact))}
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
      <div class="metric-chip is-loading" aria-label="${escapeAttribute(label)} 正在评分">
        <span>${escapeHTML(label)}</span>
        <b></b>
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
  document.querySelector("#overallMatch").textContent = overall ? `${Math.round(overall)}%` : "--";
  document.querySelector("#matchLevel").textContent = match.match_level || "等待 Job Matching";
  document.querySelector("#overallMeter").style.width = `${overall}%`;
  document.querySelector("#selectedJobTitle").textContent = selected.title || match.target_role || "等待推荐";
  document.querySelector("#matchNarrative").textContent = formatAgentDisplayText(match.fit_summary || selected.fit_summary || "Benchmark 完成后，Legato 会返回首选岗位、推荐理由和需要补齐的证据。");
  document.querySelector("#matchAgentNotes").innerHTML = (match.agent_notes || []).map((note) => `<span>${escapeHTML(formatAgentDisplayText(note))}</span>`).join("");
  renderMatchingRadar(match);

  document.querySelector("#reportRows").innerHTML = (match.report_sections || []).map((row) => `
    <div class="report-row">
      <strong>${escapeHTML(row.name)}</strong>
      <div class="report-bar" aria-label="学生分值 ${row.student}"><span style="width:${safeScore(row.student)}%"></span></div>
      <div class="need-bar" aria-label="岗位要求 ${row.role_need}"><span style="width:${safeScore(row.role_need)}%"></span></div>
      <span class="delta">${row.difference > 0 ? "+" : ""}${row.difference}</span>
    </div>
  `).join("");

  document.querySelector("#gapTable").innerHTML = (match.gap_details || []).map((gap) => `
    <tr>
      <td>${escapeHTML(gap.capability)}</td>
      <td>${escapeHTML(gap.current)}</td>
      <td>${escapeHTML(gap.expected)}</td>
      <td>${escapeHTML(gap.action)}</td>
      <td><span class="severity ${gap.severity === "高" ? "high" : ""}">${escapeHTML(gap.severity)}</span></td>
    </tr>
  `).join("");
}

function renderMatchingRadar(match) {
  const svg = document.querySelector("#matchingRadarChart");
  const text = document.querySelector("#matchingRadarText");
  const legend = document.querySelector("#matchingRadarLegend");
  if (!svg) return;
  const student = normalizeMatchingRadar(match?.student_radar);
  const target = normalizeMatchingRadar(match?.target_radar || match?.selected_job?.requirement_radar);
  if (student.length !== benchmarkDimensions.length || target.length !== benchmarkDimensions.length) {
    renderMatchingRadarWaiting(svg);
    if (legend) legend.innerHTML = "";
    if (text) text.textContent = "等待首选岗位目标能力雷达。";
    return;
  }
  const center = { x: 180, y: 158 };
  const radius = 104;
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
      return `
        <line class="radar-axis" x1="${center.x}" y1="${center.y}" x2="${outer.x.toFixed(2)}" y2="${outer.y.toFixed(2)}"></line>
        <text class="radar-label" x="${label.x.toFixed(2)}" y="${label.y.toFixed(2)}" text-anchor="middle">${escapeHTML(name)}</text>
      `;
    }).join("")}
    <polygon class="matching-radar-area is-target" points="${targetPoints}"></polygon>
    <polygon class="matching-radar-area is-student" points="${studentPoints}"></polygon>
    ${student.map((item, index) => {
      const point = pointFor(index, benchmarkDimensions.length, radius * radarVisualRatio(item.score), center);
      return `<circle class="matching-radar-dot is-student" cx="${point.x.toFixed(2)}" cy="${point.y.toFixed(2)}" r="3.5"></circle>`;
    }).join("")}
    ${target.map((item, index) => {
      const point = pointFor(index, benchmarkDimensions.length, radius * radarVisualRatio(item.score), center);
      return `<circle class="matching-radar-dot is-target" cx="${point.x.toFixed(2)}" cy="${point.y.toFixed(2)}" r="3"></circle>`;
    }).join("")}
  `;
  if (legend) {
    legend.innerHTML = `
      <span><i class="legend-dot is-student"></i>个人画像</span>
      <span><i class="legend-dot is-target"></i>岗位目标</span>
    `;
  }
  if (text) {
    const gaps = benchmarkDimensions.map((name, index) => ({
      name,
      gap: student[index].score - target[index].score
    }));
    const weakest = gaps.reduce((current, item) => item.gap < current.gap ? item : current, gaps[0]);
    const strongest = gaps.reduce((current, item) => item.gap > current.gap ? item : current, gaps[0]);
    text.textContent = `${match?.target_role || match?.selected_job?.title || "首选岗位"}要求下，最大短板为${weakest.name}${weakest.gap}分，当前相对优势为${strongest.name}${strongest.gap >= 0 ? "+" : ""}${strongest.gap}分。`;
  }
}

function renderMatchingRadarWaiting(svg) {
  const center = { x: 180, y: 158 };
  const radius = 104;
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
  document.querySelector("#pathStages").innerHTML = plan.stages.map((stage) => `
    <article class="stage-panel reveal is-visible">
      <div class="stage-heading">
        <h3>${escapeHTML(stage.stage)}</h3>
        <span class="sim-badge">模拟数据</span>
      </div>
      <p class="stage-goal">${escapeHTML(stage.goal)}</p>
      <ol class="task-list">
        ${stage.weeks.map((week) => `
          <li>
            <strong>${escapeHTML(week.week)}：${escapeHTML(week.task)}</strong>
            <span>${escapeHTML(week.metric)}，优先级 ${escapeHTML(week.priority)}</span>
          </li>
        `).join("")}
      </ol>
      <strong>达标标准</strong>
      <ul>
        ${stage.standards.map((standard) => `<li>${escapeHTML(standard)}</li>`).join("")}
      </ul>
      <div class="resource-list">
        ${stage.resources.map((resource) => `<a href="${safeURLAttribute(resource.url)}" target="_blank" rel="noreferrer">${escapeHTML(resource.label)}</a>`).join("")}
      </div>
    </article>
  `).join("");
}

function renderTopJobs(jobs) {
  jobs = jobs || [];
  const matchingList = document.querySelector("#matchingJobs");
  const outputList = document.querySelector("#topJobs");
  if (matchingList) {
    matchingList.innerHTML = jobs.map((job) => renderMatchingJobCard(job)).join("");
  }
  if (outputList) {
    outputList.innerHTML = jobs.map((job) => renderOutputJobCard(job)).join("");
  }
}

function renderMatchingJobCard(job) {
  const reasons = Array.isArray(job.reasons) ? job.reasons : [];
  const category = job.category || "推荐岗位";
  const educationGate = job.education_gate || "";
  return `
    <article class="matching-job-card">
      <header>
        <div>
          <span class="job-rank">${job.rank || ""}</span>
          <h3>${escapeHTML(job.title || "")}</h3>
        </div>
        <strong>${safeScore(job.match)}%</strong>
      </header>
      <div class="job-tags">
        <span>${escapeHTML(category)}</span>
        ${educationGate ? `<span>${escapeHTML(educationGate)}</span>` : ""}
      </div>
      <div class="job-progress-group">
        ${renderJobProgress("综合", job.match)}
        ${renderJobProgress("六维", job.ability_match || job.match)}
        ${renderJobProgress("经历", job.experience_match)}
      </div>
      ${job.fit_summary ? `<p>${escapeHTML(formatAgentDisplayText(job.fit_summary))}</p>` : ""}
      ${reasons.length ? `<p>${escapeHTML(formatAgentDisplayText(reasons.join("；")))}</p>` : ""}
      ${job.next_proof ? `<small>${escapeHTML(formatAgentDisplayText(job.next_proof))}</small>` : ""}
    </article>
  `;
}

function renderOutputJobCard(job) {
  const reasons = Array.isArray(job.reasons) ? job.reasons : [];
  const mockBadge = isMockMatchingResult() ? `<span class="sim-badge">模拟数据</span>` : "";
  return `
    <article class="job-item">
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
  document.querySelector("#requirementsList").innerHTML = requirements.map((item) => `
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
      window.location.href = "/api/export/ability-profile.xlsx";
    }
    if (type === "path-doc") {
      window.location.href = "/api/export/path-plan.doc";
    }
    if (type === "path-pdf") {
      showToast("即将打开打印面板，请选择另存为 PDF。");
      window.setTimeout(() => window.print(), 180);
    }
    if (type === "match-json") {
      downloadJSON("matching-report.json", diagnosis.matching_result);
    }
  });
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
    addEvidenceToAssistant(button.dataset.evidenceChat, Number(button.dataset.evidenceIndex));
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
    agentStream: message.streamType === "agent_team" ? normalizeAgentStreamState(message.agentStream) : null
  };
}

function shouldSplitLegacyAgentStreamMessage(rawMessage, normalizedMessage) {
  const rawStream = rawMessage?.agentStream;
  const stream = normalizedMessage?.agentStream;
  if (normalizedMessage?.streamType !== "agent_team" || !stream) return false;
  const keys = new Set(stream.order || []);
  const mixedLegacyTeam = keys.has("adaptive_planner") && keys.has("synthesis_arbiter") && stream.order.length > 2;
  return mixedLegacyTeam && (!rawStream?.phaseGroup || rawStream.phaseGroup === "team");
}

function splitLegacyAgentStreamMessage(message) {
  const source = message.agentStream;
  const groups = {
    planning: source.order.filter((key) => key === "adaptive_planner"),
    team: source.order.filter((key) => key !== "adaptive_planner" && key !== "synthesis_arbiter"),
    synthesis: source.order.filter((key) => key === "synthesis_arbiter")
  };
  return ["planning", "team", "synthesis"]
    .filter((group) => groups[group].length > 0)
    .map((group) => {
      const stream = createAgentTeamStreamState(group);
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
  const defaults = createAgentTeamStreamState(phaseGroup);
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
  const item = stream.agents.synthesis_arbiter;
  if (!item || item.status === "done" || item.status === "failed") return stream;
  const updatedAt = Date.parse(item.updatedAt || stream.updatedAt || "");
  if (!Number.isFinite(updatedAt) || Date.now() - updatedAt < 2 * 60 * 1000) return stream;
  item.status = "done";
  item.message = "Synthesis Arbiter 已返回结构化岗位匹配结果。";
  item.typingDone = true;
  if (item.outputPreview && !item.typedOutput) item.typedOutput = item.outputPreview;
  stream.status = "done";
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
  assistantState.sessions = assistantState.sessions.slice(0, assistantMaxSessions).map((session) => ({
    ...session,
    messages: session.messages.slice(-assistantMaxMessages)
  }));
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
  const session = {
    id: uniqueId("chat"),
    title: String(options.title || "新诊断对话").slice(0, 80),
    prestoSessionId: "",
    diagnosisJobId: String(options.diagnosisJobId || ""),
    diagnosisFileName: String(options.diagnosisFileName || ""),
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
  if (!isAssistantReady()) {
    showToast("全部诊断结果生成后才能追问。");
    return;
  }
  const evidence = evidenceContextByKey(kind, index);
  if (!evidence) {
    showToast("未找到这条证据，请重新生成诊断。");
    return;
  }
  assistantFocusedEvidence = evidence;
  const session = ensureAssistantSession();
  const now = new Date().toISOString();
  const previous = session.messages[session.messages.length - 1];
  if (previous?.streamType !== "evidence_context" || previous.focusEvidenceKey !== evidence.key) {
    session.messages.push({
      id: uniqueId("msg"),
      role: "assistant",
      content: assistantEvidenceContextMessage(evidence),
      createdAt: now,
      updatedAt: now,
      status: "done",
      retryPrompt: "",
      streamType: "evidence_context",
      focusEvidenceKey: evidence.key
    });
  }
  session.updatedAt = now;
  assistantInput.value = assistantEvidencePrompt(evidence);
  updateAssistantInputMeta();
  saveAssistantState();
  setAssistantExpanded(true);
  renderAssistant();
  assistantInput.focus();
  showToast("已加入聊天上下文。");
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
    : cleanDisplayText([item.type, item.role].filter(Boolean).join(" · ") || item.contribution);
  return {
    ...context,
    key: `${kind}:${index}`,
    kind,
    title: title || (kind === "award" ? "奖项与证书" : "经历"),
    score_summary: evidenceScoreSummary(item),
    dimension_summary: evidenceDimensionSummary(item)
  };
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

function assistantEvidencePrompt(evidence) {
  const score = evidence.score_summary.level || evidence.score_summary.impact_factor
    ? `Level ${evidence.score_summary.level || "未返回"}、Impact ${evidence.score_summary.impact_factor || "未返回"}`
    : "当前评分";
  return `请重点解释「${evidence.title}」为什么给出 ${score}，它对岗位匹配和后续补强有什么影响？`;
}

function isAssistantReady() {
  const allModulesReady = moduleLocks.profile && moduleLocks.matching && moduleLocks.path && moduleLocks.outputs;
  const benchmarkStatus = diagnosis?.ability_profile?.benchmark_status || "";
  return Boolean(
    diagnosis &&
    allModulesReady &&
    !diagnosisEvents &&
    !benchmarkRequestInFlight &&
    !failedRunStep &&
    benchmarkStatus !== "benchmarking" &&
    benchmarkStatus !== "failed"
  );
}

function isAssistantInspectable() {
  return isAssistantReady() || assistantAgentStreamActive || Boolean(diagnosisEvents) || benchmarkRequestInFlight || Boolean(diagnosis);
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
  assistantInput.disabled = assistantBusy || !ready;
  assistantSend.disabled = assistantBusy || !ready;
  assistantNewSession.disabled = assistantBusy || !ready;
  assistantArchiveSession.disabled = assistantBusy || !ready;
  assistantSuggestions.querySelectorAll("button").forEach((button) => {
    button.disabled = assistantBusy || !ready;
  });
  document.querySelectorAll("[data-evidence-chat]").forEach((button) => {
    const selected = assistantFocusedEvidence?.key === button.dataset.evidenceKey;
    button.disabled = !ready;
    button.setAttribute("aria-disabled", String(!ready));
    button.setAttribute("aria-pressed", String(selected));
    button.classList.toggle("is-added", selected);
    button.title = selected ? "已加入聊天" : ready ? "添加到聊天" : "全部结果生成后才能追问";
  });
  if (!inspectable && !assistant.classList.contains("is-collapsed")) {
    setAssistantExpanded(false, { silent: true });
  }
  if (ready) {
    updateAssistantRailStatus(assistantBusy ? "生成中" : "可追问");
  } else if (inspectOnly) {
    updateAssistantRailStatus("生成中可查看");
  } else {
    updateAssistantRailStatus(diagnosisEvents || benchmarkRequestInFlight ? "结果生成中" : "结果完成后可追问");
  }
}

function renderAssistant() {
  ensureAssistantSession();
  applyAssistantExpandedState(assistantState.expanded);
  renderAssistantArchive();
  renderAssistantMessages();
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
    const stateClass = message.status === "loading" ? " is-loading" : message.status === "error" ? " is-error" : "";
    const streamClass = message.streamType === "agent_team" ? " is-agent-stream" : message.streamType === "evidence_context" ? " is-evidence-context" : "";
    const content = message.role === "user" ? message.content : formatAgentDisplayText(message.content);
    const body = message.streamType === "agent_team" && message.agentStream
      ? renderAgentTeamStreamMessage(message.agentStream, message.id)
      : `<div class="assistant-bubble">${escapeHTML(content)}</div>`;
    return `
      <article class="assistant-message is-${escapeAttribute(message.role)}${stateClass}${streamClass}">
        <b>${escapeHTML(label)} · ${formatTime(message.createdAt)}</b>
        ${body}
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
  const config = agentStreamPhaseConfig(stream.phaseGroup);
  const completed = stream.order.filter((key) => stream.agents[key]?.status === "done").length;
  const failed = stream.order.filter((key) => stream.agents[key]?.status === "failed").length;
  const total = stream.phaseGroup === "team" ? stream.agentCount || stream.order.length : 1;
  const statusLabel = stream.status === "done" ? "完成" : stream.status === "failed" ? "失败" : "运行中";
  const progressLabel = stream.phaseGroup === "team"
    ? `${completed}/${total || "--"} 个视角${failed ? ` · ${failed} 个失败` : ""}`
    : config.label;
  return `
    <div class="assistant-bubble agent-stream-card is-${escapeAttribute(config.group)}">
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
  const text = hasOutput ? agent.typedOutput || agent.outputPreview || "" : agent.message || agent.focus || "等待 Agent 输出。";
  const cursor = hasOutput && !agent.typingDone ? `<span class="agent-stream-cursor" aria-hidden="true"></span>` : "";
  const typeoutKey = `${messageID}:${agent.key}`;
  return `
    <span class="agent-stream-detail"><span class="agent-stream-typeout" data-agent-typeout="${escapeAttribute(typeoutKey)}" data-agent-message="${escapeAttribute(messageID)}" data-agent-key="${escapeAttribute(agent.key)}"><span data-agent-typeout-text="${escapeAttribute(typeoutKey)}" data-agent-message="${escapeAttribute(messageID)}" data-agent-key="${escapeAttribute(agent.key)}">${escapeHTML(formatAgentDisplayText(text))}</span>${cursor}</span></span>
  `;
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
    if (!agent.typingDone && !cursor) {
      cursor = document.createElement("span");
      cursor.className = "agent-stream-cursor";
      cursor.setAttribute("aria-hidden", "true");
      typeout.append(cursor);
    }
    if (cursor) cursor.hidden = Boolean(agent.typingDone);
    if (stickToBottom) {
      window.requestAnimationFrame(() => {
        typeout.scrollTop = typeout.scrollHeight;
      });
    }
  }
  return true;
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
        <span>${escapeHTML(formatTime(item.time))} · ${escapeHTML(formatAgentDisplayText(item.agent || item.agentKey || "Agent"))} · ${escapeHTML(formatAgentDisplayText(item.message || item.status))}</span>
      `).join("")}
    </details>
  `;
}

function renderAssistantSuggestions() {
  const suggestions = isAssistantReady() ? [
    "优先补哪项能力？",
    "首位岗位为什么匹配？",
    "本周行动清单"
  ] : [];
  assistantSuggestions.innerHTML = suggestions.map((text) => `
    <button type="button" data-suggestion="${escapeAttribute(text)}">${escapeHTML(text)}</button>
  `).join("");
}

function updateAssistantContext() {
  if (!assistantContext) return;
  const ready = isAssistantReady();
  const label = ready ? "可追问" : assistantAgentStreamActive ? "Agent Team 流式生成" : diagnosisEvents || benchmarkRequestInFlight || diagnosis ? "诊断生成中" : "等待诊断";
  const pillClass = ready ? "is-real" : "is-warning";
  assistantContext.setAttribute("aria-label", label);
  assistantContext.innerHTML = `<span class="status-pill ${pillClass}">${escapeHTML(label)}</span>`;
  syncAssistantAvailability();
}

function updateAssistantInputMeta() {
  const length = assistantInput.value.length;
  const ready = isAssistantReady();
  assistantInput.placeholder = ready ? "追问能力短板、岗位理由或任务优先级" : "诊断完成后可追问";
  assistantInputMeta.textContent = ready
    ? length ? `${length}/${assistantPromptLimit}` : `最多 ${assistantPromptLimit} 字`
    : isAssistantInspectable() ? "当前仅可查看 Agent Team 进度" : `最多 ${assistantPromptLimit} 字`;
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

async function sendAssistantMessage(promptOverride = "") {
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
  session.messages.push({ id: uniqueId("msg"), role: "user", content: prompt, createdAt: now, status: "done" });
  const loadingID = uniqueId("msg");
  session.messages.push({ id: loadingID, role: "assistant", content: "正在通过 Legato Chat workflow 生成回答。", createdAt: now, status: "loading", retryPrompt: prompt });
  session.title = titleForPrompt(prompt);
  session.updatedAt = now;
  assistantInput.value = "";
  saveAssistantState();
  renderAssistant();

  try {
    const answer = await requestAssistantAnswer(session, prompt);
    const loading = session.messages.find((message) => message.id === loadingID);
    if (loading) {
      loading.content = answer || "模型没有返回可用内容。";
      loading.status = "done";
      loading.retryPrompt = "";
    }
  } catch (error) {
    const loading = session.messages.find((message) => message.id === loadingID);
    if (loading) {
      loading.content = assistantErrorMessage(error);
      loading.status = "error";
      loading.retryPrompt = prompt;
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
  session.messages = session.messages.filter((item) => item.id !== messageID);
  saveAssistantState();
  await sendAssistantMessage(message.retryPrompt);
}

async function requestAssistantAnswer(session, prompt) {
  assistantAbort = new AbortController();
  const timeout = window.setTimeout(() => assistantAbort.abort(), 70000);
  try {
    const payload = await fetchJSON("/api/chat", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        question: prompt,
        diagnosis: assistantContextForModel(),
        history: assistantHistoryForModel(session, prompt)
      }),
      signal: assistantAbort.signal
    });
    return String(payload.answer || payload.chat?.answer || "").trim();
  } finally {
    window.clearTimeout(timeout);
    assistantAbort = null;
  }
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

function assistantContextForModel() {
  if (!diagnosis) return { status: "no_diagnosis", message: "尚未生成诊断。用户可能仍在材料上传阶段。" };
  const profile = diagnosis.ability_profile || {};
  const info = profile.basic_info || {};
  const match = diagnosis.matching_result || {};
  const topJobs = Array.isArray(profile.top5_matching_jobs) ? profile.top5_matching_jobs.slice(0, 5) : [];
  const sixDimScores = Array.isArray(match.student_radar) && match.student_radar.length
    ? match.student_radar
    : Array.isArray(profile.radar_data) ? profile.radar_data : [];
  const radarItems = benchmarkedEvidenceItems(profile);
  const radarSeries = radarItems.length ? buildRadarSeries(profile, radarItems) : [];
  const gaps = Array.isArray(match.gap_details) ? match.gap_details.slice(0, 5) : [];
  const stages = Array.isArray(diagnosis.path_plan?.stages) ? diagnosis.path_plan.stages.slice(0, 3) : [];
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
      gaps
    },
    path_stages: stages.map((stage) => ({
      stage: stage.stage,
      goal: stage.goal,
      deliverable: stage.deliverable
    })),
    focused_evidence: assistantFocusedEvidence || null,
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
    .filter((message) => message.status === "done" && message.streamType !== "agent_team" && ["user", "assistant", "system"].includes(message.role) && message.content)
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

function showToast(message) {
  window.clearTimeout(toastTimer);
  toast.textContent = formatAgentDisplayText(message);
  toast.classList.add("is-visible");
  toastTimer = window.setTimeout(() => toast.classList.remove("is-visible"), 2600);
}

function cleanDisplayText(value) {
  return String(value ?? "").trim();
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
