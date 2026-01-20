#!/bin/bash
# New-API 压力测试脚本集
# 使用 wrk 进行性能测试
#
# 环境变量:
#   API_TOKEN       - API 认证令牌 (必需)
#   BASE_URL        - API 基础 URL (默认: http://localhost:3000)
#   THREADS         - 并发线程数 (默认: 4)
#   CONNECTIONS     - 并发连接数 (默认: 100)
#   DURATION        - 测试持续时间 (默认: 30s)
#   SCRIPT_DIR      - 脚本目录 (默认: ./scripts/load-test)

set -e

# 默认配置
BASE_URL="${BASE_URL:-http://localhost:3000}"
THREADS="${THREADS:-4}"
CONNECTIONS="${CONNECTIONS:-100}"
DURATION="${DURATION:-30s}"
SCRIPT_DIR="${SCRIPT_DIR:-./scripts/load-test}"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 检查 wrk 是否安装
check_wrk() {
    if ! command -v wrk &> /dev/null; then
        echo -e "${RED}错误: wrk 未安装${NC}"
        echo "请访问 https://github.com/wg/wrk 安装 wrk"
        exit 1
    fi
}

# 检查 API Token
check_token() {
    if [ -z "$API_TOKEN" ]; then
        echo -e "${RED}错误: API_TOKEN 环境变量未设置${NC}"
        echo "使用方法: API_TOKEN=sk-xxx ./run_load_test.sh"
        exit 1
    fi
}

# 打印配置信息
print_config() {
    echo -e "${GREEN}=== New-API 压力测试配置 ===${NC}"
    echo "Base URL: $BASE_URL"
    echo "Threads: $THREADS"
    echo "Connections: $CONNECTIONS"
    echo "Duration: $DURATION"
    echo "API Token: ${API_TOKEN:0:10}..."
    echo ""
}

# 运行单个测试
run_test() {
    local test_name=$1
    local script=$2
    local endpoint=$3

    echo -e "${YELLOW}开始测试: $test_name${NC}"
    echo "端点: $endpoint"
    echo "脚本: $script"
    echo ""

    wrk -t "$THREADS" -c "$CONNECTIONS" -d "$DURATION" \
        -s "$SCRIPT_DIR/$script" \
        "$BASE_URL$endpoint"

    echo ""
    echo -e "${GREEN}✓ $test_name 测试完成${NC}"
    echo "================================"
    echo ""
}

# 主测试流程
main() {
    check_wrk
    check_token
    print_config

    # 测试 1: Chat Completions (文本生成)
    run_test "Chat Completions" "chat_completions.lua" "/v1/chat/completions"

    # 测试 2: Embeddings (向量嵌入)
    run_test "Embeddings" "embeddings.lua" "/v1/embeddings"

    # 测试 3: Models (模型列表查询)
    run_test "Models List" "models.lua" "/v1/models"

    # 测试 4: Streaming Chat (流式响应)
    run_test "Streaming Chat" "chat_stream.lua" "/v1/chat/completions"

    echo -e "${GREEN}=== 所有测试完成 ===${NC}"
}

# 运行所有测试
main
