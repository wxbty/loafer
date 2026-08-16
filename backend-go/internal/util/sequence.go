package util

import (
	"sort"
	"strconv"
	"strings"
)

// CompareSequenceNumber 按数字语义比较形如 "1.2.10" 的序号字符串。
// 字符串排序下 "1.10" < "1.2"，会导致 1.10 在 1.2 之前执行而依赖未满足，
// 因此所有 sequence_number 排序都应使用本函数，而非数据库的字符串 ORDER BY。
// 返回 -1 / 0 / 1，空字符串视为最小。
func CompareSequenceNumber(a, b string) int {
	if a == b {
		return 0
	}
	pa := strings.Split(a, ".")
	pb := strings.Split(b, ".")
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		var na, nb int
		if i < len(pa) {
			na, _ = strconv.Atoi(strings.TrimSpace(pa[i]))
		}
		if i < len(pb) {
			nb, _ = strconv.Atoi(strings.TrimSpace(pb[i]))
		}
		if na < nb {
			return -1
		}
		if na > nb {
			return 1
		}
	}
	return 0
}

// SortBySequenceNumber 对任意切片按 SequenceNumber 字段做稳定排序（数字语义）。
// 调用方需保证返回的切片元素类型含 SequenceNumber 字段；seq 提取该字段。
// 使用稳定排序，使序号相同的元素保持查询返回（id 升序）的相对顺序。
func SortBySequenceNumber[T any](items []T, seq func(T) string) {
	sort.SliceStable(items, func(i, j int) bool {
		return CompareSequenceNumber(seq(items[i]), seq(items[j])) < 0
	})
}
