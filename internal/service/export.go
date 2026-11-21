package service

import (
	"bytes"
	"context"
	"errors"

	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/engine"
	"github.com/angwz/clash-launch/internal/kernel"
	"github.com/angwz/clash-launch/internal/store"
)

// 导出配置（需求 R17）：生成一份客户端可以直接使用的 YAML，与订阅链接同源（同一渲染、同一内核检查），
// 可选去掉节点或规则集。「交给客户端」模式的节点 provider 指向本站托管的地址，地址里要有链接令牌：
// 含节点导出时用指定的（或第一条）订阅链接的令牌；配置还没有订阅链接时拒绝，而不是写出无法使用的地址。

// ErrExportNeedsLink 表示含节点导出需要一条订阅链接。
var ErrExportNeedsLink = errors.New("service: 这份配置有「交给客户端」的节点源，导出含节点的配置前先创建一条订阅链接")

// ExportResult 是一次导出。
type ExportResult struct {
	YAML        []byte
	Name        string
	Diagnostics []diag.Diagnostic
	Kernel      kernel.Result
	Fatal       string
}

// ExportConfig 导出配置。linkID 为空时用第一条订阅链接（只在需要令牌时）。
func (s *Service) ExportConfig(ctx context.Context, owner, id string, ex engine.Export, linkID string) (*ExportResult, error) {
	p, err := s.Store.Profiles().Get(ctx, owner, id)
	if err != nil {
		return nil, err
	}
	res, err := s.render(ctx, owner, p.Doc, p.UpdatedAt, true, ex)
	if err != nil {
		return nil, err
	}
	links, err := s.Store.Links().List(ctx, owner, id)
	if err != nil {
		return nil, err
	}
	var l *store.Link
	for i := range links {
		if linkID == "" || links[i].ID == linkID {
			l = &links[i]
			break
		}
	}
	if linkID != "" && l == nil {
		return nil, store.ErrNotFound // 指定的链接必须属于这份配置
	}
	out := &ExportResult{YAML: res.YAML, Name: p.Name, Diagnostics: res.Diagnostics, Kernel: res.Kernel, Fatal: res.Fatal}
	if res.Fatal != "" || !bytes.Contains(out.YAML, []byte(LinkTokenPlaceholder)) {
		return out, nil
	}
	if l == nil {
		return nil, ErrExportNeedsLink
	}
	out.YAML = bytes.ReplaceAll(out.YAML, []byte(LinkTokenPlaceholder), []byte(l.Token))
	return out, nil
}
