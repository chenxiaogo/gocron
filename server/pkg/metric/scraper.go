package metric

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ScrapeResult 一次 Prometheus 指标抓取结果。
type ScrapeResult struct {
	CPUUsage    float64 `json:"cpuUsage"`    // CPU 使用率 0-100
	MemoryUsage float64 `json:"memoryUsage"` // 内存使用率 0-100
	DiskUsage   float64 `json:"diskUsage"`   // 磁盘使用率 0-100
	LoadAverage float64 `json:"loadAverage"` // 系统负载/goroutine 数
	RawMetrics  int     `json:"rawMetrics"`  // 解析到的指标总数
	Error       string  `json:"error"`       // 错误信息
}

// prevSample 记录上一次抓取的 CPU counter 值，用于计算速率。
type prevSample struct {
	cpuSeconds float64
	at         time.Time
}

var (
	prevMu   sync.Mutex
	prevMap  = make(map[string]*prevSample) // url -> previous sample
)

// ScrapePrometheus 从指定 URL 抓取 Prometheus 文本格式指标，转换为节点指标。
// 支持 node_exporter 标准指标，也兼容 Go 运行时指标（process_* / go_memstats_*）。
func ScrapePrometheus(url string, timeout time.Duration) *ScrapeResult {
	return ScrapePrometheusWith(url, timeout, nil)
}

