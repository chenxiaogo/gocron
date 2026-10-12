// Package proxyx 提供全局出站代理能力（机场类 HTTP / SOCKS5 代理）。
// 配置来自 store 的 GlobalProxy，作用于：HTTP 任务执行、节点 Prometheus 指标抓取等出站请求。
package proxyx

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// TypeHTTP / TypeSOCKS5 支持的代理协议。
const (
	TypeHTTP   = "http"
	TypeSOCKS5 = "socks5"
)

// Config 代理配置。
type Config struct {
	Enabled  bool
	Type     string
	Host     string
	Port     int
	Username string
	Password string
}

// Valid 配置是否可用：启用且主机与端口合法。
func (c Config) Valid() bool {
	return c.Enabled && c.Host != "" && c.Port > 0
}

// Addr 代理地址 host:port。
func (c Config) Addr() string {
	return net.JoinHostPort(strings.TrimSpace(c.Host), strconv.Itoa(c.Port))
}

// FromURL 解析单条代理 URL 为配置，供节点级代理使用。
// 支持 http://user:pass@host:port 与 socks5://host:port；scheme 缺省按 http，端口缺省按协议默认。
// 解析失败或为空返回零值（Valid()==false），调用方据此回落默认/全局代理。
func FromURL(raw string) Config {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Config{}
	}
	if !strings.Contains(raw, "://") {
		raw = TypeHTTP + "://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return Config{}
	}
	typ := strings.ToLower(u.Scheme)
	if typ != TypeSOCKS5 {
		typ = TypeHTTP
	}
	port := 0
	if p := u.Port(); p != "" {
		port, _ = strconv.Atoi(p)
	}
	if port <= 0 {
		if typ == TypeSOCKS5 {
			port = 1080
		} else {
			port = 8080
		}
	}
	cfg := Config{Enabled: true, Type: typ, Host: u.Hostname(), Port: port}
	if u.User != nil {
		cfg.Username = u.User.Username()
		cfg.Password, _ = u.User.Password()
	}
	return cfg
}

// URL 代理 URL（含认证信息），供 http.Transport.Proxy 使用。
func (c Config) URL() *url.URL {
	scheme := c.Type
	if scheme == "" {
		scheme = TypeHTTP
	}
	u := &url.URL{Scheme: scheme, Host: c.Addr()}
	if c.Username != "" {
		u.User = url.UserPassword(c.Username, c.Password)
	}
	return u
}

// Transport 按配置生成 http.Transport；未启用时返回 nil（调用方用默认客户端）。
// http 类型交给标准库处理（自动加 Proxy-Authorization，HTTPS 走 CONNECT）；
// socks5 类型用自实现拨号器（因为依赖里没有 golang.org/x/net/proxy）。
func (c Config) Transport() *http.Transport {
	if !c.Valid() {
		return nil
	}
	if strings.EqualFold(c.Type, TypeSOCKS5) {
		return &http.Transport{DialContext: c.DialContext}
	}
	u := c.URL()
	return &http.Transport{
		// 内网/本机地址直连，避免机场代理无法回连内网节点
		Proxy: func(req *http.Request) (*url.URL, error) {
			if isPrivateHost(req.URL.Hostname()) {
				return nil, nil
			}
			return u, nil
		},
	}
}

// isPrivateHost 判断是否为内网/本机地址（走代理通常不可达，应直连）。
func isPrivateHost(host string) bool {
	h := strings.ToLower(strings.Trim(host, "[]"))
	if h == "" || h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		return true
	}
	// 100.64.0.0/10 运营商级 NAT
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 100 && ip4[1] >= 64 && ip4[1] < 128 {
		return true
	}
	return false
}

// Client 生成带代理的 HTTP 客户端；timeout<=0 表示不限制超时。
// 未启用代理时返回默认客户端。
func (c Config) Client(timeout time.Duration) *http.Client {
	t := c.Transport()
	if t == nil {
		return &http.Client{Timeout: timeout}
	}
	return &http.Client{Transport: t, Timeout: timeout}
}

