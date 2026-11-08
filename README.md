# 人工智能学习平台

这是一个面向小学课堂场景的人工智能学习平台，包含学生答题端、教师管理端、Go 后端 API，以及已经整理好的 SQLite 数据。

当前仓库已经过一轮较大的演进，现状可以简单理解为：

- `frontend/` 是当前在用的前端静态页面
- `backend-go/` 是当前默认在用的后端
- `backend/` 是历史 Node 版本后端，仅作兼容参考，不再作为默认运行链路
- `backend-go/data/quiz-system.db` 是当前默认业务数据库

这份 README 主要服务于后续交接和日常运维，尤其适合 Claude Code / Claude Code CLI 在新机器上接手时快速建立上下文。

## 1. 当前运行方式

默认是前后端分开启动：

- 前端静态服务：`http://127.0.0.1:3000`
- 后端 API：`http://127.0.0.1:8080/api`

前端会根据访问页面的主机名动态推导 API 地址：

- 如果前端地址是 `http://127.0.0.1:3000`，则默认请求 `http://127.0.0.1:8080/api`
- 如果前端地址是 `http://10.8.3.4:3000`，则默认请求 `http://10.8.3.4:8080/api`

这一逻辑定义在 [frontend/js/config.js](/Users/fuhaotong/Documents/class_system/frontend/js/config.js:1)。

## 2. 环境要求

建议环境：

- `Go 1.22+`
- `Python 3`
- `Node.js 18+`

说明：

- 当前主链路并不依赖 Node 来运行后端，但根目录保留了 `package.json`，用于统一启动命令和保留历史 Node 依赖
- 前端静态资源默认通过 `python3 -m http.server` 提供
- 仓库已经自带 SQLite 数据库，通常不需要额外初始化

## 3. 快速启动

在仓库根目录执行。

### 启动前端

```bash
npm run frontend:start
```

等价命令：

```bash
python3 -m http.server 3000 --directory frontend
```

### 启动 Go 后端

```bash
npm start
```

等价命令：

```bash
cd backend-go
env GOPROXY=https://goproxy.cn,direct GOCACHE=$(pwd)/.cache/go-build go run ./cmd/server
```

### 构建 Go 后端

```bash
npm run backend:build
```

## 4. 访问地址

- 学生端：`http://127.0.0.1:3000/student.html`
- 教师端：`http://127.0.0.1:3000/teacher.html`
- 学生历史页：`http://127.0.0.1:3000/student-history.html`

常见说明：

- `student-history.html` 一般不直接手工访问，而是从学生端完成答题后点击按钮进入
- 学生端会根据班级绑定的课程和当前课堂阶段自动展示对应内容

## 5. 健康检查与联通检查

### 后端健康检查

```bash
curl -s http://127.0.0.1:8080/healthz
```

期望返回：

```json
{"success":true,"data":{"status":"ok"}}
```

### 常用接口检查

```bash
curl -s http://127.0.0.1:8080/api/teacher/courses
curl -s "http://127.0.0.1:8080/api/student/questions/2?courseId=1"
curl -I http://127.0.0.1:3000/student.html
curl -I http://127.0.0.1:3000/teacher.html
```

## 6. 仓库结构

```text
frontend/                    当前前端静态页面
  student.html               学生端入口
  teacher.html               教师端入口
  student-history.html       学生历史答题页
  js/
    config.js                动态 API 地址推导
    student.js               学生端主逻辑
    teacher.js               教师端主逻辑
    student-history.js       历史页逻辑
  css/
    style.css                统一样式

backend-go/                  当前默认后端（已分层）
  cmd/server/main.go         启动入口、路由注册
  internal/
    models/models.go         数据模型
    utils/utils.go           通用工具函数
    middleware/middleware.go CORS、日志中间件
    migrations/migrations.go 数据库初始化与迁移
    repository/repository.go 数据库 CRUD
    service/service.go       业务逻辑（统计、导出、Part2）
    handler/handler.go       HTTP handlers
  data/quiz-system.db        当前正式 SQLite 数据
  go.mod / go.sum            Go 依赖

backend/                     历史 Node 后端（默认不使用）
  server.js                  历史入口
  database.js                历史 DB 操作
  logger.js                  历史日志

package.json                 常用启动脚本
ecosystem.config.js          PM2 配置（已指向 Go 后端）
架构文档.md                  更详细的系统架构与维护说明
AGENTS.md                    之前协作约束，当前仅作历史参考
```

