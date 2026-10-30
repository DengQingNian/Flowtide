package migration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/DengQingNian/Flowtide/internal/persistence/flowable"
)

// cutoverJobTables 列出静默检查必须报告前后计数的作业表，顺序固定以保证 JSON 输出稳定。
var cutoverJobTables = []string{
	"ACT_RU_JOB",
	"ACT_RU_TIMER_JOB",
	"ACT_RU_SUSPENDED_JOB",
	"ACT_RU_DEADLETTER_JOB",
	"ACT_RU_EXTERNAL_JOB",
	"ACT_RU_HISTORY_JOB",
}

// TableQuiescence 表示一张源表在观察窗口前后的行数与内容摘要。
type TableQuiescence struct {
	Name              string `json:"name"`                        // 规范化的大写表名
	BeforeRows        int64  `json:"beforeRows"`                  // 窗口前快照行数
	AfterRows         int64  `json:"afterRows"`                   // 窗口后快照行数
	ContentCompared   bool   `json:"contentCompared"`             // 是否具备逐行内容指纹
	BeforeFingerprint string `json:"beforeFingerprint,omitempty"` // 窗口前内容指纹摘要
	AfterFingerprint  string `json:"afterFingerprint,omitempty"`  // 窗口后内容指纹摘要
	Changed           bool   `json:"changed"`                     // 行数或内容是否变化
}

// JobCountQuiescence 表示一张作业表在观察窗口前后的行数。
type JobCountQuiescence struct {
	Table  string `json:"table"`  // 作业表名
	Before int64  `json:"before"` // 窗口前行数，表不存在时为零
	After  int64  `json:"after"`  // 窗口后行数，表不存在时为零
}

// CutoverReport 描述停写切换前的只读静默检查结果；只包含表名、计数与指纹摘要。
type CutoverReport struct {
	Dialect       string               `json:"dialect"`       // 数据库方言
	SchemaVersion string               `json:"schemaVersion"` // 窗口前快照读取的源库版本
	WindowSeconds float64              `json:"windowSeconds"` // 观察窗口秒数
	Stable        bool                 `json:"stable"`        // 窗口内是否未观察到写入或作业推进
	ScanTimes     []string             `json:"scanTimes"`     // 两次快照完成时间，RFC3339Nano
	Tables        []TableQuiescence    `json:"tables"`        // 表级前后行数与内容摘要
	ChangedTables []string             `json:"changedTables"` // 发生变化的表名，按字典序
	JobCounts     []JobCountQuiescence `json:"jobCounts"`     // 六类作业表前后计数
	Limitations   []string             `json:"limitations"`   // 检查边界与无法覆盖的内容
}

// CheckQuiescence 在观察窗口前后各做一次只读扫描，比较行数与内容指纹判断源库是否静默。
// @param ctx 检查上下文，取消后不返回部分报告
// @param db 已连接的源数据库
// @param dialect 数据库方言，仅支持 mysql 和 postgres
// @param window 观察窗口，必须大于零
// @return 完整静默检查报告，失败时为空报告
// @return 参数、扫描、取消或数据库错误
func CheckQuiescence(ctx context.Context, db *sql.DB, dialect string, window time.Duration) (CutoverReport, error) {
	if window <= 0 {
		return CutoverReport{}, errors.New("静默观察窗口必须大于零")
	}
	if err := ctx.Err(); err != nil {
		return CutoverReport{}, fmt.Errorf("静默检查上下文已取消: %w", err)
	}
	// 两次扫描各自开启可重复读只读事务并在返回前回滚；只有相互独立的快照才能观察到窗口内提交的写入，
	// 若共用一个事务，可重复读会隐藏第二次扫描本应发现的变化。
	before, err := Scan(ctx, db, dialect)
	if err != nil {
		return CutoverReport{}, fmt.Errorf("第一次静默扫描失败: %w", err)
	}
	scanTimes := []string{time.Now().UTC().Format(time.RFC3339Nano)}
	timer := time.NewTimer(window)
	defer timer.Stop()
	// 观察窗口必须可被调用方取消，避免停写演练被长时间挂起。
	select {
	case <-ctx.Done():
		return CutoverReport{}, fmt.Errorf("静默观察窗口被取消: %w", ctx.Err())
	case <-timer.C:
	}
	if err := ctx.Err(); err != nil {
		return CutoverReport{}, fmt.Errorf("静默观察窗口结束时上下文已取消: %w", err)
	}
	after, err := Scan(ctx, db, dialect)
	if err != nil {
		return CutoverReport{}, fmt.Errorf("第二次静默扫描失败: %w", err)
	}
	scanTimes = append(scanTimes, time.Now().UTC().Format(time.RFC3339Nano))
	return buildCutoverReport(before, after, window, scanTimes), nil
}

