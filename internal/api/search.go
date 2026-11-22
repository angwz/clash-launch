package api

import "net/http"

// searchIndex 返回命令面板的本人数据索引（需求 R34）。不在令牌可访问的范围内：索引含消息联系人。
func (s *Server) searchIndex(w http.ResponseWriter, r *http.Request, p *principal) {
	idx, err := s.Svc.Search(r.Context(), p.User)
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, idx)
}
