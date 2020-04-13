package util

import "strings"

func KeysStringInt(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}

func KeysStringBool(m map[string]bool) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}

type StringInt struct {
	Str string
	I   int
}

func EnumListToMap(list []StringInt) map[string]int {
	out := make(map[string]int, len(list))
	for _, tf := range list {
		out[tf.Str] = tf.I
	}
	return out
}
