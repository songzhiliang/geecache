package gee

import (
	"fmt"
	"log"
	"net/http"
	"runtime"
	"strconv"
	"strings"
)

// print stack trace for debug
func trace(message string) string {
	var pcs [32]uintptr
	//Callers 用来返回调用栈的程序计数器。
	n := runtime.Callers(0, pcs[:]) //n就是有多少个堆栈调用信息

	var str strings.Builder
	str.WriteString(message + "\nTraceback:n=" + strconv.Itoa(n))
	for _, pc := range pcs[:n] { //遍历每一个堆栈信息
		fn := runtime.FuncForPC(pc)   //获取触发的的函数
		file, line := fn.FileLine(pc) //获取到调用该函数的文件名和行号
		str.WriteString(fmt.Sprintf("\n\t%s:%d-funcName:%v", file, line, fn.Name()))
	}
	return str.String()
}

func Recovery() HandlerFunc {
	return func(c *Context) {
		defer func() {
			if err := recover(); err != nil {
				message := fmt.Sprintf("%s", err)
				log.Printf("%s\n\n", trace(message))
				c.Fail(http.StatusInternalServerError, "Internal Server Error")
			}
		}()

		c.Next()
	}
}
