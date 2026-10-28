package flowable

import (
	"fmt"
	"testing"
)

// benchRows 构造指定数量的运行任务行。
// @param b 基准上下文
// @param count 行数
// @return 行列表
func benchRows(b *testing.B, count int) []Row {
	b.Helper()
	rows := make([]Row, 0, count)
	for index := 0; index < count; index++ {
		row, err := NewRow("ACT_RU_TASK")
		if err != nil {
			b.Fatalf("创建行失败: %v", err)
		}
		if err := row.Set("ID_", fmt.Sprintf("task-%04d", index)); err != nil {
			b.Fatalf("设置主键失败: %v", err)
		}
		if err := row.Set("REV_", int64(1)); err != nil {
			b.Fatalf("设置修订号失败: %v", err)
		}
		if err := row.Set("NAME_", "Approve"); err != nil {
			b.Fatalf("设置名称失败: %v", err)
		}
		if err := row.Set("TENANT_ID_", "tenant-a"); err != nil {
			b.Fatalf("设置租户失败: %v", err)
		}
		rows = append(rows, *row)
	}
	return rows
}

// BenchmarkDiffOperations1000 测量 1000 行快照中单行变化时的增量操作生成开销。
// @param b 基准上下文
func BenchmarkDiffOperations1000(b *testing.B) {
	oldRows := benchRows(b, 1000)
	newRows := append([]Row(nil), oldRows...)
	newRows[500].Values = copyRowValues(newRows[500].Values)
	newRows[500].Values["NAME_"] = "Changed"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		operations := DiffOperations(oldRows, newRows)
		if len(operations) != 1 {
			b.Fatalf("期望 1 个操作，实际 %d", len(operations))
		}
	}
}

// BenchmarkSnapshotOperations1000 测量 1000 行全量替换操作生成开销。
// @param b 基准上下文
func BenchmarkSnapshotOperations1000(b *testing.B) {
	rows := benchRows(b, 1000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		operations := SnapshotOperations(rows)
		if len(operations) != len(rows)*2 {
			b.Fatalf("操作数量错误: %d", len(operations))
		}
	}
}

// copyRowValues 复制行值映射，避免基准数据共享底层映射。
// @param source 源映射
// @return 新映射
func copyRowValues(source map[string]any) map[string]any {
	target := make(map[string]any, len(source))
	for key, value := range source {
		target[key] = value
	}
	return target
}
