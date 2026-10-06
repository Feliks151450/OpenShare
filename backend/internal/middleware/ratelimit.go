package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/config"
)

// RateLimit 基于 IP 的固定窗口限流。
// 配置来源于现有 RateLimitConfig（upload/search 两组规则），此前仅存在于配置中
// 未被任何中间件消费；本实现补齐消费端，覆盖登录、访客密钥校验、上传、搜索与
// 访问打点等公开写入口。
//
// 实现说明：内存态、单进程有效（与 SQLite 单实例部署模型一致）；不启动后台
// goroutine，过期条目在请求路径上惰性清理（桶数量超阈值时批量清理一次）。

type rateLimitBucket struct {
	windowStart time.Time
	count       int
}

type rateLimiter struct {
	mu          sync.Mutex
	limit       int
	window      time.Duration
	buckets     map[string]*rateLimitBucket
	lastCleanup time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		limit:   limit,
		window:  window,
		buckets: make(map[string]*rateLimitBucket),
	}
}

// allow 返回是否放行；桶数量超过阈值且距上次清理超过一个窗口时，批量删除过期桶。
func (rl *rateLimiter) allow(key string, now time.Time) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if len(rl.buckets) > 4096 && now.Sub(rl.lastCleanup) >= rl.window {
		for k, b := range rl.buckets {
			if now.Sub(b.windowStart) >= rl.window {
				delete(rl.buckets, k)
			}
		}
		rl.lastCleanup = now
	}

	b, ok := rl.buckets[key]
	if !ok || now.Sub(b.windowStart) >= rl.window {
		rl.buckets[key] = &rateLimitBucket{windowStart: now, count: 1}
		return true
	}
	if b.count >= rl.limit {
		return false
	}
	b.count++
	return true
}

// RateLimit 按规则构造限流中间件；规则未启用时返回直通中间件。
func RateLimit(class string, rule config.RateLimitRule) gin.HandlerFunc {
	if !rule.Enabled || rule.Limit <= 0 || rule.Window <= 0 {
		return func(c *gin.Context) { c.Next() }
	}
	rl := newRateLimiter(rule.Limit, time.Duration(rule.Window)*time.Second)
	return func(c *gin.Context) {
		if !rl.allow(c.ClientIP(), time.Now()) {
			c.Header("Retry-After", strconv.Itoa(rule.Window))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			return
		}
		c.Next()
	}
}
