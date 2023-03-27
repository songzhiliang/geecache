package gee

import (
	"fmt"
	"strings"
)

// node就是一个树下的某一个节点，children下面存储该节点下的子节点
type node struct {
	pattern  string  // 有值即标识该节点为叶子节点，值为注册时的路由/:lang/doc，这里值为什么是注册时的路由，因为需要将模糊匹配到的值赋值给参数lang
	part     string  // 路由中的一部分，其值是该节点的值，例如 :lang。
	children []*node // 该节点下的子节点，例如:lang节点下的子节点有 [doc, tutorial, intro]，
	isWild   bool    // 表示该节点是否是模糊匹配，part 含有 : 或 * 时为true,
}

// 遍历节点n的子节点，寻找值为part的节点，
// 每次都是从根节点/开始找
func (n *node) findParent(part string) *node {
	for _, child := range n.children {
		//if child.part == part || child.isWild {//大佬这里经过测试有问题，我也没看出来加了一个child.isWild的作用，所以暂时注释掉
		if child.part == part {
			return child
		}
	}
	return nil
}

// 遍历n节点下的子节点
// 查找节点值为part 或者 节点属性isWild为true代表模糊匹配的节点
func (n *node) matchChildren(part string) []*node {
	nodes := make([]*node, 0)
	for _, child := range n.children {
		if child.part == part || child.isWild {
			nodes = append(nodes, child)
		}
	}
	return nodes
}

// 第一个元素是注册的路由字符串,第二个参数是经过parsePattern函数处理过之后的路由字符串切片，元素是两个符号/中间的字符串
// 如：
// 注册是路由为/:lang/doc：pattern=/:lang/doc，parts=[]string{":lang", "doc"}
// 注册是路由为/static/*filepath：pattern=/static/*filepath，parts=[]string{"static", "*filepath"}
// 第三个参数代表当前处理的是parts切片里的第几个元素，0代表第一个元素！

// insert：功能就是最终将注册的路由组装成一个树
// 每次调用addRoute注册路由规则时,n都是根节点
func (n *node) insert(pattern string, parts []string, height int) {
	fmt.Printf("n=%p;pattern=%v;parts=%#v;height=%v\n", n, pattern, parts, height)
	if height == 0 {
		fmt.Println("---start---")
		printChild(n)
		fmt.Println("---start---")
	}
	//将树的叶子节点上的pattern值修改为注册的路由
	//如果注册的路由为"/"，因为parts为nil,height其实值为0，所以会直接修改n.pattern = pattern，直接将根节点的pattern修改为为/
	if len(parts) == height {
		n.pattern = pattern
		fmt.Println("---end---")
		printChild(n)
		fmt.Println("---end---")
		return
	}

	//如果注册的路由为/:lang/doc
	part := parts[height]       //那么刚进入insert函数时，part的值为:lang
	child := n.findParent(part) //在n节点的子节点上，寻找值为:lang的节点
	if child == nil {           //没找到的话
		child = &node{part: part, isWild: part[0] == ':' || part[0] == '*'} //初始化part节点
		n.children = append(n.children, child)                              //将该节点当作n节点的子节点
	}
	child.insert(pattern, parts, height+1)
}

// 也是一个递归调用函数
// 找到匹配成功之后的最后一个节点
// 第一个参数：parts是请求URL符号/分割的子串，如请求URL为http://localhost:9999//static/fav.ico/aa/bb?name=aa，
// 那么parts={"static","fav.ico","aa","bb"}
// 第二个参数跟insert方法的第三个参数一样，起始值为0
func (n *node) search(parts []string, height int) *node {
	fmt.Printf("n=%p;parts=%#v;height=%v\n", n, parts, height)
	//len(parts) == height：该请求url的path已经全部匹配完毕
	//匹配到了一个节点值首字母是*的节点，代表不用再往下匹配了
	if len(parts) == height || strings.HasPrefix(n.part, "*") {
		if n.pattern == "" { //如果该节点不是叶子节点，代表匹配失败。其实这里可以修改一下，因为strings.HasPrefix(n.part, "*")为ture的话，n.pattern == "" 肯定为假
			fmt.Printf("[匹配失败]n=%p;parts=%#v;height=%v\n", n, parts, height)
			return nil
		}
		fmt.Printf("[匹配成功]n=%p;parts=%#v;height=%v\n", n, parts, height)
		return n //匹配成功，返回匹配到的最后一个节点
	}

	part := parts[height]
	children := n.matchChildren(part) //在节点n上查找n的子节点，返回子节点值为part或者子节点属性isWild为true的节点，即支持模糊匹配的节点

	for _, child := range children {
		fmt.Printf("[匹配中]n=%p;parts=%#v;height=%v;child.val=%#v\n", child, parts, height, *child)
		result := child.search(parts, height+1) //继续基于该节点，往下找。即找child节点的子节点
		if result != nil {                      //匹配成功，无需再匹配了，直接return
			return result
		}
	}

	return nil
}
