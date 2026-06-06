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

const benchmarkDimensions = ["逻辑", "语言", "专业", "领导", "抗压", "成长"];
const radarEvidenceGain = 1.0;
const radarEvidenceDiminishThreshold = 0.65;
const radarEvidenceTailThreshold = 0.7;
const radarEvidenceTailGain = 0.04;
const radarEvidenceSoftCap = 0.88;
const cappedEvidenceBucketRules = {
  lowImpactAwardCertificate: { singleCap: 0.035, totalCap: 0.08 },
  campusAward: { singleCap: 0.045, totalCap: 0.1 },
  genericCampusRole: { singleCap: 0.045, totalCap: 0.1 },
  untitledProject: { singleCap: 0.06, totalCap: 0.15 }
};
const academicPriorWeight = 0.28;
const academicPriorFloorRatio = 0.85;

const agentSteps = ["resume_agent", "transcript_agent", "profile", "matching", "path", "outputs"];
const moduleLocks = {
  profile: false,
  matching: false,
  path: false,
  outputs: false
};

const runStepDetails = {
  resume_agent: "简历解析 agent 正在读取材料并抽取基础信息。",
  transcript_agent: "成绩单解析 agent 正在整理课程和成绩证据。",
  profile: "画像 agent 正在合并简历与成绩单证据。",
  matching: "岗位匹配 agent 正在计算推荐岗位和匹配度。",
  path: "路径规划 agent 正在生成阶段目标和周任务。",
  outputs: "导出 agent 正在整理结构化输出。"
};