## 7. 数据库说明

当前默认数据库文件：

```text
backend-go/data/quiz-system.db
```

特点：

- 仓库已自带数据，可直接运行
- 启动 Go 后端时会自动执行轻量迁移
- 最近一轮迁移已包含：
  - `questions.enabled`
  - `questions.annotation_enabled`
  - 第二部分“理解程度”单选题数据

通常不要手动删除、重建或回滚这个数据库，除非明确知道影响。

## 8. 当前主要功能

- 教师端课程管理、阶段切换、题目管理、课堂提示语、统计查看、导出 Excel
- 学生端按班级/姓名进入课堂答题
- 班级和课程做了绑定，学生名单按班级共享，不按课程拆开
- 第二部分支持：
  - 单选题
  - 多选题
  - 填空题
  - 可配置的颜色标注要求
- 第二部分颜色标注当前规则：
  - 只标注文字背景色
  - 不修改文字颜色
  - 多行选中已修复为原地处理文本节点，避免异常换行
- 第三部分支持客观题判分和解析展示
- 第四部分支持实际得分与课堂反思采集
- 学生端支持历史课堂答题记录与原题/解析回看

## 9. 当前代码约定

- 当前维护主线是 `backend-go/`，不是 `backend/`
- 新需求优先改 Go 后端和前端静态页
- 如果要改 API 地址逻辑，优先看 [frontend/js/config.js](/Users/fuhaotong/Documents/class_system/frontend/js/config.js:1)
- 如果要改学生端行为，优先看 [frontend/js/student.js](/Users/fuhaotong/Documents/class_system/frontend/js/student.js:1)
- 如果要改教师端行为，优先看 [frontend/js/teacher.js](/Users/fuhaotong/Documents/class_system/frontend/js/teacher.js:1)
- 如果要改接口或参数校验，优先看 [backend-go/internal/handler/handler.go](/Users/fuhaotong/Documents/class_system/backend-go/internal/handler/handler.go:1)
- 如果要改数据库查询，优先看 [backend-go/internal/repository/repository.go](/Users/fuhaotong/Documents/class_system/backend-go/internal/repository/repository.go:1)
- 如果要改统计或导出逻辑，优先看 [backend-go/internal/service/service.go](/Users/fuhaotong/Documents/class_system/backend-go/internal/service/service.go:1)
- 如果要改数据模型，优先看 [backend-go/internal/models/models.go](/Users/fuhaotong/Documents/class_system/backend-go/internal/models/models.go:1)

## 10. 常见维护操作

### 端口被占用

```bash
lsof -nP -iTCP:3000 -sTCP:LISTEN
lsof -nP -iTCP:8080 -sTCP:LISTEN
kill <PID>
```

### 后端起不来

优先检查：

- `8080` 是否被占用
- `backend-go/data/quiz-system.db` 是否可读写
- Go 编译缓存目录是否可写：`backend-go/.cache/go-build`

### 前端能打开但接口报错

优先检查：

- 当前访问域名是否能推导出正确的后端 IP
- `frontend/js/config.js` 是否被改坏
- `http://<host>:8080/healthz` 是否可通

### 导出或统计异常

优先检查：

- 第二部分题型是否新增了字段但导出逻辑没同步
- [backend-go/internal/service/service.go](/Users/fuhaotong/Documents/class_system/backend-go/internal/service/service.go:1)（BuildExportRow、BuildStats）
- 统计逻辑是否和 `part2_answer` 的 JSON 结构兼容

## 11. PM2 部署说明

[ecosystem.config.js](/Users/fuhaotong/Documents/class_system/ecosystem.config.js:1) 已更新为启动 Go 后端编译后的二进制：

- `script: 'backend-go/server'`

使用 PM2 前需要先构建：

```bash
npm run backend:build
npm run pm2:start
```

