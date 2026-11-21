// Package metrics 收集运行指标并以 Prometheus 文本格式输出（需求 R26），同时记录后台任务的运行情况（需求 R36）。
//
// 不引入 Prometheus 客户端库：只需要计数器、直方图与少量即时值，标签组合在启动后基本固定（路由模式、任务名），
// 自己实现几十行即可，也不会把新的依赖带进镜像。全部方法可并发调用；包级默认实例 Default 供各包直接使用，
// 测试可以新建独立的实例。
package metrics

import (
	"fmt"
	"io"
	"math"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 直方图的桶（秒）：请求、渲染与数据库语句从亚毫秒到十秒的量级。
var buckets = []float64{0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

type histogram struct {
	counts []uint64 // 与 buckets 对应，非累计
	inf    uint64
	sum    float64
	n      uint64
}

func (h *histogram) observe(v float64) {
	h.sum += v
	h.n++
	for i, b := range buckets {
		if v <= b {
			h.counts[i]++
			return
		}
	}
	h.inf++
}

// Job 是一个后台任务的运行记录。
type Job struct {
	Name         string        `json:"name"`
	Runs         uint64        `json:"runs"`
	Failures     uint64        `json:"failures"`
	LastStart    time.Time     `json:"lastStart"`
	LastDuration time.Duration `json:"-"`
	LastMs       float64       `json:"lastMs"` // 最近一次的耗时（毫秒）
	LastError    string        `json:"lastError,omitempty"`
	LastErrorAt  time.Time     `json:"lastErrorAt"`
	Running      bool          `json:"running"`
}

// Metrics 是一组指标。
type Metrics struct {
	mu       sync.Mutex
	requests map[[2]string]uint64 // {路由, 状态码类别}
	reqDur   map[string]*histogram
	render   *histogram
	db       *histogram
	fetches  map[[2]string]uint64 // {kind: source|rule, result: ok|failed}
	jobs     map[string]*Job
	gauges   map[string]func() float64
	started  time.Time
	version  string
}

// New 创建一组空的指标。
func New() *Metrics {
	return &Metrics{requests: map[[2]string]uint64{}, reqDur: map[string]*histogram{}, render: newHist(), db: newHist(),
		fetches: map[[2]string]uint64{}, jobs: map[string]*Job{}, gauges: map[string]func() float64{}, started: time.Now()}
}

func newHist() *histogram { return &histogram{counts: make([]uint64, len(buckets))} }

// Default 是进程内共用的指标。
var Default = New()

// SetVersion 记录程序版本（clash_launch_build_info）。
func (m *Metrics) SetVersion(v string) {
	m.mu.Lock()
	m.version = v
	m.mu.Unlock()
}

// Gauge 登记一个即时值（如在线的实时连接数），输出时调用 f 取值。
func (m *Metrics) Gauge(name string, f func() float64) {
	m.mu.Lock()
	m.gauges[name] = f
	m.mu.Unlock()
}

// Request 记录一次 HTTP 请求。route 是路由模式（如 "GET /api/v1/profiles/{id}"），未匹配路由时为空。
func (m *Metrics) Request(route string, status int, d time.Duration) {
	if route == "" {
		route = "unmatched"
	}
	class := strconv.Itoa(status/100) + "xx"
	m.mu.Lock()
	m.requests[[2]string{route, class}]++
	h := m.reqDur[route]
	if h == nil {
		h = newHist()
		m.reqDur[route] = h
	}
	h.observe(d.Seconds())
	m.mu.Unlock()
}

// Render 记录一次配置生成（含内核检查）的耗时。
func (m *Metrics) Render(d time.Duration) {
	m.mu.Lock()
	m.render.observe(d.Seconds())
	m.mu.Unlock()
}

// DB 记录一条数据库语句的耗时。
func (m *Metrics) DB(d time.Duration) {
	m.mu.Lock()
	m.db.observe(d.Seconds())
	m.mu.Unlock()
}

// Fetch 记录一次拉取：kind 为 source 或 rule。
func (m *Metrics) Fetch(kind string, ok bool) {
	r := "ok"
	if !ok {
		r = "failed"
	}
	m.mu.Lock()
	m.fetches[[2]string{kind, r}]++
	m.mu.Unlock()
}

// JobStart 记录任务开始，返回结束时调用的函数；errText 非空表示这次运行有失败。
func (m *Metrics) JobStart(name string, now time.Time) func(end time.Time, errText string) {
	m.mu.Lock()
	j := m.jobs[name]
	if j == nil {
		j = &Job{Name: name}
		m.jobs[name] = j
	}
	j.Running, j.LastStart = true, now
	m.mu.Unlock()
	return func(end time.Time, errText string) {
		m.mu.Lock()
		defer m.mu.Unlock()
		j.Running = false
		j.Runs++
		j.LastDuration = end.Sub(now)
		j.LastMs = float64(j.LastDuration.Microseconds()) / 1000
		if errText != "" {
			j.Failures++
			j.LastError, j.LastErrorAt = errText, end
		}
	}
}

// Jobs 返回全部任务的运行记录（按名称排序）。
func (m *Metrics) Jobs() []Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		out = append(out, *j)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out
}

// WriteText 以 Prometheus 文本格式（0.0.4）输出全部指标。输出按名称与标签排序，便于比对。
func (m *Metrics) WriteText(w io.Writer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var b strings.Builder
	help := func(name, typ, text string) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s %s\n", name, text, name, typ)
	}

	help("clash_launch_build_info", "gauge", "Build information; the value is always 1.")
	fmt.Fprintf(&b, "clash_launch_build_info{version=%s} 1\n", quote(m.version))
	help("clash_launch_start_time_seconds", "gauge", "Unix time when the process started.")
	fmt.Fprintf(&b, "clash_launch_start_time_seconds %d\n", m.started.Unix())

	help("clash_launch_http_requests_total", "counter", "HTTP requests by route pattern and status class.")
	reqKeys := make([][2]string, 0, len(m.requests))
	for k := range m.requests {
		reqKeys = append(reqKeys, k)
	}
	slices.SortFunc(reqKeys, func(a, c [2]string) int { return strings.Compare(a[0]+"\x00"+a[1], c[0]+"\x00"+c[1]) })
	for _, k := range reqKeys {
		fmt.Fprintf(&b, "clash_launch_http_requests_total{route=%s,code=%s} %d\n", quote(k[0]), quote(k[1]), m.requests[k])
	}

	help("clash_launch_http_request_duration_seconds", "histogram", "HTTP request duration by route pattern.")
	routes := make([]string, 0, len(m.reqDur))
	for r := range m.reqDur {
		routes = append(routes, r)
	}
	sort.Strings(routes)
	for _, r := range routes {
		writeHist(&b, "clash_launch_http_request_duration_seconds", "route="+quote(r), m.reqDur[r])
	}

	help("clash_launch_render_duration_seconds", "histogram", "Config generation time, including the kernel check.")
	writeHist(&b, "clash_launch_render_duration_seconds", "", m.render)
	help("clash_launch_db_query_duration_seconds", "histogram", "Database statement time.")
	writeHist(&b, "clash_launch_db_query_duration_seconds", "", m.db)

	help("clash_launch_fetches_total", "counter", "Fetches of node and rule sources by result.")
	fk := make([][2]string, 0, len(m.fetches))
	for k := range m.fetches {
		fk = append(fk, k)
	}
	slices.SortFunc(fk, func(a, c [2]string) int { return strings.Compare(a[0]+a[1], c[0]+c[1]) })
	for _, k := range fk {
		fmt.Fprintf(&b, "clash_launch_fetches_total{kind=%s,result=%s} %d\n", quote(k[0]), quote(k[1]), m.fetches[k])
	}

	names := make([]string, 0, len(m.jobs))
	for n := range m.jobs {
		names = append(names, n)
	}
	sort.Strings(names)
	help("clash_launch_job_runs_total", "counter", "Background job runs.")
	for _, n := range names {
		fmt.Fprintf(&b, "clash_launch_job_runs_total{job=%s} %d\n", quote(n), m.jobs[n].Runs)
	}
	help("clash_launch_job_failures_total", "counter", "Background job runs that reported a failure.")
	for _, n := range names {
		fmt.Fprintf(&b, "clash_launch_job_failures_total{job=%s} %d\n", quote(n), m.jobs[n].Failures)
	}
	help("clash_launch_job_last_duration_seconds", "gauge", "Duration of the most recent run of each background job.")
	for _, n := range names {
		fmt.Fprintf(&b, "clash_launch_job_last_duration_seconds{job=%s} %s\n", quote(n), num(m.jobs[n].LastDuration.Seconds()))
	}
	help("clash_launch_job_last_run_timestamp_seconds", "gauge", "Unix time when each background job last started.")
	for _, n := range names {
		fmt.Fprintf(&b, "clash_launch_job_last_run_timestamp_seconds{job=%s} %d\n", quote(n), m.jobs[n].LastStart.Unix())
	}

	gnames := make([]string, 0, len(m.gauges))
	for n := range m.gauges {
		gnames = append(gnames, n)
	}
	sort.Strings(gnames)
	for _, n := range gnames {
		help(n, "gauge", "Current value.")
		fmt.Fprintf(&b, "%s %s\n", n, num(m.gauges[n]()))
	}
	help("go_goroutines", "gauge", "Number of goroutines.")
	fmt.Fprintf(&b, "go_goroutines %d\n", runtime.NumGoroutine())
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	help("go_memstats_heap_alloc_bytes", "gauge", "Bytes of allocated heap objects.")
	fmt.Fprintf(&b, "go_memstats_heap_alloc_bytes %d\n", ms.HeapAlloc)
	_, err := io.WriteString(w, b.String())
	return err
}

