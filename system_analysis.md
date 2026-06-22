# AI 课堂互动答题系统（Quiz System）分析文档

## 1. 系统概述

这是一个面向小学五年级（AI 课程）课堂的互动答题系统，分为**教师端**和**学生端**：
- **教师端**：管理课程、班级花名册、题库、控制课堂阶段（4 个 Parts）、查看统计数据、导出 Excel 报表、发布课堂提示语。
- **学生端**：按班级+姓名登录，跟随教师开启的阶段依次完成 4 部分答题：
  1. **Part 1**：自评预测（0-5 分）+ 学习方法多选。
  2. **Part 2**：开放思考/讨论题（支持富文本颜色标注）。
  3. **Part 3**：客观小测（单选/判断）。
  4. **Part 4**：课后反思（根据 Part 3 得分与 Part 1 预测分对比，动态生成不同选项）。

技术栈：
- **后端**：Go 1.26 + 标准库 `net/http` + `sqlite3`（mattn/go-sqlite3）+ `excelize`（导出报表）。
- **前端**：原生 HTML/CSS/JS（无框架），3 个页面：`teacher.html`、`student.html`、`student-history.html`。
- **架构**：单体服务，前后端不分离（前端静态文件需另外托管或同域部署）。

---

## 2. 存在的问题

### 2.1 安全与认证（严重）

| 问题 | 影响 | 说明 |
|------|------|------|
| **教师端密码在前端硬编码** | 极高 | `teacher.html` 中 `CORRECT_PASSWORD = "0506"`，任何用户查看源码即可绕过，后端完全没有教师认证接口。 |
| **无用户鉴权/会话管理** | 极高 | 没有任何 JWT、Cookie、Session 机制；所有 API 完全开放，知道 URL 即可随意调用。 |
| **CORS 开放为 `*`** | 高 | `middleware.go` 设置 `Access-Control-Allow-Origin: *`，且允许所有方法，任何站点都可跨域调用 API。 |
| **学生验证仅靠花名册** | 中 | 学生只需输入班级+姓名即可登录，无密码或二次验证，同名不同人无法区分。 |
| **文件上传路径遍历防护有限** | 中 | `HandleServeImage` 做了 `filepath.Clean` + `strings.HasPrefix` 检查，但原文件名直接参与路径拼接，虽无直接漏洞，但逻辑较脆弱。 |
| **SQL 注入风险低但存在** | 低 | 大部分使用 `?` 占位符，但 `GetCompletionStats` 中存在 `fmt.Sprintf` 拼接列名（`column := fmt.Sprintf("part%d_answers", part)`），虽然 part 是整数可控，但属于不良实践。 |

### 2.2 数据库设计（中高）

| 问题 | 影响 | 说明 |
|------|------|------|
| **JSON 数据存入 TEXT 列** | 高 | `part1_answers`、`part3_answers`、`part4_answers` 都是 JSON 字符串，导致无法对答案内容做 SQL 查询、统计困难，只能全表拉取后在内存解析。 |
| **学生 ID 用 `班级_姓名` 拼接** | 高 | `student_id = className + "_" + studentName`，姓名修改后历史数据断裂；无法处理转班、重名等情况。 |
| **缺少外键约束与索引** | 中 | 表结构定义有 `FOREIGN KEY`，但 SQLite 外键需手动开启；代码中 `PRAGMA foreign_keys = ON` 已开启，但未见索引优化，大数据量时统计查询慢。 |
| **无数据迁移版本管理** | 中 | 迁移逻辑在 `InitDB` 中通过 `ALTER TABLE ... ADD COLUMN` 和 `strings.Contains(err.Error(), "duplicate column name")` 判断，属于“补丁式”迁移，无法回滚，无法追踪版本。 |
| **默认题目硬编码在 Go 代码中** | 中 | `buildDefaultQuestions` 把 12 节课的默认题写死在 `migrations.go` 里，修改题目需重新编译后端。 |
| **Part 4 题目完全不在数据库** | 高 | 第四部分的题目和选项是 `student.js` 中 `generatePart4Content()` 硬编码 HTML，没有题库管理，后端只存储答案，无法复用/修改。 |
| **SQLite 单文件并发瓶颈** | 中 | 课堂场景下多个学生同时提交，SQLite WAL 模式能缓解，但教师端导出全量 Excel 时可能长时间占用锁。 |

