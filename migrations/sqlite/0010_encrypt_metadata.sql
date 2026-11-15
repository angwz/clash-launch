-- 加密剩余的节点相关明文（反馈 #77）：订阅响应头（流量、订阅名称、机场网站）、下发配置的响应头、
-- 检查结果（含节点名称与无法识别内容的预览）、渲染失败的原因。新数据写入 *_enc 列，原明文列置为默认值；
-- 已有的明文在启动迁移后由 store.encryptLegacy 一次性加密并清除。
ALTER TABLE snapshots ADD COLUMN headers_enc BLOB;
ALTER TABLE renders ADD COLUMN good_headers_enc BLOB;
ALTER TABLE renders ADD COLUMN last_diags_enc BLOB;
ALTER TABLE renders ADD COLUMN last_error_enc BLOB;
