package engine

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
	flowablerow "github.com/DengQingNian/Flowtide/internal/persistence/flowable"
)

// identityValidationRows 构建有标准资料及内部角色控制的独立身份快照。
// @param t 测试上下文
// @return 标准行集合
func identityValidationRows(t *testing.T) []flowablerow.Row {
	t.Helper()
	ctx := context.Background()
	app := newTestEngine(t)
	if _, err := app.CreateUser(ctx, api.User{ID: "alice", FirstName: "用户甲"}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.CreateGroup(ctx, api.Group{ID: "review", Name: "审批组", TenantID: "tenant-a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.CreateRole(ctx, api.Role{ID: "reader", Permissions: []string{"read"}}); err != nil {
		t.Fatal(err)
	}
	if err := app.SetUserRoles(ctx, "alice", []string{"reader"}); err != nil {
		t.Fatal(err)
	}
	if err := app.AddMembership(ctx, "alice", "review"); err != nil {
		t.Fatal(err)
	}
	if err := app.SetUserInfo(ctx, "alice", "team", "原团队"); err != nil {
		t.Fatal(err)
	}
	if err := app.SetUserPicture(ctx, "alice", []byte{1, 0, 255}, "image/png"); err != nil {
		t.Fatal(err)
	}
	rows, err := app.SnapshotRows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// identityValidationControl 修改样本控制对象，同时保持原标准资料不变。
// @param t 测试上下文
// @param rows 待修改行集合
// @param change 控制变更
// @return 无
func identityValidationControl(t *testing.T, rows []flowablerow.Row, change func(map[string]any)) {
	t.Helper()
	for _, row := range rows {
		if row.Table != "ACT_ID_BYTEARRAY" || row.Values["NAME_"] != identityControlName {
			continue
		}
		var control map[string]any
		if err := json.Unmarshal(row.Values["BYTES_"].([]byte), &control); err != nil {
			t.Fatal(err)
		}
		change(control)
		data, err := json.Marshal(control)
		if err != nil {
			t.Fatal(err)
		}
		row.Values["BYTES_"] = data
		return
	}
	t.Fatal("参考行缺少身份控制")
}

// TestIdentityRowsRejectPartial 验证不支持的身份资料、悬空引用和损坏控制不能发布部分状态或误删原资料。
// @param t 测试上下文
// @return 无
func TestIdentityRowsRejectPartial(t *testing.T) {
	original := identityValidationRows(t)
	cases := []struct {
		name   string
		change func([]flowablerow.Row) []flowablerow.Row
	}{
		{"空头像引用", func(rows []flowablerow.Row) []flowablerow.Row {
			for _, row := range rows {
				if row.Table == "ACT_ID_USER" {
					row.Values["PICTURE_ID_"] = "missing"
				}
			}
			return rows
		}},
		{"成员无用户", func(rows []flowablerow.Row) []flowablerow.Row {
			for _, row := range rows {
				if row.Table == "ACT_ID_MEMBERSHIP" {
					row.Values["USER_ID_"] = "missing"
				}
			}
			return rows
		}},
		{"信息无用户", func(rows []flowablerow.Row) []flowablerow.Row {
			for _, row := range rows {
				if row.Table == "ACT_ID_INFO" {
					row.Values["USER_ID_"] = "missing"
				}
			}
			return rows
		}},
		{"未支持账户", func(rows []flowablerow.Row) []flowablerow.Row {
			for _, row := range rows {
				if row.Table == "ACT_ID_INFO" {
					row.Values["TYPE_"] = "account"
				}
			}
			return rows
		}},
		{"非空密码列", func(rows []flowablerow.Row) []flowablerow.Row {
			for _, row := range rows {
				if row.Table == "ACT_ID_INFO" {
					row.Values["PASSWORD_"] = []byte{}
				}
			}
			return rows
		}},
		{"账户明细父级", func(rows []flowablerow.Row) []flowablerow.Row {
			for _, row := range rows {
				if row.Table == "ACT_ID_INFO" {
					row.Values["PARENT_ID_"] = "parent"
				}
			}
			return rows
		}},
		{"重复信息键", func(rows []flowablerow.Row) []flowablerow.Row {
			for _, row := range rows {
				if row.Table == "ACT_ID_INFO" {
					copied := copyValue(row).(flowablerow.Row)
					copied.Values["ID_"] = "duplicate"
					return append(rows, copied)
				}
			}
			return rows
		}},
		{"未知字节行", func(rows []flowablerow.Row) []flowablerow.Row {
			row, _ := identityRow("ACT_ID_BYTEARRAY", map[string]any{"ID_": "unreferenced", "REV_": 1, "NAME_": "unknown", "BYTES_": []byte{1}})
			return append(rows, row)
		}},
		{"未知控制版本", func(rows []flowablerow.Row) []flowablerow.Row {
			identityValidationControl(t, rows, func(control map[string]any) { control["version"] = 2 })
			return rows
		}},
		{"未知控制字段", func(rows []flowablerow.Row) []flowablerow.Row {
			identityValidationControl(t, rows, func(control map[string]any) { control["firstName"] = "伪造名称" })
			return rows
		}},
		{"安全属性无用户", func(rows []flowablerow.Row) []flowablerow.Row {
			identityValidationControl(t, rows, func(control map[string]any) {
				control["users"].(map[string]any)["missing"] = map[string]any{"roles": []string{"reader"}}
			})
			return rows
		}},
		{"负失败次数", func(rows []flowablerow.Row) []flowablerow.Row {
			identityValidationControl(t, rows, func(control map[string]any) {
				control["users"].(map[string]any)["alice"].(map[string]any)["failedAttempts"] = -1
			})
			return rows
		}},
		{"租户属性无组", func(rows []flowablerow.Row) []flowablerow.Row {
			identityValidationControl(t, rows, func(control map[string]any) { control["groupTenants"].(map[string]any)["missing"] = "tenant-b" })
			return rows
		}},
		{"角色标识冲突", func(rows []flowablerow.Row) []flowablerow.Row {
			identityValidationControl(t, rows, func(control map[string]any) {
				control["roles"].(map[string]any)["reader"].(map[string]any)["id"] = "other"
			})
			return rows
		}},
	}
	for _, scenario := range cases {
		t.Run(scenario.name, func(t *testing.T) {
			rows := scenario.change(copyValue(original).([]flowablerow.Row))
			target := newTestEngine(t)
			if _, err := target.CreateUser(context.Background(), api.User{ID: "survivor"}); err != nil {
				t.Fatal(err)
			}
			before := cloneState(target.state)
			if err := target.RestoreRows(context.Background(), rows); !errors.Is(err, api.ErrBadRequest) {
				t.Fatalf("非法身份状态未拒绝: %v", err)
			}
			if !reflect.DeepEqual(before, target.state) {
				t.Fatal("恢复失败仍发布部分身份状态")
			}
		})
	}
}
