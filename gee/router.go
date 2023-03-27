/*
 * @Description:
 * @version:
 * @Author: Steven
 * @Date: 2023-03-25 13:34:00
 */
package gee

import (
	"net/http"
	"strings"
)

type router struct {
	roots    map[string]*node
	handlers map[string]HandlerFunc
}

// roots key eg, roots['GET'] roots['POST']
// handlers key eg, handlers['GET-/p/:lang/doc'], handlers['POST-/p/book']

func newRouter() *router {
	return &router{
		roots:    make(map[string]*node),
		handlers: make(map[string]HandlerFunc),
	}
}

// Only one * is allowed
func parsePattern(pattern string) []string {
	vs := strings.Split(pattern, "/") //将注册的路由用/分段

	parts := make([]string, 0, len(vs))
	for _, item := range vs {
		//去掉分段结果vs当中元素值为空的元素！
		//如注册的路由为"/"，经过strings.Split分段之后vs的值为[]string{"", ""}，需要将两个值为空字符串的元素去掉
		//这类的路由/about，因为符号/位于字符串首位，所以经过 strings.Split分割之后的切片首个元素值也是空的！
		if item != "" {
			parts = append(parts, item)
			if item[0] == '*' { //首个字母是*符号，匹配终止,后面的路径子串不需要再匹配了！
				break
			}
		}
	}
	return parts
}

func (r *router) addRoute(method string, pattern string, handler HandlerFunc) {
	parts := parsePattern(pattern)

	key := method + "-" + pattern
	_, ok := r.roots[method] //同一个请求方法的路由放到一个键值对里，比如get请求的路由放到一个键值对，post请求放到另一个键值对
	if !ok {
		r.roots[method] = &node{} //&node{}就是根节点！
	}
	r.roots[method].insert(pattern, parts, 0) //注册路由规则，这里是重点
	r.handlers[key] = handler                 //路由到处理方法handler的映射
}

// 路由规则匹配
// 找到匹配成功之后的节点，并且将参数和值映射为键值对
func (r *router) getRoute(method string, path string) (*node, map[string]string) {
	searchParts := parsePattern(path) //这里的path是请求的path而不是注册的路由
	params := make(map[string]string)
	root, ok := r.roots[method] //根据请求动作获取对应的树信息

	if !ok {
		return nil, nil
	}

	n := root.search(searchParts, 0) //具体怎么匹配的在该方法里

	if n != nil { //匹配成功
		parts := parsePattern(n.pattern) //这里就需要拿到注册时
		//处理参数到值的键值对映射
		for index, part := range parts { //parts={":land","doc"}
			//比如注册的路由为/:land/doc,那么此时n.pattern="/:land/doc"，parts={":land","doc"}
			//请求的Url为http://127.0.0.1:9999/go/doc，那么path="/go/doc"，searchParts={"go","doc"}
			if part[0] == ':' {
				params[part[1:]] = searchParts[index] //part[1:]=land，searchParts[index]=go，最终就将go映射到了land里
			}
			//注册路由为/static/*filepath,那么此时n.pattern="/static/*filepath"，parts={"static","*filepath"}
			//请求URL为http://localhost:9999/static/fav.ico/aa/bb?name=aa，那么path="/static/fav.ico/aa/bb"，searchParts={"static","fav.ico","aa","bb"}
			if part[0] == '*' && len(part) > 1 {
				params[part[1:]] = strings.Join(searchParts[index:], "/") //part[1:]=filepath，searchParts[index:]=searchParts[1:]={"fav.ico","aa","bb"}
				//{"fav.ico","aa","bb"}经过 strings.Join({"fav.ico","aa","bb"}, "/")处理之后变为字符串，值为fav.ico/aa/bb
				//strings.Join和strings.Split(pattern, "/")是相对的，好比php的implode和explode
				break
			}
		}
		return n, params
	}

	return nil, nil
}

func (r *router) handle(c *Context) {
	n, params := r.getRoute(c.Method, c.Path)

	if n != nil {
		key := c.Method + "-" + n.pattern
		c.Params = params
		c.handlers = append(c.handlers, r.handlers[key])
	} else {
		c.handlers = append(c.handlers, func(c *Context) {
			c.String(http.StatusNotFound, "404 NOT FOUND: %s\n", c.Path)
		})
	}
	c.Next()
}
