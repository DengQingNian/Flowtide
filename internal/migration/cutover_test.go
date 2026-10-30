package migration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/DengQingNian/Flowtide/internal/persistence/flowable"
)

// fingerprintValue 返回单列单值行的内容指纹，便于比较值编码差异。
// @param value 列值
// @return 内容指纹摘要
func fingerprintValue(value any) string {
	return tableFingerprint([]flowable.Row{{Table: "ACT_RU_JOB", Values: map[string]any{"VALUE_": value}}})
}

// TestTableFingerprintDeterministic 验证指纹不受行顺序、列顺序影响，且能发现内容变化与重复行。
// @param t 测试上下文
// @return 无
func TestTableFingerprintDeterministic(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)
	first := flowable.Row{Table: "ACT_RU_JOB", Values: map[string]any{"ID_": "job-a", "REV_": int64(1), "CREATE_TIME_": created}}
	second := flowable.Row{Table: "ACT_RU_JOB", Values: map[string]any{"CREATE_TIME_": created, "REV_": int64(1), "ID_": "job-b"}}
	baseline := tableFingerprint([]flowable.Row{first, second})
	if len(baseline) != 32 {
		t.Fatalf("指纹摘要长度错误: %d", len(baseline))
	}
	// 行顺序与列顺序都是数据库返回顺序，不得影响指纹。
	if reversed := tableFingerprint([]flowable.Row{second, first}); reversed != baseline {
		t.Fatal("行顺序改变了指纹")
	}
	changed := flowable.Row{Table: "ACT_RU_JOB", Values: map[string]any{"ID_": "job-c", "REV_": int64(1), "CREATE_TIME_": created}}
	if tableFingerprint([]flowable.Row{first, changed}) == baseline {
		t.Fatal("内容变化未改变指纹")
	}
	// 行数不变但重复行增加时，重复行必须各自计入指纹。
	if tableFingerprint([]flowable.Row{first, first}) == tableFingerprint([]flowable.Row{first}) {
		t.Fatal("重复行未改变指纹")
	}
}

// TestTableFingerprintValueTypes 验证字符串、字节、数字、时间、空值与未知类型被区分编码。
// @param t 测试上下文
// @return 无
func TestTableFingerprintValueTypes(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)
	cases := []struct {
		name  string
		left  any
		right any
		same  bool
	}{
		{"字符串与字节", "1", []byte("1"), false},
		{"字符串与数字", "1", int64(1), false},
		{"空值与空串", nil, "", false},
		{"时间与文本", created, "2026-01-02T03:04:05.000000006Z", false},
		{"整数类型归一", int64(7), uint64(7), true},
		{"字节值相同", []byte{1, 2}, []byte{1, 2}, true},
		{"同一时刻时区归一", created, created.In(time.FixedZone("东八区", 8*3600)), true},
		{"布尔与数字", true, int64(1), false},
		{"浮点与整数", 1.5, int64(1), false},
		{"未知类型与文本", struct{ ID int }{ID: 3}, "struct { ID int }|{3}", false},
	}
	for _, test := range cases {
		// 每类值都必须按类型标记区分，避免不同类型产生相同指纹。
		if got := fingerprintValue(test.left) == fingerprintValue(test.right); got != test.same {
			t.Fatalf("%s: 指纹相等=%v，期望 %v", test.name, got, test.same)
		}
	}
}

// queryJob 描述静默检查夹具中的一行作业。
type queryJob struct {
	ID      string    // 作业标识
	Created time.Time // 作业创建时间
}

