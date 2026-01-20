# New-API 压力测试方案 (使用 wrk)

## 目录
1. [项目概述](#项目概述)
2. [测试环境准备](#测试环境准备)
3. [关键API端点分析](#关键api端点分析)
4. [测试策略](#测试策略)
5. [测试场景设计](#测试场景设计)
6. [执行步骤](#执行步骤)
7. [性能指标](#性能指标)
8. [结果分析](#结果分析)
9. [优化建议](#优化建议)
10. [故障排查](#故障排查)

---

## 项目概述

### 项目简介
New-API 是一个 AI API 中继服务,支持多种 AI 模型提供商(OpenAI、Claude、Gemini 等)的统一接入。本方案使用 wrk 工具对其进行压力测试。

### 测试目标
- 评估系统在不同并发级别下的性能表现
- 识别性能瓶颈和潜在问题
- 验证系统的稳定性和可靠性
- 为容量规划提供数据支持

### 测试范围
- ✅ Chat Completions API (文本生成)
- ✅ Embeddings API (向量嵌入)
- ✅ Models API (模型列表查询)
- ✅ Streaming Chat API (流式响应)
- ⚠️ 中间件性能 (Token认证、限流、分发)
- ⚠️ 渠道选择与负载均衡
- ⚠️ 错误处理与重试机制

---

## 测试环境准备

### 1. 系统要求

#### 服务器配置建议
- CPU: 4核及以上
- 内存: 8GB 及以上
- 磁盘: SSD (数据库和日志)
- 网络: 千兆网络

#### 软件要求
- Go 1.21+ (编译运行 New-API)
- wrk 4.2.0+ (压测工具)
- MySQL 8.0+ 或 PostgreSQL 13+ (数据库)
- Redis 6.0+ (可选,用于缓存和限流)

### 2. 安装 wrk

#### Linux/macOS
```bash
# 克隆 wrk 仓库
git clone https://github.com/wg/wrk.git
cd wrk

# 编译安装
make

# 验证安装
./wrk --version
```

#### Windows
使用 WSL (Windows Subsystem for Linux):
```bash
# 在 WSL 中执行上述 Linux 安装步骤
```

### 3. 启动 New-API 服务

#### 开发模式启动
```bash
# 设置环境变量
export GIN_MODE=release
export PORT=3000

# 启动服务
go run main.go
```

#### 生产模式启动
```bash
# 编译二进制文件
go build -o new-api main.go

# 启动服务
GIN_MODE=release PORT=3000 ./new-api
```

### 4. 准备测试数据

#### 创建测试 Token
通过 API 或管理面板创建测试用的 API Token:
```bash
curl -X POST http://localhost:3000/api/token \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN" \
  -d '{
    "name": "load-test-token",
    "quota": 1000000,
    "models": ["gpt-3.5-turbo", "gpt-4", "text-embedding-ada-002"]
  }'
```

#### 配置测试渠道
确保在管理面板中配置了可用的上游渠道:
- OpenAI API
- Azure OpenAI
- 或其他兼容的 API 提供商

---

## 关键API端点分析

基于对 `router/relay-router.go` 和 `controller/relay.go` 的分析,以下是关键测试端点:

### 1. Chat Completions API
**路由**: `POST /v1/chat/completions`
**功能**: 文本生成,最常用的API
**处理流程**:
- Token 认证 (`middleware.TokenAuth`)
- 模型请求限流 (`middleware.ModelRequestRateLimit`)
- 渠道分发 (`middleware.Distribute`)
- 请求解析与验证
- 渠道选择与负载均衡
- 价格计算与预扣费
- 请求转发到上游
- 响应处理与计费

**相关代码**:
- `router/relay-router.go:88`
- `controller/relay.go:65`

### 2. Embeddings API
**路由**: `POST /v1/embeddings`
**功能**: 文本向量化
**特点**: 通常响应时间较短,适合高频测试

**相关代码**:
- `router/relay-router.go:109`

### 3. Models API
**路由**: `GET /v1/models`
**功能**: 获取可用模型列表
**特点**: 只读操作,响应速度快

**相关代码**:
- `router/relay-router.go:21`

### 4. Streaming Chat API
**路由**: `POST /v1/chat/completions` (with `stream: true`)
**功能**: 流式文本生成
**特点**: 保持长连接,测试并发连接处理能力

---

## 测试策略

### 1. 渐进式加压策略

从低并发到高并发逐步增加,观察系统性能变化:

```
阶段1: 基准测试 (10 并发)
阶段2: 正常负载 (50 并发)
阶段3: 高负载 (100 并发)
阶段4: 峰值负载 (200 并发)
阶段5: 极限压力 (500+ 并发)
```

### 2. 混合负载策略

模拟真实场景,不同API端点的请求比例:

| API | 比例 | 说明 |
|-----|------|------|
| Chat Completions | 60% | 主要负载 |
| Embeddings | 20% | 高频小请求 |
| Models | 10% | 低频查询 |
| Streaming | 10% | 长连接 |

### 3. 持续压测策略

长时间持续压测,检测内存泄漏和性能衰减:

- 测试时长: 1小时、4小时、24小时
- 监控指标: 内存使用、响应时间、错误率

### 4. 突发流量测试

模拟突发流量,测试系统的弹性:

- 短时间内将并发从10提升到500
- 观察系统的响应和恢复能力

---

## 测试场景设计

### 场景1: Chat Completions 性能测试

**目标**: 测试文本生成API的吞吐量和响应时间

**参数**:
```bash
并发线程: 4
并发连接: 10, 50, 100, 200, 500
测试时长: 30s
请求模型: gpt-3.5-turbo
max_tokens: 50-150
```

**执行命令**:
```bash
export API_TOKEN="sk-your-test-token"
wrk -t 4 -c 100 -d 30s \
  -s scripts/load-test/chat_completions.lua \
  http://localhost:3000/v1/chat/completions
```

**预期指标**:
- QPS: 100+ (取决于上游API)
- 平均延迟: < 1000ms (不含上游时间)
- 错误率: < 1%

### 场景2: Embeddings 高频测试

**目标**: 测试高频小请求的处理能力

**参数**:
```bash
并发线程: 8
并发连接: 100, 200, 500
测试时长: 30s
请求模型: text-embedding-ada-002
```

**执行命令**:
```bash
wrk -t 8 -c 200 -d 30s \
  -s scripts/load-test/embeddings.lua \
  http://localhost:3000/v1/embeddings
```

**预期指标**:
- QPS: 500+ (纯转发,无AI处理)
- 平均延迟: < 100ms
- 错误率: < 0.1%

### 场景3: Models 列表查询测试

**目标**: 测试只读接口的缓存效果

**参数**:
```bash
并发线程: 4
并发连接: 50, 100, 200
测试时长: 30s
```

**执行命令**:
```bash
wrk -t 4 -c 100 -d 30s \
  -s scripts/load-test/models.lua \
  http://localhost:3000/v1/models
```

**预期指标**:
- QPS: 1000+ (缓存命中时)
- 平均延迟: < 10ms
- 错误率: 0%

### 场景4: Streaming Chat 长连接测试

**目标**: 测试流式响应的并发处理能力

**参数**:
```bash
并发线程: 4
并发连接: 20, 50, 100 (流式连接占用资源较多)
测试时长: 30s
```

**执行命令**:
```bash
wrk -t 4 -c 50 -d 30s \
  -s scripts/load-test/chat_stream.lua \
  http://localhost:3000/v1/chat/completions
```

**预期指标**:
- 并发连接: 100+ (取决于系统配置)
- 首字节时间: < 500ms
- 错误率: < 2%

### 场景5: 混合负载测试

**目标**: 模拟真实生产环境

**使用多个 wrk 实例同时运行**:
```bash
# Terminal 1: Chat Completions (60%)
wrk -t 4 -c 60 -d 60s \
  -s scripts/load-test/chat_completions.lua \
  http://localhost:3000/v1/chat/completions &

# Terminal 2: Embeddings (20%)
wrk -t 2 -c 20 -d 60s \
  -s scripts/load-test/embeddings.lua \
  http://localhost:3000/v1/embeddings &

# Terminal 3: Models (10%)
wrk -t 1 -c 10 -d 60s \
  -s scripts/load-test/models.lua \
  http://localhost:3000/v1/models &

# Terminal 4: Streaming (10%)
wrk -t 1 -c 10 -d 60s \
  -s scripts/load-test/chat_stream.lua \
  http://localhost:3000/v1/chat/completions &

# 等待所有测试完成
wait
```

### 场景6: 极限压力测试

**目标**: 找到系统的性能极限

**步骤**:
1. 从低并发开始 (10)
2. 每次增加 50,直到出现大量错误
3. 记录崩溃点的并发数和QPS

```bash
for conn in 10 50 100 200 300 400 500 600 700 800; do
  echo "Testing with $conn connections..."
  wrk -t 8 -c $conn -d 30s --timeout 10s \
    -s scripts/load-test/chat_completions.lua \
    http://localhost:3000/v1/chat/completions
  sleep 5
done
```

---

## 执行步骤

### 步骤1: 预检查

```bash
# 1. 确认服务运行
curl http://localhost:3000/api/status

# 2. 检查数据库连接
curl http://localhost:3000/api/status/test

# 3. 验证 Token 有效
export API_TOKEN="sk-your-test-token"
curl -H "Authorization: Bearer $API_TOKEN" \
  http://localhost:3000/v1/models
```

### 步骤2: 运行基准测试

使用提供的自动化脚本:

```bash
# 设置环境变量
export API_TOKEN="sk-your-test-token"
export BASE_URL="http://localhost:3000"
export THREADS=4
export CONNECTIONS=100
export DURATION="30s"

# 运行所有测试
bash scripts/load-test/run_load_test.sh
```

### 步骤3: 手动运行特定测试

```bash
# Chat Completions
wrk -t 4 -c 100 -d 30s \
  -s scripts/load-test/chat_completions.lua \
  http://localhost:3000/v1/chat/completions

# 查看详细延迟分布
wrk -t 4 -c 100 -d 30s --latency \
  -s scripts/load-test/chat_completions.lua \
  http://localhost:3000/v1/chat/completions
```

### 步骤4: 监控系统资源

在测试期间,使用工具监控系统资源:

```bash
# 使用 htop 监控 CPU 和内存
htop

# 使用 iotop 监控磁盘 I/O
sudo iotop

# 监控网络连接
watch -n 1 'netstat -an | grep :3000 | wc -l'

# 监控进程资源使用
watch -n 1 'ps aux | grep new-api'
```

### 步骤5: 查看应用日志

```bash
# 实时查看日志
tail -f /path/to/new-api.log

# 查看错误日志
grep ERROR /path/to/new-api.log

# 查看慢请求
grep "duration" /path/to/new-api.log | awk '$3 > 1000'
```

---

## 性能指标

### 1. 吞吐量指标

| 指标 | 说明 | 目标值 |
|------|------|--------|
| QPS (Queries Per Second) | 每秒请求数 | 100+ |
| RPS (Requests Per Second) | 每秒响应数 | 接近 QPS |
| Transfer Rate | 数据传输速率 | > 1 MB/s |

### 2. 延迟指标

| 指标 | 说明 | 目标值 |
|------|------|--------|
| Latency (Mean) | 平均延迟 | < 500ms |
| Latency (Stdev) | 延迟标准差 | < 100ms |
| Latency (P50) | 中位数延迟 | < 400ms |
| Latency (P90) | 90分位延迟 | < 800ms |
| Latency (P99) | 99分位延迟 | < 1500ms |

### 3. 错误率指标

| 指标 | 说明 | 目标值 |
|------|------|--------|
| Errors (Non-2xx/3xx) | 错误响应数 | < 1% |
| Timeouts | 超时错误 | < 0.5% |
| 500 Errors | 服务器错误 | < 0.1% |
| 503 Errors | 服务不可用 | < 0.1% |

### 4. 资源利用率

| 资源 | 指标 | 警告阈值 |
|------|------|----------|
| CPU | 使用率 | < 80% |
| Memory | 使用率 | < 70% |
| Network | 带宽 | < 80% |
| Disk I/O | 使用率 | < 70% |

---

## 结果分析

### 1. wrk 输出解读

```
Running 30s test @ http://localhost:3000/v1/chat/completions
  4 threads and 100 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency   123.45ms   67.89ms   1.50s    75.43%
    Req/Sec    45.67     12.34    80.00     68.91%
  Latency Distribution
     50%  110.23ms
     75%  156.78ms
     90%  234.56ms
     99%  567.89ms
  5472 requests in 30.05s, 2.45MB read
Requests/sec:    182.05
Transfer/sec:     83.45KB
```

**关键指标说明**:
- `Latency Avg`: 平均延迟,越低越好
- `Latency Stdev`: 延迟抖动,越小越稳定
- `Latency Max`: 最大延迟,反映最坏情况
- `Req/Sec`: 每线程每秒请求数
- `Latency Distribution`: 延迟分布情况
  - P50 (50%): 一半请求的延迟低于此值
  - P90 (90%): 90%的请求延迟低于此值
  - P99 (99%): 99%的请求延迟低于此值

### 2. 性能瓶颈识别

#### 高延迟原因分析

**延迟 > 1s**:
- 检查上游API响应时间
- 检查数据库查询性能
- 检查网络带宽

**延迟抖动大**:
- 检查系统资源争用
- 检查垃圾回收(GC)影响
- 检查网络波动

#### 低吞吐量原因分析

**QPS < 50**:
- 检查限流配置
- 检查数据库连接池
- 检查上游API速率限制

**连接数不足**:
- 检查系统文件描述符限制
- 检查端口范围限制

#### 高错误率原因分析

**429 Too Many Requests**:
- 触发限流规则
- 上游API限制

**502/503/504**:
- 上游服务不可用
- 超时配置过短

**500 Internal Server Error**:
- 应用逻辑错误
- 数据库连接失败
- 内存不足

### 3. 性能优化建议

#### 应用层优化

1. **启用缓存**
   ```go
   // 在 .env 中设置
   MEMORY_CACHE_ENABLED=true
   REDIS_ENABLED=true
   ```

2. **调整连接池**
   ```go
   // 数据库连接池
   SetMaxOpenConns(100)
   SetMaxIdleConns(20)
   SetConnMaxLifetime(5 * time.Minute)
   ```

3. **优化限流策略**
   - 使用 Redis 实现分布式限流
   - 避免过于严格的限流

4. **启用响应压缩**
   ```go
   // 注意: SSE 不支持 gzip
   router.Use(gzip.Gzip(gzip.DefaultCompression))
   ```

#### 系统层优化

1. **调整文件描述符限制**
   ```bash
   ulimit -n 65535
   ```

2. **优化内核参数**
   ```bash
   # /etc/sysctl.conf
   net.core.somaxconn = 65535
   net.ipv4.tcp_max_syn_backlog = 8192
   net.ipv4.tcp_tw_reuse = 1
   ```

3. **启用 HTTP/2**
   ```go
   // 在 main.go 中配置
   ```

#### 架构层优化

1. **使用负载均衡**
   - Nginx 反向代理
   - HAProxy
   - 云负载均衡器

2. **水平扩展**
   - 部署多个实例
   - 使用容器编排 (Kubernetes)

3. **数据库优化**
   - 添加索引
   - 读写分离
   - 使用连接池

---

## 故障排查

### 1. 常见问题

#### wrk 连接被拒绝

**错误**: `Connection refused`

**解决方案**:
```bash
# 检查服务是否运行
ps aux | grep new-api

# 检查端口是否监听
netstat -tuln | grep 3000

# 检查防火墙
sudo ufw status
```

#### 401 Unauthorized

**错误**: 认证失败

**解决方案**:
```bash
# 验证 Token 有效
export API_TOKEN="sk-your-token"
curl -H "Authorization: Bearer $API_TOKEN" \
  http://localhost:3000/v1/models
```

#### 429 Too Many Requests

**错误**: 触发限流

**解决方案**:
- 降低并发数
- 调整限流配置
- 使用多个 Token

#### 连接超时

**错误**: `Timeout`

**解决方案**:
```bash
# 增加 wrk 超时时间
wrk -t 4 -c 100 -d 30s --timeout 30s \
  -s scripts/load-test/chat_completions.lua \
  http://localhost:3000/v1/chat/completions
```

### 2. 日志分析

#### 查看错误日志

```bash
# 查看最近错误
tail -100 /path/to/new-api.log | grep ERROR

# 统计错误类型
grep ERROR /path/to/new-api.log | \
  awk '{print $5}' | sort | uniq -c

# 查看慢请求
grep "duration" /path/to/new-api.log | \
  awk '$3 > 1000' | tail -20
```

#### 监控指标

使用 pprof 进行性能分析:

```bash
# 启用 pprof (在 .env 中设置)
ENABLE_PPROF=true

# 访问 pprof
go tool pprof http://localhost:8005/debug/pprof/profile

# 生成火焰图
go tool pprof -http=:8080 /path/to/profile
```

---

## 附录

### A. 测试报告模板

```
# New-API 压力测试报告

## 测试环境
- 服务器配置:
- 数据库:
- 缓存:
- New-API 版本:

## 测试配置
- 压测工具: wrk 4.2.0
- 并发线程: 4
- 并发连接: 100
- 测试时长: 30s

## 测试结果

### 场景1: Chat Completions
- QPS: 182.05
- 平均延迟: 123.45ms
- P90延迟: 234.56ms
- P99延迟: 567.89ms
- 错误率: 0.2%

### 场景2: Embeddings
...

## 结论
- 系统在100并发下表现良好
- P99延迟略高,需要优化
- 建议启用Redis缓存

## 优化建议
1. ...
2. ...
```

### B. 性能基准参考

| 场景 | 并发数 | 目标QPS | 目标延迟(P99) |
|------|--------|---------|---------------|
| Chat Completions | 100 | 100+ | < 1500ms |
| Embeddings | 200 | 500+ | < 200ms |
| Models | 100 | 1000+ | < 50ms |
| Streaming | 50 | 50+ | < 2000ms |

### C. 相关代码文件

- `router/relay-router.go`: 路由定义
- `controller/relay.go`: Relay处理逻辑
- `middleware/auth.go`: 认证中间件
- `middleware/rate-limit.go`: 限流中间件
- `middleware/distribute.go`: 渠道分发中间件

### D. 参考资源

- [wrk GitHub](https://github.com/wg/wrk)
- [New-API GitHub](https://github.com/Calcium-Ion/new-api)
- [Gin 性能优化](https://gin-gonic.com/docs/)
- [Go pprof 性能分析](https://golang.org/pkg/net/http/pprof/)

---

**文档版本**: 1.0
**创建日期**: 2025-01-20
**最后更新**: 2025-01-20
**维护者**: New-API 团队
