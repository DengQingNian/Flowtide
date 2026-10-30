package postgres_test

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/engine"
	flowablerow "github.com/DengQingNian/Flowtide/internal/persistence/flowable"
	pgstore "github.com/DengQingNian/Flowtide/internal/persistence/postgres"
)

// TestIntegrationPostgresRowsIdentityRecovery 验证身份标准行与 Go 扩展在真实库重启后完整恢复。
// @param t 测试上下文
// @return 无
func TestIntegrationPostgresRowsIdentityRecovery(t *testing.T) {
	ctx, db := postgresServiceDatabase(t)
	lock, err := pgstore.NewLockedRows(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := flowablerow.NewRepository(lock, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	first, err := engine.New(engine.Config{RowRepository: repository, LoadRows: true, HistoryLevel: engine.HistoryFull})
	if err != nil {
		t.Fatal(err)
	}
	seedIdentitySample(t, ctx, first)
	if err := first.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	// 新执行者重新取锁并从真实标准行恢复，不复用首个执行者的内存。
	restartedLock, err := pgstore.NewLockedRows(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	restartedRepository, err := flowablerow.NewRepository(restartedLock, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	second, err := engine.New(engine.Config{RowRepository: restartedRepository, LoadRows: true, HistoryLevel: engine.HistoryFull})
	if err != nil {
		t.Fatal(err)
	}
	verifyIdentityRecovery(t, ctx, second)
	var firstName, email, passwordHash string
	if err := db.QueryRowContext(ctx, "SELECT FIRST_, EMAIL_, PWD_ FROM ACT_ID_USER WHERE ID_ = $1", "alice").Scan(&firstName, &email, &passwordHash); err != nil || firstName != "用户甲" || email != "alice@example.test" {
		t.Fatalf("标准用户行不是资料真实来源: %s %s %v", firstName, email, err)
	}
	// 数据库必须保存编码后的密码，不能落明文，也不能为空导致重启后无法认证。
	if passwordHash == "" || passwordHash == "reference-password" {
		t.Fatal("PWD_ 列必须保存编码密码")
	}
	var members int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ACT_ID_MEMBERSHIP WHERE USER_ID_ = $1 AND GROUP_ID_ = $2", "alice", "review").Scan(&members); err != nil || members != 1 {
		t.Fatalf("标准成员复合主键行缺失: %d %v", members, err)
	}
	var infoRows int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ACT_ID_INFO WHERE USER_ID_ = $1 AND KEY_ = $2 AND VALUE_ IS NULL", "alice", "cleared").Scan(&infoRows); err != nil || infoRows != 1 {
		t.Fatalf("已清空信息键未保留显式空值行: %d %v", infoRows, err)
	}
	var controlRows int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ACT_ID_BYTEARRAY WHERE NAME_ = $1", "flowtide-identity-control/v1").Scan(&controlRows); err != nil || controlRows != 1 {
		t.Fatalf("Go 身份扩展控制行缺失: %d %v", controlRows, err)
	}
	if err := second.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := restartedLock.Release(); err != nil {
		t.Fatal(err)
	}
	t.Log("身份标准行、复合成员键、空值信息与 Go 扩展控制行真实库重启恢复通过")
}

// seedIdentitySample 创建覆盖标准资料、可空信息、头像与 Go 扩展的完整身份样本。
// @param t 测试上下文
// @param ctx 操作上下文
// @param app 目标引擎
func seedIdentitySample(t *testing.T, ctx context.Context, app *engine.Engine) {
	t.Helper()
	if _, err := app.CreateUser(ctx, api.User{ID: "alice", FirstName: "用户甲", LastName: "测试", Email: "alice@example.test", Password: "reference-password", TenantID: "tenant-a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.CreateGroup(ctx, api.Group{ID: "review", Name: "审批组", Type: "assignment", TenantID: "tenant-a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.CreateRole(ctx, api.Role{ID: "reader", Name: "只读", Permissions: []string{"read"}}); err != nil {
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
	if err := app.SetUserInfo(ctx, "alice", "cleared", "待清空"); err != nil {
		t.Fatal(err)
	}
	if err := app.DeleteUserInfo(ctx, "alice", "cleared"); err != nil {
		t.Fatal(err)
	}
	if err := app.SetUserPicture(ctx, "alice", []byte{1, 2, 0, 255}, "image/png"); err != nil {
		t.Fatal(err)
	}
}

// verifyIdentityRecovery 核对重启后的用户资料、密码、组、信息、头像与角色扩展。
// @param t 测试上下文
// @param ctx 操作上下文
// @param app 恢复后的引擎
func verifyIdentityRecovery(t *testing.T, ctx context.Context, app *engine.Engine) {
	t.Helper()
	user, err := app.GetUser(ctx, "alice")
	if err != nil || user.FirstName != "用户甲" || user.LastName != "测试" || user.Email != "alice@example.test" || user.TenantID != "tenant-a" {
		t.Fatalf("用户资料恢复失败: %+v %v", user, err)
	}
	if err := app.CheckPassword(ctx, "alice", "reference-password"); err != nil {
		t.Fatalf("重启后密码校验失败: %v", err)
	}
	groups, err := app.ListUserGroups(ctx, "alice")
	if err != nil || len(groups) != 1 || groups[0].ID != "review" || groups[0].Name != "审批组" || groups[0].TenantID != "tenant-a" {
		t.Fatalf("组及租户扩展恢复失败: %+v %v", groups, err)
	}
	entries, err := app.GetUserInfoEntries(ctx, "alice")
	if err != nil || entries["team"] == nil || *entries["team"] != "原团队" {
		t.Fatalf("用户信息恢复失败: %+v %v", entries, err)
	}
	// 已删除的键必须保留显式空值行，单键查询保持不存在语义。
	if value, exists := entries["cleared"]; !exists || value != nil {
		t.Fatalf("空值信息键丢失: %+v", entries)
	}
	if _, err := app.GetUserInfoValue(ctx, "alice", "cleared"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("空值单键语义错误: %v", err)
	}
	data, mimeType, err := app.GetUserPicture(ctx, "alice")
	if err != nil || !bytes.Equal(data, []byte{1, 2, 0, 255}) || mimeType != "image/png" {
		t.Fatalf("头像恢复失败: %v %s %v", data, mimeType, err)
	}
	if len(user.Roles) != 1 || user.Roles[0] != "reader" {
		t.Fatalf("用户角色扩展恢复失败: %+v", user.Roles)
	}
	roles, err := app.ListRoles(ctx)
	if err != nil || len(roles) != 1 || roles[0].ID != "reader" || !reflect.DeepEqual(roles[0].Permissions, []string{"read"}) {
		t.Fatalf("角色定义恢复失败: %+v %v", roles, err)
	}
}
