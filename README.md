# 人工智能学习平台

这是一个面向课堂教学场景的人工智能学习平台，包含：

- 学生端答题页面
- 教师端管理后台
- Go 重构后的后端 API
- 已整理好的 SQLite 业务数据

当前默认运行方式是：

- 前端静态页面运行在 `http://127.0.0.1:3000`
- 后端 API 运行在 `http://127.0.0.1:8080/api`

## 目录结构

```text
frontend/                前端静态页面
backend-go/              Go 后端
backend-go/data/         SQLite 数据文件
backend-go/cmd/server/   后端入口
```

## 环境要求

建议环境：

- Node.js 18+
- Go 1.22+
- Python 3

说明：

- 前端通过 Python 自带静态服务器启动
- 后端通过 Go 启动
- 仓库已包含可直接使用的 SQLite 数据库，无需额外初始化

## 首次启动

在仓库根目录执行：

### 1. 启动前端

```bash
npm run frontend:start
```

### 2. 启动后端

```bash
npm start
```

如果你不想通过 npm，也可以直接启动后端：

```bash
cd backend-go
env GOCACHE=$(pwd)/.cache/go-build go run ./cmd/server
```

## 访问地址

- 学生端主页: `http://127.0.0.1:3000/student.html`
- 教师端后台: `http://127.0.0.1:3000/teacher.html`
- 学生历史答题页: `http://127.0.0.1:3000/student-history.html`

常见带参数学生入口示例：

- `http://127.0.0.1:3000/student.html?courseId=1`
- `http://127.0.0.1:3000/student.html?courseId=2`

注意：

- `student-history.html` 通常通过学生端页面中的“查看我的历史课堂答题数据”按钮进入，因为它依赖 URL 参数。

## 数据说明

项目当前使用的正式数据库文件是：

```text
backend-go/data/quiz-system.db
```

仓库已经包含这份数据文件，拉取后即可直接运行。

当前代码约定：

- 班级名册按“班级维度”共享，不再按课程分裂
- 测试残留数据已清理
- 学生历史数据、教师统计和学生答题都基于当前库

## 常用命令

### 后端构建

```bash
npm run backend:build
```

### 健康检查

```bash
curl -s http://127.0.0.1:8080/healthz
```

期望返回：

```json
{"success":true,"data":{"status":"ok"}}
```

### 关闭本地端口进程

如需手动释放端口：

```bash
lsof -nP -iTCP:3000 -sTCP:LISTEN
lsof -nP -iTCP:8080 -sTCP:LISTEN
kill <PID>
```

## 当前主要特性

- 前后端逻辑已拆分
- 后端已迁移为 Go 版本
- 教师端支持课程切换、题目管理、部分开关、统计导出
- 学生端支持动态课次答题、历史答题查询
- 第二部分支持文本题、单选题、多选题
- 第三部分结果页支持答案解析富文本展示
- 历史课堂答题页面已重新整理为更清晰的学习档案视图

## 交接给 OpenClaw 的推荐指令

可以直接让 OpenClaw 在新环境里执行下面这段：

```text
请按以下步骤运行这个项目：
1. clone 仓库并切换到分支 refactor-go-backend-ui-fixes
2. 在仓库根目录运行 npm run frontend:start，启动前端静态服务
3. 在仓库根目录运行 npm start，启动 Go 后端
4. 验证 http://127.0.0.1:8080/healthz 返回 success=true
5. 打开 http://127.0.0.1:3000/teacher.html 和 http://127.0.0.1:3000/student.html 检查页面是否正常
6. 若端口被占用，先查 3000 和 8080 的监听进程并停止，再重新启动
7. 不要重建数据库，直接使用仓库内 backend-go/data/quiz-system.db
```

## 备注

- 根目录下的旧 `backend/` 是历史 Node 版本后端，当前默认运行的是 `backend-go/`
- 如无特殊需要，不要删除或重置数据库文件
