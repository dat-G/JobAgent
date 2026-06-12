# 部署文档

本文覆盖 Linux、Windows、macOS 三种部署环境。JobAgent 由三个部分组成：

- `Agents/presto`：Go 服务，负责 Agent session/run、模型调用和 SSE 事件，默认监听 `127.0.0.1:8080`。
- `backend`：Go 网关，托管 `frontend` 静态页面、接收上传、编排诊断任务，默认监听 `127.0.0.1:8090`。
- `Agents/legato`：Python CLI，由 Go 网关按需调用，不是常驻服务。

## 通用前置条件

推荐版本：

- Go 1.22+。
- Python 3.11+。
- 可访问配置中的大模型 API，或已准备可用的本地/内网兼容接口。

在项目根目录下创建 `.env`文件，并填入以下内容配置LLM API：

```text
DEEPSEEK_API_KEY=sk-your-deepseek-key
DEEPSEEK_ENDPOINT=https://api.deepseek.com
DEEPSEEK_MODEL=deepseek-v4-flash
```

## Linux 部署

以下示例以 Ubuntu/Debian 为例，仓库目录为 `/opt/JobAgent`。

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
# 在项目根目录下
cd Agents/legato
python scripts/install_dev.py
```

启动服务：

```sh
# 在项目根目录下
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

访问：

```text
http://127.0.0.1:8090
```

## macOS 部署

以下示例使用 Homebrew，仓库目录为 `/Users/sunnychen/Dev/JobAgent`。

```sh
brew install go python@3.11 curl
```

安装 Legato Python 环境：

```sh
# 在项目根目录下
cd Agents/legato
python scripts/install_dev.py
```

启动服务：

```sh
# 在项目根目录下
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

访问：

```text
http://127.0.0.1:8090
```

## Windows 部署（不推荐）

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

构建 Presto：

```powershell
cd "$root\Agents\presto"
go build -o "$root\.run\bin\presto.exe" .\cmd\presto
```

构建 JobAgent 网关：

```powershell
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

前台启动，适合调试。先开一个 PowerShell 启动 Presto：

```powershell
cd "$root\Agents\presto"
& "$root\.run\bin\presto.exe" --addr $env:PRESTO_ADDR
```

再开另一个 PowerShell，重新加载 `.env` 和上面的运行环境后启动 JobAgent：

```powershell
cd "$root\backend"
& "$root\.run\bin\jobagent.exe"
```

后台启动，适合本机部署：

```powershell
$presto = Start-Process `
  -FilePath "$root\.run\bin\presto.exe" `
  -ArgumentList "--addr", $env:PRESTO_ADDR `
  -WorkingDirectory "$root\Agents\presto" `
  -RedirectStandardOutput "$root\.run\logs\presto.log" `
  -RedirectStandardError "$root\.run\logs\presto.err.log" `
  -WindowStyle Hidden `
  -PassThru
$presto.Id | Set-Content "$root\.run\pids\presto.pid"

$jobagent = Start-Process `
  -FilePath "$root\.run\bin\jobagent.exe" `
  -WorkingDirectory "$root\backend" `
  -RedirectStandardOutput "$root\.run\logs\jobagent.log" `
  -RedirectStandardError "$root\.run\logs\jobagent.err.log" `
  -WindowStyle Hidden `
  -PassThru
$jobagent.Id | Set-Content "$root\.run\pids\jobagent.pid"
```

访问：

```powershell
Start-Process http://127.0.0.1:8090
```

查看健康状态：

```powershell
Invoke-WebRequest http://127.0.0.1:8080/healthz
Invoke-WebRequest http://127.0.0.1:8090/api/healthz
```

停止服务：

```powershell
foreach ($name in "jobagent", "presto") {
  $pidFile = "$root\.run\pids\$name.pid"
  if (Test-Path $pidFile) {
    $pidValue = Get-Content $pidFile -ErrorAction SilentlyContinue
    if ($pidValue) {
      Stop-Process -Id ([int]$pidValue) -ErrorAction SilentlyContinue
    }
    Remove-Item $pidFile -ErrorAction SilentlyContinue
  }
}
```
