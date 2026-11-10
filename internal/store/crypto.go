package store

import (
	"bufio"
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// 加密（第 14.4 节）：节点原文、订阅 URL、订阅请求头、链接令牌、方案文档、快照内容等用 AES-256-GCM 加密存储。
//
// 密文格式：版本(1 字节) | 密钥 ID 长度(1) | 密钥 ID | nonce(12) | 密文与标签。
// 附加数据（AAD）是「表.列:行 ID」，因此密文不能被搬到另一行或另一列上使用。
// 每段密文都带密钥 ID：轮换后旧密钥继续用于解密，keys rotate 用当前密钥重新加密全部数据。

const sealVersion = 1

// ErrDecrypt 表示密文无法解密（密钥缺失、数据被篡改或被搬到了别处）。
var ErrDecrypt = errors.New("store: 无法解密")

// Keyring 是主密钥集合；最后加入的是当前密钥。
type Keyring struct {
	keys    map[string]cipher.AEAD
	current string
	order   []string
	raw     map[string][]byte
}

// NewKeyring 创建空密钥环。
func NewKeyring() *Keyring {
	return &Keyring{keys: map[string]cipher.AEAD{}, raw: map[string][]byte{}}
}

// Add 加入一把 32 字节的密钥，并设为当前密钥。
func (k *Keyring) Add(id string, key []byte) error {
	if len(key) != 32 {
		return fmt.Errorf("store: 主密钥必须是 32 字节（得到 %d）", len(key))
	}
	if id == "" || len(id) > 64 || strings.ContainsAny(id, ": \n") {
		return fmt.Errorf("store: 密钥 ID %q 无效", id)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	if _, dup := k.keys[id]; !dup {
		k.order = append(k.order, id)
	}
	k.keys[id] = aead
	k.raw[id] = append([]byte{}, key...)
	k.current = id
	return nil
}

// Current 返回当前密钥 ID。
func (k *Keyring) Current() string { return k.current }

// IDs 返回全部密钥 ID（加入顺序）。
func (k *Keyring) IDs() []string { return append([]string{}, k.order...) }

// Seal 用当前密钥加密。plaintext 为 nil 时返回 nil（数据库中存 NULL）。
func (k *Keyring) Seal(plaintext []byte, aad string) ([]byte, error) {
	if plaintext == nil {
		return nil, nil
	}
	aead, ok := k.keys[k.current]
	if !ok {
		return nil, errors.New("store: 没有可用的主密钥")
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := make([]byte, 0, 2+len(k.current)+len(nonce)+len(plaintext)+aead.Overhead())
	out = append(out, sealVersion, byte(len(k.current))) //nolint:gosec // 密钥 ID 长度在 Add 中限制为 64 以内
	out = append(out, k.current...)
	out = append(out, nonce...)
	return aead.Seal(out, nonce, plaintext, []byte(aad)), nil
}

// Open 解密。blob 为 nil 时返回 nil。
func (k *Keyring) Open(blob []byte, aad string) ([]byte, error) {
	if blob == nil {
		return nil, nil
	}
	id, rest, err := splitBlob(blob)
	if err != nil {
		return nil, err
	}
	aead, ok := k.keys[id]
	if !ok {
		return nil, fmt.Errorf("%w：缺少密钥 %s", ErrDecrypt, id)
	}
	ns := aead.NonceSize()
	if len(rest) < ns+aead.Overhead() {
		return nil, ErrDecrypt
	}
	pt, err := aead.Open(nil, rest[:ns], rest[ns:], []byte(aad))
	if err != nil {
		return nil, ErrDecrypt
	}
	if pt == nil {
		pt = []byte{}
	}
	return pt, nil
}

// KeyID 返回密文使用的密钥 ID。
func KeyID(blob []byte) (string, error) {
	id, _, err := splitBlob(blob)
	return id, err
}

func splitBlob(blob []byte) (string, []byte, error) {
	if len(blob) < 2 || blob[0] != sealVersion {
		return "", nil, ErrDecrypt
	}
	n := int(blob[1])
	if len(blob) < 2+n {
		return "", nil, ErrDecrypt
	}
	return string(blob[2 : 2+n]), blob[2+n:], nil
}

// —— 主密钥来源 ——

// KeySource 说明主密钥从哪里来，用于日志与管理页。
type KeySource struct {
	From      string // env | env-file | file | generated
	Path      string
	Generated bool // 首次启动时新生成：必须提醒管理员备份
}

// LoadKeyring 按顺序读取主密钥：CLASH_LAUNCH_MASTER_KEY、CLASH_LAUNCH_MASTER_KEY_FILE、<数据目录>/master.key；
// 三者都不存在时生成 <数据目录>/master.key（权限 0600）。
//
// 密钥文件每行一把：「密钥 ID:base64(32 字节)」，最后一行是当前密钥；以 # 开头的行是注释。
// 环境变量直接给出 32 字节密钥（base64 或 64 位十六进制），密钥 ID 固定为 env。
func LoadKeyring(dataDir string, getenv func(string) string) (*Keyring, KeySource, error) {
	k := NewKeyring()
	if v := strings.TrimSpace(getenv("CLASH_LAUNCH_MASTER_KEY")); v != "" {
		key, err := decodeKey(v)
		if err != nil {
			return nil, KeySource{}, fmt.Errorf("CLASH_LAUNCH_MASTER_KEY: %w", err)
		}
		return k, KeySource{From: "env"}, k.Add("env", key)
	}
	path := getenv("CLASH_LAUNCH_MASTER_KEY_FILE")
	from := "env-file"
	if path == "" {
		path, from = filepath.Join(dataDir, "master.key"), "file"
	}
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := parseKeyFile(k, b); err != nil {
			return nil, KeySource{}, fmt.Errorf("%s: %w", path, err)
		}
		return k, KeySource{From: from, Path: path}, nil
	case errors.Is(err, os.ErrNotExist) && from == "file":
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, KeySource{}, err
		}
		id := "k1"
		content := "# Clash, Launch! 主密钥。备份数据时必须同时备份本文件：丢失密钥就无法解密。\n" + id + ":" + base64.StdEncoding.EncodeToString(key) + "\n"
		if err := os.MkdirAll(dataDir, 0o700); err != nil {
			return nil, KeySource{}, err
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return nil, KeySource{}, err
		}
		return k, KeySource{From: "generated", Path: path, Generated: true}, k.Add(id, key)
	default:
		return nil, KeySource{}, err
	}
}

func parseKeyFile(k *Keyring, b []byte) error {
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		id, v, ok := strings.Cut(line, ":")
		if !ok {
			return errors.New("每行应为 密钥ID:base64")
		}
		key, err := decodeKey(v)
		if err != nil {
			return err
		}
		if err := k.Add(id, key); err != nil {
			return err
		}
	}
	if k.current == "" {
		return errors.New("文件中没有密钥")
	}
	return nil
}

