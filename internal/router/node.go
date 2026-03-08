package router

import (
	"NovaGate/internal/pipline"
	"strings"
)

// Node 路由树的节点
// 按斜杠 / 将 URI 进行分段，比如 /api/user/:id 会被拆分为三个 Segment: api、user和:id
type Node struct {
	pattern  string                // 完整的路由路径，如"/api/user/:id"，只有叶子节点（真正的路由终点）的这个字段才会有值
	part     string                // 当前节点代表的 path 段落，如"user" 或 ":id"
	children []*Node               // 当前节点的所有子节点
	isWild   bool                  // 是否是通配符节点（当 part 包含 ':' 或 '*' 时为 true）
	handlers []pipline.HandlerFunc // 路由终点挂载的处理函数，用于存储这条路由上的所有中间件和最终处理器
}

// matchChild 查找第一个匹配成功的子节点（用于插入路由 Insert）
func (n *Node) matchChild(part string) *Node {
	for _, child := range n.children {
		if child.part == part || child.isWild {
			return child
		}
	}
	return nil
}

// matchChildren 查找所有匹配成功的子节点（用于匹配请求 Search）
// 当前请求片段可能同时匹配精确路由和动态参数路由
func (n *Node) matchChildren(part string) []*Node {
	nodes := make([]*Node, 0)
	for _, child := range n.children {
		if child.part == part || child.isWild {
			nodes = append(nodes, child)
		}
	}
	return nodes
}

// insert 递归插入节点
// pattern: 完整路由 /api/user/:id
// parts: 切割后的片段 [api, user, :id]
// height: 当前遍历到了第几层 (对应 parts 的索引)
func (n *Node) insert(pattern string, parts []string, height int, handlers []pipline.HandlerFunc) {
	// 递归终止条件：已经遍历完所有的 parts
	if len(parts) == height {
		n.pattern = pattern   // 标记这是一个完整的路由终点
		n.handlers = handlers // 挂载处理逻辑
		return
	}

	part := parts[height]
	child := n.matchChild(part) // 看看当前层有没有这个 part

	// 如果没有，就新建一个子节点并挂载上去
	if child == nil {
		child = &Node{
			part:   part,
			isWild: part[0] == ':' || part[0] == '*', // 首字母是 : 或 * 说明是动态参数
		}
		n.children = append(n.children, child)
	}

	// 带着下一层的片段，继续往下递归
	child.insert(pattern, parts, height+1, handlers)
}

// search 递归查找节点，发生在真实请求到来时（网关热点代码）
// 使用 DFS 深度优先遍历
func (n *Node) search(parts []string, height int) *Node {
	// 递归终止条件：
	// 1. 遍历完了请求的所有 parts
	// 2. 或者遇到了 '*' 通配符 (星号匹配其后所有内容，直接终止)
	if len(parts) == height || strings.HasPrefix(n.part, "*") {
		// 走到这里，必须确保 pattern 不为空才算匹配成功
		// 防止刚好走到中间节点，比如树里有 /api/user，请求是 /api，虽然走完了但不能算成功
		if n.pattern == "" {
			return nil
		}
		return n
	}

	part := parts[height]
	// 找出当前层所有能匹配的子节点 (包含精确匹配和 :id 这种泛匹配)
	children := n.matchChildren(part)

	for _, child := range children {
		// 深度优先，继续往下找
		result := child.search(parts, height+1)
		if result != nil {
			return result
		}
	}

	return nil // 找了一圈没找到，返回 404
}

// 判断一个 Segment 是否是动态参数
func isWildSegment(part string) bool {
	return strings.HasPrefix(part, ":") || strings.HasPrefix(part, "*")
}