// ScrapePrometheusWith 使用指定 HTTP 客户端抓取（client 为 nil 时用默认客户端）。
// 传入带代理的客户端即可让抓取走全局出站代理。
func ScrapePrometheusWith(url string, timeout time.Duration, client *http.Client) *ScrapeResult {
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	resp, err := client.Get(url)
	if err != nil {
		return &ScrapeResult{Error: fmt.Sprintf("请求失败: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &ScrapeResult{Error: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}

	metrics, err := parsePrometheusText(resp.Body)
	if err != nil {
		return &ScrapeResult{Error: fmt.Sprintf("解析失败: %v", err)}
	}

	r := &ScrapeResult{RawMetrics: len(metrics)}

	// CPU：优先 node_exporter，其次 Go 进程 CPU（需两次采样做差分）
	r.CPUUsage = extractCPU(metrics, url)

	// 内存：优先 node_exporter MemAvailable/MemTotal，其次 Go 进程 RSS/Sys
	r.MemoryUsage = extractMemory(metrics)

	// 磁盘：仅 node_exporter 有
	r.DiskUsage = extractDisk(metrics)

	// 负载：优先 node_load1，其次 go_goroutines 作为负载参考
	r.LoadAverage = extractLoad(metrics, url)

	return r
}

// parsePrometheusText 解析 Prometheus exposition 格式文本，返回 metric_name -> value 映射。
func parsePrometheusText(r io.Reader) (map[string]float64, error) {
	out := make(map[string]float64)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, val, ok := parseMetricLine(line)
		if ok {
			out[name] = val
		}
	}
	return out, scanner.Err()
}

// parseMetricLine 解析一行 Prometheus 指标数据。
func parseMetricLine(line string) (string, float64, bool) {
	braceIdx := strings.IndexByte(line, '{')
	spaceIdx := strings.IndexByte(line, ' ')

	var name string
	var rest string

	if braceIdx >= 0 && braceIdx < spaceIdx {
		name = line[:braceIdx]
		closeBrace := strings.LastIndexByte(line, '}')
		if closeBrace < 0 {
			return "", 0, false
		}
		rest = strings.TrimSpace(line[closeBrace+1:])
	} else if spaceIdx > 0 {
		name = line[:spaceIdx]
		rest = strings.TrimSpace(line[spaceIdx+1:])
	} else {
		return "", 0, false
	}

	parts := strings.Fields(rest)
	if len(parts) == 0 {
		return "", 0, false
	}

	val, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return "", 0, false
	}

	return name, val, true
}

// extractCPU 从指标中提取 CPU 使用率（0-100）。
// node_exporter: 用 node_cpu_seconds_total 计算非 idle 占比。
// Go 运行时: 用 process_cpu_seconds_total 两次采样做差分计算速率。
func extractCPU(metrics map[string]float64, url string) float64 {
	// 1. node_exporter: 计算 1 - idle/total
	var totalCPU, idleCPU float64
	for k, v := range metrics {
		if strings.HasPrefix(k, "node_cpu_seconds_total") {
			totalCPU += v
			if strings.Contains(k, `mode="idle"`) {
				idleCPU += v
			}
		}
	}
	if totalCPU > 0 {
		return math.Round((1-idleCPU/totalCPU)*10000) / 100
	}

	// 2. Go 运行时: process_cpu_seconds_total 差分计算 CPU 使用率
	cpuSec, ok := metrics["process_cpu_seconds_total"]
	if !ok || cpuSec <= 0 {
		return 0
	}

	now := time.Now()
	prevMu.Lock()
	prev, exists := prevMap[url]
	prevMap[url] = &prevSample{cpuSeconds: cpuSec, at: now}
	prevMu.Unlock()

	if !exists || prev.cpuSeconds <= 0 {
		return 0 // 首次采样，暂无差分
	}

	deltaSec := cpuSec - prev.cpuSeconds
	deltaTime := now.Sub(prev.at).Seconds()
	if deltaTime <= 0 {
		return 0
	}

	// CPU 使用率 = delta_cpu_seconds / delta_time * 100
	// 注：process_cpu_seconds_total 是单核累计，需除以 GOMAXPROCS 得到整体占比
	// 但用户通常想看进程占用的 CPU 核心数百分比，这里直接按单核比计算
	pct := (deltaSec / deltaTime) * 100
	if pct > 100 {
		pct = 100
	}
	return math.Round(pct*100) / 100
}

// extractMemory 从指标中提取内存使用率（0-100）。
// node_exporter: MemAvailable / MemTotal。
// Go 运行时: process_resident_memory_bytes / go_memstats_sys_bytes。
func extractMemory(metrics map[string]float64) float64 {
	// 1. node_exporter
	total, hasTotal := metrics["node_memory_MemTotal_bytes"]
	avail, hasAvail := metrics["node_memory_MemAvailable_bytes"]
	if hasTotal && hasAvail && total > 0 {
		return math.Round((1-avail/total)*10000) / 100
	}

	// 2. Go 运行时: RSS / 系统分配内存
	rss, hasRSS := metrics["process_resident_memory_bytes"]
	sysBytes, hasSys := metrics["go_memstats_sys_bytes"]
	if hasRSS && hasSys && sysBytes > 0 && rss > 0 {
		// RSS 是实际物理内存占用，sys_bytes 是 Go 向 OS 申请的总量
		return math.Round((rss/sysBytes)*10000) / 100
	}

	// 3. Go 运行时降级: heap_alloc / sys_bytes
	heapAlloc, hasHeap := metrics["go_memstats_heap_alloc_bytes"]
	if hasHeap && hasSys && sysBytes > 0 && heapAlloc > 0 {
		return math.Round((heapAlloc/sysBytes)*10000) / 100
	}

	return 0
}

// extractDisk 从指标中提取磁盘使用率（0-100），仅 node_exporter 支持。
// 优先取根分区 / 的使用率。
func extractDisk(metrics map[string]float64) float64 {
	// 收集所有 mountpoint 的 size 和 avail
	type fsInfo struct {
		size  float64
		avail float64
	}
	fsMap := make(map[string]*fsInfo)

	for k, v := range metrics {
		var mp string
		var isSize bool

		if strings.HasPrefix(k, "node_filesystem_size_bytes") {
			mp = extractLabel(k, "mountpoint")
			isSize = true
		} else if strings.HasPrefix(k, "node_filesystem_avail_bytes") {
			mp = extractLabel(k, "mountpoint")
			isSize = false
		} else {
			continue
		}

		if mp == "" {
			mp = "/"
		}
		// 跳过伪文件系统（tmpfs / devtmpfs 等）——通过 fstype 标签过滤，这里简单跳过空挂载和非 / 的特殊路径
		if strings.HasPrefix(mp, "/dev") || strings.HasPrefix(mp, "/proc") || strings.HasPrefix(mp, "/sys") {
			continue
		}

		if _, ok := fsMap[mp]; !ok {
			fsMap[mp] = &fsInfo{}
		}
		if isSize {
			fsMap[mp].size = v
		} else {
			fsMap[mp].avail = v
		}
	}

	// 优先取根分区 /
	if fs, ok := fsMap["/"]; ok && fs.size > 0 {
		return math.Round((1-fs.avail/fs.size)*10000) / 100
	}

	// 没有根分区，取第一个有数据的分区
	for _, fs := range fsMap {
		if fs.size > 0 && fs.avail > 0 {
			return math.Round((1-fs.avail/fs.size)*10000) / 100
		}
	}

	return 0
}

// extractLoad 从指标中提取系统负载。
// node_exporter: node_load1 / node_load5。
// Go 运行时: go_goroutines 作为负载参考。
func extractLoad(metrics map[string]float64, _ string) float64 {
	if v, ok := metrics["node_load1"]; ok {
		return math.Round(v*100) / 100
	}
	if v, ok := metrics["node_load5"]; ok {
		return math.Round(v*100) / 100
	}
	// Go 运行时：goroutine 数量
	if v, ok := metrics["go_goroutines"]; ok && v > 0 {
		return v
	}
	return 0
}

// extractLabel 从指标名中提取指定标签的值。
func extractLabel(metricName, label string) string {
	brace := strings.IndexByte(metricName, '{')
	if brace < 0 {
		return ""
	}
	closeBrace := strings.LastIndexByte(metricName, '}')
	if closeBrace < 0 {
		return ""
	}
	labels := metricName[brace+1 : closeBrace]
	for _, part := range strings.Split(labels, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) == 2 && kv[0] == label {
			return strings.Trim(kv[1], `"`)
		}
	}
	return ""
}