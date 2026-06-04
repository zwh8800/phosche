// Package util provides common helper functions.
package util

// Truncate 截断字符串用于日志输出，避免超长内容淹没日志。
// 如果字符串长度超过 maxLen，截断后追加 "..." 后缀。
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// TruncateBytes 截断字节数组用于日志输出，避免超长内容淹没日志。
// 如果字节数组长度超过 maxLen，截断后追加 "..." 后缀。
func TruncateBytes(b []byte, maxLen int) string {
	if len(b) <= maxLen {
		return string(b)
	}
	return string(b[:maxLen]) + "..."
}
