# Backend

Go stdlib 服务，用于托管 `../frontend`、接收上传材料、创建异步诊断任务，并通过 SSE 把 agent 进度推送给前端。

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
- `GET /api/diagnosis/mock`
- `GET /api/export/ability-profile.json`
- `GET /api/export/ability-profile.xlsx`
- `GET /api/export/path-plan.doc`
- `GET /api/export/backend-requirements.json`

`POST /api/diagnosis` 当前会创建异步诊断任务并返回：

```json
{"job_id":"diag_1","status":"queued","events_url":"/api/diagnosis/diag_1/events"}
```

前端通过 `GET /api/diagnosis/{job_id}/events` 监听 SSE 事件。后端会并发调用 `../Agents/legato` 解析简历和已上传成绩单；简历 Legato 解析失败会直接触发 `job.failed`，成绩单解析失败会作为可选材料警告保留并继续生成诊断。`experience_hybrid` 返回后，前端把当前奖项和经历拼成 evidence items，并调用 `POST /api/diagnosis/{job_id}/benchmark` 补充 `impact_factor`、校内/校外分类与六维分布。前端雷达图由这些证据聚合为综合、校内、校外三层画像；岗位推荐、匹配差距和路径规划仍为模拟数据。`/api/diagnosis/mock` 仅作为兼容和调试接口保留，不参与正式诊断流程。
