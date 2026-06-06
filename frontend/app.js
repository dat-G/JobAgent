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

let diagnosis = null;
let resumeReady = false;
let toastTimer = 0;
let diagnosisEvents = null;
let firstResultRevealed = false;

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
      four_dim_scores: [],
      radar_data: [],
      evidence_summary: [],
      awards_status: "waiting",
      awards: [],
      experiences_status: "waiting",
      experiences: [],
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
  setupUploads();
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
    document.querySelectorAll("[data-step-link]").forEach((link) => {
      link.classList.toggle("is-active", link.dataset.stepLink === step);
    });
  }, { root: deck, threshold: [0.45, 0.65] });

  document.querySelectorAll("[data-step]").forEach((section) => observer.observe(section));
}

function setupUploads() {
  document.querySelector(".step-nav").addEventListener("click", (event) => {
    const link = event.target.closest("a[href]");
    if (!link) return;
    const target = link.getAttribute("href").replace("#", "");
    if (!resumeReady && target !== "upload") {
      event.preventDefault();
      showToast("请先上传简历，再查看后续诊断页面。");
      return;
    }
    if (moduleLocks[target] === false) {
      event.preventDefault();
      showToast("该模块还在生成中，完成后会自动解锁。");
    }
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

function updateFileState(input, stateId, dropId, fallback) {
  const state = document.querySelector(`#${stateId}`);
  const drop = document.querySelector(`#${dropId}`);
  const count = input.files.length;
  drop.classList.toggle("is-ready", count > 0);
  state.textContent = count === 0 ? fallback : count === 1 ? input.files[0].name : `${count} 个文件`;
}

function unlockDeck() {
  deck.classList.remove("is-locked");
  unlockHint.classList.add("is-unlocked");
  lockState.classList.add("is-unlocked");
  lockState.textContent = "已解锁";
  runButton.disabled = false;
  uploadMessage.textContent = "已上传简历，可以继续浏览或生成诊断。";
  document.querySelector("#runDetail").textContent = "材料已就绪，点击生成诊断后会显示实时进度。";
  showToast("简历已上传，页面已解锁。");
}

async function runDiagnosis() {
  runButton.disabled = true;
  runButton.textContent = "生成中";
  resetRunSteps();
  resetResultModules();

  const payload = new FormData(form);

  try {
    const response = await fetch("/api/diagnosis", { method: "POST", body: payload });
    if (!response.ok) throw new Error("diagnosis request failed");
    const job = await response.json();
    if (job.events_url) {
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
  document.querySelectorAll("[data-run-step]").forEach((item) => {
    item.classList.remove("is-done", "is-running", "is-failed");
  });
  document.querySelector("#runStatus").textContent = "生成中";
  document.querySelector("#runDetail").textContent = "正在创建诊断任务并启动 agent。";
  setRunProgress(0);
}

function setRunDone() {
  document.querySelector("#runStatus").textContent = "诊断已生成";
  document.querySelector("#runDetail").textContent = "所有模块已解锁，可以查看和导出结果。";
  setRunProgress(100);
  runButton.disabled = false;
  runButton.textContent = "重新生成";
}

function setRunFailed(message) {
  document.querySelector("#runStatus").textContent = "诊断失败";
  document.querySelector("#runDetail").textContent = message || "Legato 必需解析失败，请检查材料或后端服务。";
  runButton.disabled = false;
  runButton.textContent = "重新生成";
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
    setRunDone();
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
    renderRadar(data.ability_profile.radar_data || []);
    renderResumeEvidence(data.ability_profile);
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
  } else if (status === "failed") {
    item.classList.add("is-failed");
    item.classList.remove("is-running");
  } else if (status === "running" && !alreadyDone) {
    item.classList.add("is-running");
    item.classList.remove("is-failed");
  }
  const doneCount = agentSteps.filter((name) => document.querySelector(`[data-run-step="${name}"]`)?.classList.contains("is-done")).length;
  const runningCount = agentSteps.filter((name) => document.querySelector(`[data-run-step="${name}"]`)?.classList.contains("is-running")).length;
  const perceivedProgress = doneCount + runningCount * 0.35;
  setRunProgress(Math.round((perceivedProgress / agentSteps.length) * 100));
}

function resetResultModules() {
  diagnosis = null;
  firstResultRevealed = false;
  Object.keys(moduleLocks).forEach((module) => {
    moduleLocks[module] = false;
    const section = document.querySelector(`#${module}`);
    if (section) section.classList.add("is-module-locked");
  });
  document.querySelector("#basicInfo").innerHTML = "";
  document.querySelector("#basicInfoDataState").textContent = "等待结构化数据";
  document.querySelector("#basicInfoDataState").className = "status-pill is-warning";
  document.querySelector("#sourceList").innerHTML = "";
  document.querySelector("#resumeEvidenceState").textContent = "等待结构化数据";
  document.querySelector("#resumeEvidenceState").className = "status-pill is-warning";
  document.querySelector("#awardDataState").textContent = "等待奖项 agent";
  document.querySelector("#awardDataState").className = "status-pill is-warning";
  document.querySelector("#experienceDataState").textContent = "等待经历 agent";
  document.querySelector("#experienceDataState").className = "status-pill is-warning";
  document.querySelector("#awardList").innerHTML = "";
  document.querySelector("#experienceList").innerHTML = "";
  document.querySelector("#dimensionScores").innerHTML = "";
  document.querySelector("#radarChart").innerHTML = "";
  document.querySelector("#radarText").textContent = "";
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
  if (section) section.classList.remove("is-module-locked");
  if (module === "profile" && !firstResultRevealed) {
    firstResultRevealed = true;
    window.setTimeout(() => scrollToModule("profile"), 80);
  }
}

function scrollToModule(module) {
  const target = document.querySelector(`#${module}`);
  if (!target) return;
  const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
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
  renderRadar(data.ability_profile.radar_data);
  renderResumeEvidence(data.ability_profile);
  renderMatching(data.matching_result);
  renderPath(data.path_plan);
  renderTopJobs(data.ability_profile.top5_matching_jobs);
  renderRequirements(data.backend_requirements || []);
  renderLimitations(data.production_limitations || []);
}

function renderBasicInfo(data) {
  const info = data.ability_profile.basic_info;
  const basicInfo = document.querySelector("#basicInfo");
  const fields = [
    ["姓名", info.name],
    ["学校", info.school],
    ["专业", info.major],
    ["学历", info.degree],
    ["毕业年份", info.graduation_year],
    ["推荐首选岗位", info.target_role]
  ];
  basicInfo.innerHTML = fields.map(([label, value]) => `
    <div>
      <dt>${escapeHTML(label)}</dt>
      <dd>${escapeHTML(value || "未解析")}</dd>
    </div>
  `).join("");

  const parsedCount = fields.filter(([, value]) => String(value || "").trim()).length;
  const state = document.querySelector("#basicInfoDataState");
  if (parsedCount >= 4) {
    state.textContent = "Legato 已连接";
    state.className = "status-pill is-real";
  } else if (parsedCount > 0) {
    state.textContent = "部分字段待解析";
    state.className = "status-pill is-warning";
  } else {
    state.textContent = "等待结构化数据";
    state.className = "status-pill is-warning";
  }

  const sourceList = document.querySelector("#sourceList");
  sourceList.innerHTML = (data.input_sources || []).map((file) => {
    const size = file.size ? `${Math.round(file.size / 1024)} KB` : "已记录";
    const parseState = file.kind === "resume" || file.kind === "transcript" ? "Legato 解析" : "未解析";
    return `<span>${escapeHTML(file.kind)}：${escapeHTML(file.name)}，${size}，${parseState}</span>`;
  }).join("");
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

function renderRadar(items) {
  items = items || [];
  const svg = document.querySelector("#radarChart");
  if (items.length === 0) {
    svg.innerHTML = "";
    document.querySelector("#radarText").textContent = "等待能力画像 agent 返回雷达图数据。";
    return;
  }
  const center = { x: 180, y: 158 };
  const radius = 104;
  const levels = [0.25, 0.5, 0.75, 1];
  const points = items.map((item, index) => pointFor(index, items.length, radius * safeScore(item.score) / 100, center));
  const area = points.map((point) => `${point.x.toFixed(2)},${point.y.toFixed(2)}`).join(" ");
  const maxPolygon = (scale) => items.map((_, index) => {
    const point = pointFor(index, items.length, radius * scale, center);
    return `${point.x.toFixed(2)},${point.y.toFixed(2)}`;
  }).join(" ");

  svg.innerHTML = `
    <title id="radarChartTitle">学生能力雷达图</title>
    <desc id="radarChartDesc">${escapeHTML(items.map((item) => `${item.name}${item.score}分`).join("，"))}</desc>
    ${levels.map((level) => `<polygon class="radar-grid" points="${maxPolygon(level)}"></polygon>`).join("")}
    ${items.map((item, index) => {
      const outer = pointFor(index, items.length, radius, center);
      const label = pointFor(index, items.length, radius + 28, center);
      return `
        <line class="radar-axis" x1="${center.x}" y1="${center.y}" x2="${outer.x.toFixed(2)}" y2="${outer.y.toFixed(2)}"></line>
        <text class="radar-label" x="${label.x.toFixed(2)}" y="${label.y.toFixed(2)}" text-anchor="middle">${escapeHTML(item.name)}</text>
      `;
    }).join("")}
    <polygon class="radar-area" points="${area}"></polygon>
    ${points.map((point) => `<circle class="radar-dot" cx="${point.x.toFixed(2)}" cy="${point.y.toFixed(2)}" r="4"></circle>`).join("")}
  `;

  const top = [...items].sort((a, b) => b.score - a.score)[0];
  const bottom = [...items].sort((a, b) => a.score - b.score)[0];
  document.querySelector("#radarText").textContent = `模拟数据：最高维度为${top.name}${top.score}分，当前优先补强${bottom.name}${bottom.score}分。`;
}

function renderResumeEvidence(profile) {
  profile = profile || {};
  const awards = profile.awards || [];
  const experiences = profile.experiences || [];
  const awardStatus = profile.awards_status || (awards.length ? "ready" : "waiting");
  const experienceStatus = profile.experiences_status || (experiences.length ? "ready" : "waiting");

  setEvidencePill("#awardDataState", awardStatus, {
    waiting: "等待奖项 agent",
    ready: "Legato 已返回",
    empty: "未识别到奖项",
    failed: "奖项解析失败",
    mock: "模拟数据"
  });
  setEvidencePill("#experienceDataState", experienceStatus, {
    waiting: "等待经历 agent",
    ready: "Legato 已返回",
    empty: "未识别到经历",
    failed: "经历解析失败",
    mock: "模拟数据"
  });

  const overallStatus = evidenceOverallStatus(awardStatus, experienceStatus, awards.length, experiences.length);
  setEvidencePill("#resumeEvidenceState", overallStatus, {
    waiting: "等待结构化数据",
    ready: "已有结构化证据",
    empty: "无可用证据",
    failed: "部分解析失败",
    mock: "模拟数据"
  });

  const awardList = document.querySelector("#awardList");
  awardList.innerHTML = awards.length
    ? awards.map((item) => renderAwardItem(item)).join("")
    : renderEvidenceEmpty(awardStatus, "等待 Resume workflow 返回奖项与证书。", "Legato 未识别到奖项或证书。", "奖项 agent 未返回可用结果。");

  const experienceList = document.querySelector("#experienceList");
  experienceList.innerHTML = experiences.length
    ? experiences.map((item) => renderExperienceItem(item)).join("")
    : renderEvidenceEmpty(experienceStatus, "等待 Resume workflow 返回经历评分。", "Legato 未识别到项目、实习或活动经历。", "经历 agent 未返回可用结果。");
}

function renderAwardItem(item) {
  const score = safeScore(item.score);
  return `
    <article class="evidence-item">
      <header>
        <div>
          <strong>${escapeHTML(item.name || "未命名奖项")}</strong>
          <span>${escapeHTML(item.result || "结果未解析")}</span>
        </div>
        <div class="evidence-score">
          <b>${score}</b>
          <small>/100</small>
        </div>
      </header>
      <div class="evidence-meter" style="--score:${score}%"><span></span></div>
      <p><strong>${escapeHTML(item.level || "未评级")}：</strong>${escapeHTML(item.reason || "暂无评分说明。")}</p>
      <div class="evidence-meta">
        <span class="${sourceBadgeClass(item.data_source)}">${escapeHTML(item.data_source || "未知来源")}</span>
        <span class="sim-badge">${escapeHTML(item.score_source || "模拟评分")}</span>
      </div>
    </article>
  `;
}

function renderExperienceItem(item) {
  const score = safeScore(item.score);
  const type = item.type || "经历";
  const role = item.role || "角色未解析";
  return `
    <article class="evidence-item">
      <header>
        <div>
          <strong>${escapeHTML(type)} · ${escapeHTML(role)}</strong>
          <span>Legato level ${Number(item.level || 0)}/10</span>
        </div>
        <div class="evidence-score">
          <b>${score}</b>
          <small>/100</small>
        </div>
      </header>
      <div class="evidence-meter" style="--score:${score}%"><span></span></div>
      <p><strong>${escapeHTML(item.signal || "未评级")}：</strong>${escapeHTML(item.contribution || "贡献描述未解析。")}</p>
      <p>${escapeHTML(item.reason || "暂无评分说明。")}</p>
      <div class="evidence-meta">
        <span class="${sourceBadgeClass(item.data_source)}">${escapeHTML(item.data_source || "未知来源")}</span>
        <span class="sim-badge">${escapeHTML(item.score_source || "模拟评分")}</span>
      </div>
    </article>
  `;
}

function renderEvidenceEmpty(status, waitingText, emptyText, failedText) {
  const text = status === "empty"
    ? emptyText
    : status === "failed"
      ? failedText
      : waitingText;
  return `<div class="evidence-empty">${escapeHTML(text)}</div>`;
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
  element.textContent = labels[normalized] || labels.waiting;
  element.className = `status-pill ${statusPillClass(normalized)}`;
}

function statusPillClass(status) {
  if (status === "ready") return "is-real";
  if (status === "mock") return "is-simulated";
  if (status === "failed") return "is-danger";
  return "is-warning";
}

function sourceBadgeClass(source) {
  return String(source || "").includes("模拟") ? "sim-badge" : "status-pill is-real";
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
        ${stage.resources.map((resource) => `<a href="${escapeAttribute(resource.url)}" target="_blank" rel="noreferrer">${escapeHTML(resource.label)}</a>`).join("")}
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
