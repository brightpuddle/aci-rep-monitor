package main

import (
	"github.com/tidwall/sjson"
)

type json struct {
	str string
}

func (j json) set(k string, v interface{}) json {
	switch value := v.(type) {
	case json:
		j.str, _ = sjson.SetRaw(j.str, k, value.str)
	default:
		j.str, _ = sjson.Set(j.str, k, v)
	}
	return j
}