func decodeKey(s string) ([]byte, error) {
	if len(s) == 64 {
		if b, err := hex.DecodeString(s); err == nil {
			return b, nil
		}
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, errors.New("主密钥必须是 base64 或 64 位十六进制")
	}
	return b, nil
}

// AppendKey 生成一把新密钥并追加到密钥文件（成为当前密钥），返回新的密钥 ID。用于 keys rotate。
func AppendKey(path string, k *Keyring) (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	id := fmt.Sprintf("k%d", len(k.order)+1)
	for _, exists := k.keys[id]; exists; _, exists = k.keys[id] {
		id += "x"
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	if _, err := fmt.Fprintf(f, "%s:%s\n", id, base64.StdEncoding.EncodeToString(key)); err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return id, k.Add(id, key)
}

// encrypted 列出全部加密列：表、主键列、列名前缀（AAD 中的列名）。
var encrypted = []struct{ table, pk, col, aad string }{
	{"sources", "id", "url_enc", "url"}, {"sources", "id", "content_enc", "content"}, {"sources", "id", "headers_enc", "headers"},
	{"snapshots", "key", "content_enc", "content"}, {"profiles", "id", "doc_enc", "doc"}, {"revisions", "id", "doc_enc", "doc"},
	{"links", "id", "token_enc", "token"}, {"renders", "profile_id", "good_yaml_enc", "good_yaml"},
	{"user_totp", "user_id", "secret_enc", "secret"}, {"user_smtp", "user_id", "password_enc", "password"},
}

// ReEncrypt 用当前密钥重新加密全部数据（keys rotate 的第二步），返回改写的单元数。在一个事务中完成。
func (d *DB) ReEncrypt(ctx context.Context) (int, error) {
	n := 0
	err := d.Tx(ctx, func(s Store) error {
		tx := s.(*DB)
		for _, e := range encrypted {
			rows, err := tx.query(ctx, "SELECT "+e.pk+", "+e.col+" FROM "+e.table+" WHERE "+e.col+" IS NOT NULL")
			if err != nil {
				return err
			}
			type item struct {
				id   string
				blob []byte
			}
			var items []item
			for rows.Next() {
				var it item
				if err := rows.Scan(&it.id, &it.blob); err != nil {
					_ = rows.Close()
					return err
				}
				items = append(items, it)
			}
			_ = rows.Close()
			for _, it := range items {
				if kid, err := KeyID(it.blob); err == nil && kid == tx.keys.Current() {
					continue
				}
				pt, err := tx.open(it.blob, e.table, e.aad, it.id)
				if err != nil {
					return fmt.Errorf("%s.%s %s: %w", e.table, e.col, it.id, err)
				}
				blob, err := tx.seal(pt, e.table, e.aad, it.id)
				if err != nil {
					return err
				}
				if _, err := tx.exec(ctx, "UPDATE "+e.table+" SET "+e.col+"=? WHERE "+e.pk+"=?", blob, it.id); err != nil {
					return err
				}
				n++
			}
		}
		return nil
	})
	return n, err
}