### 2.3 后端架构与代码质量

| 问题 | 影响 | 说明 |
|------|------|------|
| **无 Graceful Shutdown** | 高 | `main.go` 使用裸 `http.ListenAndServe`，没有 `http.Server` + `Shutdown` 机制，部署时直接中断正在处理的请求。 |
| **无请求超时/限流** | 高 | 没有 `ReadTimeout`、`WriteTimeout`、`IdleTimeout`，也没有 rate limiter，恶意请求或大数据导出可拖垮服务。 |
| **Handler 层过于臃肿** | 高 | `handler.go` 1197 行，包含课程、班级、题目、阶段、统计、导出、上传、学生答题等全部逻辑，应进一步拆分。 |
| **Service 层职责不清** | 中 | `Service` 只负责统计聚合和导出行构建，大量业务逻辑（如 Part 2 答案格式转换）放在 `service.go`，但 `handler.go` 直接调用 `repo` 和 `svc` 混用，分层不一致。 |
| **错误处理不一致** | 中 | 部分接口返回 `{"success": false, "error": "..."}`，部分返回 `{"success": false, "message": "..."}`，前端需分别处理。 |
| **无结构化日志** | 中 | 使用标准库 `log`，无日志级别、无 JSON 格式、无请求追踪 ID，生产环境难以排查。 |
| **无健康检查深度** | 低 | `/healthz` 只返回 `{"status": "ok"}`，不检查数据库连通性。 |
| **硬编码业务文本** | 低 | `Part2UnderstandingQuestionText` 等中文业务文本硬编码在 Go 和 JS 中，不利于国际化或修改。 |

### 2.4 前端代码质量

| 问题 | 影响 | 说明 |
|------|------|------|
| **JS 文件过大且无模块化** | 高 | `student.js` 2072 行、`teacher.js` 1605 行、`student-history.js` 605 行，全部全局变量，命名冲突风险高，维护困难。 |
| **同一段工具函数重复定义** | 高 | `escapeHtml`、`renderTextWithImages`、`resolveImageUrl`、`formatPart2Answer`、`formatPart2ResponseLabel`、`parseQuestionOptions` 等在三份 JS 文件中几乎完全重复。 |
| **硬编码班级名称** | 高 | `student.html` 和 `teacher.html` 中“五年级（1）班”到“五年级（7）班”写死在 HTML option 和 JS 循环中，增加/修改班级必须改代码。 |
| **内联样式与 HTML 字符串拼接** | 中 | `teacher.js` 和 `student.js` 中大量用字符串拼接 HTML（如 `html += \`<div style="...">\``），难以维护，无 XSS 校验（虽然做了 `escapeHtml`，但拼接逻辑复杂容易遗漏）。 |
| **无状态管理** | 中 | 全局变量 `currentCourseId`、`currentStage`、`surveyData` 等散落在全局作用域，刷新页面后丢失，需重新加载。 |
| **轮询效率低** | 中 | 学生端和教师端都使用 `setInterval` 每 10 秒轮询阶段状态，没有 WebSocket 或 Server-Sent Events，浪费带宽和电池。 |
| **无前端路由** | 低 | 学生端登录页和答题页通过 `classList.add/remove('hidden')` 切换，URL 不变，无法刷新后保持状态，无法回退。 |
| **图片上传后前端缓存不刷新** | 低 | 题目中图片 URL 无缓存控制，修改后学生端可能仍显示旧图。 |

### 2.5 API 设计

