# Backend

Go stdlib 服务，用于托管 `../frontend`、接收上传材料、创建异步诊断任务，并通过 SSE 把 Agent 进度推送给前端。

## 运行

推荐从仓库根目录使用统一启动脚本，监听地址和端口在 `.env` 中配置：

```sh
cd /Users/sunnychen/Dev/JobAgent
./up.sh
```

常用命令：

```sh
./up.sh status
./up.sh restart
./up.sh down
./up.sh logs
```

脚本会拉起 `Agents/presto` 和当前 Go 网关。Legato 当前由 Go 网关以 CLI 方式调用，不是独立常驻进程。

也可以只启动当前 Go 网关：

```sh
cd /Users/sunnychen/Dev/JobAgent/backend
go run .
```

默认监听 `127.0.0.1:8090`。可通过环境变量调整：

```sh
JOBAGENT_ADDR=127.0.0.1:8091 go run .
```

如果要代理现有 Presto 后端：

```sh
PRESTO_URL=http://127.0.0.1:8080 go run .
```

前端可以通过 `/api/presto/*` 访问 Presto，例如 `/api/presto/healthz`。

## API

- `GET /api/healthz`
- `POST /api/diagnosis`
- `GET /api/diagnosis/{job_id}`
- `GET /api/diagnosis/{job_id}/events`
- `POST /api/diagnosis/{job_id}/benchmark`
- `POST /api/diagnosis/{job_id}/matching`
- `GET /api/diagnosis/mock`
- `GET /api/export/ability-profile.json`
- `GET /api/export/ability-profile.xlsx`
- `GET /api/export/path-plan.doc`
- `GET /api/export/backend-requirements.json`

`POST /api/diagnosis` 当前会创建异步诊断任务并返回：

```json
{"job_id":"diag_1","status":"queued","events_url":"/api/diagnosis/diag_1/events"}
```

前端通过 `GET /api/diagnosis/{job_id}/events` 监听 SSE 事件。后端会并发调用 `../Agents/legato` 解析简历和已上传成绩单；简历 Legato 解析失败会直接触发 `job.failed`，成绩单解析失败会作为可选材料警告保留并继续生成诊断。简历经历使用整体 `experience_hybrid` stage，返回后会以 hybrid 结果整体替换经历列表，条目数量和条目内容均以 hybrid 为准。前端随后把当前奖项和经历拼成 evidence items，并调用 `POST /api/diagnosis/{job_id}/benchmark` 补充 `impact_factor`、校内/校外分类与六维分布。Item Benchmark 默认最多并发 5 个批次，超过 5 条证据时会合并为 5 个请求同时发出，数量可通过 `.env` 的 `ITEM_BENCHMARK_MAX_REQUESTS` 调整；任意批次返回即通过 SSE 更新对应证据卡片。Benchmark 完成后，Go 后端会使用独立的 `JOB_MATCHING_TIMEOUT_SECONDS` 超时启动 `Legato Job Matching Team`，先由 Adaptive Planner 根据简历复杂度、证据数量、学历门槛和岗位不确定性生成受限 Agent plan，再由后端校验并派生 3 到 6 个多视角 Presto Agent 并发分析，最后由固定 Synthesis Arbiter 综合全部结果并输出结构化岗位匹配。若只有 Job Matching 失败，前端可调用 `POST /api/diagnosis/{job_id}/matching` 基于当前画像重跑岗位匹配，不重复 Item Benchmark。每个 Presto run 的 `/runs/{run_id}/events` 会被后端读取并转发到诊断 SSE，前端 chat 中会以结构化状态卡显示 Planner、多视角 Agent 和综合 Agent 的流式进度；最终结构化结果仍渲染为推荐岗位、目标岗位雷达、匹配差距和简短说明。路径规划仍为模拟数据。`/api/diagnosis/mock` 仅作为兼容和调试接口保留，不参与正式诊断流程。
