# JobAgent Windows 部署指南

本文面向 Windows 10/11 或 Windows Server 上的原生 PowerShell 部署。macOS/Linux 可继续使用仓库根目录的 `up.sh`，Windows 原生环境建议按本文步骤启动，避免 Bash 路径、Go 构建缓存和 Python 控制台编码差异带来的问题。

## 组件说明

JobAgent 在 Windows 下仍然由三个部分组成：

- `Agents/presto`：Go 服务，负责 Agent session/run、模型调用和 SSE 事件，默认监听 `127.0.0.1:8080`。
- `backend`：Go 网关，托管 `frontend` 静态页面、接收上传、编排诊断任务，默认监听 `127.0.0.1:8090`。
- `Agents/legato`：Python CLI，由 Go 网关按需调用，不是常驻服务。

## 前置条件

推荐版本：

- PowerShell 5.1 或 PowerShell 7。
- Go 1.22+。示例假设安装在 `D:\Program Files\Go`。
- Python 3.11+。Legato 的 `pyproject.toml` 要求 `>=3.11`。
- 可访问配置中的大模型 API，或已准备可用的本地/内网兼容接口。

打开 PowerShell 后先确认工具可用：

```powershell
$env:Path = "D:\Program Files\Go\bin;$env:Path"
go version
python --version
```

如果希望永久加入 Go 到用户 PATH：

```powershell
$goBin = "D:\Program Files\Go\bin"
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$goBin*") {
  [Environment]::SetEnvironmentVariable("Path", "$goBin;$userPath", "User")
}
```

修改用户 PATH 后重新打开 PowerShell。

## 准备配置

进入仓库根目录：

```powershell
cd E:\Dev\JobAgent
```

创建并编辑 `.env`：

```powershell
Copy-Item .env.example .env -ErrorAction SilentlyContinue
notepad .env
```

至少需要确认以下模型配置可用：

```text
DEEPSEEK_API_KEY=sk-your-deepseek-key
DEEPSEEK_ENDPOINT=https://api.deepseek.com
DEEPSEEK_MODEL=deepseek-v4-flash
```

当前 `model-routing.json` 默认使用 `legato.presto` 路由，并从 `DEEPSEEK_*` 环境变量读取密钥、endpoint 和模型名。

PowerShell 不会自动读取 `.env`。每次部署会话可用下面的片段加载：

```powershell
$root = "E:\Dev\JobAgent"
Get-Content "$root\.env" | ForEach-Object {
  $line = $_.Trim()
  if ($line -and -not $line.StartsWith("#") -and $line -match "=") {
    $name, $value = $line -split "=", 2
    Set-Item -Path "env:$($name.Trim())" -Value $value.Trim()
  }
}
```

## 安装 Legato Python 环境

Windows 下建议使用独立虚拟环境：

```powershell
$root = "E:\Dev\JobAgent"
cd "$root\Agents\legato"
python -m venv .venv
.\.venv\Scripts\Activate.ps1
python -m pip install -U pip setuptools wheel
python -m pip install -e .
python -m pip install "markitdown[pdf]==0.1.6" pdfplumber pdfminer-six pypdfium2 pillow
python -m legato.cli --help
deactivate
```

说明：

- 仓库内 `Agents/legato/vendor/wheelhouse` 目前按平台准备，旧 wheelhouse 未必适用于 Windows。若要离线部署，需要提前准备 Windows/Python 版本匹配的 wheelhouse。
- 如果 PowerShell 禁止激活脚本，可只用完整解释器路径执行：`.\.venv\Scripts\python.exe -m legato.cli --help`。

## 构建目录和缓存

Windows 沙箱或受限用户目录可能无法写默认 Go build cache。建议把缓存放到仓库内：

```powershell
$root = "E:\Dev\JobAgent"
$env:Path = "D:\Program Files\Go\bin;$env:Path"
$env:GOCACHE = "$root\.run\go-build"

New-Item -ItemType Directory -Force `
  "$root\.run\bin", `
  "$root\.run\logs", `
  "$root\.run\pids", `
  $env:GOCACHE | Out-Null
```

## 构建 Presto

```powershell
$root = "E:\Dev\JobAgent"
$env:Path = "D:\Program Files\Go\bin;$env:Path"
$env:GOCACHE = "$root\.run\go-build"

cd "$root\Agents\presto"
go build -o "$root\.run\bin\presto.exe" .\cmd\presto
```

## 启动 Presto

先在当前 PowerShell 加载 `.env`，然后设置 Presto 环境：

```powershell
$root = "E:\Dev\JobAgent"
$env:PRESTO_ADDR = "127.0.0.1:8080"
$env:PRESTO_ROUTE = "legato.presto"
$env:PRESTO_ASYNC_RUN_TIMEOUT = "10m"
$env:MODEL_ROUTING_CONFIG = "$root\model-routing.json"
```

前台启动，适合调试：

```powershell
cd "$root\Agents\presto"
& "$root\.run\bin\presto.exe" --addr $env:PRESTO_ADDR
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
Invoke-WebRequest http://127.0.0.1:8080/healthz
```

## 构建 JobAgent 网关

```powershell
$root = "E:\Dev\JobAgent"
$env:Path = "D:\Program Files\Go\bin;$env:Path"
$env:GOCACHE = "$root\.run\go-build"

cd "$root\backend"
go build -o "$root\.run\bin\jobagent.exe" .
```

## 启动 JobAgent 网关

设置运行环境：

