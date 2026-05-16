package model

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
)

// Usage log CSV export limits and column keys (aligned with web COLUMN_KEYS).
const (
	LogExportMaxRows   int64 = 50000
	LogExportBatchSize int   = 1000
	LogExportMaxDetailRunes int = 4096

	LogExportColTime        = "time"
	LogExportColChannel     = "channel"
	LogExportColUsername    = "username"
	LogExportColToken       = "token"
	LogExportColGroup       = "group"
	LogExportColType        = "type"
	LogExportColModel       = "model"
	LogExportColUseTime     = "use_time"
	LogExportColPrompt      = "prompt"
	LogExportColCompletion  = "completion"
	LogExportColCost        = "cost"
	LogExportColRetry       = "retry"
	LogExportColIP          = "ip"
	LogExportColDetails     = "details"
)

var (
	ErrLogExportNoColumns = errors.New("no export columns selected")

	logExportColumnsAdmin = []string{
		LogExportColTime, LogExportColChannel, LogExportColUsername, LogExportColToken,
		LogExportColGroup, LogExportColType, LogExportColModel, LogExportColUseTime,
		LogExportColPrompt, LogExportColCompletion, LogExportColCost, LogExportColRetry,
		LogExportColIP, LogExportColDetails,
	}
	logExportColumnsUser = []string{
		LogExportColTime, LogExportColToken, LogExportColGroup, LogExportColType, LogExportColModel,
		LogExportColUseTime, LogExportColPrompt, LogExportColCompletion, LogExportColCost,
		LogExportColIP, LogExportColDetails,
	}

	logExportHeaderZH = map[string]string{
		LogExportColTime:       "时间",
		LogExportColChannel:    "渠道",
		LogExportColUsername:   "用户",
		LogExportColToken:      "令牌",
		LogExportColGroup:      "分组",
		LogExportColType:       "类型",
		LogExportColModel:      "模型",
		LogExportColUseTime:    "用时",
		LogExportColPrompt:     "输入",
		LogExportColCompletion: "输出",
		LogExportColCost:       "花费(额度)",
		LogExportColRetry:      "重试",
		LogExportColIP:         "IP",
		LogExportColDetails:    "详情",
	}
)

func adminExportColumnSet() map[string]struct{} {
	m := make(map[string]struct{}, len(logExportColumnsAdmin))
	for _, c := range logExportColumnsAdmin {
		m[c] = struct{}{}
	}
	return m
}

func userExportColumnSet() map[string]struct{} {
	m := make(map[string]struct{}, len(logExportColumnsUser))
	for _, c := range logExportColumnsUser {
		m[c] = struct{}{}
	}
	return m
}

