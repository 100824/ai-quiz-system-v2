# 新版与老版边界约束

## 范围划分

- **新版页面入口**
  - `/teacher.html`
  - `/student.html`

- **新版模块**
  - 前端：`frontend/v2/`
  - 后端：`backend-go/internal/v2/`

- **已归档的老功能（禁止修改）**
  - 前端：`frontend/history/`
  - 后端：`backend-go/history-data/`
  - 任何明确标记为 history / legacy / 归档的文件或目录

## 约束

后续需求只调整新版页面和新版模块，**不得改动老功能模块**。

修改前请先确认目标文件是否在新版目录或新版入口内。
