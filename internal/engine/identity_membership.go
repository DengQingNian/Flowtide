package engine

import (
	"encoding/json"
	"strings"
)

// membershipKey 对用户和组进行无碰撞编码，标识中可保留竖线及其他普通字符。
// @param userID 用户标识
// @param groupID 组标识
// @return 成员关系键
func membershipKey(userID, groupID string) string {
	data, _ := json.Marshal([2]string{userID, groupID})
	return string(data)
}

// membershipPair 读取新格式成员键，兼容旧快照的用户与组分隔形式。
// @param key 成员关系键
// @return 用户标识
// @return 组标识
// @return 是否有效
func membershipPair(key string) (string, string, bool) {
	var pair []string
	if strings.HasPrefix(key, "[") && json.Unmarshal([]byte(key), &pair) == nil && len(pair) == 2 {
		return pair[0], pair[1], true
	}
	// 旧格式按首次竖线分割；旧实现已覆盖的冲突关系无法凭快照恢复。
	legacy := strings.SplitN(key, "|", 2)
	if len(legacy) != 2 {
		return "", "", false
	}
	return legacy[0], legacy[1], true
}

// hasMembership 查询当前成员关系，并支持旧测试和旧内存状态的普通成员键。
// @param userID 用户标识
// @param groupID 组标识
// @return 是否为成员
func (e *Engine) hasMembership(userID, groupID string) bool {
	if e.state.Memberships[membershipKey(userID, groupID)] {
		return true
	}
	key := userID + "|" + groupID
	user, group, valid := membershipPair(key)
	return valid && user == userID && group == groupID && e.state.Memberships[key]
}