// expectQuiescenceScan 为一次完整只读扫描添加有序的 sqlmock 期望。
// @param mock sqlmock 句柄
// @param tables 扫描发现的表名，按返回顺序
// @param version ACT_GE_PROPERTY 中的 Schema 版本
// @param jobs ACT_RU_JOB 的行
// @param counts 未知表行数
// @return 无
func expectQuiescenceScan(mock sqlmock.Sqlmock, tables []string, version string, jobs []queryJob, counts map[string]int64) {
	mock.ExpectBegin()
	tableRows := sqlmock.NewRows([]string{"table_name"})
	for _, name := range tables {
		tableRows.AddRow(name)
	}
	mock.ExpectQuery("SELECT table_name FROM information_schema.tables").WillReturnRows(tableRows)
	for _, name := range tables {
		switch name {
		case "ACT_GE_PROPERTY":
			mock.ExpectQuery("SELECT \\* FROM `ACT_GE_PROPERTY`").WillReturnRows(sqlmock.NewRows([]string{"NAME_", "VALUE_"}).AddRow([]byte("schema.version"), []byte(version)))
		case "ACT_RU_JOB":
			rows := sqlmock.NewRows([]string{"ID_", "CREATE_TIME_"})
			for _, job := range jobs {
				rows.AddRow(job.ID, job.Created)
			}
			mock.ExpectQuery("SELECT \\* FROM `ACT_RU_JOB`").WillReturnRows(rows)
		case "ACT_RU_TASK":
			mock.ExpectQuery("SELECT \\* FROM `ACT_RU_TASK`").WillReturnRows(sqlmock.NewRows([]string{"ID_"}))
		default:
			mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM `" + name + "`").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(counts[name]))
		}
	}
	mock.ExpectRollback()
}

// findQuiescenceTable 返回报告中的指定表项。
// @param report 静默检查报告
// @param name 表名
// @return 表项
// @return 是否存在
func findQuiescenceTable(report CutoverReport, name string) (TableQuiescence, bool) {
	for _, table := range report.Tables {
		if table.Name == name {
			return table, true
		}
	}
	return TableQuiescence{}, false
}

// findJobCount 返回报告中的指定作业表计数。
// @param report 静默检查报告
// @param name 作业表名
// @return 作业表计数
// @return 是否存在
func findJobCount(report CutoverReport, name string) (JobCountQuiescence, bool) {
	for _, count := range report.JobCounts {
		if count.Table == name {
			return count, true
		}
	}
	return JobCountQuiescence{}, false
}

