package migration_test

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/engine"
	"github.com/DengQingNian/Flowtide/internal/migration"
	flowablerow "github.com/DengQingNian/Flowtide/internal/persistence/flowable"
	"github.com/DengQingNian/Flowtide/internal/persistence/schema"
	statestore "github.com/DengQingNian/Flowtide/internal/persistence/state"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// TestIntegrationJavaDatabaseCutover 在双库临时库演练 Java 黄金行的预检、核对、行模式续跑及备份还原。
// @param t 测试上下文
// @return 无
func TestIntegrationJavaDatabaseCutover(t *testing.T) {
	for _, dialect := range []string{"mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			ctx, db := preflightDatabase(t, dialect)
			fixture := loadJavaReferenceFixture(t)
			for _, scenario := range fixture.Scenarios {
				t.Run(scenario.Name, func(t *testing.T) {
					before, err := migration.Scan(ctx, db, dialect)
					if err != nil {
						t.Fatal(err)
					}
					replaceJavaDatabaseRows(t, ctx, db, dialect, before.Rows, scenario.Before.Rows)
					// 备份来自真实数据库的一致性快照，不把 H2 JSON 原文当作数据库备份成功。
					backup, err := migration.Scan(ctx, db, dialect)
					if err != nil {
						t.Fatal(err)
					}
					data, report, err := migration.Convert(ctx, backup, migration.Options{})
					if err != nil || report.Blocked {
						t.Fatalf("真实 Java 行转换: %+v %v", report.Issues, err)
					}
					reconciled, err := migration.ReconcileState(ctx, backup, data, migration.Options{})
					if err != nil || !reconciled.Matched {
						t.Fatalf("真实库状态核对: %+v %v", reconciled, err)
					}
					quiet, err := migration.CheckQuiescence(ctx, db, dialect, time.Millisecond)
					if err != nil || !quiet.Stable {
						t.Fatalf("临时源库静默: %+v %v", quiet, err)
					}
					store := statestore.NewMemory()
					if err := store.Save(ctx, data); err != nil {
						t.Fatal(err)
					}
					prepared, err := engine.New(engine.Config{StateStore: store, HistoryLevel: engine.HistoryAudit})
					if err != nil {
						t.Fatal(err)
					}
					projected, err := prepared.SnapshotRows(ctx)
					if err != nil {
						t.Fatal(err)
					}
					if err := prepared.Close(ctx); err != nil {
						t.Fatal(err)
					}
					replaceJavaDatabaseRows(t, ctx, db, dialect, backup.Rows, projected)
					repository, err := flowablerow.NewRepository(flowablerow.SQLDB{DB: db}, dialect)
					if err != nil {
						t.Fatal(err)
					}
					workflow, err := engine.New(engine.Config{RowRepository: repository, LoadRows: true, HistoryLevel: engine.HistoryAudit})
					if err != nil {
						t.Fatal(err)
					}
					tenantCtx := tenant.With(ctx, scenario.BeforeExpected.TenantID)
					assertJavaReferenceExpected(t, tenantCtx, workflow, scenario.Before, scenario.BeforeExpected)
					assertJavaReferenceTypedValues(t, tenantCtx, workflow, scenario.BeforeExpected.ProcessInstanceID)
					tasks, err := workflow.ListTasks(tenantCtx, api.TaskFilter{ProcessInstanceID: scenario.BeforeExpected.ProcessInstanceID})
					if err != nil || len(tasks.Data) != 1 {
						t.Fatalf("接管任务: %+v %v", tasks, err)
					}
					request := api.CompleteTaskRequest{Variables: []api.Variable{{Name: "status", Value: "completed"}}}
					if scenario.Name == "approve" {
						request.Variables = []api.Variable{{Name: "approved", Value: true}, {Name: "status", Value: "approved"}}
					}
					if err := workflow.Complete(tenantCtx, tasks.Data[0].ID, request); err != nil {
						t.Fatal(err)
					}
					assertJavaReferenceExpected(t, tenantCtx, workflow, scenario.Before, scenario.Expected)
					if err := workflow.Close(ctx); err != nil {
						t.Fatal(err)
					}
					// 接管后的数据库再次独立恢复，确认续跑结果确实提交到行表。
					reopened, err := engine.New(engine.Config{RowRepository: repository, LoadRows: true, HistoryLevel: engine.HistoryAudit})
					if err != nil {
						t.Fatal(err)
					}
					assertJavaReferenceExpected(t, tenantCtx, reopened, scenario.Before, scenario.Expected)
					if err := reopened.Close(ctx); err != nil {
						t.Fatal(err)
					}
					current, err := migration.Scan(ctx, db, dialect)
					if err != nil {
						t.Fatal(err)
					}
					replaceJavaDatabaseRows(t, ctx, db, dialect, current.Rows, backup.Rows)
					restored, err := migration.Scan(ctx, db, dialect)
					if err != nil || snapshotFingerprint(t, restored) != snapshotFingerprint(t, backup) {
						t.Fatalf("备份还原后源库不同: %v", err)
					}
					t.Log("真实 Java 黄金行预检、静默、逐表核对、Go 行表续跑、重启与源快照还原通过")
				})
			}
		})
	}
}

