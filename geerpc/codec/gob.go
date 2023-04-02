/*
 * @Description:GOB编解码
 * @version:
 * @Author: Steven
 * @Date: 2023-03-29 22:58:40
 */
package codec

import (
	"bufio"
	"encoding/gob"
	"io"
	"log"
)

type GobCodec struct {
	conn io.ReadWriteCloser //由构造函数传入，通常是通过 TCP 或者 Unix 建立 socket 时得到的链接实例
	buf  *bufio.Writer      //为了防止阻塞而创建的带缓冲的 Writer，一般这么做能提升性能
	dec  *gob.Decoder       //解码
	enc  *gob.Encoder       //编码
}

var _ Codec = (*GobCodec)(nil)

// GOB的构造函数
func NewGobCodec(conn io.ReadWriteCloser) Codec {
	//我们不直接往conn里写入数据，采用带有缓冲区的buf
	buf := bufio.NewWriter(conn) //因为conn里包含一次请求的诸多信息，比较大，为了提升性能，防止造成内存泄露，我们将其写入待缓冲区的buf中
	rdbuf := bufio.NewReader(conn)
	return &GobCodec{
		conn: conn,
		buf:  buf,                   //用来将缓冲区的数据冲洗到conn中
		dec:  gob.NewDecoder(rdbuf), //初始化解码器，不可能没发送一次请求数据，生成一个解码器，那样效率很低。这里采用带缓冲的读！

		enc: gob.NewEncoder(buf),
		//因为直接往conn里写，效率很低，所以这里我们用带缓冲的buf来创建一个编码器！当调用GobCodec.Write方法时，先往这个编码器里写!
		//因为buff是指针类型，地址是一个，所以也就是往字段buf中写，写完之后再将buf中的数据冲洗到conn中即可！
		//为什么不直接用buf字段，来生成编码器，因为一次连接多次发送请求数据，我们不可能没发一次请求数据，生成一次编码器，那样效率很低！
	}
}

// 读取header数据，从c.dec解码器中读取数据赋值给h
func (c *GobCodec) ReadHeader(h *Header) error {
	return c.dec.Decode(h)
}

// 读取body数据，从c.dec解码器中读取数据赋值给body
func (c *GobCodec) ReadBody(body interface{}) error {
	return c.dec.Decode(body)
}

// 基于该编解码实例向服务端发送请求
func (c *GobCodec) Write(h *Header, body interface{}) (err error) {
	defer func() {
		_ = c.buf.Flush() //函数执行完毕，将缓冲区的数据，冲洗到conn中
		if err != nil {   //一次请求报错，直接关闭连接
			_ = c.Close()
		}
	}()
	//将header数据先写入带缓冲的buf中， 即gob.NewEncoder(buf)
	if err := c.enc.Encode(h); err != nil {
		log.Println("rpc codec: gob error encoding header:", err)
		return err
	}
	//log.Println("Write body before", body)
	if err := c.enc.Encode(body); err != nil {
		log.Println("rpc codec: gob error encoding body:", err)
		return err
	}
	return nil
}

func (c *GobCodec) Close() error {
	return c.conn.Close()
}
