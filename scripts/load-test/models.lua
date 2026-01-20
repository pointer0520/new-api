#!/usr/bin/env lua
-- wrk 压力测试脚本 - Models API (只读接口)
-- 测试模型列表查询能力
--
-- 使用方法:
-- wrk -t 4 -c 100 -d 30s -s models.lua http://localhost:3000/v1/models

local os = require "os"

request = function()
    local api_token = os.getenv("API_TOKEN") or "sk-test-token-12345"

    wrk.method = "GET"
    wrk.headers["Authorization"] = "Bearer " .. api_token

    return wrk.format()
end
