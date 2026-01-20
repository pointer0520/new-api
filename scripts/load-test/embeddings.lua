#!/usr/bin/env lua
-- wrk 压力测试脚本 - Embeddings API
-- 测试向量嵌入能力
--
-- 使用方法:
-- wrk -t 4 -c 100 -d 30s -s embeddings.lua http://localhost:3000/v1/embeddings

local math = require "math"
local os = require "os"

math.randomseed(os.time())

-- 测试文本列表
local test_texts = {
    "Hello, world!",
    "The quick brown fox jumps over the lazy dog.",
    "Artificial intelligence is transforming the world.",
    "Machine learning models can process large amounts of data.",
    "Natural language processing enables computers to understand text.",
    "Deep learning neural networks are inspired by the human brain.",
    "The weather today is sunny with a chance of rain.",
    "Programming requires logical thinking and problem-solving skills.",
    "Quantum computing could revolutionize computational power.",
    "Space exploration continues to push the boundaries of human knowledge."
}

function get_random_text()
    local index = math.random(#test_texts)
    return test_texts[index]
end

function generate_json_body()
    local text = get_random_text()
    local body = string.format(
        '{"model":"text-embedding-ada-002","input":"%s","encoding_format":"float"}',
        text:gsub('"', '\\"')
    )
    return body
end

request = function()
    local body = generate_json_body()
    local api_token = os.getenv("API_TOKEN") or "sk-test-token-12345"

    wrk.method = "POST"
    wrk.body = body
    wrk.headers["Content-Type"] = "application/json"
    wrk.headers["Authorization"] = "Bearer " .. api_token

    return wrk.format()
end