| 问题 | 影响 | 说明 |
|------|------|------|
| **REST 风格不统一** | 中 | 更新操作用 `POST`（如 `POST /api/teacher/question/{id}`），但更符合语义应为 `PUT` 或 `PATCH`；获取列表用 `GET`，但部分参数在 query，部分在 body，混用。 |
| **无 API 版本** | 中 | URL 无前缀版本号，如 `/api/v1/...`，未来迭代难以兼容。 |
| **分页缺失** | 中 | `/api/teacher/stats/{courseId}` 拉取全量学生数据，班级人数多时一次性返回巨大 JSON，可能导致内存和传输压力。 |
| **学生接口无防刷机制** | 中 | 学生可以反复提交同一部分，后端没有幂等控制或提交次数限制（虽然 UI 上锁定，但 API 层面未限制）。 |
| **Part 2 答案格式过度设计** | 高 | 为了兼容“单题”和“多题”两种历史格式，设计了 `Part2AnswerPayload`、`Part2ResponseItem`、`Part2ResponseInput` 三层嵌套，读取时还要做 `JSON.parse` 的 fallback，实际业务中 Part 2 通常只有 1-2 题，过度复杂。 |

### 2.6 业务逻辑

| 问题 | 影响 | 说明 |
|------|------|------|
| **Part 3 答题用数组索引映射题目** | 高 | `buildPart3Results` 中用 `key := fmt.Sprintf("q%d", i+1)` 匹配 `questions[i]`，意味着学生答案的 `q1` 永远对应 `sort_order` 第一题，如果教师调整了题目顺序或禁用某题，索引会错位，导致判分错误。 |
| **ActualScore 是内存计算字段** | 中 | `StudentSurvey` 的 `ActualScore` 和 `ActualScoreSource` 在 `setStudentActualScore` 中根据 `TeacherScore` 或 `Part3Score` 动态计算，不持久化，导致统计导出时重复计算逻辑，且历史记录中难以直接查询。 |
| **Part 1 第一题禁止编辑但逻辑脆弱** | 低 | 前端用 `!(q.part == 1 && q.sort_order == 1)` 判断是否显示编辑按钮，后端 `HandleUpdateQuestion` 未做同样校验，绕过前端可直接修改预测评分题。 |
| **教师评分 0-5 分，但 Part 3 也是 0-5** | 低 | 无校验说明，教师可能误输大于 5 的分（虽然后端有 `0-5` 校验），但业务上 2 个分数体系合并为 `actual_score` 的优先级规则（teacher > part3）缺乏明确说明。 |
| **自定义学习方法输入为空时允许提交** | 低 | 前端选了“其他”但未输入内容时，提交空字符串，业务上未做校验。 |

---

## 3. 设计不合理之处

### 3.1 前后端不分离 + 硬编码 API 地址
前端通过 `window.APP_CONFIG?.apiBase || \`${window.location.protocol}//${window.location.hostname}:8080/api\`` 确定后端地址，意味着前端页面和后端必须同域或已知端口，无法独立部署、CDN 加速。

### 3.2 单体 Go 服务同时承载 API 和静态文件（但静态文件缺失）
`main.go` 中只注册了 API 路由，没有 `http.FileServer` 托管前端文件，导致部署时通常需要 Nginx 或另外启动静态服务器，但仓库里无相关配置。

### 3.3 题目数据与选项数据分离存储
`questions` 表的 `options` 是 JSON 字符串，但 `correct_answer` 是字符串，如果选项内容修改，正确答案没有外键约束，可能不匹配。

### 3.4 阶段控制（Stage）与部分开关（PartSettings）两套独立机制
- `course_stages` 控制当前课程进行到第几部分。
- `course_part_settings` 控制每个部分是否启用。
两者联动逻辑在 `UpdatePartSettings` 中手动处理（如果禁用当前 stage，则自动切换到下一个启用的 stage），容易出 bug，且教师端 UI 中两者展示在不同 tab，体验割裂。

