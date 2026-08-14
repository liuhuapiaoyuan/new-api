package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildLogTextFilter(t *testing.T) {
	originalLogDatabaseType := common.LogDatabaseType()
	t.Cleanup(func() {
		common.SetLogDatabaseType(originalLogDatabaseType)
	})

	for _, dbType := range []common.DatabaseType{
		common.DatabaseTypeSQLite,
		common.DatabaseTypeMySQL,
		common.DatabaseTypePostgreSQL,
	} {
		t.Run(string(dbType)+" wraps substring and escapes underscore", func(t *testing.T) {
			common.SetLogDatabaseType(dbType)

			cases := []struct {
				name      string
				value     string
				condition string
				arg       string
			}{
				{
					name:      "plain model name becomes substring like",
					value:     "gpt-4",
					condition: "logs.model_name LIKE ? ESCAPE '!'",
					arg:       "%gpt-4%",
				},
				{
					name:      "underscore is escaped",
					value:     "gpt_4",
					condition: "logs.model_name LIKE ? ESCAPE '!'",
					arg:       "%gpt!_4%",
				},
				{
					name:      "explicit wildcard is not wrapped again",
					value:     "gpt%",
					condition: "logs.model_name LIKE ? ESCAPE '!'",
					arg:       "gpt%",
				},
				{
					name:      "single character stays exact",
					value:     "g",
					condition: "logs.model_name = ?",
					arg:       "g",
				},
				{
					name:      "empty value skips filter",
					value:     "",
					condition: "",
					arg:       "",
				},
			}

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					condition, arg, err := buildLogTextFilter("logs.model_name", tc.value)
					require.NoError(t, err)
					assert.Equal(t, tc.condition, condition)
					assert.Equal(t, tc.arg, arg)
				})
			}
		})
	}

	t.Run("clickhouse wraps substring and escapes without ESCAPE", func(t *testing.T) {
		common.SetLogDatabaseType(common.DatabaseTypeClickHouse)

		condition, arg, err := buildLogTextFilter("logs.username", "gpt_4")
		require.NoError(t, err)
		assert.Equal(t, "logs.username LIKE ?", condition)
		assert.Equal(t, `%gpt\_4%`, arg)

		condition, arg, err = buildLogTextFilter("logs.username", "admin%")
		require.NoError(t, err)
		assert.Equal(t, "logs.username LIKE ?", condition)
		assert.Equal(t, "admin%", arg)
	})
}
