package log

import (
	"fmt"
	"strings"
)

// Sprint 格式化参数，每个参数中间隔一个空格，返回格式化后的字符串
// 高效实现：使用strings.Builder预分配容量，减少内存分配，比fmt.Sprint更高效且确保参数间有空格
func Sprint(args ...interface{}) string {
	if len(args) == 0 {
		return ""
	}
	var builder strings.Builder
	// 预分配足够容量，每个参数预估16字节，减少扩容次数
	builder.Grow(len(args) * 16)
	for i, arg := range args {
		if i > 0 {
			builder.WriteByte(' ')
		}
		fmt.Fprint(&builder, arg)
	}
	return builder.String()
}