// DialContext 经代理建立到 addr 的 TCP 连接（SOCKS5 场景使用）。
func (c Config) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, fmt.Errorf("proxy: 不支持的网络类型 %s", network)
	}
	if !c.Valid() {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	// 内网/本机地址直连
	if h, _, err := net.SplitHostPort(addr); err == nil && isPrivateHost(h) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	d := &net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", c.Addr())
	if err != nil {
		return nil, fmt.Errorf("连接代理 %s 失败: %w", c.Addr(), err)
	}
	if strings.EqualFold(c.Type, TypeSOCKS5) {
		if err := socks5Handshake(conn, addr, c.Username, c.Password); err != nil {
			conn.Close()
			return nil, err
		}
		return conn, nil
	}
	// HTTP 代理：手动 CONNECT（供非 Transport 场景使用）
	if err := httpConnect(conn, addr, c); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// httpConnect 通过 HTTP CONNECT 隧道建立连接。
func httpConnect(conn net.Conn, addr string, c Config) error {
	req := "CONNECT " + addr + " HTTP/1.1\r\nHost: " + addr + "\r\n"
	if c.Username != "" {
		raw := c.Username + ":" + c.Password
		req += "Proxy-Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(raw)) + "\r\n"
	}
	req += "\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		return err
	}
	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil {
		return err
	}
	line := string(buf[:n])
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return fmt.Errorf("代理响应异常: %q", line)
	}
	if code, _ := strconv.Atoi(parts[1]); code < 200 || code >= 300 {
		return fmt.Errorf("代理 CONNECT 失败: %s", strings.TrimSpace(line))
	}
	return nil
}

// socks5Handshake 完成 SOCKS5 协商（无认证 / 用户名密码）与 CONNECT 请求。
func socks5Handshake(conn net.Conn, addr, user, pass string) error {
	// 1) 协商方法：有账号则用 USERNAME/PASSWORD(0x02)，否则也带上 0x02 备选
	var methods []byte
	if user != "" {
		methods = []byte{0x02}
	} else {
		methods = []byte{0x00, 0x02}
	}
	greet := append([]byte{0x05, byte(len(methods))}, methods...)
	if _, err := conn.Write(greet); err != nil {
		return err
	}
	resp := make([]byte, 2)
	if _, err := readFull(conn, resp); err != nil {
		return err
	}
	if resp[0] != 0x05 {
		return fmt.Errorf("SOCKS5 版本不匹配: 0x%02x", resp[0])
	}
	switch resp[1] {
	case 0x02:
		if user == "" {
			return fmt.Errorf("代理要求用户名密码认证但未配置账号")
		}
		if err := socks5Auth(conn, user, pass); err != nil {
			return err
		}
	case 0x00:
		// 无需认证
	default:
		return fmt.Errorf("SOCKS5 不支持的认证方式: 0x%02x", resp[1])
	}

	// 2) CONNECT 请求
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("解析目标地址失败: %w", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("解析目标端口失败: %w", err)
	}
	req := []byte{0x05, 0x01, 0x00, 0x03, byte(len(host))}
	req = append(req, []byte(host)...)
	req = append(req, byte(port>>8), byte(port&0xff))
	if _, err := conn.Write(req); err != nil {
		return err
	}

	// 3) 读取响应：VER REP RSV ATYP [BND.ADDR BND.PORT]
	head := make([]byte, 4)
	if _, err := readFull(conn, head); err != nil {
		return err
	}
	if head[1] != 0x00 {
		return fmt.Errorf("SOCKS5 CONNECT 失败，REP=0x%02x", head[1])
	}
	var tail []byte
	switch head[3] {
	case 0x01: // IPv4
		tail = make([]byte, 4+2)
	case 0x03: // 域名
		l := make([]byte, 1)
		if _, err := readFull(conn, l); err != nil {
			return err
		}
		tail = make([]byte, int(l[0])+2)
	case 0x04: // IPv6
		tail = make([]byte, 16+2)
	default:
		return fmt.Errorf("SOCKS5 未知地址类型: 0x%02x", head[3])
	}
	_, err = readFull(conn, tail)
	return err
}

// socks5Auth 用户名密码子协商（RFC1929）。
func socks5Auth(conn net.Conn, user, pass string) error {
	req := []byte{0x01, byte(len(user))}
	req = append(req, []byte(user)...)
	req = append(req, byte(len(pass)))
	req = append(req, []byte(pass)...)
	if _, err := conn.Write(req); err != nil {
		return err
	}
	resp := make([]byte, 2)
	if _, err := readFull(conn, resp); err != nil {
		return err
	}
	if resp[1] != 0x00 {
		return fmt.Errorf("SOCKS5 认证失败，STATUS=0x%02x", resp[1])
	}
	return nil
}

// readFull 读满 buf（SOCKS5 报文小，逐字节读足够）。
func readFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
