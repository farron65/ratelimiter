local key = KEYS[1]

local now = tonumber(ARGV[1])
local maxTokens = tonumber(ARGV[2])
local refillRate = tonumber(ARGV[3])
local ttl = tonumber(ARGV[4])

local data = redis.call('GET', key)

local state 

if not data then
    state = {tokens = maxTokens, lastRefillTime = now}

else 
    state = cjson.decode(data)
end

local elapsedTime = now - state.lastRefillTime
local tokensToAdd = elapsedTime * refillRate

state.tokens = math.min(maxTokens, state.tokens + tokensToAdd)
state.lastRefillTime = now

local allowed = 0

if state.tokens >= 1 then
    state.tokens = state.tokens - 1
    allowed = 1
end

local serializedState = cjson.encode(state)

redis.call('SET', key, serializedState, 'EX', ttl)

-- x100 to keep 2 decimals through RESP truncation; divided back in redisbucket.go 
return {allowed, math.floor(state.tokens * 100)} 