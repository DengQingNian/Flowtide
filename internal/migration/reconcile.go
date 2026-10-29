package migration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	"github.com/flowable-go/flowable/internal/engine"
	"github.com/flowable-go/flowable/internal/persistence/flowable"
	statestore "github.com/flowable-go/flowable/internal/persistence/state"
)

// StateTableComparison 表示源库预期转换结果和待接管状态的逐表核对。
type StateTableComparison struct {
	Table               string `json:"table"`               // 表名称
	ExpectedRows        int    `json:"expectedRows"`        // 源快照转换后的行数
	ActualRows          int    `json:"actualRows"`          // 状态恢复后的行数
	ExpectedFingerprint string `json:"expectedFingerprint"` // 预期内容指纹
	ActualFingerprint   string `json:"actualFingerprint"`   // 实际内容指纹
	Matched             bool   `json:"matched"`             // 数量及内容均一致
}

// ReconcileReport 仅返回计数和摘要，不包含业务值、源码或凭据。
type ReconcileReport struct {
	Matched       bool                   `json:"matched"`       // 所有可转换内容是否一致
	StateSHA256   string                 `json:"stateSha256"`   // 被核对状态文件摘要
	SourceRows    int                    `json:"sourceRows"`    // 同一只读快照中的已扫描行数
	Tables        []StateTableComparison `json:"tables"`        // 确定排序的表级核对
	ChangedTables []string               `json:"changedTables"` // 内容或数量不同的表
	Limitations   []string               `json:"limitations"`   // 固定边界说明
}

// ReconcileState 将同一源快照重新转换为预期状态，与待接管状态的完整行投影比较。
// @param ctx 核对上下文
// @param source 同一只读事务扫描的源快照，调用方须另行保证源执行者已停写
// @param data 待接管状态文件字节
// @param options 目标租户范围，与转换时一致
// @return 逐表数量与内容指纹报告
// @return 预检阻断、转换、恢复或取消错误
func ReconcileState(ctx context.Context, source SourceSnapshot, data []byte, options Options) (ReconcileReport, error) {
	expected, preflight, err := Convert(ctx, source, options)
	if err != nil {
		return ReconcileReport{}, fmt.Errorf("准备源库核对状态失败: %w", err)
	}
	if preflight.Blocked {
		return ReconcileReport{}, errors.New("源库预检存在阻断，不能执行状态核对")
	}
	expectedRows, err := stateProjection(ctx, expected)
	if err != nil {
		return ReconcileReport{}, err
	}
	actualRows, err := stateProjection(ctx, data)
	if err != nil {
		return ReconcileReport{}, err
	}
	expectedTables, actualTables := rowsByTable(expectedRows), rowsByTable(actualRows)
	names := make(map[string]bool)
	for name := range expectedTables {
		names[name] = true
	}
	for name := range actualTables {
		names[name] = true
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	sum := sha256.Sum256(data)
	report := ReconcileReport{Matched: true, StateSHA256: hex.EncodeToString(sum[:]), SourceRows: len(source.Rows), Tables: make([]StateTableComparison, 0, len(names)), ChangedTables: []string{}}
	for _, name := range ordered {
		// 同数改值必须通过内容指纹发现，不能只比较转换后总数。
		left, right := expectedTables[name], actualTables[name]
		comparison := StateTableComparison{Table: name, ExpectedRows: len(left), ActualRows: len(right), ExpectedFingerprint: tableFingerprint(left), ActualFingerprint: tableFingerprint(right)}
		comparison.Matched = comparison.ExpectedRows == comparison.ActualRows && comparison.ExpectedFingerprint == comparison.ActualFingerprint
		if !comparison.Matched {
			report.Matched = false
			report.ChangedTables = append(report.ChangedTables, name)
		}
		report.Tables = append(report.Tables, comparison)
	}
	if err := ctx.Err(); err != nil {
		return ReconcileReport{}, err
	}
	report.Limitations = []string{"仅核对当前转换器支持的源实体，不扩展复杂作用域、作业或订阅的迁移范围。", "源库必须保持停写；核对成功不能替代静默检查、备份、回退或实际执行者切换。", "比较的是正式转换后的规范行投影，源表与 Go 状态的原始行数可能因控制扩展和字节外置不同。"}
	return report, nil
}

// stateProjection 在隔离内存引擎恢复状态并导出规范行，不写回源库或文件。
// @param ctx 核对上下文
// @param data 完整状态字节
// @return 全部状态的行投影
// @return 输入、恢复、导出或取消错误
func stateProjection(ctx context.Context, data []byte) ([]flowable.Row, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("待核对状态为空")
	}
	store := statestore.NewMemory()
	if err := store.Save(ctx, data); err != nil {
		return nil, err
	}
	app, err := engine.New(engine.Config{StateStore: store, HistoryLevel: engine.HistoryAudit})
	if err != nil {
		return nil, fmt.Errorf("恢复核对状态失败: %w", err)
	}
	defer func() { _ = app.Close(context.Background()) }()
	return app.SnapshotRows(ctx)
}
