package main

import (
	"errors"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/catalog"
	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/profile"
)

// buildFromWorkspace 读取工作区文件中的源与规则源，生成节点、策略组与规则（M2 实现）。
func buildFromWorkspace(path string, p *profile.Profile, cat *catalog.Catalog) (*yaml.Node, []diag.Diagnostic, time.Time, error) {
	return nil, nil, time.Time{}, errors.New("工作区输入在 M2 中实现")
}
