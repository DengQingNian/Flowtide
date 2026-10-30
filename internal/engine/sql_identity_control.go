package engine

import (
	"bytes"
	"encoding/json"
	"io"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	flowablerow "github.com/DengQingNian/Flowtide/internal/persistence/flowable"
)

// identityControlName 是仅保存 Go 身份扩展属性的保留字节行名称。
const identityControlName = "flowtide-identity-control/v1"

// identityUserControl 保存标准用户表没有的 Go 安全属性，不保存用户公开资料或密码副本。
type identityUserControl struct {
	Roles          []string   `json:"roles,omitempty"`          // 角色
	Disabled       bool       `json:"disabled,omitempty"`       // 是否禁用
	FailedAttempts int        `json:"failedAttempts,omitempty"` // 连续失败次数
	LockedUntil    *time.Time `json:"lockedUntil,omitempty"`    // 锁定截止时间
}

// identityControl 保存 Go 角色及安全扩展，标准资料仍由身份行唯一提供。
type identityControl struct {
	Version      int                            `json:"version"`                // 格式版本
	Users        map[string]identityUserControl `json:"users,omitempty"`        // 用户安全属性
	GroupTenants map[string]string              `json:"groupTenants,omitempty"` // 标准组表没有的租户属性
	Roles        map[string]api.Role            `json:"roles,omitempty"`        // Go 角色权限扩展
}

// identityControlRow 在存在 Go 扩展属性时生成独立字节行，避免复制标准列的权威值。
// @return 可空的内部控制行
// @return 编码或行定义错误
func (e *Engine) identityControlRow() (*flowablerow.Row, error) {
	control := identityControl{Version: 1, Users: map[string]identityUserControl{}, GroupTenants: map[string]string{}, Roles: map[string]api.Role{}}
	for id, user := range e.state.Users {
		if len(user.Roles) > 0 || user.Disabled || user.FailedAttempts != 0 || user.LockedUntil != nil {
			control.Users[id] = identityUserControl{Roles: append([]string(nil), user.Roles...), Disabled: user.Disabled, FailedAttempts: user.FailedAttempts, LockedUntil: cloneTime(user.LockedUntil)}
		}
	}
	for id, group := range e.state.Groups {
		if group.TenantID != "" {
			control.GroupTenants[id] = group.TenantID
		}
	}
	for id, role := range e.state.Roles {
		copied := *role
		copied.Permissions = append([]string(nil), role.Permissions...)
		control.Roles[id] = copied
	}
	// 普通原版身份资料不需要控制行，只有标准表无法表达的属性才进入此扩展。
	if len(control.Users) == 0 && len(control.GroupTenants) == 0 && len(control.Roles) == 0 {
		return nil, nil
	}
	data, err := json.Marshal(control)
	if err != nil {
		return nil, err
	}
	row, err := identityRow("ACT_ID_BYTEARRAY", map[string]any{"ID_": byteArrayID("", identityControlName), "REV_": 1, "NAME_": identityControlName, "BYTES_": data})
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// decodeIdentityControl 严格读取控制版本及字段，拒绝尾随内容和未来扩展被静默丢弃。
// @param data 控制字节
// @return 完整控制对象
// @return 版本、未知字段或格式错误
func decodeIdentityControl(data []byte) (identityControl, error) {
	control := identityControl{}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&control); err != nil {
		return identityControl{}, api.NewError("ENGINE-IDM-CONTROL", "身份控制格式无效", api.ErrBadRequest)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF || control.Version != 1 {
		return identityControl{}, api.NewError("ENGINE-IDM-CONTROL", "身份控制版本或尾随内容无效", api.ErrBadRequest)
	}
	return control, nil
}

// restoreIdentityControl 校验所有扩展归属后恢复安全属性，不能覆盖标准用户资料和密码。
// @param control 版本化扩展
// @param users 本次标准用户集合
// @param groups 本次标准组集合
// @return 归属、角色标识或安全属性非法
func (e *Engine) restoreIdentityControl(control identityControl, users map[string]flowablerow.IdentityUser, groups map[string]bool) error {
	// 先拒绝悬空引用和非法安全计数，标准用户必须独立存在。
	for id, extra := range control.Users {
		if _, exists := users[id]; !exists || extra.FailedAttempts < 0 {
			return api.NewError("ENGINE-IDM-CONTROL", "身份安全属性缺少用户或计数非法", api.ErrBadRequest)
		}
	}
	for id := range control.GroupTenants {
		if !groups[id] {
			return api.NewError("ENGINE-IDM-CONTROL", "身份租户属性缺少组", api.ErrBadRequest)
		}
	}
	for id, role := range control.Roles {
		if id == "" || role.ID != id {
			return api.NewError("ENGINE-IDM-CONTROL", "身份角色标识不一致", api.ErrBadRequest)
		}
	}
	for id, extra := range control.Users {
		user := e.state.Users[id]
		user.Roles = append([]string(nil), extra.Roles...)
		user.Disabled = extra.Disabled
		user.FailedAttempts = extra.FailedAttempts
		user.LockedUntil = cloneTime(extra.LockedUntil)
	}
	for id, scope := range control.GroupTenants {
		e.state.Groups[id].TenantID = scope
	}
	e.state.Roles = map[string]*api.Role{}
	for id, role := range control.Roles {
		copied := role
		copied.Permissions = append([]string(nil), role.Permissions...)
		e.state.Roles[id] = &copied
	}
	return nil
}