// buildCutoverReport 汇总两次快照的表级内容指纹、变化清单与作业计数。
// @param before 观察窗口前的只读快照
// @param after 观察窗口后的只读快照
// @param window 观察窗口
// @param scanTimes 两次快照完成时间
// @return 静默检查报告
func buildCutoverReport(before, after SourceSnapshot, window time.Duration, scanTimes []string) CutoverReport {
	known := sourceKnownTables()
	beforeCounts := tableCountMap(before.Tables)
	afterCounts := tableCountMap(after.Tables)
	beforeRows := rowsByTable(before.Rows)
	afterRows := rowsByTable(after.Rows)
	names := make([]string, 0, len(beforeCounts)+len(afterCounts))
	seen := make(map[string]bool, len(beforeCounts)+len(afterCounts))
	// 表集合取两次快照并集，窗口内新建或删除的表都必须进入变化清单。
	for name := range beforeCounts {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	for name := range afterCounts {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	report := CutoverReport{
		Dialect:       before.Dialect,
		SchemaVersion: before.SchemaVersion,
		WindowSeconds: window.Seconds(),
		Stable:        true,
		ScanTimes:     scanTimes,
		Tables:        make([]TableQuiescence, 0, len(names)),
		ChangedTables: make([]string, 0),
		JobCounts:     make([]JobCountQuiescence, 0, len(cutoverJobTables)),
		Limitations:   cutoverLimitations(),
	}
	unknown := make([]string, 0)
	for _, name := range names {
		beforeCount, beforePresent := beforeCounts[name]
		afterCount, afterPresent := afterCounts[name]
		table := TableQuiescence{Name: name, BeforeRows: beforeCount, AfterRows: afterCount}
		// 两次快照都存在且属于已知表时，才具备逐行内容指纹的可比性。
		if beforePresent && afterPresent && known[name] {
			table.ContentCompared = true
			table.BeforeFingerprint = tableFingerprint(beforeRows[name])
			table.AfterFingerprint = tableFingerprint(afterRows[name])
		}
		// 行数变化必判定为不稳定；内容可比时再比较指纹，避免行数相同的改值被漏检。
		if beforeCount != afterCount || (table.ContentCompared && table.BeforeFingerprint != table.AfterFingerprint) {
			table.Changed = true
		}
		// 表只在单侧出现说明窗口内发生建表或删表，同样判定为变化。
		if !beforePresent || !afterPresent {
			table.Changed = true
		}
		// 未知表没有载入行内容，只能如实标记无法比对内容。
		if beforePresent && afterPresent && !known[name] {
			unknown = append(unknown, name)
		}
		if table.Changed {
			report.Stable = false
			report.ChangedTables = append(report.ChangedTables, name)
		}
		report.Tables = append(report.Tables, table)
	}
	// 两次快照的 Schema 版本不一致意味着元数据被写入，即使指纹异常也必须判定为不稳定。
	if before.SchemaVersion != after.SchemaVersion {
		report.Stable = false
		report.Limitations = append(report.Limitations, "两次扫描读取的 Schema 版本不一致，源库元数据在观察窗口内发生变化。")
	}
	// 未知表只有计数，明确记录其内容无法比对。
	if len(unknown) > 0 {
		report.Limitations = append(report.Limitations, "未知表 "+strings.Join(unknown, "、")+" 仅有行数统计，无法比对内容变化。")
	}
	for _, name := range cutoverJobTables {
		report.JobCounts = append(report.JobCounts, JobCountQuiescence{Table: name, Before: beforeCounts[name], After: afterCounts[name]})
	}
	return report
}

// tableCountMap 将表统计转换为表名到行数的映射。
// @param tables 表统计列表
// @return 表名到行数的映射
func tableCountMap(tables []TableSummary) map[string]int64 {
	counts := make(map[string]int64, len(tables))
	for _, table := range tables {
		counts[table.Name] = table.Count
	}
	return counts
}

// rowsByTable 将快照行按表名分组。
// @param rows 快照行
// @return 表名到行集合的映射
func rowsByTable(rows []flowable.Row) map[string][]flowable.Row {
	grouped := make(map[string][]flowable.Row)
	for _, row := range rows {
		grouped[row.Table] = append(grouped[row.Table], row)
	}
	return grouped
}

// cutoverLimitations 返回静默检查始终存在的边界说明。
// @return 边界说明
func cutoverLimitations() []string {
	return []string{
		"静默检查只比较两次只读快照的行数与内容指纹，不读取变量值、部署源码或连接凭据。",
		"观察窗口内提交的写入会出现在第二次快照中；两次快照之间未提交或已回滚的写入可能不可见。",
		"stable=true 仅表示观察窗口内未观察到写入或后台作业推进，不替代停写、备份与回退演练。",
	}
}

// tableFingerprint 返回一张表全部行的确定性内容指纹；行顺序与列顺序均不影响结果。
// @param rows 该表的行集合
// @return 十六进制指纹摘要，空表也有稳定摘要
func tableFingerprint(rows []flowable.Row) string {
	encoded := make([][]byte, 0, len(rows))
	for _, row := range rows {
		encoded = append(encoded, rowFingerprint(row))
	}
	// 行级排序消除数据库返回顺序差异，重复行各自保留不会被合并。
	sort.Slice(encoded, func(left, right int) bool { return bytes.Compare(encoded[left], encoded[right]) < 0 })
	digest := sha256.New()
	var size [8]byte
	for _, row := range encoded {
		binary.BigEndian.PutUint64(size[:], uint64(len(row)))
		_, _ = digest.Write(size[:])
		_, _ = digest.Write(row)
	}
	sum := digest.Sum(nil)
	// 报告只暴露摘要，不暴露任何行内容。
	return hex.EncodeToString(sum[:16])
}

// rowFingerprint 返回单行数据的确定性内容编码，列名按字典序排列。
// @param row 单行数据
// @return 行内容编码
func rowFingerprint(row flowable.Row) []byte {
	columns := make([]string, 0, len(row.Values))
	for name := range row.Values {
		columns = append(columns, name)
	}
	sort.Strings(columns)
	encoded := make([]byte, 0, len(columns)*16)
	for _, name := range columns {
		encoded = appendFingerprintText(encoded, 'C', []byte(name))
		encoded = encodeFingerprintValue(encoded, row.Values[name])
	}
	return encoded
}

// appendFingerprintText 追加带类型标记和长度前缀的内容，消除字段直接拼接产生的歧义。
// @param dst 目标字节缓冲
// @param tag 类型标记
// @param data 内容字节
// @return 追加后的字节缓冲
func appendFingerprintText(dst []byte, tag byte, data []byte) []byte {
	dst = append(dst, tag)
	dst = binary.AppendUvarint(dst, uint64(len(data)))
	return append(dst, data...)
}

// encodeFingerprintValue 将列值编码为带类型标记和长度前缀的字节，区分字符串、字节、数字、时间与空值。
// @param dst 目标字节缓冲
// @param value 列值
// @return 追加后的字节缓冲
func encodeFingerprintValue(dst []byte, value any) []byte {
	switch typed := value.(type) {
	case nil:
		return append(dst, 'N')
	case string:
		return appendFingerprintText(dst, 'S', []byte(typed))
	case []byte:
		return appendFingerprintText(dst, 'B', typed)
	case bool:
		// 布尔值单独标记，避免与数字零一或字符串混淆。
		if typed {
			return append(dst, 'O', '1')
		}
		return append(dst, 'O', '0')
	case time.Time:
		// 时间统一为 UTC 纳秒文本，同一时刻的不同时区表示产生相同指纹。
		return appendFingerprintText(dst, 'T', []byte(typed.UTC().Format(time.RFC3339Nano)))
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		// 各类整数统一十进制文本，同值不同类型不会产生不同指纹。
		return appendFingerprintText(dst, 'D', []byte(fmt.Sprintf("%d", typed)))
	case float32, float64:
		// 浮点同样归入数字类别，保证与整数的值语义一致。
		return appendFingerprintText(dst, 'D', []byte(fmt.Sprintf("%v", typed)))
	default:
		// 未知驱动类型保留类型名与值文本，保证不静默丢弃列。
		return appendFingerprintText(dst, 'X', []byte(fmt.Sprintf("%T|%v", value, value)))
	}
}
