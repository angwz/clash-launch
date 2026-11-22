package service

import (
	"context"
	"encoding/json"

	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/node"
	"github.com/angwz/clash-launch/internal/store"
)

// 命令面板搜数据（需求 R34）：打开命令面板时取一次本人数据的名称索引，前端在本地匹配。
// 一次请求、按键时不再访问服务器；索引只含名称与 ID，不含内容、地址或凭据。

// SearchIndex 是命令面板可以搜到的本人数据。
type SearchIndex struct {
	Profiles []SearchItem   `json:"profiles"`
	Sources  []SearchItem   `json:"sources"`
	Rules    []SearchItem   `json:"rules"`
	Nodes    []SearchNode   `json:"nodes"`
	Shorts   []SearchShort  `json:"shortLinks"`
	Friends  []SearchFriend `json:"friends"`
	// Truncated 表示节点名超过上限，只返回了前一部分
	Truncated bool `json:"truncated,omitempty"`
}

type SearchItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind,omitempty"`
}

type SearchNode struct {
	Source string `json:"source"`
	Name   string `json:"name"`
}

type SearchShort struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Note string `json:"note,omitempty"`
}

type SearchFriend struct {
	Username string `json:"username"`
	Nickname string `json:"nickname,omitempty"`
}

// 上限：配额范围内的数据都能完整返回；节点名按源依次收集，超过 searchNodeCap 截断并标记。
const (
	searchListCap = 1000
	searchNodeCap = 5000
)

// Search 收集 u 的搜索索引。节点名与节点源页显示的一致：订阅取最近一次拉取的快照，再按该源的整理设置处理。
func (s *Service) Search(ctx context.Context, u *store.User) (*SearchIndex, error) {
	out := &SearchIndex{Profiles: []SearchItem{}, Sources: []SearchItem{}, Rules: []SearchItem{}, Nodes: []SearchNode{}, Shorts: []SearchShort{}, Friends: []SearchFriend{}}
	page := store.Page{Limit: searchListCap}
	ps, err := s.Store.Profiles().List(ctx, u.ID, page)
	if err != nil {
		return nil, err
	}
	for _, p := range ps {
		out.Profiles = append(out.Profiles, SearchItem{ID: p.ID, Name: p.Name})
	}
	rs, err := s.Store.RuleSources().List(ctx, u.ID, page)
	if err != nil {
		return nil, err
	}
	for _, r := range rs {
		out.Rules = append(out.Rules, SearchItem{ID: r.ID, Name: r.Name, Kind: r.Kind})
	}
	srcs, err := s.Store.Sources().List(ctx, u.ID, page)
	if err != nil {
		return nil, err
	}
	for i := range srcs {
		src := &srcs[i]
		out.Sources = append(out.Sources, SearchItem{ID: src.ID, Name: src.Name, Kind: src.Kind})
		if out.Truncated {
			continue
		}
		content := src.Content
		if src.Kind == "subscription" {
			snap, err := s.Store.Snapshots().Get(ctx, u.ID, SourceKey(src.ID))
			if err != nil {
				continue // 尚未拉取：没有节点
			}
			content = snap.Content
		}
		var proc node.Processing
		_ = json.Unmarshal(src.Processing, &proc)
		// 解析中的诊断已在节点源页显示；这里只要名称，丢弃诊断不改变任何输出
		var ds diag.List
		nodes, _ := node.Parse(content, src.ID, s.Catalog, &ds)
		for _, n := range node.Process(nodes, proc, s.Tools, src.ID, &ds) {
			if len(out.Nodes) >= searchNodeCap {
				out.Truncated = true
				break
			}
			out.Nodes = append(out.Nodes, SearchNode{Source: src.ID, Name: n.Name})
		}
	}
	shorts, err := s.Store.ShortLinks().List(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	for _, x := range shorts {
		out.Shorts = append(out.Shorts, SearchShort{ID: x.ID, Code: x.Code, Note: x.Note})
	}
	// 好友：消息里的联系人（不含回收站中的对话与已注销的账号）
	cs, err := s.Store.Contacts().List(ctx, u.ID, false)
	if err != nil {
		return nil, err
	}
	for _, c := range cs {
		peer, err := s.Store.Users().ByID(ctx, c.PeerID)
		if err != nil || !peer.DeletedAt.IsZero() {
			continue
		}
		out.Friends = append(out.Friends, SearchFriend{Username: peer.Username, Nickname: peer.Nickname})
	}
	return out, nil
}