构建产物 `backend-go/server` 是一个独立的可执行文件，不依赖本地 Go 环境即可运行。

## AI 主模型与备用厂商（新版课堂）

新版学生端的 AI 对话、AI 学习计划、AI 学习评价及追问，统一支持 DeepSeek / GLM 双厂商。默认主厂商为 DeepSeek，备用为 `glm-5.3-flash`。教师端「课堂管理 → AI 模型设置」可选择主厂商，设置对所有课堂生效、持久保存，新请求立即生效。仅密钥已配置的厂商可设为主模型；页面只展示配置状态，不返回密钥。

密钥沿用后端环境变量配置方式，不在网页或仓库中保存：

```bash
# 在运行 Go 后端的服务器上配置真实值
export DEEPSEEK_KEY='你的 DeepSeek API Key'
export GLM_KEY='你的智谱 API Key'
# 可选：DeepSeek 型号；GLM 固定使用 glm-5.3-flash
export DEEPSEEK_MODEL='deepseek-v4-flash'

npm run backend:build
pm2 restart ai-quiz-system --update-env
```

也兼容 `DEEPSEEK_API_KEY` / `GLM_API_KEY`，其中 `*_KEY` 优先。环境变量必须传入后端进程；只重启前端不会加载密钥。生产环境应通过已有的密钥注入/进程管理方式保存这些变量，确保服务器重启后仍可读取；不要将实际 Key 写进提交的配置文件。

