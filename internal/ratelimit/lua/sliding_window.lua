-- Sliding window rate limiter using Redis sorted sets.
--
-- This script runs atomically in Redis (via EVAL/EVALSHA), guaranteeing that
-- the check-and-increment operation cannot race with concurrent requests.
--
-- Algorithm:
--   1. Remove all entries outside the current sliding window
--   2. Count remaining entries (= requests made in the window)
--   3. If under the limit, add a new entry and refresh the TTL
--   4. Return 1 (allowed) or 0 (rejected)
--
-- KEYS[1] = rate limit key (e.g. "rl:sub:{subscription_id}")
-- ARGV[1] = max requests allowed in window
-- ARGV[2] = window size in milliseconds
-- ARGV[3] = current timestamp in milliseconds

local key    = KEYS[1]
local limit  = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local now    = tonumber(ARGV[3])

-- Remove entries outside the sliding window.
-- This keeps the sorted set bounded and ensures we only count recent requests.
redis.call('ZREMRANGEBYSCORE', key, 0, now - window)

-- Count current entries in window.
local count = redis.call('ZCARD', key)

if count < limit then
    -- Add current timestamp as both score and member.
    -- Append a random suffix to the member to prevent collisions when
    -- multiple requests arrive within the same millisecond. Without this,
    -- two requests at the same ms would be de-duped by Redis ZADD.
    redis.call('ZADD', key, now, tostring(now) .. ':' .. tostring(math.random(1000000)))
    -- Set TTL equal to the window so keys auto-expire if traffic stops.
    redis.call('PEXPIRE', key, window)
    return 1  -- allowed
else
    return 0  -- rejected
end
