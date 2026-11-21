package metrics

import (
	"strings"
	"testing"
	"time"
)

func TestWriteText(t *testing.T) {
	m := New()
	m.SetVersion("1.2.3")
	m.Request("GET /api/v1/profiles", 200, 3*time.Millisecond)
	m.Request("GET /api/v1/profiles", 404, 20*time.Second)
	m.Request("", 404, time.Millisecond)
	m.Render(120 * time.Millisecond)
	m.DB(300 * time.Microsecond)
	m.Fetch("source", true)
	m.Fetch("source", false)
	m.Gauge("clash_launch_realtime_connections", func() float64 { return 3 })
	done := m.JobStart("fetch", time.Unix(1000, 0))
	done(time.Unix(1002, 0), "")
	done = m.JobStart("fetch", time.Unix(2000, 0))
	done(time.Unix(2000, int64(500*time.Millisecond)), "拉取失败")

	var b strings.Builder
	if err := m.WriteText(&b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		`clash_launch_build_info{version="1.2.3"} 1`,
		`clash_launch_http_requests_total{route="GET /api/v1/profiles",code="2xx"} 1`,
		`clash_launch_http_requests_total{route="GET /api/v1/profiles",code="4xx"} 1`,
		`clash_launch_http_requests_total{route="unmatched",code="4xx"} 1`,
		`clash_launch_http_request_duration_seconds_bucket{route="GET /api/v1/profiles",le="0.005"} 1`,
		`clash_launch_http_request_duration_seconds_bucket{route="GET /api/v1/profiles",le="+Inf"} 2`,
		`clash_launch_http_request_duration_seconds_count{route="GET /api/v1/profiles"} 2`,
		`clash_launch_render_duration_seconds_bucket{le="0.25"} 1`,
		`clash_launch_render_duration_seconds_count 1`,
		`clash_launch_db_query_duration_seconds_count 1`,
		`clash_launch_fetches_total{kind="source",result="failed"} 1`,
		`clash_launch_job_runs_total{job="fetch"} 2`,
		`clash_launch_job_failures_total{job="fetch"} 1`,
		`clash_launch_job_last_duration_seconds{job="fetch"} 0.5`,
		`clash_launch_realtime_connections 3`,
		"# TYPE clash_launch_http_request_duration_seconds histogram",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("缺少 %s\n%s", want, out)
		}
	}
	jobs := m.Jobs()
	if len(jobs) != 1 || jobs[0].Runs != 2 || jobs[0].Failures != 1 || jobs[0].LastError != "拉取失败" || jobs[0].LastMs != 500 || jobs[0].Running {
		t.Errorf("任务记录：%+v", jobs)
	}
	// 输入含双引号、反斜杠与换行，输出把三者转义
	if quote("a\"b"+string(rune(92))+"c\nd") != `"a\"b\\c\nd"` {
		t.Error("标签转义")
	}
}
