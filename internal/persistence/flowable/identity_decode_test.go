package flowable

import (
	"bytes"
	"errors"
	"testing"

	"github.com/flowable-go/flowable/api"
)

// identityDecodeFixture 提供标准列中的空值、精确引用及包含非UTF字节的身份样本。
// @param t 测试上下文
// @return 独立的标准行集合
func identityDecodeFixture(t *testing.T) []Row {
	t.Helper()
	specs := []struct {
		table  string
		values map[string]any
	}{
		{"ACT_ID_USER", map[string]any{"ID_": "alice", "REV_": 1, "FIRST_": "用户甲", "LAST_": "测试", "DISPLAY_NAME_": "显示名", "EMAIL_": "alice@example.test", "PWD_": "编码文本", "PICTURE_ID_": "picture", "TENANT_ID_": "tenant-a"}},
		{"ACT_ID_GROUP", map[string]any{"ID_": "review", "REV_": 1, "NAME_": "审批组", "TYPE_": "assignment"}},
		{"ACT_ID_MEMBERSHIP", map[string]any{"USER_ID_": "alice", "GROUP_ID_": "review"}},
		{"ACT_ID_INFO", map[string]any{"ID_": "info", "REV_": 1, "USER_ID_": "alice", "TYPE_": "userinfo", "KEY_": "team", "VALUE_": nil}},
		{"ACT_ID_BYTEARRAY", map[string]any{"ID_": "picture", "REV_": 1, "NAME_": "image/png", "BYTES_": []byte{1, 2, 0, 255}}},
	}
	out := make([]Row, 0, len(specs))
	for _, spec := range specs {
		row, err := NewRow(spec.table)
		if err != nil {
			t.Fatal(err)
		}
		for _, column := range row.Columns() {
			row.Values[column] = nil
		}
		for column, value := range spec.values {
			if err := row.Set(column, value); err != nil {
				t.Fatal(err)
			}
		}
		out = append(out, *row)
	}
	return out
}

// TestDecodeIdentityRows 验证标准资料、引用、空值键及二进制无损解码。
// @param t 测试上下文
// @return 无
func TestDecodeIdentityRows(t *testing.T) {
	rows := identityDecodeFixture(t)
	snapshot, err := DecodeIdentityRows(rows)
	if err != nil || len(snapshot.Users) != 1 || snapshot.Users[0].User.FirstName != "用户甲" || snapshot.Users[0].User.PasswordHash != "编码文本" || snapshot.Users[0].User.TenantID != "tenant-a" || snapshot.Users[0].PictureID != "picture" {
		t.Fatalf("用户解码错误: %+v %v", snapshot.Users, err)
	}
	if len(snapshot.Groups) != 1 || snapshot.Groups[0].ID != "review" || len(snapshot.Memberships) != 1 || snapshot.Memberships[0].UserID != "alice" || snapshot.Memberships[0].GroupID != "review" {
		t.Fatalf("组或成员解码错误: %+v", snapshot)
	}
	if len(snapshot.Infos) != 1 || snapshot.Infos[0].Value != nil || snapshot.Infos[0].ID != "info" {
		t.Fatalf("空值信息丢失: %+v", snapshot.Infos)
	}
	if !bytes.Equal(snapshot.Bytes["picture"].Data, []byte{1, 2, 0, 255}) || snapshot.Bytes["picture"].Name != "image/png" {
		t.Fatal("头像解码损坏")
	}
	// 空字符串不同于 SQL NULL，返回的字节数据也不能与来源行共享可写数组。
	rows[3].Values["VALUE_"] = ""
	snapshot, err = DecodeIdentityRows(rows)
	if err != nil || snapshot.Infos[0].Value == nil || *snapshot.Infos[0].Value != "" {
		t.Fatal("空字符串被误转为空值")
	}
	snapshot.Bytes["picture"].Data[0] = 9
	if rows[4].Values["BYTES_"].([]byte)[0] != 1 {
		t.Fatal("解码暴露原始字节数组")
	}
	rows = append(rows, Row{Table: "ACT_GE_PROPERTY", Values: map[string]any{"NAME_": "schema.version", "VALUE_": "8.1.0.3"}})
	if _, err := DecodeIdentityRows(rows); err != nil {
		t.Fatalf("非身份行不应参与解码: %v", err)
	}
}

// TestDecodeIdentityRowsMalformed 验证损坏的来源列和重复标识被拒绝，不能静默变成空资料。
// @param t 测试上下文
// @return 无
func TestDecodeIdentityRowsMalformed(t *testing.T) {
	cases := []struct {
		name   string
		index  int
		column string
		value  any
	}{
		{"修订号空值", 0, "REV_", nil}, {"修订号非法", 0, "REV_", "bad"}, {"修订号为零", 0, "REV_", 0}, {"用户主键", 0, "ID_", struct{}{}}, {"用户名", 0, "FIRST_", true}, {"用户引用", 0, "PICTURE_ID_", true},
		{"组数值标识", 1, "ID_", int64(1)}, {"组空白标识", 1, "ID_", " "}, {"组名称", 1, "NAME_", true}, {"组类型", 1, "TYPE_", true},
		{"成员空用户", 2, "USER_ID_", nil}, {"成员空组", 2, "GROUP_ID_", nil}, {"成员白用户", 2, "USER_ID_", " "}, {"成员白组", 2, "GROUP_ID_", " "},
		{"信息归属", 3, "USER_ID_", true}, {"信息类型", 3, "TYPE_", true}, {"信息键", 3, "KEY_", true}, {"信息父级", 3, "PARENT_ID_", true}, {"信息值", 3, "VALUE_", true}, {"信息密码", 3, "PASSWORD_", "无效字节"},
		{"头像数值标识", 4, "ID_", int64(1)}, {"头像空白标识", 4, "ID_", " "}, {"头像名称", 4, "NAME_", true}, {"头像数据", 4, "BYTES_", "无效字节"},
	}
	for _, scenario := range cases {
		t.Run(scenario.name, func(t *testing.T) {
			rows := identityDecodeFixture(t)
			rows[scenario.index].Values[scenario.column] = scenario.value
			if _, err := DecodeIdentityRows(rows); !errors.Is(err, api.ErrBadRequest) {
				t.Fatalf("非法身份行未拒绝: %v", err)
			}
		})
	}
	for _, missing := range []struct {
		index  int
		column string
	}{{0, "ID_"}, {3, "USER_ID_"}, {3, "PASSWORD_"}, {4, "BYTES_"}} {
		rows := identityDecodeFixture(t)
		delete(rows[missing.index].Values, missing.column)
		if _, err := DecodeIdentityRows(rows); !errors.Is(err, api.ErrBadRequest) {
			t.Fatalf("缺失列未拒绝: %s %v", missing.column, err)
		}
	}
	rows := identityDecodeFixture(t)
	rows = append(rows, rows[0])
	if _, err := DecodeIdentityRows(rows); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("重复用户标识未拒绝: %v", err)
	}
	if _, err := DecodeIdentityRows([]Row{{Table: "ACT_ID_UNKNOWN", Values: map[string]any{"ID_": "unknown"}}}); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("未知身份表未拒绝: %v", err)
	}
}
