package flowable

import (
	"strings"

	"github.com/flowable-go/flowable/api"
)

// IdentityUser 保存标准用户资料与头像字节行引用。
type IdentityUser struct {
	User      api.User // 用户资料及密码编码
	PictureID string   // 头像字节行标识
}

// IdentityMembership 表示标准成员表的真实两列主键。
type IdentityMembership struct {
	UserID  string // 用户标识
	GroupID string // 组标识
}

// IdentityInfo 保存用户信息及用于拒绝未支持账户类型的原始属性。
type IdentityInfo struct {
	ID              string  // 标准信息行标识
	UserID          string  // 用户标识
	Type            string  // 信息类型
	Key             string  // 信息键
	Value           *string // 可空信息值
	Password        []byte  // 账户密码数据
	PasswordPresent bool    // 密码列是否非空，包括零长度字节
	ParentID        string  // 父信息标识
}

// IdentityBytes 保存头像或 Go 身份控制的独立字节行。
type IdentityBytes struct {
	ID   string // 字节行标识
	Name string // 媒体类型或控制名称
	Data []byte // 原始字节
}

// IdentitySnapshot 汇总可独立校验引用的标准身份行。
type IdentitySnapshot struct {
	Users       []IdentityUser           // 用户
	Groups      []api.Group              // 组
	Memberships []IdentityMembership     // 成员关系
	Infos       []IdentityInfo           // 信息
	Bytes       map[string]IdentityBytes // 字节行
}

// DecodeIdentityRows 解码标准身份列并拒绝重复主键，引用完整性由引擎统一检查。
// @param rows 标准行集合
// @return 身份快照
// @return 列类型或重复标识错误
func DecodeIdentityRows(rows []Row) (IdentitySnapshot, error) {
	snapshot := IdentitySnapshot{Bytes: map[string]IdentityBytes{}}
	seen := map[string]bool{}
	for _, row := range rows {
		if !strings.HasPrefix(row.Table, "ACT_ID_") {
			continue
		}
		id, valid := rowIDValue(row)
		if !valid || id == "" {
			return IdentitySnapshot{}, api.NewError("ROW-IDM-KEY", "身份行缺少主键", api.ErrBadRequest)
		}
		if row.Table != "ACT_ID_MEMBERSHIP" {
			var err error
			id, err = identityRequiredText(row, "ID_")
			if err != nil {
				return IdentitySnapshot{}, err
			}
			if revision, exists := revisionValue(row); !exists || revision < 1 {
				return IdentitySnapshot{}, api.NewError("ROW-IDM-REVISION", "身份行修订号非法", api.ErrBadRequest)
			}
		}
		key := row.Table + "\x00" + id
		if seen[key] {
			return IdentitySnapshot{}, api.NewError("ROW-IDM-DUPLICATE", "身份行主键重复", api.ErrBadRequest)
		}
		seen[key] = true
		// 各表只转换自身标准字段，复合成员键不能被误当成 ID_。
		switch row.Table {
		case "ACT_ID_USER":
			user, err := decodeIdentityUser(row)
			if err != nil {
				return IdentitySnapshot{}, err
			}
			snapshot.Users = append(snapshot.Users, user)
		case "ACT_ID_GROUP":
			name, err := optionalStringValue(row, "NAME_")
			if err != nil {
				return IdentitySnapshot{}, err
			}
			kind, err := optionalStringValue(row, "TYPE_")
			if err != nil {
				return IdentitySnapshot{}, err
			}
			snapshot.Groups = append(snapshot.Groups, api.Group{ID: id, Name: name, Type: kind})
		case "ACT_ID_MEMBERSHIP":
			user, err := identityRequiredText(row, "USER_ID_")
			if err != nil {
				return IdentitySnapshot{}, err
			}
			group, err := identityRequiredText(row, "GROUP_ID_")
			if err != nil {
				return IdentitySnapshot{}, err
			}
			snapshot.Memberships = append(snapshot.Memberships, IdentityMembership{UserID: user, GroupID: group})
		case "ACT_ID_INFO":
			info, err := decodeIdentityInfo(row)
			if err != nil {
				return IdentitySnapshot{}, err
			}
			snapshot.Infos = append(snapshot.Infos, info)
		case "ACT_ID_BYTEARRAY":
			name, err := optionalStringValue(row, "NAME_")
			if err != nil {
				return IdentitySnapshot{}, err
			}
			data, err := bytesValue(row, "BYTES_")
			if err != nil {
				return IdentitySnapshot{}, err
			}
			snapshot.Bytes[id] = IdentityBytes{ID: id, Name: name, Data: append([]byte(nil), data...)}
		default:
			return IdentitySnapshot{}, api.NewError("ROW-IDM-TABLE", "身份表尚未支持", api.ErrBadRequest)
		}
	}
	return snapshot, nil
}

// decodeIdentityUser 解码用户标准行，密码字段保持原编码而不经过明文转换。
// @param row 用户行
// @return 用户及头像引用
// @return 标识或字段类型错误
func decodeIdentityUser(row Row) (IdentityUser, error) {
	id, err := identityRequiredText(row, "ID_")
	if err != nil {
		return IdentityUser{}, err
	}
	fields := map[string]string{}
	for _, column := range []string{"FIRST_", "LAST_", "DISPLAY_NAME_", "EMAIL_", "PWD_", "PICTURE_ID_", "TENANT_ID_"} {
		value, err := optionalStringValue(row, column)
		if err != nil {
			return IdentityUser{}, err
		}
		fields[column] = value
	}
	return IdentityUser{User: api.User{ID: id, FirstName: fields["FIRST_"], LastName: fields["LAST_"], DisplayName: fields["DISPLAY_NAME_"], Email: fields["EMAIL_"], PasswordHash: fields["PWD_"], TenantID: fields["TENANT_ID_"]}, PictureID: fields["PICTURE_ID_"]}, nil
}

// decodeIdentityInfo 解码可空值并保留原版行标识及账户类型元数据。
// @param row 用户信息行
// @return 信息记录
// @return 列类型错误
func decodeIdentityInfo(row Row) (IdentityInfo, error) {
	fields := map[string]string{}
	for _, column := range []string{"ID_", "USER_ID_", "TYPE_", "KEY_", "PARENT_ID_"} {
		value, err := stringValue(row, column)
		if err != nil {
			return IdentityInfo{}, err
		}
		fields[column] = value
	}
	var value *string
	if row.Values["VALUE_"] != nil {
		text, err := stringValue(row, "VALUE_")
		if err != nil {
			return IdentityInfo{}, err
		}
		value = &text
	}
	password, err := bytesValue(row, "PASSWORD_")
	if err != nil {
		return IdentityInfo{}, err
	}
	return IdentityInfo{ID: fields["ID_"], UserID: fields["USER_ID_"], Type: fields["TYPE_"], Key: fields["KEY_"], ParentID: fields["PARENT_ID_"], Value: value, Password: append([]byte(nil), password...), PasswordPresent: row.Values["PASSWORD_"] != nil}, nil
}

// identityRequiredText 读取不能缺省或为空白的身份引用列。
// @param row 标准行
// @param column 引用列
// @return 非空引用
// @return 列类型或空引用错误
func identityRequiredText(row Row, column string) (string, error) {
	value, err := stringValue(row, column)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(value) == "" {
		return "", api.NewError("ROW-IDM-REFERENCE", "身份标识或引用为空", api.ErrBadRequest)
	}
	return value, nil
}
