package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrTimezone 表示时区名称无效。
var ErrTimezone = errors.New("service: 时区无效，应为 IANA 时区名（例如 Asia/Shanghai）")

// SetTimezone 保存用户的时区（反馈 #72）。空字符串表示跟随浏览器；其余必须是 IANA 时区名。
// 界面上的时间按它显示；配置没有指定时区时，生成的文件头时间也按它显示。返回规范写法。
func (s *Service) SetTimezone(ctx context.Context, userID, tz string) (string, error) {
	tz = strings.TrimSpace(tz)
	if tz != "" {
		// Local 取决于服务器所在地，不是用户能理解的时区
		if tz == "Local" || len(tz) > 64 {
			return "", ErrTimezone
		}
		loc, err := time.LoadLocation(tz)
		if err != nil {
			return "", fmt.Errorf("%w：%s", ErrTimezone, tz)
		}
		tz = loc.String()
	}
	u, err := s.Store.Users().ByID(ctx, userID)
	if err != nil {
		return "", err
	}
	u.Timezone = tz
	if err := s.Store.Users().Update(ctx, u); err != nil {
		return "", err
	}
	return tz, nil
}
