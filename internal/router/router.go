package router

import (
	"NovaGate/internal/pipline"
	"strings"
)

type Router struct {
	// O(1) 的哈希表，用于快速匹配静态路由
	// 例如: "/ping", "/health"
	static map[string][]pipline.HandlerFunc

	// roots 用于存储动态路由树，用于匹配包含 :id 或 *name 的路由
	// 按 Method 区分，为每个 HTTP Method (GET, POST...) 建立一棵独立的树
	roots map[string]*Node
}

// NewRouter 创建路由
func NewRouter() *Router {
	return &Router{
		roots:  make(map[string]*Node),
		static: make(map[string][]pipline.HandlerFunc),
	}
}

// AddRoute 注册路由
func (r *Router) AddRoute(method string, pattern string, handlers ...pipline.HandlerFunc) {
	// 简单判断：如果不包含 : 也没有 *，就是静态路由，直接进哈希表
	if !strings.Contains(pattern, ":") && !strings.Contains(pattern, "*") {
		r.static[method+"-"+pattern] = handlers
		return
	}

	// 动态路由，插入前缀树
	if _, ok := r.roots[method]; !ok {
		r.roots[method] = &Node{}
	}
	parts := parsePattern(pattern)
	r.roots[method].insert(pattern, parts, 0, handlers)
}

// GetRoute 匹配路由并提取动态参数
// 返回值：处理函数, 解析出的参数map (如 {"id": "9527"}), 是否匹配成功
func (r *Router) GetRoute(method string, path string) ([]pipline.HandlerFunc, map[string]string, bool) {
	// 1. 优先去 O(1) 的 Hash 表里找
	if handlers, ok := r.static[method+"-"+path]; ok {
		return handlers, nil, true
	}

	// 2. Hash 表没命中，去树里按段落搜索
	root, ok := r.roots[method]
	if !ok {
		return nil, nil, false
	}

	searchParts := parsePattern(path)
	node := root.search(searchParts, 0)

	if node != nil {
		// 匹配成功，把动态参数提取出来，比如把 /api/user/9527 里的 9527 提取成 id
		params := make(map[string]string)
		parts := parsePattern(node.pattern)
		for i, part := range parts {
			if part[0] == ':' { // 解析 :id
				params[part[1:]] = searchParts[i]
			}
			if part[0] == '*' && len(part) > 1 { // 解析 *filepath
				params[part[1:]] = strings.Join(searchParts[i:], "/")
				break
			}
		}
		return node.handlers, params, true
	}

	return nil, nil, false
}

// 解析路径，将 "/api/user/:id" 切割成 ["api", "user", ":id"]
func parsePattern(pattern string) []string {
	vs := strings.Split(pattern, "/")
	parts := make([]string, 0)
	for _, item := range vs {
		if item != "" {
			parts = append(parts, item)
			if item[0] == '*' { // 如果遇到 *，后面就算有斜杠也不切了，直接结束
				break
			}
		}
	}
	return parts
}
