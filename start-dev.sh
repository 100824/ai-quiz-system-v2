#!/bin/bash
# 一键启动开发环境脚本
# 用法：./start-dev.sh
# 作用：自动检测端口占用、启动前后端、打印访问地址

set -e

PROJECT_DIR="$(cd "$(dirname "$0")" && pwd)"
FRONTEND_PORT=3000
BACKEND_PORT=8080

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查端口占用并释放
check_and_kill_port() {
    local port=$1
    local name=$2
    local pids

    pids=$(lsof -t -iTCP:${port} -sTCP:LISTEN 2>/dev/null || true)

    if [ -n "$pids" ]; then
        log_warn "${name} 端口 ${port} 被占用，正在释放..."
        echo "$pids" | xargs kill -9 2>/dev/null || true
        sleep 1
        log_info "端口 ${port} 已释放"
    else
        log_info "端口 ${port} 可用"
    fi
}

# 检查后端是否已编译
check_backend_binary() {
    if [ ! -f "${PROJECT_DIR}/backend-go/server" ]; then
        log_warn "后端二进制不存在，正在编译..."
        cd "${PROJECT_DIR}"
        npm run backend:build
        log_info "编译完成"
    else
        log_info "后端二进制已存在"
    fi
}

# 启动前端
start_frontend() {
    log_info "正在启动前端服务 (端口 ${FRONTEND_PORT})..."
    cd "${PROJECT_DIR}"
    nohup python3 -m http.server ${FRONTEND_PORT} --directory frontend >/tmp/ai-quiz-frontend.log 2>&1 &
    local pid=$!
    sleep 2

    if curl -s -o /dev/null -w "%{http_code}" --max-time 3 http://127.0.0.1:${FRONTEND_PORT}/student.html | grep -q "200"; then
        log_info "前端启动成功 (PID: ${pid})"
        return 0
    else
        log_error "前端启动失败，日志: /tmp/ai-quiz-frontend.log"
        return 1
    fi
}

# 启动后端
start_backend() {
    log_info "正在启动后端服务 (端口 ${BACKEND_PORT})..."
    cd "${PROJECT_DIR}/backend-go"
    nohup ./server >/tmp/ai-quiz-backend.log 2>&1 &
    local pid=$!
    sleep 2

    if curl -s -o /dev/null -w "%{http_code}" --max-time 3 http://127.0.0.1:${BACKEND_PORT}/healthz | grep -q "200"; then
        log_info "后端启动成功 (PID: ${pid})"
        return 0
    else
        log_error "后端启动失败，日志: /tmp/ai-quiz-backend.log"
        cat /tmp/ai-quiz-backend.log
        return 1
    fi
}

# 验证接口
verify_apis() {
    log_info "正在验证关键接口..."

    local healthz
    healthz=$(curl -s --max-time 3 http://127.0.0.1:${BACKEND_PORT}/healthz)
    if echo "$healthz" | grep -q '"status":"ok"'; then
        log_info "健康检查通过"
    else
        log_error "健康检查失败: $healthz"
    fi

    local courses
    courses=$(curl -s --max-time 3 http://127.0.0.1:${BACKEND_PORT}/api/teacher/courses)
    if echo "$courses" | grep -q '"success":true'; then
        log_info "教师端课程接口正常"
    else
        log_error "课程接口异常"
    fi

    local questions
    questions=$(curl -s --max-time 3 "http://127.0.0.1:${BACKEND_PORT}/api/student/questions/2?courseId=1")
    if echo "$questions" | grep -q '"success":true'; then
        log_info "学生端题目接口正常"
    else
        log_error "题目接口异常"
    fi
}

# 打印访问地址
print_urls() {
    echo ""
    echo "=========================================="
    echo "  服务已全部启动，访问地址："
    echo "=========================================="
    echo ""
    echo "  前端服务:"
    echo "    - 学生端:   http://127.0.0.1:${FRONTEND_PORT}/student.html"
    echo "    - 教师端:   http://127.0.0.1:${FRONTEND_PORT}/teacher.html"
    echo "    - 历史页:   http://127.0.0.1:${FRONTEND_PORT}/student-history.html"
    echo ""
    echo "  后端 API:"
    echo "    - 健康检查: http://127.0.0.1:${BACKEND_PORT}/healthz"
    echo "    - 课程列表: http://127.0.0.1:${BACKEND_PORT}/api/teacher/courses"
    echo ""
    echo "  停止服务:"
    echo "    lsof -t -iTCP:${FRONTEND_PORT} -sTCP:LISTEN | xargs kill"
    echo "    lsof -t -iTCP:${BACKEND_PORT} -sTCP:LISTEN | xargs kill"
    echo ""
    echo "=========================================="
}

# 主流程
main() {
    echo "=========================================="
    echo "  AI 课堂学习平台 - 开发环境启动脚本"
    echo "=========================================="
    echo ""

    cd "$PROJECT_DIR"

    # 1. 释放端口
    check_and_kill_port $FRONTEND_PORT "前端"
    check_and_kill_port $BACKEND_PORT "后端"

    echo ""

    # 2. 检查编译产物
    check_backend_binary

    echo ""

    # 3. 启动服务
    start_frontend
    start_backend

    echo ""

    # 4. 验证
    verify_apis

    # 5. 打印地址
    print_urls

    # 6. 保持脚本运行（可选，按 Ctrl+C 停止）
    echo "按 Ctrl+C 停止此脚本（服务将继续在后台运行）"
    echo ""
    while true; do
        sleep 5
        if ! curl -s -o /dev/null --max-time 2 http://127.0.0.1:${BACKEND_PORT}/healthz; then
            log_error "后端服务异常退出，日志如下："
            tail -20 /tmp/ai-quiz-backend.log
            break
        fi
    done
}

# 捕获 Ctrl+C
trap 'echo ""; log_info "脚本退出"; exit 0' INT

main
