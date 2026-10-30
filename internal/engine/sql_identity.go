package engine

import (
	"context"
	"sort"

	"github.com/DengQingNian/Flowtide/api"
	flowablerow "github.com/DengQingNian/Flowtide/internal/persistence/flowable"
)

// identityRowsLocked 导出标准身份行；头像和资料的真实值不放进 Go 控制副本。
// @return 标准身份行和可选的内部控制行
// @return 无效内部成员键或行编码错误
func (e *Engine) identityRowsLocked() ([]flowablerow.Row, error) {
	rows := make([]flowablerow.Row, 0)
	for _, id := range identitySortedKeys(e.state.Users) {
		user := e.state.Users[id]
		pictureID := ""
		if picture := e.state.UserPictures[id]; picture != nil {
			pictureID = picture.ID
			if pictureID == "" {
				pictureID = byteArrayID("", "identity/picture/"+id)
			}
			row, err := identityRow("ACT_ID_BYTEARRAY", map[string]any{"ID_": pictureID, "REV_": 1, "NAME_": picture.MimeType, "BYTES_": append([]byte(nil), picture.Data...)})
			if err != nil {
				return nil, err
			}
			rows = append(rows, row)
		}
		row, err := identityRow("ACT_ID_USER", map[string]any{"ID_": id, "REV_": 1, "FIRST_": identitySQLText(user.FirstName), "LAST_": identitySQLText(user.LastName), "DISPLAY_NAME_": identitySQLText(user.DisplayName), "EMAIL_": identitySQLText(user.Email), "PWD_": identitySQLText(user.PasswordHash), "PICTURE_ID_": identitySQLText(pictureID), "TENANT_ID_": user.TenantID})
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
		// 信息行保留原标识及显式空值，已清空的键也必须写入标准表。
		for _, key := range identitySortedKeys(e.state.UserInfo[id]) {
			infoID := e.state.UserInfoIDs[id][key]
			if infoID == "" {
				infoID = byteArrayID("", "identity/info/"+membershipKey(id, key))
			}
			var value any
			if entry := e.state.UserInfo[id][key]; entry != nil {
				value = *entry
			}
			row, err := identityRow("ACT_ID_INFO", map[string]any{"ID_": infoID, "REV_": 1, "USER_ID_": id, "TYPE_": "userinfo", "KEY_": key, "VALUE_": value, "PASSWORD_": nil, "PARENT_ID_": nil})
			if err != nil {
				return nil, err
			}
			rows = append(rows, row)
		}
	}
	for _, id := range identitySortedKeys(e.state.Groups) {
		group := e.state.Groups[id]
		row, err := identityRow("ACT_ID_GROUP", map[string]any{"ID_": id, "REV_": 1, "NAME_": identitySQLText(group.Name), "TYPE_": identitySQLText(group.Type)})
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	for _, key := range identitySortedKeys(e.state.Memberships) {
		if !e.state.Memberships[key] {
			continue
		}
		user, group, valid := membershipPair(key)
		if !valid {
			return nil, api.NewError("ENGINE-IDM-KEY", "成员关系键非法", api.ErrBadRequest)
		}
		row, err := identityRow("ACT_ID_MEMBERSHIP", map[string]any{"USER_ID_": user, "GROUP_ID_": group})
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	control, err := e.identityControlRow()
	if err != nil {
		return nil, err
	}
	if control != nil {
		rows = append(rows, *control)
	}
	return rows, nil
}

// identityRow 构造标准行并校验所有列，插入顺序由固定表元数据提供。
// @param table 表名
// @param values 标准列值
// @return 标准行
// @return 表或列定义错误
func identityRow(table string, values map[string]any) (flowablerow.Row, error) {
	row, err := flowablerow.NewRow(table)
	if err != nil {
		return flowablerow.Row{}, err
	}
	for column, value := range values {
		if err := row.Set(column, value); err != nil {
			return flowablerow.Row{}, err
		}
	}
	return *row, nil
}

// identitySQLText 将未设置的可空资料表示为 SQL NULL。
// @param value 可空资料文本
// @return 文本或空值
func identitySQLText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// identitySortedKeys 稳定排列身份实体和属性，避免导出依赖映射遍历顺序。
// @param values 以字符串为键的身份集合
// @return 升序键列表
func identitySortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// ImportIdentityRows 在命令事务中导入标准身份行，遇到不支持的数据时拒绝发布部分状态。
// @param ctx 请求上下文
// @param rows 标准身份行
// @return 列解码、引用或控制版本错误
func (e *Engine) ImportIdentityRows(ctx context.Context, rows []flowablerow.Row) error {
	snapshot, err := flowablerow.DecodeIdentityRows(rows)
	if err != nil {
		return err
	}
	_, err = e.execute(ctx, func(cc *commandContext) (any, error) { return nil, e.importIdentitySnapshot(snapshot) })
	return err
}

// importIdentitySnapshot 先校验全组引用及未支持资料，再发布用户、组、头像和成员状态。
// @param snapshot 解码后的身份行集合
// @return 引用、未支持资料或控制错误
func (e *Engine) importIdentitySnapshot(snapshot flowablerow.IdentitySnapshot) error {
	users := map[string]flowablerow.IdentityUser{}
	groups := map[string]bool{}
	usedBytes := map[string]bool{}
	seenInfo := map[string]map[string]bool{}
	for _, user := range snapshot.Users {
		users[user.User.ID] = user
	}
	for _, group := range snapshot.Groups {
		groups[group.ID] = true
	}
	for _, user := range snapshot.Users {
		if user.PictureID != "" {
			if _, exists := snapshot.Bytes[user.PictureID]; !exists {
				return api.NewError("ENGINE-IDM-PICTURE", "用户头像引用不存在", api.ErrBadRequest)
			}
			usedBytes[user.PictureID] = true
		}
	}
	// 身份账户明细尚未实现，必须拒绝启动，不能忽略后在下一次保存时删除原库资料。
	for _, info := range snapshot.Infos {
		if _, exists := users[info.UserID]; !exists {
			return api.NewError("ENGINE-IDM-INFO", "用户信息缺少所属用户", api.ErrBadRequest)
		}
		if info.Type != "userinfo" || info.ParentID != "" || info.PasswordPresent {
			return api.NewError("ENGINE-IDM-INFO-TYPE", "身份账户资料类型尚不支持", api.ErrBadRequest)
		}
		if seenInfo[info.UserID] == nil {
			seenInfo[info.UserID] = map[string]bool{}
		}
		if seenInfo[info.UserID][info.Key] {
			return api.NewError("ENGINE-IDM-INFO-DUPLICATE", "同一用户的信息键重复", api.ErrBadRequest)
		}
		seenInfo[info.UserID][info.Key] = true
	}
	for _, member := range snapshot.Memberships {
		if _, exists := users[member.UserID]; !exists || !groups[member.GroupID] {
			return api.NewError("ENGINE-IDM-MEMBER", "成员关系的用户或组不存在", api.ErrBadRequest)
		}
	}
	var control *identityControl
	for _, record := range snapshot.Bytes {
		if record.Name == identityControlName {
			if control != nil || usedBytes[record.ID] {
				return api.NewError("ENGINE-IDM-CONTROL", "身份控制重复或被头像引用", api.ErrBadRequest)
			}
			parsed, err := decodeIdentityControl(record.Data)
			if err != nil {
				return err
			}
			control = &parsed
			usedBytes[record.ID] = true
		}
	}
	for id := range snapshot.Bytes {
		if !usedBytes[id] {
			return api.NewError("ENGINE-IDM-BYTES", "身份字节行未被引用，禁止丢弃原数据", api.ErrBadRequest)
		}
	}
	for _, user := range snapshot.Users {
		copied := user.User
		e.state.Users[copied.ID] = &copied
		e.state.UserInfo[copied.ID] = map[string]*string{}
		e.state.UserInfoIDs[copied.ID] = map[string]string{}
		delete(e.state.UserPictures, copied.ID)
		if user.PictureID != "" {
			record := snapshot.Bytes[user.PictureID]
			e.state.UserPictures[copied.ID] = &userPictureState{ID: record.ID, Data: append([]byte(nil), record.Data...), MimeType: record.Name}
		}
	}
	for _, group := range snapshot.Groups {
		copied := group
		e.state.Groups[group.ID] = &copied
	}
	for _, info := range snapshot.Infos {
		e.state.UserInfo[info.UserID][info.Key] = info.Value
		e.state.UserInfoIDs[info.UserID][info.Key] = info.ID
	}
	for _, member := range snapshot.Memberships {
		e.state.Memberships[membershipKey(member.UserID, member.GroupID)] = true
	}
	if control != nil {
		return e.restoreIdentityControl(*control, users, groups)
	}
	return nil
}
