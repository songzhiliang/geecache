/*
 * @Description:不负责缓存值的存储和获取，只负责缓存值的类型转化！
 * @version:
 * @Author: Steven
 * @Date: 2023-03-27 15:40:40
 */
package geecache

// A ByteView holds an immutable view of bytes.
type ByteView struct {
	b []byte //缓存值，为什么不用字符串，因为还可以支持存储图片
}

// Len returns the view's length
func (v ByteView) Len() int { //既然处理缓存值，那么必然得实现缓存值接口Value
	return len(v.b)
}

// ByteSlice returns a copy of the data as a byte slice.
func (v ByteView) ByteSlice() []byte {
	return cloneBytes(v.b) //返回一个拷贝，防止缓存值被修改
}

// String returns the data as a string, making a copy if necessary.
func (v ByteView) String() string {
	return string(v.b)
}

func cloneBytes(b []byte) []byte {
	c := make([]byte, len(b))
	copy(c, b)
	return c
}
