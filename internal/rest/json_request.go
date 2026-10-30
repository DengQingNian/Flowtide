package rest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/DengQingNian/Flowtide/api"
)

// decodeSingleJSON 解码唯一的非空 JSON 值并检查尾部，防止拼接请求只执行前半部分。
// @param r 请求
// @param target 目标对象
// @param allowEmpty 是否兼容空查询体
// @param strict 是否拒绝未知字段
// @return 输入非法或读取错误
func decodeSingleJSON(r *http.Request, target any, allowEmpty, strict bool) error {
	reader := &io.LimitedReader{R: r.Body, N: MaxUploadSize + 1}
	decoder := json.NewDecoder(reader)
	var raw json.RawMessage
	err := decoder.Decode(&raw)
	// 只有查询支持空体；null 不能被当作空对象或合法创建请求。
	if err == io.EOF && allowEmpty {
		return nil
	}
	if err != nil || bytes.Equal(raw, []byte("null")) || len(raw) > MaxUploadSize {
		return api.NewError("REST-0109", "请求体 JSON 非法", api.ErrBadRequest)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF || reader.N == 0 {
		return api.NewError("REST-0109", "请求体必须只包含一个 JSON 值", api.ErrBadRequest)
	}
	decoder = json.NewDecoder(bytes.NewReader(raw))
	if strict {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(target); err != nil {
		return api.NewError("REST-0109", "请求体 JSON 非法", api.ErrBadRequest)
	}
	return nil
}
