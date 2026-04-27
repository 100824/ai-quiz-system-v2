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

backend-go/                  当前默认后端
  cmd/server/main.go         Go 后端唯一主入口
  data/quiz-system.db        当前正式 SQLite 数据
  go.mod / go.sum            Go 依赖

backend/                     历史 Node 后端（默认不使用）
  server.js                  历史入口
  database.js                历史 DB 操作
  logger.js                  历史日志

package.json                 常用启动脚本
ecosystem.config.js          历史 PM2 配置，当前仍指向旧 Node 后端
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
- 如果要改接口或数据结构，优先看 [backend-go/cmd/server/main.go](/Users/fuhaotong/Documents/class_system/backend-go/cmd/server/main.go:1)

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
- [backend-go/cmd/server/main.go](/Users/fuhaotong/Documents/class_system/backend-go/cmd/server/main.go:896) 附近的导出逻辑
- 统计逻辑是否和 `part2_answer` 的 JSON 结构兼容

## 11. PM2 与旧后端说明

仓库里还有一个 [ecosystem.config.js](/Users/fuhaotong/Documents/class_system/ecosystem.config.js:1)，但它当前指向的是旧的 Node 后端：

- `script: 'backend/server.js'`

这意味着：

- 如果直接跑 `npm run pm2:start`，默认拉起的不是 Go 后端
- 如果后续要长期部署当前版本，建议把 PM2 配置改为启动 Go 可执行文件或 Go 启动命令

## 12. 交接给 Claude Code 的建议提示词

可以直接给 Claude Code 一段这样的任务说明：

```text
请先阅读 README.md 和 架构文档.md，理解当前项目结构。
当前主链路是 frontend/ + backend-go/，不要默认使用 backend/ 旧 Node 后端。
先检查 3000 和 8080 端口，再分别启动前端和 Go 后端。
运行后验证 /healthz、教师端课程接口、学生端第二部分题目接口。
如要改功能：
1. 前端行为看 frontend/js/student.js 或 frontend/js/teacher.js
2. API 和数据结构看 backend-go/cmd/server/main.go
3. 涉及数据库结构时，注意兼容已有 quiz-system.db
4. 不要随意重置数据库
```

## 13. 后续建议

- 把 `ecosystem.config.js` 更新到 Go 部署链路
- 把 Go 后端拆分出 `handlers / service / repository`，降低 `main.go` 复杂度
- 为第二部分富文本标注、统计导出、阶段切换补最小回归测试
- 为数据库迁移建立显式版本管理，而不是继续把迁移逻辑全部堆在启动流程里
