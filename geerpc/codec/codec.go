/*
 * @Description:不同消息编解码实现
 * @version:
 * @Author: Steven
 * @Date: 2023-03-29 22:58:40
 */
package codec

import "io"

type Header struct {
	ServiceMethod string // format "Service.Method" ServiceMethod 是服务名和方法名，通常与 Go 语言中的结构体和方法相映射
	Seq           uint64 // sequence number chosen by client 。请求的序号，也可以认为是某个请求的 ID，用来区分不同的请求
	Error         string //错误信息，客户端置为空，服务端如果如果发生错误，将错误信息置于 Error 中
}

//消息体进行编解码的接口
type Codec interface {
	io.Closer //实现io.Closer接口即可，io.Closer接口里约定了Close() error方法，所以必须得实现Close() error方法
	ReadHeader(*Header) error
	ReadBody(interface{}) error
	Write(*Header, interface{}) error
}

//抽象出 Codec 的构造函数
//约定客户端和服务端创建Codec，
type NewCodecFunc func(io.ReadWriteCloser) Codec

type Type string

//制定了两种Codec,即gob和json
const (
	GobType  Type = "application/gob"
	JsonType Type = "application/json" // not implemented
)

var NewCodecFuncMap map[Type]NewCodecFunc

func init() { //NewCodecFuncMap的初始化
	NewCodecFuncMap = make(map[Type]NewCodecFunc)
	NewCodecFuncMap[GobType] = NewGobCodec //客户端和服务端可以通过 Codec 的 Type 得到构造函数
}
