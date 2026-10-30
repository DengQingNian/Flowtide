package rest

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/DengQingNian/Flowtide/api"
)

// writeIdentityError 将有明确原版依据的身份资源及创建冲突映射为原版错误。
// @param w 响应
// @param r 请求
// @param err 服务错误
// @param group 是否组资源
// @return 无
func writeIdentityError(w http.ResponseWriter, r *http.Request, err error, group bool) {
	var detail *api.Error
	if errors.As(err, &detail) {
		// 只转换身份资源中已有原版证据的错误，其他故障保留公共状态码。
		switch detail.Code {
		case "ENGINE-1001", "ENGINE-1003":
			writeRuntimeFailure(w, 400, "Id cannot be null.")
			return
		case "ENGINE-1701":
			writeRuntimeFailure(w, 404, fmt.Sprintf("User info with key '%s' does not exists for user '%s'.", r.PathValue("key"), r.PathValue("id")))
			return
		case "ENGINE-1703":
			writeRuntimeFailure(w, 404, fmt.Sprintf("The user with id '%s' does not have a picture.", r.PathValue("id")))
			return
		}
	}
	if errors.Is(err, api.ErrNotFound) {
		resource := "user"
		if group {
			resource = "group"
		}
		writeRuntimeFailure(w, 404, fmt.Sprintf("Could not find a %s with id '%s'.", resource, r.PathValue("id")))
		return
	}
	writeError(w, err)
}

// identityUserResponse 生成原版公开用户对象，不将内部角色、失败次数或密码哈希混入响应。
// @param r 请求
// @param user 用户
// @return 完整公开对象
// @return 读取头像状态失败
func (s *Server) identityUserResponse(r *http.Request, user api.User) (map[string]any, error) {
	response := map[string]any{"id": user.ID, "firstName": runtimeNullableString(user.FirstName), "lastName": runtimeNullableString(user.LastName), "displayName": runtimeNullableString(user.DisplayName), "email": runtimeNullableString(user.Email), "url": taskResourceURL(r, "identity/users", user.ID), "pictureUrl": nil}
	if user.TenantID != "" {
		response["tenantId"] = user.TenantID
	}
	// 只有实际存在头像时才公开其资源链接，其他读取故障不能伪装成无头像。
	_, _, err := s.engine.IdentityService().GetUserPicture(r.Context(), user.ID)
	if err == nil {
		response["pictureUrl"] = taskResourceURL(r, "identity/users", user.ID) + "/picture"
	} else if !errors.Is(err, api.ErrNotFound) {
		return nil, err
	}
	return response, nil
}

// writeIdentityUser 输出用户响应并处理头像读取故障。
// @param w 响应
// @param r 请求
// @param status 状态码
// @param user 用户
// @return 无
func (s *Server) writeIdentityUser(w http.ResponseWriter, r *http.Request, status int, user api.User) {
	response, err := s.identityUserResponse(r, user)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, status, response)
}

// identityGroupResponse 生成含显式空值及资源 URL 的组对象。
// @param r 请求
// @param group 组
// @return 原版组对象
func identityGroupResponse(r *http.Request, group api.Group) map[string]any {
	return map[string]any{"id": group.ID, "name": runtimeNullableString(group.Name), "type": runtimeNullableString(group.Type), "url": taskResourceURL(r, "identity/groups", group.ID)}
}

// identityPagination 解析身份列表分页并限制原版排序字段。
// @param r 请求
// @param group 是否组列表
// @return 起点
// @return 数量
// @return 排序字段
// @return 排序方向
// @return 参数错误
func identityPagination(r *http.Request, group bool) (int, int, string, string, error) {
	body, err := taskQueryFromRequest(r, nil)
	if err != nil {
		return 0, 0, "", "", err
	}
	start, size, field, order, err := body.pagination()
	if err != nil {
		return 0, 0, "", "", err
	}
	// 零大小及大页沿用原版语义，实际条数由结果切片决定。
	if requested, err := body.intField("size"); err != nil {
		return 0, 0, "", "", err
	} else if requested != nil && *requested >= 0 {
		size = *requested
	}
	if field == "" {
		field = "id"
	}
	allowed := map[string]bool{"id": true, "firstName": true, "lastName": true, "displayName": true, "email": true}
	if group {
		allowed = map[string]bool{"id": true, "name": true, "type": true}
	}
	if !allowed[field] {
		return 0, 0, "", "", api.NewError("REST-RUNTIME-SORT", fmt.Sprintf("Value for param 'sort' is not valid, '%s' is not a valid property", field), api.ErrBadRequest)
	}
	return start, size, field, order, nil
}

// identityUserSortKey 返回原版用户排序字段访问器。
// @param field 排序字段
// @return 访问器
// @return 是否有效
func identityUserSortKey(field string) (func(api.User) any, bool) {
	// 每个字段保持字符串排序，不能把显示名与姓名拼接后排序。
	switch field {
	case "id":
		return func(user api.User) any { return user.ID }, true
	case "firstName":
		return func(user api.User) any { return user.FirstName }, true
	case "lastName":
		return func(user api.User) any { return user.LastName }, true
	case "displayName":
		return func(user api.User) any { return user.DisplayName }, true
	case "email":
		return func(user api.User) any { return user.Email }, true
	}
	return nil, false
}

// identityGroupSortKey 返回原版组排序字段访问器。
// @param field 排序字段
// @return 访问器
// @return 是否有效
func identityGroupSortKey(field string) (func(api.Group) any, bool) {
	// 原版仅公开标识、名称和类型排序。
	switch field {
	case "id":
		return func(group api.Group) any { return group.ID }, true
	case "name":
		return func(group api.Group) any { return group.Name }, true
	case "type":
		return func(group api.Group) any { return group.Type }, true
	}
	return nil, false
}

// identityQueryMatches 判断列表中字符串等值及模糊条件。
// @param r 请求
// @param fields 当前对象公开的可查询字段
// @return 是否命中全部条件
func identityQueryMatches(r *http.Request, fields map[string]string) bool {
	// 原版只有姓名、显示名、邮箱及组名称提供模糊查询，未知的 Like 参数必须忽略。
	for field, value := range fields {
		if expected, ok := r.URL.Query()[field]; ok && len(expected) > 0 && value != expected[0] {
			return false
		}
		if field == "id" || field == "tenantId" || field == "type" {
			continue
		}
		if pattern := r.URL.Query().Get(field + "Like"); pattern != "" && !identityLikeMatches(value, pattern) {
			return false
		}
	}
	return true
}

// identityLikeMatches 按原版 SQL 模式匹配完整字符串，百分号和下划线分别代表任意串及单字符。
// @param value 待匹配属性
// @param pattern SQL 模式
// @return 是否匹配
func identityLikeMatches(value, pattern string) bool {
	var expression strings.Builder
	expression.WriteString("(?s)^")
	// 普通字符先转义，避免调用方输入被当作正则表达式执行。
	for _, char := range pattern {
		switch char {
		case '%':
			expression.WriteString(".*")
		case '_':
			expression.WriteString(".")
		default:
			expression.WriteString(regexp.QuoteMeta(string(char)))
		}
	}
	expression.WriteString("$")
	matched, _ := regexp.MatchString(expression.String(), value)
	return matched
}
