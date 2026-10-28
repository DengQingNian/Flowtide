package migration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/flowable-go/flowable/internal/engine"
	"github.com/flowable-go/flowable/internal/persistence/flowable"
	statestore "github.com/flowable-go/flowable/internal/persistence/state"
)

// PreparedStateSummary 绑定离线准备的 Go 状态制品，不表示源引擎已停写或完成切换。
type PreparedStateSummary struct {
	Format        string `json:"format"`        // 状态格式
	SHA256        string `json:"sha256"`        // 状态字节摘要
	Bytes         int    `json:"bytes"`         // 状态字节数
	SourceRows    int    `json:"sourceRows"`    // 全库扫描实体行数，不含仅统计的表
	ConvertedRows int    `json:"convertedRows"` // 本次进入状态恢复的实体行数
}

// Convert 对源快照执行完整预检，并在隔离内存引擎中准备可供 Go 服务恢复的状态。
// @param ctx 转换上下文，取消后不返回产物
// @param snapshot 同一只读事务取得的源数据
// @param options 目标租户范围
// @return 完整 Go 状态字节，阻断或错误时为空
// @return 预检报告及成功产物摘要
// @return 解码、恢复、序列化或取消错误；预检阻断本身不作为操作错误
func Convert(ctx context.Context, snapshot SourceSnapshot, options Options) ([]byte, Report, error) {
	if err := ctx.Err(); err != nil {
		return nil, Report{}, err
	}
	report := Analyze(snapshot, options)
	if report.Blocked {
		return nil, report, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, report, err
	}
	rows := snapshot.Rows
	if options.TenantID != nil {
		rows = flowable.FilterRowsByTenant(rows, *options.TenantID)
	}
	// Java 运行时活动与同标识历史活动已由预检逐列验证相等，仅保留历史容器恢复。
	converted := make([]flowable.Row, 0, len(rows))
	for _, row := range rows {
		if row.Table != "ACT_RU_ACTINST" {
			converted = append(converted, row)
		}
	}
	store := statestore.NewMemory()
	// Java 存量在 audit 级别记录顺序流历史，续跑引擎保持相同审计形态。
	workflow, err := engine.New(engine.Config{StateStore: store, HistoryLevel: engine.HistoryAudit, RecordSequenceFlowHistory: true})
	if err != nil {
		return nil, report, fmt.Errorf("创建隔离转换引擎失败: %w", err)
	}
	if err := workflow.RestoreRows(ctx, converted); err != nil {
		return nil, report, fmt.Errorf("恢复源实体失败: %w", err)
	}
	if err := workflow.Close(ctx); err != nil {
		return nil, report, fmt.Errorf("准备 Go 状态失败: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, report, err
	}
	data, err := store.Load(ctx)
	if err != nil {
		return nil, report, fmt.Errorf("读取准备状态失败: %w", err)
	}
	sum := sha256.Sum256(data)
	report.PreparedState = &PreparedStateSummary{Format: StateFormat, SHA256: hex.EncodeToString(sum[:]), Bytes: len(data), SourceRows: len(snapshot.Rows), ConvertedRows: len(converted)}
	report.Limitations = append(report.Limitations, "已离线准备 Go 状态；未执行源引擎停写、实例归属切换或回退演练，禁止据此同时启用 Java 和 Go。")
	return data, report, nil
}
