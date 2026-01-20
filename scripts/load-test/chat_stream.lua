#!/usr/bin/env lua
-- wrk 压力测试脚本 - Streaming Chat Completions
-- 测试流式响应能力
--
-- 使用方法:
-- wrk -t 4 -c 50 -d 30s -s chat_stream.lua http://localhost:3000/v1/chat/completions

local math = require "math"
local os = require "os"

math.randomseed(os.time())

local test_cases = {
    {
        model = "gpt-3.5-turbo",
        messages = {
            {role = "user", content = "Tell me a short story."}
        },
        max_tokens = 100
    },
    {
        model = "gpt-3.5-turbo",
        messages = {
            {role = "user", content = "Count from 1 to 10."}
        },
        max_tokens = 50
    }
}

function get_random_request()
    local index = math.random(#test_cases)
    return test_cases[index]
end

function generate_json_body(test_case)
    local body = {
        model = test_case.model,
        messages = test_case.messages,
        max_tokens = test_case.max_tokens,
        temperature = 0.7,
        stream = true  -- 启用流式响应
    }

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
    json = json .. '"stream":true'
    json = json .. '}'

    return json
end

request = function()
    local test_case = get_random_request()
    local body = generate_json_body(test_case)
    local api_token = os.getenv("API_TOKEN") or "sk-test-token-12345"

    wrk.method = "POST"
    wrk.body = body
    wrk.headers["Content-Type"] = "application/json"
    wrk.headers["Authorization"] = "Bearer " .. api_token

    return wrk.format()
end