// ParseLogExportColumns parses the `columns` query (comma-separated keys). Order is preserved; duplicates dropped.
func ParseLogExportColumns(isAdmin bool, columnsParam string) []string {
	allow := userExportColumnSet()
	if isAdmin {
		allow = adminExportColumnSet()
	}
	seen := map[string]struct{}{}
	var out []string
	for _, part := range strings.Split(columnsParam, ",") {
		key := strings.TrimSpace(part)
		if key == "" {
			continue
		}
		if _, ok := allow[key]; !ok {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

// CountAdminLogsForExport counts rows matching admin log filters (no pagination cap).
func CountAdminLogsForExport(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, group string, requestId string) (int64, error) {
	tx := buildAdminLogFilterTx(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group, requestId)
	var total int64
	err := tx.Model(&Log{}).Count(&total).Error
	return total, err
}

// CountUserLogsForExport counts rows matching self log filters (full count, not capped at logSearchCountLimit).
func CountUserLogsForExport(userId int, logType int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string, group string, requestId string) (int64, error) {
	tx, err := buildUserLogFilterTx(userId, logType, startTimestamp, endTimestamp, modelName, tokenName, group, requestId)
	if err != nil {
		return 0, err
	}
	var total int64
	err = tx.Model(&Log{}).Count(&total).Error
	return total, err
}

// WriteAdminUsageLogsCSV writes CSV with UTF-8 BOM. Call CountAdminLogsForExport and enforce LogExportMaxRows in the controller before invoking. total must match the same filters.
func WriteAdminUsageLogsCSV(w io.Writer, total int64, logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, group string, requestId string, columns []string) error {
	if len(columns) == 0 {
		return ErrLogExportNoColumns
	}
	if total == 0 {
		return writeLogsCSVHeaderOnly(w, columns)
	}
	if _, err := w.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	header := make([]string, len(columns))
	for i, col := range columns {
		if h, ok := logExportHeaderZH[col]; ok {
			header[i] = h
		} else {
			header[i] = col
		}
	}
	if err := cw.Write(header); err != nil {
		return err
	}

	var lastID int
	for {
		tx := buildAdminLogFilterTx(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group, requestId)
		if lastID > 0 {
			tx = tx.Where("logs.id < ?", lastID)
		}
		var batch []*Log
		if err := tx.Order("logs.id desc").Limit(LogExportBatchSize).Find(&batch).Error; err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}
		if err := fillLogChannelNames(batch); err != nil {
			return err
		}
		if err := fillTokenNamesForLogs(batch); err != nil {
			return err
		}
		for _, log := range batch {
			row := buildLogCSVRow(log, columns, true)
			if err := cw.Write(row); err != nil {
				return err
			}
		}
		cw.Flush()
		if err := cw.Error(); err != nil {
			return err
		}
		lastID = batch[len(batch)-1].Id
	}
	return nil
}

// WriteUserUsageLogsCSV writes CSV for the current user's logs. Enforce row limit in the controller using CountUserLogsForExport.
func WriteUserUsageLogsCSV(w io.Writer, total int64, userId int, logType int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string, group string, requestId string, columns []string) error {
	if len(columns) == 0 {
		return ErrLogExportNoColumns
	}
	if total == 0 {
		return writeLogsCSVHeaderOnly(w, columns)
	}
	if _, err := w.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	header := make([]string, len(columns))
	for i, col := range columns {
		if h, ok := logExportHeaderZH[col]; ok {
			header[i] = h
		} else {
			header[i] = col
		}
	}
	if err := cw.Write(header); err != nil {
		return err
	}

	var lastID int
	for {
		tx, err := buildUserLogFilterTx(userId, logType, startTimestamp, endTimestamp, modelName, tokenName, group, requestId)
		if err != nil {
			return err
		}
		if lastID > 0 {
			tx = tx.Where("logs.id < ?", lastID)
		}
		var batch []*Log
		if err := tx.Order("logs.id desc").Limit(LogExportBatchSize).Find(&batch).Error; err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}
		if err := fillTokenNamesForLogs(batch); err != nil {
			return err
		}
		for _, log := range batch {
			row := buildLogCSVRow(log, columns, false)
			if err := cw.Write(row); err != nil {
				return err
			}
		}
		cw.Flush()
		if err := cw.Error(); err != nil {
			return err
		}
		lastID = batch[len(batch)-1].Id
	}
	return nil
}

func writeLogsCSVHeaderOnly(w io.Writer, columns []string) error {
	if _, err := w.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	header := make([]string, len(columns))
	for i, col := range columns {
		if h, ok := logExportHeaderZH[col]; ok {
			header[i] = h
		} else {
			header[i] = col
		}
	}
	if err := cw.Write(header); err != nil {
		return err
	}
	cw.Flush()
	return cw.Error()
}

func fillTokenNamesForLogs(logs []*Log) error {
	ids := types.NewSet[int]()
	for _, l := range logs {
		if strings.TrimSpace(l.TokenName) == "" && l.TokenId > 0 {
			ids.Add(l.TokenId)
		}
	}
	if ids.Len() == 0 {
		return nil
	}
	type tokRow struct {
		Id   int    `gorm:"column:id"`
		Name string `gorm:"column:name"`
	}
	var rows []tokRow
	if err := DB.Table("tokens").Select("id, name").Where("id IN ?", ids.Items()).Find(&rows).Error; err != nil {
		return err
	}
	names := make(map[int]string, len(rows))
	for _, r := range rows {
		names[r.Id] = r.Name
	}
	for i := range logs {
		if strings.TrimSpace(logs[i].TokenName) == "" && logs[i].TokenId > 0 {
			if n := names[logs[i].TokenId]; n != "" {
				logs[i].TokenName = n
			}
		}
	}
	return nil
}

func buildLogCSVRow(log *Log, columns []string, isAdmin bool) []string {
	out := make([]string, len(columns))
	for i, col := range columns {
		out[i] = logCSVCell(log, col, isAdmin)
	}
	return out
}

func logCSVCell(log *Log, col string, isAdmin bool) string {
	switch col {
	case LogExportColTime:
		return time.Unix(log.CreatedAt, 0).Format("2006-01-02 15:04:05")
	case LogExportColChannel:
		if !isAdmin {
			return ""
		}
		return formatChannelCell(log)
	case LogExportColUsername:
		if !isAdmin {
			return ""
		}
		return log.Username
	case LogExportColToken:
		return log.TokenName
	case LogExportColGroup:
		if strings.TrimSpace(log.Group) != "" {
			return log.Group
		}
		if strings.TrimSpace(log.Other) == "" {
			return ""
		}
		otherMap, err := common.StrToMap(log.Other)
		if err != nil || otherMap == nil {
			return ""
		}
		if g, ok := otherMap["group"].(string); ok {
			return g
		}
		return ""
	case LogExportColType:
		return logTypeLabelZH(log.Type)
	case LogExportColModel:
		return log.ModelName
	case LogExportColUseTime:
		return strconv.Itoa(log.UseTime)
	case LogExportColPrompt:
		return strconv.Itoa(log.PromptTokens)
	case LogExportColCompletion:
		return strconv.Itoa(log.CompletionTokens)
	case LogExportColCost:
		return strconv.FormatInt(int64(log.Quota), 10)
	case LogExportColRetry:
		if !isAdmin {
			return ""
		}
		return formatRetryCell(log)
	case LogExportColIP:
		return log.Ip
	case LogExportColDetails:
		return truncateRunes(log.Content, LogExportMaxDetailRunes)
	default:
		return ""
	}
}

func formatChannelCell(log *Log) string {
	name := strings.TrimSpace(log.ChannelName)
	id := log.ChannelId
	if id == 0 {
		return ""
	}
	if name != "" {
		return fmt.Sprintf("%s (%d)", name, id)
	}
	return strconv.Itoa(id)
}

func formatRetryCell(log *Log) string {
	uc := extractUseChannelFromOther(log.Other)
	if uc != "" {
		return "渠道：" + uc
	}
	if log.ChannelId != 0 {
		return fmt.Sprintf("渠道：%s", formatChannelCell(log))
	}
	return ""
}

func extractUseChannelFromOther(other string) string {
	otherMap, err := common.StrToMap(other)
	if err != nil || otherMap == nil {
		return ""
	}
	ai, ok := otherMap["admin_info"].(map[string]interface{})
	if !ok || ai == nil {
		return ""
	}
	raw, ok := ai["use_channel"]
	if !ok || raw == nil {
		return ""
	}
	switch v := raw.(type) {
	case []interface{}:
		parts := make([]string, 0, len(v))
		for _, x := range v {
			parts = append(parts, fmt.Sprint(x))
		}
		return strings.Join(parts, "->")
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

func logTypeLabelZH(t int) string {
	switch t {
	case LogTypeTopup:
		return "充值"
	case LogTypeConsume:
		return "消费"
	case LogTypeManage:
		return "管理"
	case LogTypeSystem:
		return "系统"
	case LogTypeError:
		return "错误"
	case LogTypeRefund:
		return "退款"
	default:
		return "未知"
	}
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max]) + "..."
}