### 3.5 图片存储与文本存储耦合
题目文本和选项支持 `![alt](url)` Markdown 语法插入图片，但图片存储在本地文件系统，无数据库记录，迁移或备份时容易丢失图片与题目的关联关系。

### 3.6 班级花名册与答题记录数据冗余
`class_lists` 存储班级花名册，`student_surveys` 存储答题记录，两者都有 `course_id + class_name + student_name`，但无统一的学生主表。导入花名册后，若学生已答题，会出现两份数据，查询花名册时还要做 UNION 查询。

---

## 4. 优化建议

### 4.1 安全加固（优先级：最高）

1. **后端引入真正的认证机制**：
   - 教师端：使用 bcrypt 存储密码哈希，登录后下发 JWT（或 Session Cookie），所有 `/api/teacher/*` 接口需验证 Token。
   - 学生端：可考虑生成一次性验证码或保持现状（姓名+班级），但 `/api/student/*` 的提交接口需校验当前班级和课程绑定关系。
2. **CORS 白名单化**：根据环境变量配置允许的 Origin，生产环境绝不开放 `*`。
3. **增加 Rate Limiting**：使用 `golang.org/x/time/rate` 对学生提交和教师导出做限流，防止刷题和暴力导出。
4. **API 输入校验**：引入 `go-playground/validator` 对请求体做统一校验，避免手写 `if` 判断。

### 4.2 数据库重构（优先级：高）

1. **引入迁移工具**：使用 `golang-migrate/migrate` 或 `pressly/goose` 管理 Schema 版本，替代 `InitDB` 中的补丁式 ALTER TABLE。
2. **拆分 JSON 字段为独立表**（可选，视查询需求）：
   - 若需频繁按答案内容统计，可新增 `student_survey_answers` 表：`(survey_id, question_id, answer_text, is_correct)`。
   - 若答案只做展示，保留 JSON 亦可，但建议用 SQLite 的 JSON1 扩展做索引。
3. **引入学生主表**：
   - `students` 表：`(id, class_name, student_name, created_at)`，使用自增 ID 作为 `student_id`，彻底替代 `className_studentName` 拼接。
   - 答题记录用 `student_id` 外键关联，支持转班、改名。
4. **索引优化**：
   - `student_surveys` 增加 `(course_id, class_name)`、`(student_id, course_id)` 索引。
   - `questions` 增加 `(course_id, part, enabled)` 复合索引。
5. **图片存储记录到数据库**：新增 `uploads` 表记录文件元数据，便于迁移和清理孤儿文件。

### 4.3 后端架构优化（优先级：高）

1. **引入 `http.Server` + Graceful Shutdown**：
   ```go
   srv := &http.Server{Addr: addr, Handler: appHandler, ReadTimeout: 15s, WriteTimeout: 15s}
   go func() { srv.ListenAndServe() }()
   // 捕获 os.Signal 后 srv.Shutdown(ctx)
   ```
2. **Handler 拆分**：按领域拆分为 `course_handler.go`、`class_handler.go`、`question_handler.go`、`stats_handler.go`、`student_handler.go`。
3. **统一响应封装**：使用 `pkg/response` 统一 `{success, data, error, code}` 结构，前端统一处理。
4. **引入结构化日志**：使用 `uber-go/zap` 或 `sirupsen/logrus`，输出 JSON 格式，携带 request_id。
5. **配置管理**：使用 `spf13/viper` 或环境变量 + `config.yaml`，将端口、密码盐、上传大小、CORS 等提取到配置中，不再硬编码。
6. **Part 4 题目入库**：将第四部分的动态题也抽象为 `questions` 表中的特殊类型（如 `question_type = 'dynamic_reflection'`），或至少在后端管理模板，而非纯前端写死。
7. **Part 3 判分改为 question_id 匹配**：学生提交时以 `question_id -> answer` 的 map 提交，判分时按 ID 找 `CorrectAnswer`，彻底消除顺序错位风险。

