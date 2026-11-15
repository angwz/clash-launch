package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/node"
	"github.com/angwz/clash-launch/internal/store"
)

// 节点源的概况（反馈 #76）：节点页每一项显示可用节点数、协议分布，订阅另显示提供方给出的流量与到期时间。

type sourceSummary struct {
	Nodes int         `json:"nodes"` // 整理后可用的节点数（已去掉信息节点与重复节点）
	Types []typeCount `json:"types"` // 协议分布（按数量降序）
	// 订阅提供方在 subscription-userinfo 响应头中给出的流量（字节）与到期时间；没有时省略
	Upload   int64      `json:"upload,omitempty"`
	Download int64      `json:"download,omitempty"`
	Total    int64      `json:"total,omitempty"`
	Expire   *time.Time `json:"expire,omitempty"`
}

type typeCount struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// summaryCache 按「内容 + 整理设置」的哈希缓存解析结果：同一份内容只解析一次，节点页列表不必每次重新解析。
var summaryCache sync.Map // string → sourceSummary（不含流量信息）

func (s *Server) summarize(src *store.Source, content []byte, headers map[string]string) *sourceSummary {
	h := sha256.New()
	h.Write(content)
	h.Write([]byte{0})
	h.Write(src.Processing)
	key := hex.EncodeToString(h.Sum(nil))
	var sum sourceSummary
	if v, ok := summaryCache.Load(key); ok {
		sum = v.(sourceSummary)
	} else {
		var proc node.Processing
		_ = json.Unmarshal(src.Processing, &proc)
		var ds diag.List
		nodes, _ := node.Parse(content, src.ID, s.Svc.Catalog, &ds)
		nodes = node.Process(nodes, proc, s.Svc.Tools, src.ID, &ds)
		counts := map[string]int{}
		for _, n := range nodes {
			counts[n.Type]++
		}
		sum = sourceSummary{Nodes: len(nodes), Types: []typeCount{}}
		for t, c := range counts {
			sum.Types = append(sum.Types, typeCount{t, c})
		}
		sort.Slice(sum.Types, func(i, j int) bool {
			if sum.Types[i].Count != sum.Types[j].Count {
				return sum.Types[i].Count > sum.Types[j].Count
			}
			return sum.Types[i].Type < sum.Types[j].Type
		})
		summaryCache.Store(key, sum)
	}
	if v := headers["subscription-userinfo"]; v != "" {
		parseUserinfo(v, &sum)
	}
	return &sum
}

// parseUserinfo 解析 subscription-userinfo：upload=…; download=…; total=…; expire=…（Unix 秒）。
// 这是订阅提供方的事实标准写法；无法识别的项忽略。
func parseUserinfo(v string, s *sourceSummary) {
	for _, part := range strings.Split(v, ";") {
		k, val, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
		if err != nil || n < 0 {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "upload":
			s.Upload = n
		case "download":
			s.Download = n
		case "total":
			s.Total = n
		case "expire":
			if n > 0 {
				t := time.Unix(n, 0).UTC()
				s.Expire = &t
			}
		}
	}
}
