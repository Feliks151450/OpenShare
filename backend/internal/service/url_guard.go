package service

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// SSRF 防护：统一校验服务端代为发起的出站请求目标。
//
// 两级策略：
//   - strict：探测类功能（probe-url、虚拟文件 HEAD），拒绝一切非公网地址；
//   - proxy：代理下载（proxy_source_url 的设计目标就是拉取内网存储），仅拒绝
//     loopback / link-local（含 169.254.169.254 云元数据）与未指定地址，保留
//     RFC1918 内网段。
//
// 校验在 URL 解析、DNS 解析和实际建连（Dialer.Control，防 DNS rebinding）三层生效。

var errSSRFBlocked = fmt.Errorf("target resolves to a blocked address")

type ssrfMode int

const (
	// SSRFModeStrict 探测类功能：拒绝一切内网/环回/链路本地地址。
	SSRFModeStrict ssrfMode = iota
	// SSRFModeProxy 代理下载：保留 RFC1918 内网段（产品功能），拒绝环回与链路本地。
	SSRFModeProxy
)

func validatePublicIPForSSRF(ip net.IP, mode ssrfMode) error {
	if ip == nil {
		return errSSRFBlocked
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return errSSRFBlocked
	}
	if ip4 := ip.To4(); ip4 != nil {
		// 0.0.0.0/8、"this network" 段
		if ip4[0] == 0 {
			return errSSRFBlocked
		}
		// 100.64.0.0/10（CGNAT / 常见云内网桥段）
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] < 128 {
			return errSSRFBlocked
		}
		// proxy 模式保留 RFC1918（内网存储代理是产品功能）；strict 模式全拒绝
		if mode == SSRFModeStrict && ip.IsPrivate() {
			return errSSRFBlocked
		}
	}
	return nil
}

// validateSSRFHost 校验主机名：字面 IP 直接判定；域名解析后逐个 IP 判定。
func validateSSRFHost(hostPort string, mode ssrfMode) error {
	host := hostPort
	if h, _, err := net.SplitHostPort(hostPort); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return errSSRFBlocked
	}
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") || lower == "metadata.google.internal" {
		return errSSRFBlocked
	}
	if ip := net.ParseIP(host); ip != nil {
		return validatePublicIPForSSRF(ip, mode)
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("resolve %q: %w", host, err)
	}
	if len(ips) == 0 {
		return errSSRFBlocked
	}
	for _, ip := range ips {
		if err := validatePublicIPForSSRF(ip, mode); err != nil {
			return err
		}
	}
	return nil
}

// validateSSRFURL 校验 URL 的 scheme 与目标主机。每跳重定向都会重新调用。
func validateSSRFURL(rawURL string, mode ssrfMode) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return errSSRFBlocked
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errSSRFBlocked
	}
	return validateSSRFHost(parsed.Host, mode)
}

// NewSSRFFetchClient 返回带 SSRF 防护的 HTTP 客户端：
//   - CheckRedirect 对每一跳重新执行目标校验；
//   - Dialer.Control 在实际建连时再次校验对端 IP，防止 DNS rebinding 绕过。
func NewSSRFFetchClient(mode ssrfMode, timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return errSSRFBlocked
			}
			ip := net.ParseIP(strings.Trim(host, "[]"))
			if ip == nil {
				return errSSRFBlocked
			}
			return validatePublicIPForSSRF(ip, mode)
		},
	}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		MaxIdleConns:          10,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("stopped after 5 redirects")
			}
			if err := validateSSRFURL(req.URL.String(), mode); err != nil {
				return err
			}
			return nil
		},
	}
}

// ValidateRemoteURL 快速校验远程 URL 可否被服务端访问（探测类功能用 strict 模式）。
func ValidateRemoteURL(rawURL string) error {
	return validateSSRFURL(rawURL, SSRFModeStrict)
}

// ValidateProxyURL 校验代理下载源地址（proxy 模式：允许内网段，拒绝环回/链路本地）。
func ValidateProxyURL(rawURL string) error {
	return validateSSRFURL(rawURL, SSRFModeProxy)
}
