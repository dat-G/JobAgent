# Growth Lens

Growth Lens 是面向大学生求职准备和高校就业指导场景的学生成长诊断与岗位匹配平台。平台从简历、成绩单和补充材料出发，生成能力画像、岗位推荐、差距分析和成长路径规划，帮助学生理解当前证据与目标岗位能力要求之间的距离。

## 平台功能

- 材料上传与异步诊断：支持上传简历，成绩单和其他材料可选；后端通过 SSE 持续推送解析、评分、匹配和路径规划进度。
- 简历与成绩单结构化：Legato 负责将 PDF、图片或 Markdown-like 文本转为结构化 JSON，PDF 简历优先走文字层快速抽取，必要时回退到 MarkItDown 或可选 OCR。
- 能力画像：从教育背景、奖项证书、实习项目和校园经历中提取证据，形成逻辑、语言、专业、领导、抗压、成长六维能力画像。
- 证据评分与雷达分析：Item Benchmark 为每条证据生成六维分布和影响因子，Major Baseline 根据专业、学校层级和学业线索生成能力基线。
- 岗位自动推荐：系统无需用户预先选择目标岗位，会结合能力画像、经历相关性和学历门槛，自动生成首选岗位和 TOP 岗位队列。
- 匹配差距分析：展示学生当前能力雷达、目标岗位能力雷达、匹配度、主要短板、推荐理由和下一步补证据方向。
- 成长路径规划：根据首选岗位和差距明细生成阶段目标、周任务、资源链接、达标标准和交付物。
- 结果导出：支持能力画像 JSON/Excel、匹配结果 JSON/PDF、路径规划 Word/PDF 等导出方式。
- 对话式解释与调整：前端 Chat 可以基于当前诊断上下文回答问题，并在用户明确要求时更新部分生成结果。

## 系统组成

Growth Lens 由三个核心部分组成：

- `Agents/presto`：Go Agent 运行时，负责 session/run、模型调用、Agent workflow、SSE 事件和结构化输出校验，默认监听 `127.0.0.1:8080`。
- `backend`：Go 网关，托管 `frontend` 静态页面，接收上传材料，编排异步诊断任务并转发 SSE，默认监听 `127.0.0.1:8090`。
- `Agents/legato`：Python CLI，由 Go 网关按需调用，负责简历和成绩单结构化，不是常驻服务。

## 通用前置条件

推荐版本：

- Go 1.22+
- Python 3.11+
- 可访问配置中的大模型 API，或已准备可用的本地/内网兼容接口

在项目根目录创建 `.env` 文件，并配置 LLM API：

```text
DEEPSEEK_API_KEY=sk-your-deepseek-key
DEEPSEEK_ENDPOINT=https://api.deepseek.com
DEEPSEEK_MODEL=deepseek-v4-flash
```

## Linux 部署

以下示例以 Ubuntu/Debian 为例，仓库目录为 `/opt/JobAgent`。

安装基础依赖：

```sh
sudo apt-get update
sudo apt-get install -y git curl build-essential golang python3.11 python3.11-venv python3-pip
```

进入仓库并准备配置：

```sh
cd /opt/JobAgent
cp .env.example .env
vi .env
```

安装 Legato Python 环境：

```sh
cd Agents/legato
python scripts/install_dev.py
```

启动服务：

```sh
cd /opt/JobAgent
chmod +x ./up.sh
./up.sh
```

日常操作：

```sh
./up.sh status
./up.sh logs
./up.sh restart
./up.sh down
```

访问平台：

```text
http://127.0.0.1:8090
```

## macOS 部署

以下示例使用 Homebrew，仓库目录为 `/Users/sunnychen/Dev/JobAgent`。

安装基础依赖：

```sh
brew install go python@3.11 curl
```

安装 Legato Python 环境：

```sh
cd /Users/sunnychen/Dev/JobAgent/Agents/legato
python scripts/install_dev.py
```

启动服务：

```sh
cd /Users/sunnychen/Dev/JobAgent
chmod +x ./up.sh
./up.sh
```

日常操作：

```sh
./up.sh status
./up.sh logs
./up.sh restart
./up.sh down
```

访问平台：

```text
http://127.0.0.1:8090
```

## Windows 部署

Windows 原生 PowerShell 部署不推荐作为首选方式，但可以手动构建和启动。

以下示例面向 Windows 10/11 或 Windows Server 的原生 PowerShell，仓库目录为 `具体路径\JobAgent`。

确认工具可用：

```powershell
$env:Path = "D:\Program Files\Go\bin;$env:Path"
go version
py -3.11 --version
```

进入仓库并准备配置：

```powershell
$root = "具体路径\JobAgent"
cd $root
Copy-Item .env.example .env -ErrorAction SilentlyContinue
notepad .env
```

加载 `.env` 到当前 PowerShell 会话：

```powershell
Get-Content "$root\.env" | ForEach-Object {
  $line = $_.Trim()
  if ($line -and -not $line.StartsWith("#") -and $line -match "=") {
    $name, $value = $line -split "=", 2
    Set-Item -Path "env:$($name.Trim())" -Value $value.Trim()
  }
}
```

安装 Legato Python 环境：

```powershell
cd "$root\Agents\legato"
python scripts\install_dev.py
python -m legato.cli --help
```

准备构建目录和 Go 缓存：

```powershell
$env:GOCACHE = "$root\.run\go-build"
New-Item -ItemType Directory -Force `
  "$root\.run\bin", `
  "$root\.run\logs", `
  "$root\.run\pids", `
  $env:GOCACHE | Out-Null
```

构建 Presto 和 JobAgent 网关：

```powershell
cd "$root\Agents\presto"
go build -o "$root\.run\bin\presto.exe" .\cmd\presto

cd "$root\backend"
go build -o "$root\.run\bin\jobagent.exe" .
```

设置运行环境：

```powershell
$env:PRESTO_ADDR = "127.0.0.1:8080"
$env:PRESTO_URL = "http://127.0.0.1:8080"
$env:PRESTO_ROUTE = "legato.presto"
$env:PRESTO_ASYNC_RUN_TIMEOUT = "10m"

$env:JOBAGENT_ADDR = "127.0.0.1:8090"
$env:LEGATO_USE_PRESTO = "1"
$env:LEGATO_PRESTO_URL = $env:PRESTO_URL
$env:LEGATO_TIMEOUT_MS = "60000"
$env:DIAGNOSIS_TIMEOUT_SECONDS = "120"
$env:JOB_MATCHING_TIMEOUT_SECONDS = "600"
$env:ITEM_BENCHMARK_MAX_REQUESTS = "30"
$env:ITEM_BENCHMARK_BATCH_WORKERS = "30"

$env:LEGATO_PYTHON = "$root\Agents\legato\.venv\Scripts\python.exe"
$env:FRONTEND_DIR = "$root\frontend"
$env:MODEL_ROUTING_CONFIG = "$root\model-routing.json"

$env:PYTHONIOENCODING = "utf-8"
$env:PYTHONUTF8 = "1"
```

先启动 Presto：

```powershell
cd "$root\Agents\presto"
& "$root\.run\bin\presto.exe" --addr $env:PRESTO_ADDR
```

再开一个 PowerShell，重新加载 `.env` 和上面的运行环境后启动 Growth Lens：

```powershell
cd "$root\backend"
& "$root\.run\bin\jobagent.exe"
```

访问平台：

```powershell
Start-Process http://127.0.0.1:8090
```

查看健康状态：

```powershell
Invoke-WebRequest http://127.0.0.1:8080/healthz
Invoke-WebRequest http://127.0.0.1:8090/api/healthz
```
