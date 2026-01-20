# New-API 压力测试快速指南

本指南帮助你快速开始使用 wrk 对 New-API 进行压力测试。

## 快速开始

### 1. 安装 wrk

```bash
# macOS
brew install wrk

# Linux
git clone https://github.com/wg/wrk.git
cd wrk && make

# Windows (使用 WSL)
# 在 WSL 中执行上述 Linux 安装步骤
```

### 2. 设置环境变量

```bash
# 设置你的 API Token
export API_TOKEN="sk-your-test-token-here"

# 设置 API Base URL (可选)
export BASE_URL="http://localhost:3000"
```

### 3. 运行快速测试

```bash
# 测试 Chat Completions
wrk -t 4 -c 100 -d 30s \
  -s scripts/load-test/chat_completions.lua \
  http://localhost:3000/v1/chat/completions

# 测试 Embeddings
wrk -t 4 -c 100 -d 30s \
  -s scripts/load-test/embeddings.lua \
  http://localhost:3000/v1/embeddings

# 测试 Models 列表
wrk -t 4 -c 100 -d 30s \
  -s scripts/load-test/models.lua \
  http://localhost:3000/v1/models
```

### 4. 运行完整测试套件

```bash
bash scripts/load-test/run_load_test.sh
```

## 测试场景说明

| 脚本 | 测试内容 | 并发建议 | 说明 |
|------|----------|----------|------|
| `chat_completions.lua` | 文本生成 | 100-500 | 主要测试场景 |
| `embeddings.lua` | 向量嵌入 | 200-500 | 高频小请求 |
| `models.lua` | 模型列表 | 100-200 | 只读查询 |
| `chat_stream.lua` | 流式响应 | 20-100 | 长连接测试 |

## 性能指标参考

- **Chat Completions**: QPS 100+, P99延迟 < 1500ms
- **Embeddings**: QPS 500+, P99延迟 < 200ms
- **Models**: QPS 1000+, P99延迟 < 50ms

## 常见问题

**Q: 如何获取测试 Token?**

A: 通过管理面板或 API 创建:
```bash
curl -X POST http://localhost:3000/api/token \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN" \
  -d '{"name":"load-test","quota":1000000}'
```

**Q: 测试时出现 429 错误?**

A: 降低并发数或调整限流配置:
```bash
wrk -t 2 -c 50 -d 30s ...
```

**Q: 如何调整测试参数?**

A: 修改 wrk 命令行参数:
- `-t`: 并发线程数
- `-c`: 并发连接数
- `-d`: 测试时长
- `--timeout`: 超时时间

**Q: 如何查看详细延迟分布?**

A: 添加 `--latency` 参数:
```bash
wrk -t 4 -c 100 -d 30s --latency ...
```

## 下一步

查看完整的压力测试方案文档: [LOAD_TESTING_GUIDE.md](./LOAD_TESTING_GUIDE.md)

## 脚本位置

所有测试脚本位于: `scripts/load-test/`

- `chat_completions.lua` - Chat Completions API 测试
- `embeddings.lua` - Embeddings API 测试
- `models.lua` - Models API 测试
- `chat_stream.lua` - Streaming Chat API 测试
- `run_load_test.sh` - 自动化测试脚本