// replaceJavaDatabaseRows 仅在临时验收库内按外键顺序原子替换测试数据，所有表列来自固定 DDL 白名单。
// @param t 测试上下文
// @param ctx 数据库上下文
// @param db 临时验收库
// @param dialect 数据库方言
// @param before 删除前的全部非空支持表行
// @param after 待写入的 JDBC 或 Go 行
// @return 无
func replaceJavaDatabaseRows(t *testing.T, ctx context.Context, db *sql.DB, dialect string, before, after []flowablerow.Row) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	ops := append(flowablerow.DiffOperations(before, nil), flowablerow.DiffOperations(nil, after)...)
	for _, op := range ops {
		columns, types, err := schema.TableColumns(dialect, op.Row.Table)
		if err != nil {
			t.Fatal(err)
		}
		allowed := make(map[string]string, len(columns))
		for i, name := range columns {
			allowed[name] = types[i]
		}
		var query string
		var args []any
		if op.Kind == flowablerow.OperationDelete {
			key := "ID_"
			if op.Row.Table == "ACT_GE_PROPERTY" || op.Row.Table == "ACT_ID_PROPERTY" {
				key = "NAME_"
			}
			query = "DELETE FROM " + op.Row.Table + " WHERE " + key + " = ?"
			args = []any{op.Row.Values[key]}
		} else {
			selected := make([]string, 0, len(op.Row.Values))
			for name := range op.Row.Values {
				if _, ok := allowed[name]; !ok {
					t.Fatalf("测试行存在非规范列: %s.%s", op.Row.Table, name)
				}
				selected = append(selected, name)
			}
			sort.Strings(selected)
			for _, name := range selected {
				value := op.Row.Values[name]
				// JDBC JSON 的时间文本按真实目标列类型恢复，业务文本列不做时间猜测。
				if text, ok := value.(string); ok && (strings.Contains(strings.ToLower(allowed[name]), "timestamp") || strings.Contains(strings.ToLower(allowed[name]), "datetime")) {
					parsed, err := time.Parse(time.RFC3339Nano, text)
					if err != nil {
						t.Fatal(err)
					}
					value = parsed
				}
				args = append(args, value)
			}
			query = "INSERT INTO " + op.Row.Table + " (" + strings.Join(selected, ",") + ") VALUES (" + strings.TrimSuffix(strings.Repeat("?,", len(selected)), ",") + ")"
		}
		if dialect == "postgres" {
			for index := 1; strings.Contains(query, "?"); index++ {
				query = strings.Replace(query, "?", fmt.Sprintf("$%d", index), 1)
			}
		}
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("临时库替换 %s: %v", op.Row.Table, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}
