package middleware

import (
	"sync"

	"github.com/gofiber/fiber/v2"

	"github.com/nebu-25/MVdownloderS_0709/internal/model"
)

type ipLimiter struct {
	mu     sync.Mutex
	active map[string]int
	limit  int
}

type concurrencyLease struct {
	once    sync.Once
	release func()
	claimed bool
}

const concurrencyLeaseKey = "download-concurrency-lease"

func ConcurrentByIP(limit int) fiber.Handler {
	limiter := &ipLimiter{active: make(map[string]int), limit: limit}
	return func(c *fiber.Ctx) error {
		ip := c.IP()
		if !limiter.acquire(ip) {
			return c.Status(fiber.StatusTooManyRequests).JSON(model.ErrorBody{
				Error: model.ErrorDetail{
					Code: "RATE_LIMITED", Message: "동시에 처리할 수 있는 다운로드 수를 초과했습니다.",
				},
			})
		}
		lease := &concurrencyLease{release: func() { limiter.release(ip) }}
		c.Locals(concurrencyLeaseKey, lease)
		err := c.Next()
		if !lease.claimed {
			lease.Release()
		}
		return err
	}
}

// ClaimConcurrency transfers release responsibility to a streaming handler.
func ClaimConcurrency(c *fiber.Ctx) func() {
	lease, ok := c.Locals(concurrencyLeaseKey).(*concurrencyLease)
	if !ok {
		return func() {}
	}
	lease.claimed = true
	return lease.Release
}

func (l *concurrencyLease) Release() {
	l.once.Do(l.release)
}

func (l *ipLimiter) acquire(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.active[ip] >= l.limit {
		return false
	}
	l.active[ip]++
	return true
}

func (l *ipLimiter) release(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.active[ip]--
	if l.active[ip] <= 0 {
		delete(l.active, ip)
	}
}
