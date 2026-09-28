package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseTableSpec(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    TableSpec
		wantErr bool
	}{
		{
			name: "显式列名",
			in:   "chain.actor_actions:epoch",
			want: TableSpec{Schema: "chain", Table: "actor_actions", Column: "epoch"},
		},
		{
			name: "省略列名默认 epoch",
			in:   "fevm.evm_transfers",
			want: TableSpec{Schema: "fevm", Table: "evm_transfers", Column: "epoch"},
		},
		{
			name: "非 epoch 的高度列",
			in:   "fevm.evm_transfers:height",
			want: TableSpec{Schema: "fevm", Table: "evm_transfers", Column: "height"},
		},
		{
			name: "两侧空白被裁剪",
			in:   "  chain.miner_infos : epoch ",
			want: TableSpec{Schema: "chain", Table: "miner_infos", Column: "epoch"},
		},
		{name: "空串", in: "", wantErr: true},
		{name: "缺少 schema", in: "actor_actions:epoch", wantErr: true},
		{name: "段数过多", in: "a.b.c:epoch", wantErr: true},
		{name: "列名为空", in: "chain.actor_actions:", wantErr: true},
		{name: "注入尝试（分号）", in: "chain.actor_actions;drop table x:epoch", wantErr: true},
		{name: "注入尝试（引号）", in: `chain."actor_actions":epoch`, wantErr: true},
		{name: "注入尝试（空格）", in: "chain.actor actions:epoch", wantErr: true},
		{
			name: "大写表名归一化为小写",
			in:   "CHAIN.ACTOR_ACTIONS:EPOCH",
			want: TableSpec{Schema: "chain", Table: "actor_actions", Column: "epoch"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseTableSpec(c.in)
			if c.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.want, got)
			require.NoError(t, got.Validate())
		})
	}
}

func TestTableSpecMaxSQL(t *testing.T) {
	spec, err := ParseTableSpec("chain.actor_actions:epoch")
	require.NoError(t, err)
	require.Equal(t, `select max("epoch") from "chain"."actor_actions"`, spec.MaxSQL())
	require.Equal(t, "chain.actor_actions", spec.Qualified())
	require.Equal(t, "chain.actor_actions:epoch", spec.String())
}

func TestDefaultTableSpecs(t *testing.T) {
	specs := DefaultTableSpecs()
	require.NotEmpty(t, specs)

	seen := map[string]bool{}
	for _, s := range specs {
		require.NoError(t, s.Validate(), "内置清单必须合法: %s", s.String())
		require.False(t, seen[s.String()], "内置清单不应重复: %s", s.String())
		seen[s.String()] = true
	}

	// 事故相关：同步器游标表与任务里点名的两张表必须在清单里。
	require.True(t, seen["chain.sync_syncers:epoch"], "必须包含同步器游标表")
	require.True(t, seen["chain.actor_actions:epoch"], "必须包含 chain.actor_actions")
	require.True(t, seen["fevm.evm_transfers:epoch"], "必须包含 fevm.evm_transfers")
}

func TestParseTableSpecs(t *testing.T) {
	specs, err := ParseTableSpecs([]string{"chain.sync_syncers:epoch", "chain.actor_actions"})
	require.NoError(t, err)
	require.Len(t, specs, 2)
	require.Equal(t, "epoch", specs[1].Column)

	_, err = ParseTableSpecs([]string{"chain.sync_syncers:epoch", "bad"})
	require.Error(t, err)
}

func TestQuoteQualified(t *testing.T) {
	got, err := QuoteQualified("chain.sync_syncers")
	require.NoError(t, err)
	require.Equal(t, `"chain"."sync_syncers"`, got)

	_, err = QuoteQualified("chain.sync_syncers;drop")
	require.Error(t, err)

	_, err = QuoteQualified("sync_syncers")
	require.Error(t, err)
}

func TestNetworkName(t *testing.T) {
	require.Equal(t, "mainnet", NetworkName(false))
	require.Equal(t, "calibnet", NetworkName(true))
}