function createDiagnosisShell() {
  return {
    generated_at: "",
    mode: "legato-required",
    input_sources: [],
    ability_profile: {
      basic_info: {},
      education: [],
      four_dim_scores: [],
      radar_data: [],
      evidence_summary: [],
      awards_status: "waiting",
      awards: [],
      experiences_status: "waiting",
      experiences: [],
      benchmark_status: "waiting",
      top5_matching_jobs: []
    },
    path_plan: { export_formats: [], stages: [] },
    matching_result: { report_sections: [], gap_details: [], recommendations: [], recommended_reasons: [] },
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

  const payload = new FormData(form);

  try {
    const response = await fetch("/api/diagnosis", { method: "POST", body: payload });
    if (!response.ok) throw new Error("diagnosis request failed");
    const job = await response.json();
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
    showToast("Legato 诊断服务不可用，未生成诊断。");
  }
}

function resetRunSteps() {
  if (diagnosisEvents) {
    diagnosisEvents.close();
    diagnosisEvents = null;
  }
  baseJobDone = false;
  failedRunStep = "";
  document.querySelectorAll("[data-run-step]").forEach((item) => {
    item.classList.remove("is-done", "is-running", "is-failed");
    setRunStepRetryable(item.dataset.runStep, false);
  });
  document.querySelector("#runStatus").textContent = "生成中";
  document.querySelector("#runDetail").textContent = "正在创建诊断任务并启动 agent。";
  setRunProgress(0);
}

function setRunDone() {
  document.querySelector("#runStatus").textContent = "诊断已生成";
  document.querySelector("#runDetail").textContent = "诊断完成，可以查看和导出结果。";
  document.querySelector(".generation-dock").classList.remove("is-running");
  setRunProgress(100);
  runButton.disabled = false;
  runButton.textContent = "重新生成";
}

function setRunFailed(message) {
  document.querySelector("#runStatus").textContent = "诊断失败";
  document.querySelector("#runDetail").textContent = message || "Legato 必需解析失败，请检查材料或后端服务。";
  document.querySelector(".generation-dock").classList.remove("is-running");
  runButton.disabled = false;
  runButton.textContent = "重新生成";
}

function setRunWaitingForBenchmark() {
  document.querySelector("#runStatus").textContent = "生成中";
  document.querySelector("#runDetail").textContent = baseJobDone
    ? "基础流程已完成，等待 Item Benchmark 返回六维分布。"
    : "Item Benchmark 正在评估 Impact 和六维分布。";
  document.querySelector(".generation-dock").classList.add("is-running");
  runButton.disabled = true;
  runButton.textContent = "生成中";
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
    document.querySelector("#runDetail").textContent = payload.message || "异步诊断已开始。";
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
    if (benchmarkRequestInFlight || diagnosis?.ability_profile?.benchmark_status === "benchmarking") {
      setRunWaitingForBenchmark();
    } else if (diagnosis?.ability_profile?.benchmark_status === "failed") {
      setBenchmarkRunFailed();
    } else {
      setRunDone();
    }
    if (diagnosisEvents) {
      diagnosisEvents.close();
      diagnosisEvents = null;
    }
  });
  diagnosisEvents.addEventListener("job.failed", (event) => {
    const payload = JSON.parse(event.data);
    const errors = Array.isArray(payload.data?.errors) ? payload.data.errors : [];
    const detail = errors.filter(Boolean).join("；");
    const message = detail ? `${payload.message || "诊断失败"}：${detail}` : payload.message;
    setRunFailed(message);
    showToast(message || "简历解析失败，请检查材料或后端服务。");
    if (diagnosisEvents) {
      diagnosisEvents.close();
      diagnosisEvents = null;
    }
  });
  diagnosisEvents.onerror = () => {
    if (diagnosisEvents) {
      diagnosisEvents.close();
      diagnosisEvents = null;
    }
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
  document.querySelector("#runDetail").textContent = event.message || runStepDetails[event.step] || "正在生成诊断结果。";

  const data = event.data || {};
  if (data.ability_profile) {
    diagnosis = diagnosis || createDiagnosisShell();
    diagnosis.ability_profile = data.ability_profile;
    renderBasicInfo(diagnosis);
    renderScores(data.ability_profile.four_dim_scores || []);
    renderAbilityRadar(data.ability_profile);
    renderResumeEvidence(data.ability_profile);
    maybeRequestItemBenchmark(data.ability_profile);
    if (event.step === "profile" && event.status === "failed" && data.ability_profile.benchmark_status === "failed") {
      setBenchmarkRunFailed(data.error || event.message);
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
  document.querySelector("#dimensionScores").innerHTML = "";
  renderAbilityRadar(createDiagnosisShell().ability_profile);
  document.querySelector("#overallMatch").textContent = "--";
  document.querySelector("#matchLevel").textContent = "等待生成";
  document.querySelector("#overallMeter").style.width = "0%";
  document.querySelector("#reportRows").innerHTML = "";
  document.querySelector("#gapTable").innerHTML = "";
  document.querySelector("#pathStages").innerHTML = "";
  document.querySelector("#topJobs").innerHTML = "";
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
  renderScores(data.ability_profile.four_dim_scores);
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

function renderScores(scores) {
  scores = scores || [];
  const container = document.querySelector("#dimensionScores");
  container.innerHTML = scores.map((item) => `
    <article class="score-item">
      <div class="score-top">
        <span>${escapeHTML(item.name)}</span>
        <span class="sim-badge">模拟数据</span>
        <strong>${item.score}</strong>
      </div>
      <div class="score-bar" aria-hidden="true"><span style="width:${safeScore(item.score)}%"></span></div>
      <p>${escapeHTML(item.level)}：${escapeHTML(item.reason)}</p>
    </article>
  `).join("");
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
    renderRadarFailed(svg);
    text.textContent = "Item Benchmark 失败，六维分布未生成。点击下方 dock 中红色“画像”可从失败处继续。";
    return;
  }
  if (isLoading || evidenceItems.length === 0) {
    renderRadarLoading(svg);
    text.textContent = "等待 Item Benchmark 返回 Level、Impact 和六维分布后生成能力画像。";
    return;
  }
  const items = benchmarkDimensions.map((name) => ({ name }));
  const center = { x: 180, y: 158 };
  const radius = 104;
  const levels = [0.25, 0.5, 0.75, 1];
  const academicBaseline = academicBaselineVector(profile);
  const series = buildRadarSeries(profile, evidenceItems);
  const maxPolygon = (scale) => items.map((_, index) => {
    const point = pointFor(index, items.length, radius * scale, center);
    return `${point.x.toFixed(2)},${point.y.toFixed(2)}`;
  }).join(" ");

  svg.innerHTML = `
    <title id="radarChartTitle">六维能力雷达图</title>
    <desc id="radarChartDesc">${escapeHTML(series.map((entry) => `${entry.label}${entry.scores.map((score, index) => `${benchmarkDimensions[index]}${score}分`).join("，")}`).join("；"))}</desc>
    ${levels.map((level) => `<polygon class="radar-grid" points="${maxPolygon(level)}"></polygon>`).join("")}
    ${items.map((item, index) => {
      const outer = pointFor(index, items.length, radius, center);
      const label = pointFor(index, items.length, radius + 28, center);
      return `
        <line class="radar-axis" x1="${center.x}" y1="${center.y}" x2="${outer.x.toFixed(2)}" y2="${outer.y.toFixed(2)}"></line>
        <text class="radar-label" x="${label.x.toFixed(2)}" y="${label.y.toFixed(2)}" text-anchor="middle">${escapeHTML(item.name)}</text>
      `;
    }).join("")}
    ${series.map((entry, seriesIndex) => {
      const points = entry.scores.map((score, index) => pointFor(index, entry.scores.length, radius * safeScore(score) / 100, center));
      const area = points.map((point) => `${point.x.toFixed(2)},${point.y.toFixed(2)}`).join(" ");
      return `
        <polygon class="radar-area radar-area-${entry.key}" style="--radar-delay:${seriesIndex * 80}ms" points="${area}"></polygon>
        ${points.map((point) => `<circle class="radar-dot radar-dot-${entry.key}" cx="${point.x.toFixed(2)}" cy="${point.y.toFixed(2)}" r="${entry.key === "overall" ? 4 : 3}"></circle>`).join("")}
      `;
    }).join("")}
  `;

  const legend = document.querySelector("#radarLegend");
  if (legend) {
    legend.innerHTML = series.map((entry) => `
      <span class="radar-legend-item radar-legend-${entry.key}" tabindex="0" data-series="${entry.key}">
        <i></i>${escapeHTML(entry.label)}<b>${entry.count}</b>
      </span>
    `).join("");
  }
  const overall = series.find((entry) => entry.key === "overall") || series[0];
  const topIndex = overall.scores.reduce((best, score, index) => score > overall.scores[best] ? index : best, 0);
  const campusCount = series.find((entry) => entry.key === "campus")?.count || 0;
  const externalCount = series.find((entry) => entry.key === "external")?.count || 0;
  const baselineLabel = academicBaseline.major_family || "学业";
  text.textContent = `综合画像最高维度为${benchmarkDimensions[topIndex]}${overall.scores[topIndex]}分；校内含${baselineLabel}基础${academicBaseline.base}分，当前纳入校内证据 ${campusCount} 条、校外证据 ${externalCount} 条。`;
}

function renderRadarLoading(svg) {
  const center = { x: 180, y: 158 };
  const radius = 104;
  const items = benchmarkDimensions.map((name) => ({ name }));
  const maxPolygon = (scale) => items.map((_, index) => {
    const point = pointFor(index, items.length, radius * scale, center);
    return `${point.x.toFixed(2)},${point.y.toFixed(2)}`;
  }).join(" ");
  svg.innerHTML = `
    <title id="radarChartTitle">六维能力雷达图加载中</title>
    <desc id="radarChartDesc">Item Benchmark 正在返回六维分布。</desc>
    ${[0.25, 0.5, 0.75, 1].map((level) => `<polygon class="radar-grid" points="${maxPolygon(level)}"></polygon>`).join("")}
    ${items.map((item, index) => {
      const outer = pointFor(index, items.length, radius, center);
      const label = pointFor(index, items.length, radius + 28, center);
      return `
        <line class="radar-axis" x1="${center.x}" y1="${center.y}" x2="${outer.x.toFixed(2)}" y2="${outer.y.toFixed(2)}"></line>
        <text class="radar-label" x="${label.x.toFixed(2)}" y="${label.y.toFixed(2)}" text-anchor="middle">${escapeHTML(item.name)}</text>
      `;
    }).join("")}
    <polygon class="radar-loading-area" points="${maxPolygon(0.42)}"></polygon>
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
  const maxPolygon = (scale) => items.map((_, index) => {
    const point = pointFor(index, items.length, radius * scale, center);
    return `${point.x.toFixed(2)},${point.y.toFixed(2)}`;
  }).join(" ");
  svg.innerHTML = `
    <title id="radarChartTitle">六维能力雷达图生成失败</title>
    <desc id="radarChartDesc">Item Benchmark 未返回六维分布。</desc>
    ${[0.25, 0.5, 0.75, 1].map((level) => `<polygon class="radar-grid" points="${maxPolygon(level)}"></polygon>`).join("")}
    ${items.map((item, index) => {
      const outer = pointFor(index, items.length, radius, center);
      const label = pointFor(index, items.length, radius + 28, center);
      return `
        <line class="radar-axis" x1="${center.x}" y1="${center.y}" x2="${outer.x.toFixed(2)}" y2="${outer.y.toFixed(2)}"></line>
        <text class="radar-label" x="${label.x.toFixed(2)}" y="${label.y.toFixed(2)}" text-anchor="middle">${escapeHTML(item.name)}</text>
      `;
    }).join("")}
    <polygon class="radar-failed-area" points="${maxPolygon(0.38)}"></polygon>
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
  return [
    buildRadarSeriesEntry("overall", "综合", items, academicBaseline.scores),
    buildRadarSeriesEntry("campus", "校内", campusItems, academicBaseline.scores),
    buildRadarSeriesEntry("external", "校外", externalItems)
  ];
}

function buildRadarSeriesEntry(key, label, items, baselineScores = []) {
  const dimensionContributions = Array.from({ length: 6 }, emptyDimensionContributionBucket);
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
    const baseline = Number(baselineScores[index]);
    return combineEvidenceWithAcademicPrior(evidenceScore, baseline, items.length);
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
  return "";
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
  return { base, scores, major_family: isStem ? "工科类" : "未知", source: "frontend_fallback" };
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
      renderScores(payload.ability_profile.four_dim_scores || []);
      renderAbilityRadar(payload.ability_profile);
      renderResumeEvidence(payload.ability_profile);
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
    });
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
    waiting: "等待奖项 agent",
    loading: "奖项解析中",
    refining: "奖项解析中",
    ready: "",
    empty: "未识别到奖项",
    failed: "奖项解析失败",
    mock: "模拟数据"
  });
  setEvidencePill("#experienceDataState", experienceStatus, {
    waiting: "等待经历 agent",
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
    ? awards.map((item) => renderAwardItem(item, isAwardLoading, isBenchmarkLoading, isBenchmarkFailed)).join("")
    : awardSkeleton || renderEvidenceEmpty(awardStatus, "等待 Resume workflow 返回奖项与证书。", "Legato 未识别到奖项或证书。", "奖项 agent 未返回可用结果。");

  const experienceList = document.querySelector("#experienceList");
  experienceList.classList.toggle("is-refining", isExperienceRefining);
  const hybridSkeleton = isExperienceRefining && experiences.length === 0 ? renderExperienceHybridSkeleton(false) : "";
  experienceList.innerHTML = experiences.length
    ? experiences.map((item) => renderExperienceItem(item, isExperienceRefining, isBenchmarkLoading, isBenchmarkFailed)).join("")
    : hybridSkeleton || renderEvidenceEmpty(experienceStatus, "等待 Resume workflow 返回经历评分。", "Legato 未识别到项目、实习或活动经历。", "经历 agent 未返回可用结果。");
}

function renderAwardItem(item, loading = false, benchmarkLoading = false, benchmarkFailed = false) {
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
    </article>
  `;
}

function renderExperienceItem(item, refining = false, benchmarkLoading = false, benchmarkFailed = false) {
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
    </article>
  `;
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
  document.querySelector("#overallMatch").textContent = `${match.overall_match}%`;
  document.querySelector("#matchLevel").textContent = match.match_level;
  document.querySelector("#overallMeter").style.width = `${safeScore(match.overall_match)}%`;

  document.querySelector("#reportRows").innerHTML = match.report_sections.map((row) => `
    <div class="report-row">
      <strong>${escapeHTML(row.name)}</strong>
      <div class="report-bar" aria-label="学生分值 ${row.student}"><span style="width:${safeScore(row.student)}%"></span></div>
      <div class="need-bar" aria-label="岗位要求 ${row.role_need}"><span style="width:${safeScore(row.role_need)}%"></span></div>
      <span class="delta">${row.difference > 0 ? "+" : ""}${row.difference}</span>
    </div>
  `).join("");

  document.querySelector("#gapTable").innerHTML = match.gap_details.map((gap) => `
    <tr>
      <td>${escapeHTML(gap.capability)}</td>
      <td>${escapeHTML(gap.current)}</td>
      <td>${escapeHTML(gap.expected)}</td>
      <td>${escapeHTML(gap.action)}</td>
      <td><span class="severity ${gap.severity === "高" ? "high" : ""}">${escapeHTML(gap.severity)}</span></td>
    </tr>
  `).join("");
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
  document.querySelector("#topJobs").innerHTML = jobs.map((job) => `
    <article class="job-item">
      <div class="job-head">
        <strong>${job.rank}. ${escapeHTML(job.title)}</strong>
        <span class="sim-badge">模拟数据</span>
        <span class="match">${job.match}%</span>
      </div>
      <div class="job-match-bar" aria-label="${escapeHTML(job.title)}能力匹配度 ${job.match}%">
        <span style="width:${safeScore(job.match)}%"></span>
      </div>
      <p>${escapeHTML(job.reasons.join("；"))}</p>
      <p>${escapeHTML(job.next_proof)}</p>
    </article>
  `).join("");
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
  toast.textContent = message;
  toast.classList.add("is-visible");
  toastTimer = window.setTimeout(() => toast.classList.remove("is-visible"), 2600);
}

function cleanDisplayText(value) {
  return String(value ?? "").trim();
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
