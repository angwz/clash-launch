package service

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/angwz/clash-launch/internal/store"
)

// 数据保留设置（需求 R43）：管理员在网页上调整审计日志与订阅请求明细的保留期，立即生效、不用重启。
// 没有在网页上设置过时沿用环境变量（CLASH_LAUNCH_AUDIT_DAYS、CLASH_LAUNCH_SUB_REQUEST_DAYS，即
// Service.AuditRetention 与 RequestRetention）；0 表示永不清理。清理任务、订阅分析的提示与用户协议中的
// 保留期占位符都读这里的生效值。

const (
	settingAuditDays   = "retention.audit_days"
	settingRequestDays = "retention.request_days"
	maxRetentionDays   = 3650
)

// ErrRetention 表示保留期超出范围。
var ErrRetention = errors.New("service: 保留期应为 0（永不清理）到 3650 天")

// RetentionView 是生效的保留期（天）与来源。
type RetentionView struct {
	AuditDays          int  `json:"auditDays"`
	RequestDays        int  `json:"requestDays"`
	AuditFromSetting   bool `json:"auditFromSetting"`   // 来自网页设置；false 表示沿用环境变量
	RequestFromSetting bool `json:"requestFromSetting"` // 同上
	AuditDefault       int  `json:"auditDefault"`       // 环境变量给出的值
	RequestDefault     int  `json:"requestDefault"`
}

func durDays(d time.Duration) int { return int(d / (24 * time.Hour)) }

// Retention 返回生效的保留期。读取设置失败时沿用环境变量的值（清理宁可按旧值也不中断）。
func (s *Service) Retention(ctx context.Context) RetentionView {
	v := RetentionView{AuditDays: durDays(s.AuditRetention), RequestDays: durDays(s.RequestRetention), AuditDefault: durDays(s.AuditRetention), RequestDefault: durDays(s.RequestRetention)}
	if n, ok := s.retentionSetting(ctx, settingAuditDays); ok {
		v.AuditDays, v.AuditFromSetting = n, true
	}
	if n, ok := s.retentionSetting(ctx, settingRequestDays); ok {
		v.RequestDays, v.RequestFromSetting = n, true
	}
	return v
}

func (s *Service) retentionSetting(ctx context.Context, key string) (int, bool) {
	raw, err := s.Store.Settings().Get(ctx, key)
	if err != nil || raw == "" {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 || n > maxRetentionDays {
		return 0, false
	}
	return n, true
}

// auditRetention 与 requestRetention 是清理任务使用的生效保留期；0 表示不清理。
func (s *Service) auditRetention(ctx context.Context) time.Duration {
	return time.Duration(s.Retention(ctx).AuditDays) * 24 * time.Hour
}

func (s *Service) requestRetention(ctx context.Context) time.Duration {
	return time.Duration(s.Retention(ctx).RequestDays) * 24 * time.Hour
}

// SetRetention 设置保留期（天）；nil 表示恢复为环境变量的值。记入审计。
func (s *Service) SetRetention(ctx context.Context, admin *store.User, auditDays, requestDays *int) (RetentionView, error) {
	for _, p := range []*int{auditDays, requestDays} {
		if p != nil && (*p < 0 || *p > maxRetentionDays) {
			return RetentionView{}, ErrRetention
		}
	}
	put := func(key string, p *int) error {
		v := ""
		if p != nil {
			v = strconv.Itoa(*p)
		}
		return s.Store.Settings().Set(ctx, key, v)
	}
	if err := put(settingAuditDays, auditDays); err != nil {
		return RetentionView{}, err
	}
	if err := put(settingRequestDays, requestDays); err != nil {
		return RetentionView{}, err
	}
	v := s.Retention(ctx)
	s.audit(ctx, admin.ID, "admin.retention", map[string]string{"audit_days": strconv.Itoa(v.AuditDays), "request_days": strconv.Itoa(v.RequestDays)})
	return v, nil
}

// DataUsage 返回各表的数据量与占用（管理页按需读取）。
func (s *Service) DataUsage(ctx context.Context) (*store.DataUsage, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return s.Store.Usage(ctx)
}
