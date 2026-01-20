#!/usr/bin/env lua
-- wrk 压力测试脚本 - Chat Completions API
-- 测试文本生成能力
--
-- 使用方法:
-- wrk -t 4 -c 100 -d 30s -s chat_completions.lua http://localhost:3000/v1/chat/completions

local math = require "math"
local os = require "os"

-- 初始化随机种子
math.randomseed(os.time())

-- 测试用例列表
local test_cases = {
    {
        model = "gpt-3.5-turbo",
        messages = {
            {role = "system", content = "You are a helpful assistant."},
            {role = "user", content = "Hello!"}
        },
        max_tokens = 50
    },
    {
        model = "gpt-3.5-turbo",
        messages = {
            {role = "system", content = "You are a helpful assistant."},
            {role = "user", content = "Explain quantum computing in simple terms."}
        },
        max_tokens = 100
    },
    {
        model = "gpt-4",
        messages = {
            {role = "user", content = "What is the capital of France?"}
        },
        max_tokens = 30
    },
    {
        model = "gpt-3.5-turbo",
        messages = {
            {role = "system", content = "You are a code assistant."},
            {role = "user", content = "Write a Python function to calculate fibonacci numbers."}
        },
        max_tokens = 150
    }
}

-- 从测试用例中随机选择一个
function get_random_request()
    local index = math.random(#test_cases)
    return test_cases[index]
end

-- 生成 JSON 格式的请求体
function generate_json_body(test_case)
    local body = {
        model = test_case.model,
        messages = test_case.messages,
        max_tokens = test_case.max_tokens,
        temperature = 0.7,
        stream = false
    }

    -- 简单的 JSON 序列化
    local json = '{'
    json = json .. '"model":"' .. body.model .. '",'
    json = json .. '"messages":['

    for i, msg in ipairs(body.messages) do
        json = json .. '{"role":"' .. msg.role .. '","content":"' .. msg.content .. '"}'
        if i < #body.messages then
            json = json .. ','
        end
    end

    json = json .. '],'
    json = json .. '"max_tokens":' .. body.max_tokens .. ','
    json = json .. '"temperature":' .. body.temperature .. ','
    json = json .. '"stream":false'
    json = json .. '}'

    return json
end

-- wrk 请求回调
request = function()
    local test_case = get_random_request()
    local body = generate_json_body(test_case)

    -- 从环境变量或命令行获取 API Token
    local api_token = os.getenv("API_TOKEN") or "sk-test-token-12345"

    -- 构建请求
    wrk.method = "POST"
    wrk.body = body
    wrk.headers["Content-Type"] = "application/json"
    wrk.headers["Authorization"] = "Bearer " .. api_token

    return wrk.format(nil, nil)
end

-- 响应处理 (可选)
response = function(status, headers, body)
    -- 可以在这里记录响应数据
    -- if status == 200 then
    --     print("Success: " .. body)
    -- end
end

-- 请求完成时的处理 (可选)
done = function(summary, latency, requests)
    -- latency: min, max, mean, stdev, percentile
    print("\n=== Latency Distribution ===")
    print("50%: " .. latency:percentile(50) .. " ms")
    print("75%: " .. latency:percentile(75) .. " ms")
    print("90%: " .. latency:percentile(90) .. " ms")
    print("99%: " .. latency:percentile(99) .. " ms")
end