### 4.4 前端重构（优先级：高）

1. **引入现代构建工具**：使用 Vite + Vue 3 / React 或至少 TypeScript，将 3 个巨型 JS 文件拆分为组件和模块。
2. **共用工具库**：将 `escapeHtml`、`formatPart2Answer`、`parseQuestionOptions` 等提取到 `shared/utils.ts` 中，避免三份文件复制粘贴。
3. **班级名称配置化**：后端增加 `/api/classes` 接口返回班级列表，前端下拉框动态渲染，不再写死在 HTML 中。
4. **状态管理**：使用 Pinia / Redux 或至少一个全局 Store 管理 `currentCourseId`、`surveyData`、`partSettings` 等状态。
5. **引入前端路由**：学生端使用 `vue-router` 或原生 `history.pushState`，实现登录页、答题页、历史页 URL 分离，支持刷新保持。
6. **升级轮询为 WebSocket 或 SSE**：教师端切换阶段时主动推送，学生端无需 10 秒轮询，实时性更好、更省流量。
7. **UI 组件化**：将模态框、进度条、题目卡片、提示黑板等提取为独立组件，CSS 使用 Tailwind CSS 或 BEM 规范，减少内联样式。

### 4.5 API 改进（优先级：中）

1. **REST 规范化**：
   - `GET /api/v1/courses` → 列表
   - `POST /api/v1/courses` → 创建
   - `GET /api/v1/courses/{id}` → 详情
   - `PUT /api/v1/courses/{id}` → 更新
   - `DELETE /api/v1/courses/{id}` → 删除
2. **分页**：统计列表和学生列表支持 `?page=1&page_size=50`，避免全量拉取。
3. **导出异步化**：大数据导出改为异步任务，提交后返回 `task_id`，前端轮询任务状态，完成后下载，避免请求超时。
4. **幂等提交**：学生提交接口支持 `Idempotency-Key` 请求头，防止重复提交。

### 4.6 测试与 CI/CD（优先级：中）

1. **补充单元测试**：Handler 使用 `httptest` 测试，Repository 使用 `sqlmock` 测试，Service 层纯逻辑测试。
2. **集成测试**：使用 Docker Compose 启动完整服务，编写 Playwright 或 Cypress 测试学生端和教师端关键流程。
3. **GitHub Actions / GitLab CI**：增加 Go 测试、前端构建、Docker 镜像构建流水线。

### 4.7 部署与运维（优先级：中）

1. **Docker 化**：编写 `Dockerfile` + `docker-compose.yml`，将 Go 服务、前端 Nginx、SQLite 数据卷分离。
2. **反向代理 + HTTPS**：使用 Nginx 或 Caddy 做反向代理，自动 HTTPS，前端静态文件由 Nginx 托管，API 转发到 Go 服务。
3. **备份策略**：SQLite 文件定期备份，或未来迁移到 PostgreSQL/MySQL，获得更完善的备份和监控工具。
4. **监控**：引入 Prometheus + Grafana 监控 QPS、延迟、错误率；导出 Excel 等慢接口单独告警。

---

## 5. 总结

该系统是一个**功能完整、UI 友好的课堂互动工具**，但在**安全架构、代码组织、数据建模、前端工程化**四个方面存在明显的短期债务。如果用户量较小（一个学校内部使用），当前代码可以运行；但若计划推广到多班级、多学校或长期使用，建议优先投入时间做以下三件事：

1. **安全补漏**：后端加认证、CORS 限制、限流。
2. **学生 ID 重构**：从 `班级_姓名` 改为自增 ID，解决历史数据与转班问题。
3. **前端模块化**：将三份 2000 行的 JS 拆分为 TypeScript 组件，消除重复代码。

然后再逐步推进数据库规范化、API 标准化、测试覆盖和部署自动化。
