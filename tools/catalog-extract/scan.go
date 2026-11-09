package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// root 描述一个扫描入口：从某个结构体（或某个函数中按类型分派的 switch）出发，
// 以指定的 tag 名读取字段。2026-11-09 对照 mihomo v1.19.33 确定（第 21.2 节第 3 项）。
type root struct {
	path   string // 输出路径前缀
	dir    string // 包目录（相对源码根）
	tag    string
	strct  string // 直接从该结构体出发
	fn     string // 或：从该函数的 switch 分派出发，每个 case 的字符串值成为路径的一段
	common string // 与 fn 搭配：每个分支都共享的公共结构体（如 GroupCommonOption）
}

var roots = []root{
	{path: "", dir: "config", tag: "yaml", strct: "RawConfig"},
	{path: "proxies", dir: "adapter", tag: "proxy", fn: "ParseProxy"},
	{path: "proxy-groups", dir: "adapter/outboundgroup", tag: "group", fn: "ParseProxyGroup", common: "GroupCommonOption"},
	{path: "proxy-providers", dir: "adapter/provider", tag: "provider", strct: "proxyProviderSchema"},
	{path: "rule-providers", dir: "rules/provider", tag: "provider", strct: "ruleProviderSchema"},
	{path: "listeners", dir: "listener", tag: "inbound", fn: "ParseListener"},
}

// 被这些根结构体以 map[string]any 方式承载、另由 roots 单独扫描的字段，在根结构体中只记录到这一层。
var opaque = map[string]bool{"proxies": true, "proxy-groups": true, "proxy-providers": true, "rule-providers": true, "listeners": true}

type pkg struct {
	dir      string
	types    map[string]*ast.TypeSpec
	imports  map[string]map[string]string // 文件 → 别名 → 导入路径
	files    map[*ast.TypeSpec]string
	fset     *token.FileSet
	funcs    map[string]*ast.FuncDecl
	typeFile map[string]*ast.File
}

type scanner struct {
	src  string
	pkgs map[string]*pkg
	out  map[string]Field
}

func scan(src string) (*Result, error) {
	s := &scanner{src: src, pkgs: map[string]*pkg{}, out: map[string]Field{}}
	for _, r := range roots {
		p, err := s.load(r.dir)
		if err != nil {
			return nil, err
		}
		switch {
		case r.strct != "":
			ts, ok := p.types[r.strct]
			if !ok {
				return nil, fmt.Errorf("%s 中没有结构体 %s", r.dir, r.strct)
			}
			s.walkStruct(p, ts, r.tag, r.path, 0)
		case r.fn != "":
			if err := s.walkSwitch(p, r); err != nil {
				return nil, err
			}
		}
	}
	res := &Result{}
	for _, f := range s.out {
		res.Fields = append(res.Fields, f)
	}
	sort.Slice(res.Fields, func(i, j int) bool { return res.Fields[i].Path < res.Fields[j].Path })
	return res, nil
}

func (s *scanner) load(dir string) (*pkg, error) {
	if p, ok := s.pkgs[dir]; ok {
		return p, nil
	}
	fset := token.NewFileSet()
	full := filepath.Join(s.src, filepath.FromSlash(dir))
	entries, err := os.ReadDir(full)
	if err != nil {
		return nil, err
	}
	p := &pkg{dir: dir, types: map[string]*ast.TypeSpec{}, imports: map[string]map[string]string{}, files: map[*ast.TypeSpec]string{}, fset: fset, funcs: map[string]*ast.FuncDecl{}, typeFile: map[string]*ast.File{}}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(full, n), nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		imp := map[string]string{}
		for _, is := range f.Imports {
			ip, _ := strconv.Unquote(is.Path.Value)
			alias := filepath.Base(ip)
			if is.Name != nil {
				alias = is.Name.Name
			}
			imp[alias] = ip
		}
		rel := dir + "/" + n
		p.imports[rel] = imp
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.GenDecl:
				for _, sp := range d.Specs {
					if ts, ok := sp.(*ast.TypeSpec); ok {
						p.types[ts.Name.Name] = ts
						p.files[ts] = rel
					}
				}
			case *ast.FuncDecl:
				if d.Recv == nil {
					p.funcs[d.Name.Name] = d
				}
			}
		}
	}
	s.pkgs[dir] = p
	return p, nil
}

const modPrefix = "github.com/metacubex/mihomo/"

// resolve 把类型表达式解析为（包，类型声明）；外部模块与标准库返回 nil。
func (s *scanner) resolve(p *pkg, file string, e ast.Expr) (*pkg, *ast.TypeSpec) {
	switch t := e.(type) {
	case *ast.Ident:
		if ts, ok := p.types[t.Name]; ok {
			return p, ts
		}
	case *ast.SelectorExpr:
		x, ok := t.X.(*ast.Ident)
		if !ok {
			return nil, nil
		}
		ip := p.imports[file][x.Name]
		if !strings.HasPrefix(ip, modPrefix) {
			return nil, nil
		}
		q, err := s.load(strings.TrimPrefix(ip, modPrefix))
		if err != nil {
			return nil, nil
		}
		if ts, ok := q.types[t.Sel.Name]; ok {
			return q, ts
		}
	case *ast.StarExpr:
		return s.resolve(p, file, t.X)
	}
	return nil, nil
}

func (s *scanner) record(path, typ string, p *pkg, pos token.Pos) {
	if path == "" {
		return
	}
	position := p.fset.Position(pos)
	src := p.dir + "/" + filepath.Base(position.Filename) + ":" + strconv.Itoa(position.Line)
	if old, ok := s.out[path]; ok && old.Type != typ {
		// 同一路径在不同分支中类型不一致（罕见）：保留两者，便于人工判断。
		typ = old.Type + "|" + typ
	}
	s.out[path] = Field{Path: path, Type: typ, Source: src}
}

