local key = KEYS[1]

local now = tonumber(ARGV[1])
local windowSize = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])

local ttl = tonumber(ARGV[4])

local data = redis.call('GET', key)

local state

-- fresh client: place them in the window they're arriving in, no history yet
if not data then
    state = {windowID = math.floor(now / windowSize), prevCount = 0, currCount = 0}
else
    state = cjson.decode(data)
end

local currWindowID = math.floor(now / windowSize)

-- rollover: state.windowID is from whenever this client last hit us, 
-- currWindowID is computed fresh from "now" - if they differ,
-- real time has moved into a new window since the last request. 
-- shift what was "current" into "previous" and start this window's count at zero.
if state.windowID ~= currWindowID then
    state.windowID = currWindowID
    state.prevCount = state.currCount
    state.currCount = 0
end

local elapsed = now - (state.windowID * windowSize)

-- weight = what fraction of the previous window is still "inside" the last
local weight = (windowSize - elapsed) / windowSize

-- weighted estimate of requests within the true last-windowSize-seconds span
local estimatedCount = weight * state.prevCount + state.currCount

local allowed = 0

if estimatedCount < limit then
    state.currCount = state.currCount + 1
    allowed = 1
end

-- recompute post-increment so "remaining" reflects this request, same as
-- redisbucket.lua returning tokens after decrementing
estimatedCount = math.floor(weight * state.prevCount + state.currCount)

local serializedState = cjson.encode(state)

redis.call('SET', key, serializedState, 'EX', ttl)

-- RESP truncates floats returned from Lua to integers, so floor explicitly on line 49
return {allowed, limit - estimatedCount}