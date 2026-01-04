package bili

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// ProxyInfo 代理信息
type ProxyInfo struct {
	URL     string        // 代理URL
	Limiter *rate.Limiter // 每个代理独立的限流器
	mu      sync.Mutex
}

// ProxyPool 代理池
type ProxyPool struct {
	proxies      []*ProxyInfo
	current      int
	mu           sync.Mutex
	localLimiter *rate.Limiter // 本地IP的限流器
}

// NewProxyPool 创建代理池
// proxyURLs: 代理URL列表，格式：socks5://ip:port 或 http://user:pass@ip:port
func NewProxyPool(proxyURLs []string) *ProxyPool {
	pool := &ProxyPool{
		proxies:      make([]*ProxyInfo, 0),
		current:      0,
		localLimiter: rate.NewLimiter(rate.Every(22*time.Second), 1), // 本地IP限流：22秒1条
	}

	// 添加本地IP（nil表示不使用代理）
	pool.proxies = append(pool.proxies, &ProxyInfo{
		URL:     "", // 空字符串表示本地IP
		Limiter: pool.localLimiter,
	})

	// 添加代理IP，每个代理独立限流
	for _, proxyURL := range proxyURLs {
		proxyURL = strings.TrimSpace(proxyURL)
		if proxyURL == "" {
			continue
		}

		// 验证代理URL格式
		if _, err := url.Parse(proxyURL); err != nil {
			log.Printf("[代理池] ⚠️ 无效的代理URL: %s, 错误: %v", proxyURL, err)
			continue
		}

		pool.proxies = append(pool.proxies, &ProxyInfo{
			URL:     proxyURL,
			Limiter: rate.NewLimiter(rate.Every(22*time.Second), 1), // 每个代理独立限流：22秒1条
		})
		log.Printf("[代理池] ✓ 添加代理: %s", proxyURL)
	}

	log.Printf("[代理池] 初始化完成，共 %d 个IP (包含本地IP)", len(pool.proxies))
	return pool
}

// GetNextProxy 轮询获取下一个代理
func (p *ProxyPool) GetNextProxy() *ProxyInfo {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.proxies) == 0 {
		return nil
	}

	proxy := p.proxies[p.current]
	p.current = (p.current + 1) % len(p.proxies)
	return proxy
}

// WaitDanmaku 等待代理的弹幕限流器
func (p *ProxyInfo) WaitDanmaku() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.Limiter.Wait(context.Background())
}

// GetProxyURL 获取代理URL（如果为空则返回nil表示不使用代理）
func (p *ProxyInfo) GetProxyURL() string {
	return p.URL
}

// IsLocal 是否为本地IP
func (p *ProxyInfo) IsLocal() bool {
	return p.URL == ""
}

// String 返回代理的字符串表示
func (p *ProxyInfo) String() string {
	if p.IsLocal() {
		return "本地IP"
	}
	return p.URL
}

// GetProxyCount 获取代理数量（包含本地IP）
func (p *ProxyPool) GetProxyCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.proxies)
}

// ParseProxyList 解析代理列表字符串
func ParseProxyList(proxyList string) []string {
	if proxyList == "" {
		return []string{}
	}

	lines := strings.Split(proxyList, "\n")
	proxies := make([]string, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 验证代理URL格式
		if !strings.HasPrefix(line, "socks5://") &&
			!strings.HasPrefix(line, "http://") &&
			!strings.HasPrefix(line, "https://") {
			log.Printf("[代理池] ⚠️ 无效的代理格式（需要 socks5:// 或 http(s)://）: %s", line)
			continue
		}

		proxies = append(proxies, line)
	}

	return proxies
}

// ValidateProxy 验证代理URL格式
func ValidateProxy(proxyURL string) error {
	if proxyURL == "" {
		return nil // 空字符串表示本地IP
	}

	u, err := url.Parse(proxyURL)
	if err != nil {
		return fmt.Errorf("无效的URL格式: %w", err)
	}

	scheme := u.Scheme
	if scheme != "socks5" && scheme != "http" && scheme != "https" {
		return fmt.Errorf("不支持的代理协议: %s (仅支持 socks5, http, https)", scheme)
	}

	if u.Host == "" {
		return fmt.Errorf("代理地址不能为空")
	}

	return nil
}