func (s *scanner) walkStruct(p *pkg, ts *ast.TypeSpec, tag, prefix string, depth int) {
	st, ok := ts.Type.(*ast.StructType)
	if !ok || depth > 8 {
		return
	}
	file := p.files[ts]
	for _, f := range st.Fields.List {
		name, opts := tagOf(f, tag)
		if name == "-" {
			continue
		}
		if len(f.Names) == 0 && (name == "" || strings.Contains(opts, "inline")) {
			// 嵌入结构体：字段平铺到当前层（如 BasicOption）
			if q, inner := s.resolve(p, file, f.Type); inner != nil {
				s.walkStruct(q, inner, tag, prefix, depth+1)
			}
			continue
		}
		if name == "" {
			continue
		}
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		typ := s.typeName(p, file, f.Type)
		s.record(path, typ, p, f.Pos())
		if depth == 0 && prefix == "" && opaque[name] {
			continue
		}
		s.descend(p, file, f.Type, tag, path, depth)
	}
}

// descend 进入嵌套结构体；list/map 的元素若是结构体，用 path（不加下标）继续。
func (s *scanner) descend(p *pkg, file string, e ast.Expr, tag, path string, depth int) {
	switch t := e.(type) {
	case *ast.StarExpr:
		s.descend(p, file, t.X, tag, path, depth)
	case *ast.ArrayType:
		s.descend(p, file, t.Elt, tag, path, depth)
	case *ast.MapType:
		s.descend(p, file, t.Value, tag, path, depth)
	default:
		if q, ts := s.resolve(p, file, e); ts != nil {
			if _, ok := ts.Type.(*ast.StructType); ok {
				s.walkStruct(q, ts, tag, path, depth+1)
			}
		}
	}
}

func tagOf(f *ast.Field, tag string) (name, opts string) {
	if f.Tag == nil {
		return "", ""
	}
	raw, _ := strconv.Unquote(f.Tag.Value)
	v, ok := reflect.StructTag(raw).Lookup(tag)
	if !ok {
		return "", ""
	}
	name, opts, _ = strings.Cut(v, ",")
	return name, opts
}

func (s *scanner) typeName(p *pkg, file string, e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		switch t.Name {
		case "string":
			return "string"
		case "bool":
			return "bool"
		case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
			return "int"
		case "float32", "float64":
			return "float"
		case "any":
			return "any"
		}
		if q, ts := s.resolve(p, file, t); ts != nil {
			if _, ok := ts.Type.(*ast.StructType); ok {
				return "object"
			}
			return s.typeName(q, q.files[ts], ts.Type)
		}
		return "named:" + t.Name
	case *ast.StarExpr:
		return s.typeName(p, file, t.X)
	case *ast.ArrayType:
		return "list<" + s.typeName(p, file, t.Elt) + ">"
	case *ast.MapType:
		return "map<" + s.typeName(p, file, t.Value) + ">"
	case *ast.InterfaceType:
		return "any"
	case *ast.SelectorExpr:
		if q, ts := s.resolve(p, file, t); ts != nil {
			if _, ok := ts.Type.(*ast.StructType); ok {
				return "object"
			}
			return s.typeName(q, q.files[ts], ts.Type)
		}
		if x, ok := t.X.(*ast.Ident); ok {
			return "named:" + x.Name + "." + t.Sel.Name
		}
	}
	return "any"
}

// walkSwitch 处理 ParseProxy 这类函数：switch 的每个 case 字符串对应一种类型，
// case 体内第一个复合字面量的类型就是该类型的选项结构体。
func (s *scanner) walkSwitch(p *pkg, r root) error {
	fn, ok := p.funcs[r.fn]
	if !ok {
		return fmt.Errorf("%s 中没有函数 %s", r.dir, r.fn)
	}
	pos := p.fset.Position(fn.Pos())
	file := r.dir + "/" + filepath.Base(pos.Filename)
	var common *ast.TypeSpec
	if r.common != "" {
		common = p.types[r.common]
	}
	found := 0
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		cc, ok := n.(*ast.CaseClause)
		if !ok {
			return true
		}
		var lit *ast.CompositeLit
		for _, st := range cc.Body {
			ast.Inspect(st, func(m ast.Node) bool {
				if lit != nil {
					return false
				}
				if cl, ok := m.(*ast.CompositeLit); ok {
					if _, isSel := cl.Type.(*ast.SelectorExpr); isSel {
						lit = cl
					} else if id, isId := cl.Type.(*ast.Ident); isId && strings.HasSuffix(id.Name, "Option") {
						lit = cl
					}
				}
				return true
			})
		}
		if lit == nil {
			return false
		}
		q, ts := s.resolve(p, file, lit.Type)
		if ts == nil {
			return false
		}
		for _, v := range cc.List {
			bl, ok := v.(*ast.BasicLit)
			if !ok || bl.Kind != token.STRING {
				continue
			}
			name, _ := strconv.Unquote(bl.Value)
			prefix := r.path + "." + name
			if common != nil {
				s.walkStruct(p, common, r.tag, prefix, 1)
			}
			s.walkStruct(q, ts, r.tag, prefix, 1)
			found++
		}
		return false
	})
	if found == 0 {
		return fmt.Errorf("%s.%s 中没有找到任何分支", r.dir, r.fn)
	}
	return nil
}