```powershell
$root = "E:\Dev\JobAgent"

$env:JOBAGENT_ADDR = "127.0.0.1:8090"
$env:PRESTO_URL = "http://127.0.0.1:8080"
$env:LEGATO_USE_PRESTO = "1"
$env:LEGATO_PRESTO_URL = $env:PRESTO_URL
$env:LEGATO_TIMEOUT_MS = "60000"
$env:DIAGNOSIS_TIMEOUT_SECONDS = "120"
$env:JOB_MATCHING_TIMEOUT_SECONDS = "600"
$env:ITEM_BENCHMARK_MAX_REQUESTS = "5"
$env:ITEM_BENCHMARK_BATCH_WORKERS = "2"

$env:LEGATO_PYTHON = "$root\Agents\legato\.venv\Scripts\python.exe"
$env:FRONTEND_DIR = "$root\frontend"
$env:MODEL_ROUTING_CONFIG = "$root\model-routing.json"

# Windows 下强制 Python 子进程使用 UTF-8，避免 GBK 导致 JSON 乱码或 UnicodeEncodeError。
$env:PYTHONIOENCODING = "utf-8"
$env:PYTHONUTF8 = "1"
```

前台启动：

```powershell
cd "$root\backend"
& "$root\.run\bin\jobagent.exe"
```

后台启动：

```powershell
$jobagent = Start-Process `
  -FilePath "$root\.run\bin\jobagent.exe" `
  -WorkingDirectory "$root\backend" `
  -RedirectStandardOutput "$root\.run\logs\jobagent.log" `
  -RedirectStandardError "$root\.run\logs\jobagent.err.log" `
  -WindowStyle Hidden `
  -PassThru

$jobagent.Id | Set-Content "$root\.run\pids\jobagent.pid"
Invoke-WebRequest http://127.0.0.1:8090/api/healthz
```

打开页面：

```powershell
Start-Process http://127.0.0.1:8090
```

## 日常操作

查看健康状态：

```powershell
Invoke-WebRequest http://127.0.0.1:8080/healthz
Invoke-WebRequest http://127.0.0.1:8090/api/healthz
```

查看日志：

```powershell
Get-Content E:\Dev\JobAgent\.run\logs\presto.log -Tail 80
Get-Content E:\Dev\JobAgent\.run\logs\presto.err.log -Tail 80
Get-Content E:\Dev\JobAgent\.run\logs\jobagent.log -Tail 80
Get-Content E:\Dev\JobAgent\.run\logs\jobagent.err.log -Tail 80
```

停止服务：

```powershell
$root = "E:\Dev\JobAgent"

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

## UTF-8 与乱码排障

Windows 的旧控制台代码页常见为 GBK。若 Python CLI 输出 JSON 时走 GBK，而 Go 后端按 UTF-8 解析，就会出现前端证据卡片乱码，或出现类似错误：

```text
UnicodeEncodeError: 'gbk' codec can't encode character ...
```

建议：

- 使用本文的 `PYTHONIOENCODING=utf-8` 和 `PYTHONUTF8=1`。
- 当前后端调用 Legato CLI 时也会主动注入这两个环境变量。
- 手工调试 Legato CLI 前可执行：

```powershell
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8
chcp 65001
$env:PYTHONIOENCODING = "utf-8"
$env:PYTHONUTF8 = "1"
```

## 常见问题

### `go` 无法识别

确认 Go 安装目录正确，并把 `D:\Program Files\Go\bin` 加到当前 PowerShell：

```powershell
$env:Path = "D:\Program Files\Go\bin;$env:Path"
go version
```

### Go build cache access denied

设置工作区内缓存：

```powershell
$env:GOCACHE = "E:\Dev\JobAgent\.run\go-build"
New-Item -ItemType Directory -Force $env:GOCACHE | Out-Null
```

### Legato CLI 不可用

确认 Python 版本和虚拟环境：

```powershell
E:\Dev\JobAgent\Agents\legato\.venv\Scripts\python.exe --version
E:\Dev\JobAgent\Agents\legato\.venv\Scripts\python.exe -m legato.cli --help
```

如果提示缺少 MarkItDown、pypdfium2 或 PDF 依赖，重新安装：

```powershell
cd E:\Dev\JobAgent\Agents\legato
.\.venv\Scripts\python.exe -m pip install -e .
.\.venv\Scripts\python.exe -m pip install "markitdown[pdf]==0.1.6" pdfplumber pdfminer-six pypdfium2 pillow
```

### Presto 返回模型调用失败

检查 `.env` 是否已加载到当前 PowerShell，并确认 `model-routing.json` 中的 `api_key_env`、`base_url_env`、`model_env` 与 `.env` 一致：

```powershell
$env:DEEPSEEK_API_KEY
$env:DEEPSEEK_ENDPOINT
$env:DEEPSEEK_MODEL
```

### 端口被占用

修改端口并保持 URL 对齐：

```powershell
$env:PRESTO_ADDR = "127.0.0.1:8081"
$env:PRESTO_URL = "http://127.0.0.1:8081"
$env:LEGATO_PRESTO_URL = $env:PRESTO_URL

$env:JOBAGENT_ADDR = "127.0.0.1:8091"
```

重新启动 Presto 和 JobAgent 后访问 `http://127.0.0.1:8091`。

### 浏览器可打开但诊断卡住

优先检查：

- `jobagent.err.log` 是否有 Legato CLI 或模型调用错误。
- `presto.err.log` 是否有 provider 返回错误或超时。
- `.env` 是否加载到了启动服务的同一个 PowerShell 会话。
- `LEGATO_PYTHON` 是否指向 `Agents\legato\.venv\Scripts\python.exe`。
