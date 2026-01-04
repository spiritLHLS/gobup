package bili

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// ProxyInfo 代理信息
type ProxyInfo struct {
	URL       string        // 代理URL
	Limiter   *rate.Limiter // 每个代理独立的限流器
	mu        sync.Mutex
	Available bool // 代理是否可用
	LastCheck time.Time
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
		URL:       "", // 空字符串表示本地IP
		Limiter:   pool.localLimiter,
		Available: true,
		LastCheck: time.Now(),
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

		proxyInfo := &ProxyInfo{
			URL:       proxyURL,
			Limiter:   rate.NewLimiter(rate.Every(22*time.Second), 1), // 每个代理独立限流：22秒1条
			Available: true,                                           // 初始标记为可用
			LastCheck: time.Time{},                                    // 等待首次检查
		}

		// 异步检查代理可用性
		go pool.checkProxyHealth(proxyInfo)

		pool.proxies = append(pool.proxies, proxyInfo)
		log.Printf("[代理池] ✓ 添加代理: %s (等待健康检查...)", proxyURL)
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

// checkProxyHealth 检查代理的健康状态
func (p *ProxyPool) checkProxyHealth(proxyInfo *ProxyInfo) {
	if proxyInfo.IsLocal() {
		return // 本地IP不需要检查
	}

	u, err := url.Parse(proxyInfo.URL)
	if err != nil {
		log.Printf("[代理池] ❌ 代理URL解析失败 %s: %v", proxyInfo.URL, err)
		proxyInfo.mu.Lock()
		proxyInfo.Available = false
		proxyInfo.LastCheck = time.Now()
		proxyInfo.mu.Unlock()
		return
	}

	// 提取主机和端口
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		// 根据协议设置默认端口
		switch u.Scheme {
		case "socks5", "socks5h":
			port = "1080"
		case "http", "https":
			port = "8080"
		default:
			port = "1080"
		}
	}

	// 尝试建立TCP连接（5秒超时）
	addr := net.JoinHostPort(host, port)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)

	proxyInfo.mu.Lock()
	defer proxyInfo.mu.Unlock()

	if err != nil {
		proxyInfo.Available = false
		proxyInfo.LastCheck = time.Now()
		log.Printf("[代理池] ❌ 代理不可达 %s (%s): %v", proxyInfo.URL, addr, err)
	} else {
		conn.Close()
		proxyInfo.Available = true
		proxyInfo.LastCheck = time.Now()
		log.Printf("[代理池] ✓ 代理可用 %s (%s)", proxyInfo.URL, addr)
	}
}

// GetNextAvailableProxy 获取下一个可用的代理（跳过不可用的）
func (p *ProxyPool) GetNextAvailableProxy() *ProxyInfo {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.proxies) == 0 {
		return nil
	}

	// 最多尝试所有代理一遍
	attempts := len(p.proxies)
	for i := 0; i < attempts; i++ {
		proxy := p.proxies[p.current]
		p.current = (p.current + 1) % len(p.proxies)

		// 检查是否可用
		proxy.mu.Lock()
		available := proxy.Available
		lastCheck := proxy.LastCheck
		proxy.mu.Unlock()

		// 如果是本地IP，直接返回
		if proxy.IsLocal() {
			return proxy
		}

		// 如果代理可用，返回
		if available {
			return proxy
		}

		// 如果超过5分钟没检查，重新检查
		if time.Since(lastCheck) > 5*time.Minute {
			go p.checkProxyHealth(proxy)
		}
	}

	// 如果所有代理都不可用，返回本地IP
	log.Printf("[代理池] ⚠️ 所有代理都不可用，使用本地IP")
	return p.proxies[0] // 第一个总是本地IP
}
