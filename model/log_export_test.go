package model

import (
	"bytes"
	"encoding/csv"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogExportMaxRows(t *testing.T) {
	t.Run("default when unset", func(t *testing.T) {
		t.Setenv("LOG_EXPORT_MAX_ROWS", "")
		assert.Equal(t, int64(defaultLogExportMaxRows), LogExportMaxRows())
	})
	t.Run("custom value", func(t *testing.T) {
		t.Setenv("LOG_EXPORT_MAX_ROWS", "1000000")
		assert.Equal(t, int64(1000000), LogExportMaxRows())
	})
	t.Run("non positive falls back to default", func(t *testing.T) {
		t.Setenv("LOG_EXPORT_MAX_ROWS", "0")
		assert.Equal(t, int64(defaultLogExportMaxRows), LogExportMaxRows())
		t.Setenv("LOG_EXPORT_MAX_ROWS", "-10")
		assert.Equal(t, int64(defaultLogExportMaxRows), LogExportMaxRows())
	})
}

func TestLogExportOrderClause(t *testing.T) {
	original := common.LogDatabaseType()
	t.Cleanup(func() { common.SetLogDatabaseType(original) })

	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	assert.Equal(t, "logs.id desc", logExportOrderClause())

	common.SetLogDatabaseType(common.DatabaseTypeClickHouse)
	assert.Equal(t, "logs.created_at desc, logs.request_id desc", logExportOrderClause())
}

func TestStreamLogsCSVRejectsEmptyColumns(t *testing.T) {
	err := streamLogsCSV(&bytes.Buffer{}, nil, false, func(last *Log, limit int) ([]*Log, error) {
		t.Fatal("fetch should not run")
		return nil, nil
	})
	assert.ErrorIs(t, err, ErrLogExportNoColumns)
}

func TestStreamLogsCSVStopsAtMaxRows(t *testing.T) {
	t.Setenv("LOG_EXPORT_MAX_ROWS", "5")

	var buf bytes.Buffer
	nextID := 100
	err := streamLogsCSV(&buf, []string{LogExportColModel}, false, func(last *Log, limit int) ([]*Log, error) {
		batch := make([]*Log, 0, limit)
		for i := 0; i < limit; i++ {
			batch = append(batch, &Log{Id: nextID, ModelName: "m" + strconv.Itoa(nextID)})
			nextID--
		}
		return batch, nil
	})
	require.NoError(t, err)

	raw := buf.Bytes()
	require.GreaterOrEqual(t, len(raw), 3)
	assert.Equal(t, []byte{0xEF, 0xBB, 0xBF}, raw[:3])

	rows, err := csv.NewReader(bytes.NewReader(raw[3:])).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 6)
	assert.Equal(t, []string{"模型"}, rows[0])
	assert.Equal(t, "m100", rows[1][0])
	assert.Equal(t, "m96", rows[5][0])
}

func TestLogExportCursorStuck(t *testing.T) {
	assert.False(t, logExportCursorStuck(nil, &Log{Id: 1}))
	assert.True(t, logExportCursorStuck(&Log{Id: 1}, &Log{Id: 1}))
	assert.False(t, logExportCursorStuck(&Log{Id: 2}, &Log{Id: 1}))

	original := common.LogDatabaseType()
	t.Cleanup(func() { common.SetLogDatabaseType(original) })
	common.SetLogDatabaseType(common.DatabaseTypeClickHouse)
	assert.True(t, logExportCursorStuck(
		&Log{CreatedAt: 10, RequestId: "abc"},
		&Log{CreatedAt: 10, RequestId: "abc"},
	))
	assert.False(t, logExportCursorStuck(
		&Log{CreatedAt: 10, RequestId: "abc"},
		&Log{CreatedAt: 9, RequestId: "abc"},
	))
}