func writeHist(b *strings.Builder, name, labels string, h *histogram) {
	sep := ""
	if labels != "" {
		sep = ","
	}
	var cum uint64
	for i, le := range buckets {
		cum += h.counts[i]
		fmt.Fprintf(b, "%s_bucket{%s%sle=%s} %d\n", name, labels, sep, quote(num(le)), cum)
	}
	cum += h.inf
	fmt.Fprintf(b, "%s_bucket{%s%sle=\"+Inf\"} %d\n", name, labels, sep, cum)
	if labels != "" {
		labels = "{" + labels + "}"
	}
	fmt.Fprintf(b, "%s_sum%s %s\n%s_count%s %d\n", name, labels, num(h.sum), name, labels, h.n)
}

// quote 按 Prometheus 文本格式转义标签值：反斜杠、双引号与换行。
func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s) + `"`
}

func num(f float64) string {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return "0"
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// FetchCounts 返回进程启动以来的拉取次数：键为 kind + "_" + result（如 source_failed）。
func (m *Metrics) FetchCounts() map[string]uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]uint64{"source_ok": 0, "source_failed": 0, "rule_ok": 0, "rule_failed": 0}
	for k, v := range m.fetches {
		out[k[0]+"_"+k[1]] = v
	}
	return out
}

// Started 返回进程启动的时间。
func (m *Metrics) Started() time.Time { return m.started }
