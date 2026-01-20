# controller.Relay() 函数完整分析与端点确定机制

## 目录
1. [函数概述](#函数概述)
2. [请求路由流程](#请求路由流程)
3. [Relay() 函数详细分析](#relay-函数详细分析)
4. **[端点（Endpoints）确定机制](#端点endpoints确定机制)**
5. [适配器系统](#适配器系统)
6. [完整请求流程图](#完整请求流程图)
7. [总结](#总结)

---

## 函数概述

`Relay()` 是 new-api 系统的核心中继函数，负责将客户端的 API 请求转发到上游渠道（如 OpenAI、Claude、Gemini 等），并处理响应、计费、重试等逻辑。

**位置**: `controller/relay.go:65`

**函数签名**:
```go
func Relay(c *gin.Context, relayFormat types.RelayFormat)
```

**主要功能**:
- 请求解析与验证
- 渠道选择与负载均衡
- 价格计算与预扣费
- 请求转发到上游
- 响应处理与计费
- 错误处理与重试机制

---

## 请求路由流程

### 1. 路由注册

在 `router/relay-router.go` 中定义了所有 API 路由：

```go
relayV1Router := router.Group("/v1")
relayV1Router.Use(middleware.TokenAuth())
relayV1Router.Use(middleware.ModelRequestRateLimit())
{
    httpRouter := relayV1Router.Group("")
    httpRouter.Use(middleware.Distribute())

    // 不同端点使用不同的 RelayFormat
    httpRouter.POST("/messages", func(c *gin.Context) {
        controller.Relay(c, types.RelayFormatClaude)  // Claude API
    })

    httpRouter.POST("/chat/completions", func(c *gin.Context) {
        controller.Relay(c, types.RelayFormatOpenAI)  // OpenAI Chat API
    })

    httpRouter.POST("/embeddings", func(c *gin.Context) {
        controller.Relay(c, types.RelayFormatEmbedding)  // Embedding API
    })

    httpRouter.POST("/audio/transcriptions", func(c *gin.Context) {
        controller.Relay(c, types.RelayFormatOpenAIAudio)  // Audio API
    })

    // ... 更多路由
}
```

### 2. RelayFormat 枚举

`types.RelayFormat` 定义了支持的请求格式类型：

```go
type RelayFormat string

const (
    RelayFormatOpenAI          RelayFormat = "openai"
    RelayFormatOpenAIAudio     RelayFormat = "openai-audio"
    RelayFormatOpenAIImage     RelayFormat = "openai-image"
    RelayFormatOpenAIRealtime  RelayFormat = "openai-realtime"
    RelayFormatOpenAIResponses RelayFormat = "openai-responses"
    RelayFormatClaude          RelayFormat = "claude"
    RelayFormatGemini          RelayFormat = "gemini"
    RelayFormatRerank          RelayFormat = "rerank"
    RelayFormatEmbedding       RelayFormat = "embedding"
    RelayFormatTask            RelayFormat = "task"
    RelayFormatMjProxy         RelayFormat = "mj-proxy"
)
```

---

## Relay() 函数详细分析

### 完整执行流程

```go
func Relay(c *gin.Context, relayFormat types.RelayFormat) {
    // 1. WebSocket 升级（如果是 OpenAI Realtime）
    if relayFormat == types.RelayFormatOpenAIRealtime {
        ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
        defer ws.Close()
    }

    // 2. 解析并校验请求
    request, err := helper.GetAndValidateRequest(c, relayFormat)

    // 3. 生成 RelayInfo（包含模型、渠道、用户信息等）
    relayInfo, err := relaycommon.GenRelayInfo(c, relayFormat, request, ws)

    // 4. 敏感词检查 & Token 统计
    meta := request.GetTokenCountMeta()
    if needSensitiveCheck && meta != nil {
        contains, words := service.CheckSensitiveText(meta.CombineText)
        // 处理敏感词...
    }

    // 5. 估算 Token
    tokens, err := service.EstimateRequestToken(c, meta, relayInfo)

    // 6. 计算价格 & 预扣费
    priceData, err := helper.ModelPriceHelper(c, relayInfo, tokens, meta)
    if !priceData.FreeModel {
        newAPIError = service.PreConsumeQuota(c, priceData.QuotaToPreConsume, relayInfo)
    }

    // 7. 重试循环
    for retry := 0; retry <= common.RetryTimes; retry++ {
        // 7.1 获取渠道
        channel, err := getChannel(c, relayInfo, retryParam)

        // 7.2 根据 RelayFormat 选择处理函数
        switch relayFormat {
        case types.RelayFormatOpenAIRealtime:
            newAPIError = relay.WssHelper(c, relayInfo)
        case types.RelayFormatClaude:
            newAPIError = relay.ClaudeHelper(c, relayInfo)
        case types.RelayFormatGemini:
            newAPIError = geminiRelayHandler(c, relayInfo)
        default:
            newAPIError = relayHandler(c, relayInfo)  // TextHelper 等
        }

        // 7.3 成功则退出
        if newAPIError == nil {
            return
        }

        // 7.4 错误处理 & 判断是否重试
        processChannelError(c, channelError, newAPIError)
        if !shouldRetry(c, newAPIError, common.RetryTimes-retry) {
            break
        }
    }

    // 8. 错误响应
    if newAPIError != nil {
        c.JSON(newAPIError.StatusCode, gin.H{"error": newAPIError.ToOpenAIError()})
    }
}
```

---

## 端点（Endpoints）确定机制

### 关键问题：如何确定使用哪个上游端点？

端点确定是一个**多层次决策过程**，涉及以下几个关键因素：

### 1. 请求路径识别 (Path → RelayMode)

首先通过请求路径确定中继模式：

**文件**: `relay/constant/relay_mode.go:55`

```go
func Path2RelayMode(path string) int {
    relayMode := RelayModeUnknown
    if strings.HasPrefix(path, "/v1/chat/completions") {
        relayMode = RelayModeChatCompletions
    } else if strings.HasPrefix(path, "/v1/embeddings") {
        relayMode = RelayModeEmbeddings
    } else if strings.HasPrefix(path, "/v1/images/generations") {
        relayMode = RelayModeImagesGenerations
    } else if strings.HasPrefix(path, "/v1/audio/speech") {
        relayMode = RelayModeAudioSpeech
    } else if strings.HasPrefix(path, "/v1/responses") {
        relayMode = RelayModeResponses
    }
    // ... 更多路径
    return relayMode
}
```

**RelayMode 常量**:
```go
const (
    RelayModeUnknown              = iota
    RelayModeChatCompletions       // /v1/chat/completions
    RelayModeCompletions           // /v1/completions
    RelayModeEmbeddings            // /v1/embeddings
    RelayModeImagesGenerations     // /v1/images/generations
    RelayModeAudioSpeech           // /v1/audio/speech
    RelayModeAudioTranscription    // /v1/audio/transcriptions
    RelayModeResponses             // /v1/responses
    RelayModeRealtime              // /v1/realtime
    // ... 更多模式
)
```

### 2. 适配器选择 (API Type → Adaptor)

适配器根据渠道的 API 类型选择：

**文件**: `relay/relay_adaptor.go:52`

```go
func GetAdaptor(apiType int) channel.Adaptor {
    switch apiType {
    case constant.APITypeOpenAI:
        return &openai.Adaptor{}
    case constant.APITypeAnthropic:
        return &claude.Adaptor{}
    case constant.APITypeGemini:
        return &gemini.Adaptor{}
    case constant.APITypeAws:
        return &aws.Adaptor{}
    // ... 更多适配器
    }
    return nil
}
```

**API 类型映射**:
- 渠道类型 (ChannelType) → API 类型 (APIType)
- API 类型 → 适配器 (Adaptor)

例如：
```go
// ChannelTypeOpenAI → APITypeOpenAI → openai.Adaptor
// ChannelTypeAnthropic → APITypeAnthropic → claude.Adaptor
// ChannelTypeGemini → APITypeGemini → gemini.Adaptor
```

### 3. 端点 URL 构建 (Adaptor.GetRequestURL)

这是**端点确定的核心**，每个适配器都有自己的 `GetRequestURL()` 方法。

**文件**: `relay/channel/openai/adaptor.go:110`

```go
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
    // 3.1 特殊处理：OpenAI Realtime (WebSocket)
    if info.RelayMode == relayconstant.RelayModeRealtime {
        if strings.HasPrefix(info.ChannelBaseUrl, "https://") {
            baseUrl := strings.TrimPrefix(info.ChannelBaseUrl, "https://")
            baseUrl = "wss://" + baseUrl
            info.ChannelBaseUrl = baseUrl
        }
    }

    switch info.ChannelType {
    case constant.ChannelTypeAzure:
        // 3.2 Azure OpenAI 特殊处理
        apiVersion := info.ApiVersion
        if apiVersion == "" {
            apiVersion = constant.AzureDefaultAPIVersion
        }

        requestURL := strings.Split(info.RequestURLPath, "?")[0]
        requestURL = fmt.Sprintf("%s?api-version=%s", requestURL, apiVersion)
        task := strings.TrimPrefix(requestURL, "/v1/")

        model_ := info.UpstreamModelName
        if info.ChannelCreateTime < constant.AzureNoRemoveDotTime {
            model_ = strings.Replace(model_, ".", "", -1)
        }

        requestURL = fmt.Sprintf("/openai/deployments/%s/%s", model_, task)

        return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, requestURL, info.ChannelType), nil

    case constant.ChannelTypeCustom:
        // 3.3 自定义渠道
        url := info.ChannelBaseUrl
        url = strings.Replace(url, "{model}", info.UpstreamModelName, -1)
        return url, nil

    default:
        // 3.4 默认处理：使用客户端请求路径
        if info.RelayFormat == types.RelayFormatClaude ||
           info.RelayFormat == types.RelayFormatGemini {
            return fmt.Sprintf("%s/v1/chat/completions", info.ChannelBaseUrl), nil
        }

        return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, info.RequestURLPath, info.ChannelType), nil
    }
}
```

### 4. 端点确定的关键因素

#### 4.1 客户端请求路径 (`info.RequestURLPath`)

**来源**: `relay/common/relay_info.go:414`

```go
RequestURLPath: c.Request.URL.String()
```

这是客户端请求的完整路径，例如：
- `/v1/chat/completions`
- `/v1/embeddings`
- `/v1/audio/speech`

#### 4.2 渠道基础 URL (`info.ChannelBaseUrl`)

**来源**: 从数据库中的渠道配置获取

```go
ChannelBaseUrl: common.GetContextKeyString(c, constant.ContextKeyChannelBaseUrl)
```

例如：
- OpenAI: `https://api.openai.com`
- Azure: `https://your-resource.openai.azure.com`
- 自定义代理: `https://your-proxy.com`
- 本地模型: `http://localhost:8000`

#### 4.3 渠道类型 (`info.ChannelType`)

**来源**: 从数据库中的渠道配置获取

```go
ChannelType: common.GetContextKeyInt(c, constant.ContextKeyChannelType)
```

影响端点构建逻辑，例如：
- Azure 需要特殊的部署路径格式
- Cloudflare Gateway 需要移除 `/v1` 前缀
- 自定义渠道支持 `{model}` 模板变量

#### 4.4 上游模型名称 (`info.UpstreamModelName`)

**来源**: 渠道配置中的模型映射

```go
UpstreamModelName: common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
```

某些渠道需要将模型名称包含在 URL 中（如 Azure）。

### 5. 完整的端点构建示例

#### 示例 1: OpenAI Chat Completions

**输入**:
- 客户端请求: `POST /v1/chat/completions`
- 渠道类型: `ChannelTypeOpenAI`
- 渠道 BaseURL: `https://api.openai.com`
- 模型: `gpt-4`

**处理流程**:
```go
// 1. RelayMode = RelayModeChatCompletions
// 2. ApiType = APITypeOpenAI
// 3. Adaptor = openai.Adaptor
// 4. GetRequestURL():
//    - ChannelBaseUrl = "https://api.openai.com"
//    - RequestURLPath = "/v1/chat/completions"
//    - 返回: "https://api.openai.com/v1/chat/completions"
```

**最终端点**: `https://api.openai.com/v1/chat/completions`

#### 示例 2: Azure OpenAI

**输入**:
- 客户端请求: `POST /v1/chat/completions`
- 渠道类型: `ChannelTypeAzure`
- 渠道 BaseURL: `https://your-resource.openai.azure.com`
- 模型: `gpt-4`
- API Version: `2024-02-01`

**处理流程**:
```go
// 1. GetRequestURL():
//    - ChannelBaseUrl = "https://your-resource.openai.azure.com"
//    - RequestURLPath = "/v1/chat/completions"
//    - ApiVersion = "2024-02-01"
//    - UpstreamModelName = "gpt-4"
//    - 构建: "/openai/deployments/gpt-4/chat/completions?api-version=2024-02-01"
//    - 返回: "https://your-resource.openai.azure.com/openai/deployments/gpt-4/chat/completions?api-version=2024-02-01"
```

**最终端点**: `https://your-resource.openai.azure.com/openai/deployments/gpt-4/chat/completions?api-version=2024-02-01`

#### 示例 3: 自定义渠道

**输入**:
- 客户端请求: `POST /v1/chat/completions`
- 渠道类型: `ChannelTypeCustom`
- 渠道 BaseURL: `https://my-proxy.com/v1/{model}/chat`
- 模型: `gpt-4`

**处理流程**:
```go
// 1. GetRequestURL():
//    - ChannelBaseUrl = "https://my-proxy.com/v1/{model}/chat"
//    - UpstreamModelName = "gpt-4"
//    - 替换 {model}: "https://my-proxy.com/v1/gpt-4/chat"
//    - 返回: "https://my-proxy.com/v1/gpt-4/chat"
```

**最终端点**: `https://my-proxy.com/v1/gpt-4/chat`

#### 示例 4: Claude API (通过 OpenAI 渠道)

**输入**:
- 客户端请求: `POST /v1/chat/completions` (使用 OpenAI 格式)
- 渠道类型: `ChannelTypeOpenAI`
- 渠道 BaseURL: `https://api.anthropic.com`
- RelayFormat: `RelayFormatClaude`

**处理流程**:
```go
// 1. GetRequestURL():
//    - ChannelBaseUrl = "https://api.anthropic.com"
//    - RequestURLPath = "/v1/chat/completions"
//    - RelayFormat = RelayFormatClaude
//    - 返回: "https://api.anthropic.com/v1/chat/completions"
// 2. 请求体转换: ClaudeHelper() 将 OpenAI 格式转换为 Claude 格式
//    - 路径仍然是 /v1/chat/completions，但请求体是 Claude 格式
```

**最终端点**: `https://api.anthropic.com/v1/chat/completions`

**注意**: 这里的 `/v1/chat/completions` 不是标准的 Claude API 路径（应该是 `/v1/messages`），这是为了兼容性设计的。

### 6. 特殊端点处理

#### 6.1 Cloudflare Gateway

**文件**: `relay/common/relay_utils.go:25`

```go
func GetFullRequestURL(baseURL string, requestURL string, channelType int) string {
    fullRequestURL := fmt.Sprintf("%s%s", baseURL, requestURL)

    if strings.HasPrefix(baseURL, "https://gateway.ai.cloudflare.com") {
        switch channelType {
        case constant.ChannelTypeOpenAI:
            // Cloudflare OpenAI Gateway 不需要 /v1 前缀
            fullRequestURL = fmt.Sprintf("%s%s", baseURL, strings.TrimPrefix(requestURL, "/v1"))
        case constant.ChannelTypeAzure:
            fullRequestURL = fmt.Sprintf("%s%s", baseURL, strings.TrimPrefix(requestURL, "/openai/deployments"))
        }
    }
    return fullRequestURL
}
```

**示例**:
- 输入: `baseURL="https://gateway.ai.cloudflare.com/v1/abc123"`, `requestURL="/v1/chat/completions"`
- 输出: `https://gateway.ai.cloudflare.com/v1/abc123/chat/completions`

#### 6.2 OpenAI Realtime (WebSocket)

**文件**: `relay/channel/openai/adaptor.go:111`

```go
if info.RelayMode == relayconstant.RelayModeRealtime {
    if strings.HasPrefix(info.ChannelBaseUrl, "https://") {
        baseUrl := strings.TrimPrefix(info.ChannelBaseUrl, "https://")
        baseUrl = "wss://" + baseUrl
        info.ChannelBaseUrl = baseUrl
    } else if strings.HasPrefix(info.ChannelBaseUrl, "http://") {
        baseUrl := strings.TrimPrefix(info.ChannelBaseUrl, "http://")
        baseUrl = "ws://" + baseUrl
        info.ChannelBaseUrl = baseUrl
    }
}
```

**示例**:
- 输入: `https://api.openai.com`
- 输出: `wss://api.openai.com/v1/realtime?deployment=gpt-4&api-version=2024-02-01`

---

## 适配器系统

### 适配器接口

**文件**: `relay/channel/adapter.go:15`

```go
type Adaptor interface {
    Init(info *relaycommon.RelayInfo)

    // 核心：获取请求 URL
    GetRequestURL(info *relaycommon.RelayInfo) (string, error)

    SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error

    // 请求格式转换
    ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error)
    ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error)
    ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error)

    // 发送请求
    DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error)

    // 处理响应
    DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError)

    GetModelList() []string
    GetChannelName() string
}
```

### 不同适配器的端点策略

| 适配器 | 渠道类型 | 端点策略 |
|--------|---------|----------|
| `openai.Adaptor` | OpenAI, OpenRouter, Xinference | 使用客户端请求路径 + BaseURL |
| `claude.Adaptor` | Anthropic | 固定使用 `/v1/chat/completions`（内部转换） |
| `gemini.Adaptor` | Gemini, Vertex AI | 使用客户端请求路径 + BaseURL |
| `aws.Adaptor` | Azure | 构建特殊的部署路径格式 |
| `submodel.Adaptor` | Submodel | 从模型配置中获取自定义端点 |

---

## 完整请求流程图

```
客户端请求
    ↓
POST /v1/chat/completions
    ↓
┌─────────────────────────────────────────────┐
│ 1. 路由匹配 (relay-router.go)              │
│    /v1/chat/completions → RelayFormatOpenAI │
└─────────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────────┐
│ 2. 中间件处理                               │
│    - TokenAuth() 令牌认证                   │
│    - Distribute() 渠道分发                  │
│    - ModelRequestRateLimit() 限流           │
└─────────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────────┐
│ 3. controller.Relay()                       │
│    relayFormat = RelayFormatOpenAI          │
└─────────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────────┐
│ 4. 请求解析与验证                           │
│    helper.GetAndValidateRequest()           │
│    → *dto.GeneralOpenAIRequest              │
└─────────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────────┐
│ 5. 生成 RelayInfo                           │
│    relaycommon.GenRelayInfo()               │
│    - RelayMode = RelayModeChatCompletions   │
│    - RequestURLPath = "/v1/chat/completions"│
└─────────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────────┐
│ 6. 渠道选择                                 │
│    service.CacheGetRandomSatisfiedChannel()  │
│    → channel{                               │
│         Type: ChannelTypeOpenAI,            │
│         BaseURL: "https://api.openai.com",  │
│         ApiType: APITypeOpenAI              │
│       }                                     │
└─────────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────────┐
│ 7. 适配器初始化                             │
│    adaptor := GetAdaptor(APITypeOpenAI)     │
│    → openai.Adaptor{}                       │
│    adaptor.Init(relayInfo)                  │
└─────────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────────┐
│ 8. 端点确定 ⭐                              │
│    adaptor.GetRequestURL(relayInfo)         │
│    → "https://api.openai.com/v1/chat/completions" │
└─────────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────────┐
│ 9. 请求头设置                               │
│    adaptor.SetupRequestHeader()             │
│    - Authorization: Bearer sk-xxx           │
│    - Content-Type: application/json         │
└─────────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────────┐
│ 10. 请求体转换                              │
│     adaptor.ConvertOpenAIRequest()          │
│     (OpenAI 格式通常不需要转换)              │
└─────────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────────┐
│ 11. 发送 HTTP 请求                          │
│     adaptor.DoRequest()                     │
│     POST https://api.openai.com/v1/chat/completions │
└─────────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────────┐
│ 12. 响应处理                                │
│     adaptor.DoResponse()                    │
│     - 流式响应处理                           │
│     - Token 统计                            │
│     - 用量计算                              │
└─────────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────────┐
│ 13. 计费与扣费                              │
│     postConsumeQuota()                      │
│     - 计算配额消耗                          │
│     - 扣除用户配额                          │
│     - 记录消费日志                          │
└─────────────────────────────────────────────┘
    ↓
返回响应给客户端
```

---

## 总结

### 端点确定的核心机制

端点确定遵循以下**优先级规则**：

1. **默认规则**（最常见）
   - 端点 = `渠道 BaseURL + 客户端请求路径`
   - 适用于: OpenAI、大部分兼容 OpenAI 的渠道

2. **渠道类型特殊规则**
   - **Azure**: `BaseURL + /openai/deployments/{model}/{path}?api-version={version}`
   - **Cloudflare**: 移除 `/v1` 前缀
   - **Custom**: 支持变量替换（`{model}` 等）

3. **RelayFormat 规则**
   - Claude/Gemini 格式：强制使用 `/v1/chat/completions`（即使客户端请求其他路径）
   - 请求体格式转换由适配器处理

4. **实时通信规则**
   - WebSocket: 将 `https://` 转换为 `wss://`
   - Realtime: 添加特殊查询参数

### 关键设计特点

1. **路径透明传递**
   - 默认情况下，客户端请求的路径会原样传递给上游
   - 这确保了最大兼容性

2. **渠道配置驱动**
   - 端点主要由渠道的 BaseURL 配置决定
   - 支持灵活的自定义端点

3. **适配器模式**
   - 每种渠道类型有自己的适配器
   - 适配器负责处理该渠道的特殊逻辑

4. **格式转换分离**
   - 端点确定与请求体格式转换是独立的
   - 可以使用 OpenAI 格式的路径，但发送 Claude 格式的请求体

### 实际应用建议

1. **配置 OpenAI 兼容渠道**
   ```
   BaseURL: https://api.openai.com
   → 自动使用客户端路径
   ```

2. **配置 Azure OpenAI**
   ```
   BaseURL: https://your-resource.openai.azure.com
   API Version: 2024-02-01
   → 自动构建 Azure 特定路径
   ```

3. **配置自定义代理**
   ```
   BaseURL: https://my-proxy.com/v1/{model}/chat
   → 自动替换模型名称
   ```

4. **配置不同格式转换**
   ```
   Channel Type: OpenAI
   BaseURL: https://api.anthropic.com
   RelayFormat: Claude
   → 使用 /v1/chat/completions 路径，但发送 Claude 格式请求体
   ```

---

**文档版本**: 1.0
**最后更新**: 2025-01-19
**作者**: AI Code Analysis