// TestCheckQuiescenceStable 验证窗口前后内容与行数一致时报告稳定。
// @param t 测试上下文
// @return 无
func TestCheckQuiescenceStable(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tables := []string{"ACT_GE_PROPERTY", "ACT_RU_JOB"}
	created := time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)
	expectQuiescenceScan(mock, tables, "8.1.0.3", []queryJob{{"job-b", created}, {"job-a", created}}, nil)
	// 第二次快照返回相反行顺序，验证指纹基于内容而非数据库返回顺序。
	expectQuiescenceScan(mock, tables, "8.1.0.3", []queryJob{{"job-a", created}, {"job-b", created}}, nil)
	report, err := CheckQuiescence(context.Background(), db, "mysql", 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if report.Dialect != "mysql" || report.SchemaVersion != "8.1.0.3" || !report.Stable {
		t.Fatalf("报告头部错误: %+v", report)
	}
	if report.WindowSeconds != 0.005 {
		t.Fatalf("窗口秒数错误: %v", report.WindowSeconds)
	}
	// 两次快照时间必须可解析且不倒退。
	if len(report.ScanTimes) != 2 {
		t.Fatalf("快照时间数量错误: %v", report.ScanTimes)
	}
	firstTime, firstErr := time.Parse(time.RFC3339Nano, report.ScanTimes[0])
	secondTime, secondErr := time.Parse(time.RFC3339Nano, report.ScanTimes[1])
	if firstErr != nil || secondErr != nil || secondTime.Before(firstTime) {
		t.Fatalf("快照时间无效: %v %v %v", report.ScanTimes, firstErr, secondErr)
	}
	if len(report.ChangedTables) != 0 {
		t.Fatalf("稳定报告包含变化表: %v", report.ChangedTables)
	}
	jobTable, ok := findQuiescenceTable(report, "ACT_RU_JOB")
	if !ok || !jobTable.ContentCompared || jobTable.Changed || jobTable.BeforeRows != 2 || jobTable.AfterRows != 2 {
		t.Fatalf("作业表表项错误: %+v", jobTable)
	}
	if jobTable.BeforeFingerprint == "" || jobTable.BeforeFingerprint != jobTable.AfterFingerprint {
		t.Fatalf("稳定表指纹错误: %+v", jobTable)
	}
	property, ok := findQuiescenceTable(report, "ACT_GE_PROPERTY")
	if !ok || property.BeforeRows != 1 || property.AfterRows != 1 {
		t.Fatalf("属性表表项错误: %+v", property)
	}
	// 六类作业表必须全部报告，缺失表的计数为零。
	if len(report.JobCounts) != 6 {
		t.Fatalf("作业计数数量错误: %+v", report.JobCounts)
	}
	jobCount, _ := findJobCount(report, "ACT_RU_JOB")
	timerCount, _ := findJobCount(report, "ACT_RU_TIMER_JOB")
	if jobCount.Before != 2 || jobCount.After != 2 || timerCount.Before != 0 || timerCount.After != 0 {
		t.Fatalf("作业计数错误: %+v", report.JobCounts)
	}
	if len(report.Limitations) < 3 || strings.Contains(strings.Join(report.Limitations, " "), "未知表") {
		t.Fatalf("边界说明错误: %v", report.Limitations)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestCheckQuiescenceContentChange 验证行数不变而内容变化时判定不稳定。
// @param t 测试上下文
// @return 无
func TestCheckQuiescenceContentChange(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tables := []string{"ACT_GE_PROPERTY", "ACT_RU_JOB"}
	expectQuiescenceScan(mock, tables, "8.1.0.3", []queryJob{{"job-a", time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)}}, nil)
	expectQuiescenceScan(mock, tables, "8.1.0.3", []queryJob{{"job-a", time.Date(2026, 1, 2, 3, 4, 5, 7, time.UTC)}}, nil)
	report, err := CheckQuiescence(context.Background(), db, "mysql", time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if report.Stable || len(report.ChangedTables) != 1 || report.ChangedTables[0] != "ACT_RU_JOB" {
		t.Fatalf("内容变化未判不稳定: %+v", report)
	}
	jobTable, _ := findQuiescenceTable(report, "ACT_RU_JOB")
	if jobTable.BeforeRows != 1 || jobTable.AfterRows != 1 || !jobTable.ContentCompared || jobTable.BeforeFingerprint == jobTable.AfterFingerprint {
		t.Fatalf("内容变化表项错误: %+v", jobTable)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestCheckQuiescenceCountChange 验证行数变化时判定不稳定。
// @param t 测试上下文
// @return 无
func TestCheckQuiescenceCountChange(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tables := []string{"ACT_GE_PROPERTY", "ACT_RU_JOB"}
	created := time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)
	expectQuiescenceScan(mock, tables, "8.1.0.3", []queryJob{{"job-a", created}}, nil)
	expectQuiescenceScan(mock, tables, "8.1.0.3", []queryJob{{"job-a", created}, {"job-b", created}}, nil)
	report, err := CheckQuiescence(context.Background(), db, "mysql", time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if report.Stable || len(report.ChangedTables) != 1 || report.ChangedTables[0] != "ACT_RU_JOB" {
		t.Fatalf("行数变化未判不稳定: %+v", report)
	}
	jobCount, _ := findJobCount(report, "ACT_RU_JOB")
	if jobCount.Before != 1 || jobCount.After != 2 {
		t.Fatalf("行数变化计数错误: %+v", jobCount)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestCheckQuiescenceTableSetChange 验证窗口内删表同样判定不稳定。
// @param t 测试上下文
// @return 无
func TestCheckQuiescenceTableSetChange(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	created := time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)
	expectQuiescenceScan(mock, []string{"ACT_GE_PROPERTY", "ACT_RU_JOB", "ACT_RU_TASK"}, "8.1.0.3", []queryJob{{"job-a", created}}, nil)
	expectQuiescenceScan(mock, []string{"ACT_GE_PROPERTY", "ACT_RU_JOB"}, "8.1.0.3", []queryJob{{"job-a", created}}, nil)
	report, err := CheckQuiescence(context.Background(), db, "mysql", time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if report.Stable {
		t.Fatalf("表集合变化未判不稳定: %+v", report)
	}
	removed, ok := findQuiescenceTable(report, "ACT_RU_TASK")
	if !ok || !removed.Changed || removed.ContentCompared || removed.BeforeRows != 0 || removed.AfterRows != 0 {
		t.Fatalf("被删表项错误: %+v", removed)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestCheckQuiescenceUnknownTable 验证未知表只有计数时如实说明无法比对内容。
// @param t 测试上下文
// @return 无
func TestCheckQuiescenceUnknownTable(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tables := []string{"ACT_GE_PROPERTY", "ACT_RU_FUTURE"}
	counts := map[string]int64{"ACT_RU_FUTURE": 2}
	expectQuiescenceScan(mock, tables, "8.1.0.3", nil, counts)
	expectQuiescenceScan(mock, tables, "8.1.0.3", nil, counts)
	report, err := CheckQuiescence(context.Background(), db, "mysql", time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Stable {
		t.Fatalf("未知表同计数不应判变化: %+v", report)
	}
	future, ok := findQuiescenceTable(report, "ACT_RU_FUTURE")
	if !ok || future.ContentCompared || future.BeforeFingerprint != "" || future.AfterFingerprint != "" || future.BeforeRows != 2 {
		t.Fatalf("未知表项错误: %+v", future)
	}
	if !strings.Contains(strings.Join(report.Limitations, " "), "ACT_RU_FUTURE") {
		t.Fatalf("缺少未知表内容比对限制说明: %v", report.Limitations)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestCheckQuiescenceSchemaVersionChange 验证 Schema 版本变化判定不稳定。
// @param t 测试上下文
// @return 无
func TestCheckQuiescenceSchemaVersionChange(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tables := []string{"ACT_GE_PROPERTY"}
	expectQuiescenceScan(mock, tables, "8.1.0.3", nil, nil)
	expectQuiescenceScan(mock, tables, "8.1.0.4", nil, nil)
	report, err := CheckQuiescence(context.Background(), db, "mysql", time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if report.Stable || !strings.Contains(strings.Join(report.Limitations, " "), "Schema 版本不一致") {
		t.Fatalf("Schema 版本变化未判不稳定: %+v", report)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestCheckQuiescenceArgumentErrors 验证非法窗口与已取消上下文在任何扫描之前失败。
// @param t 测试上下文
// @return 无
func TestCheckQuiescenceArgumentErrors(t *testing.T) {
	for _, window := range []time.Duration{0, -time.Second} {
		// 非法窗口必须返回参数错误且不产生部分报告。
		if report, err := CheckQuiescence(context.Background(), nil, "mysql", window); err == nil || report.Dialect != "" {
			t.Fatalf("窗口 %v 未返回参数错误", window)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CheckQuiescence(ctx, nil, "mysql", time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("已取消上下文未返回取消错误: %v", err)
	}
}

// TestCheckQuiescenceContextCanceledDuringWindow 验证观察窗口内取消立即失败且不返回部分报告。
// @param t 测试上下文
// @return 无
func TestCheckQuiescenceContextCanceledDuringWindow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tables := []string{"ACT_GE_PROPERTY", "ACT_RU_JOB"}
	expectQuiescenceScan(mock, tables, "8.1.0.3", []queryJob{{"job-a", time.Now().UTC()}}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	timer := time.AfterFunc(20*time.Millisecond, cancel)
	defer timer.Stop()
	report, err := CheckQuiescence(ctx, db, "mysql", 5*time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("窗口取消未返回取消错误: %v", err)
	}
	if report.Dialect != "" || report.Stable || report.Tables != nil {
		t.Fatalf("取消后返回了部分报告: %+v", report)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestCheckQuiescenceScanErrors 验证两次扫描、非法方言与空连接错误均不返回部分报告。
// @param t 测试上下文
// @return 无
func TestCheckQuiescenceScanErrors(t *testing.T) {
	t.Run("第一次扫描开始失败", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		mock.ExpectBegin().WillReturnError(errors.New("失败"))
		if report, err := CheckQuiescence(context.Background(), db, "mysql", time.Millisecond); err == nil || report.Dialect != "" {
			t.Fatalf("第一次扫描错误未返回空报告: %+v %v", report, err)
		}
	})
	t.Run("第二次扫描开始失败", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		tables := []string{"ACT_GE_PROPERTY"}
		expectQuiescenceScan(mock, tables, "8.1.0.3", nil, nil)
		mock.ExpectBegin().WillReturnError(errors.New("失败"))
		if report, err := CheckQuiescence(context.Background(), db, "mysql", time.Millisecond); err == nil || report.Dialect != "" {
			t.Fatalf("第二次扫描错误未返回空报告: %+v %v", report, err)
		}
	})
	t.Run("非法方言", func(t *testing.T) {
		db, _, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		if _, err := CheckQuiescence(context.Background(), db, "oracle", time.Millisecond); err == nil {
			t.Fatal("非法方言未失败")
		}
	})
	t.Run("空连接", func(t *testing.T) {
		if _, err := CheckQuiescence(context.Background(), nil, "mysql", time.Millisecond); err == nil {
			t.Fatal("空连接未失败")
		}
	})
}