GLM 使用智谱通用 API 端点 `https://open.bigmodel.cn/api/paas/v4/chat/completions`，需要该端点可用的 API Key，配置参考[官方快速开始](https://docs.bigmodel.cn/cn/guide/start/quick-start)。GLM 固定使用 `glm-5.3-flash`，不读取 `GLM_MODEL`，也不尝试其他 GLM 型号。按该型号要求启用思考，使用 `reasoning_effort=low`。

每次请求先尝试教师选择的主厂商；HTTP 错误、网络错误、空回答、流解析错误、中断或单厂商调用超过 20 秒时，尝试另一家已配置的厂商，每家最多一次。两次尝试共用原有 45 秒总时限。切换不会改变教师保存的主厂商，下次请求仍从主厂商开始。两家都不可用时返回明确提示；浏览器断开时不再调用备用厂商。

流中途切换会发送 `reset` 事件，学生端清除当前未完成的输出，然后继续显示备用回答；只有完整成功的回答才写入学习指导记录，并保存实际使用的模型名。上线时需同时更新后端、教师端及学生端静态文件。

## 学生方案设计

- 学生入口：`/design.html`；教师端「方案设计」页签可创建方案、配置有序开放题、绑定班级、查看两版答案及导出 Excel。
- 教师在「方案进度」选择班级后，点击「设为该班学生当前方案」。学生不能自行选择方案；未指定时等待教师指定，切换后约 3 秒自动更新。每个班级独立保存当前方案，历史提交记录保留。首次升级后需教师主动指定。
- 进度由教师在「方案设计 → 方案进度」选择方案及班级后控制：默认第一版，依次点击「开始第二版」「开启图片环节」。不同班级分别保存，只能向后开放；取消原 120 秒自动等待。学生每 3 秒检查进度，未完成前置提交的学生仍须补齐两版。后端同时校验进度，保存配置、解绑再绑定不会重置。
- 每道题必填；第二版至少一道答案须有变化（忽略空白）。两版独立保存，第二版提交后只能查看与复制。已有提交后锁定题目，解绑班级不删除历史记录。
- 后端启动时自动创建独立的 `design_*` 表，不改动课堂答题表；部署需构建、重启 Go 后端并更新前端文件。
- 新增接口组：`/api/v2/design-plans`（配置、记录、导出、学生状态、版本提交），班级绑定列表：`/api/v2/classes/{id}/design-plans?studentId=...`。学生身份沿用现有班级名单匹配方式。
- 浏览器验收：在项目根目录执行 `node --test frontend/tests/design.test.mjs`（需已安装 Playwright Chromium）。使用模拟接口，不修改课堂数据库或调用 AI；Go 测试使用内存数据库验证保存、锁定及真实 Excel 导出。
- 真实后端全流程验收：`node --test frontend/tests/design-real.test.mjs`。自动启动使用内存 SQLite 的 Go HTTP 服务，走教师真实登录和学生两版提交流程，检查真实 Excel 单元格及慢请求交互；不访问业务数据库。验收记录写入 `/private/tmp/design-acceptance-results.json`。

### 图片创作

- 教师开启图片环节后，两版已完成的学生直接进入图片区。自动填入第二版内容，首次生成前可逐题修改并勾选确认，不改动两版已提交方案；生图内容单独保存，可查看第一版对照。只有点击「生成图片」或「发送并生成」才请求模型。每次修改基于最新成功图片，可无限次修改，同一学生同一方案同时只执行一个任务。
- 从任意成功历史图片中选择最终作品，确认提交后锁定。图片、修改要求、生成结果、失败状态和最终选择均长期保存。教师可查看、下载单图，并按方案及班级导出包含所有成功图片及 `成果清单.xlsx`（完成状态、图片版本、对话记录）的 ZIP；原文字方案 Excel 保留。
- 学生页面只显示图片版本及操作结果，不提供对话历史；完整对话仅在教师详情和导出中展示。生成时显示独立的排队/生成等待卡片，失败后恢复编辑。
- 固定模型 `doubao-seedream-5-0-flash-260915`，默认 `1K` JPEG。Go 后端读取 `ARK_API_KEY`，兼容 `ARK_KEY`（前者优先）；这与 DeepSeek / GLM 的文本主模型设置独立。
- 部署时向 Go 后端进程注入 Key，构建后执行 `pm2 restart ai-quiz-system --update-env`。不要把实际密钥写入仓库。后端启动时自动创建新表和默认进度，不删除已有两版方案。
- 图片存于数据目录下的 `design-images/`，通常为 `backend-go/data/design-images/`（沿用现有数据目录解析方式）；数据库保存文件位置和元信息。**备份、迁移时必须同时保留数据库和图片目录**。生成文件不得提交 Git。
- 后台两路执行生成任务。排队任务持久化并可在重启后继续；执行中但结果不确定的任务标记中断，不自动重复调用付费接口。首次生成、修改和失败重试均使用独立请求标识防止重复点击扣费。
- 默认测试不访问真实模型。真实联调显式设置 `DESIGN_IMAGE_LIVE_TEST=1` 和 `ARK_API_KEY`，运行 `go test ./internal/v2 -run '^TestDesignImageLive$' -v -timeout 10m`，最多一次首图、一次修改；可用 `DESIGN_IMAGE_LIVE_OUTPUT_DIR` 指定测试图片目录。

## 12. 交接给 Claude Code 的建议提示词

可以直接给 Claude Code 一段这样的任务说明：

```text
请先阅读 README.md 和 架构文档.md，理解当前项目结构。
当前主链路是 frontend/ + backend-go/，不要默认使用 backend/ 旧 Node 后端。
Go 后端已拆分为 models / utils / middleware / migrations / repository / service / handler。
先检查 3000 和 8080 端口，再分别启动前端和 Go 后端。
运行后验证 /healthz、教师端课程接口、学生端第二部分题目接口。
如要改功能：
1. 前端行为看 frontend/js/student.js 或 frontend/js/teacher.js
2. HTTP 接口和参数校验看 backend-go/internal/handler/handler.go
3. 数据库查询看 backend-go/internal/repository/repository.go
4. 统计/导出/Part2 业务逻辑看 backend-go/internal/service/service.go
5. 数据模型看 backend-go/internal/models/models.go
6. 涉及数据库结构时，注意兼容已有 quiz-system.db
7. 不要随意重置数据库
```

## 13. 后续建议

- ~~把 `ecosystem.config.js` 更新到 Go 部署链路~~（已完成）
- ~~把 Go 后端拆分出 `handlers / service / repository`~~（已完成）
- 为第二部分富文本标注、统计导出、阶段切换补最小回归测试
- 为数据库迁移建立显式版本管理，而不是继续把迁移逻辑全部堆在启动流程里
