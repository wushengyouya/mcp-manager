package entity

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBaseBeforeCreateGeneratesID(t *testing.T) {
	base := &Base{}
	require.NoError(t, base.BeforeCreate(nil))
	require.NotEmpty(t, base.ID)

	existing := &Base{ID: "custom-id"}
	require.NoError(t, existing.BeforeCreate(nil))
	require.Equal(t, "custom-id", existing.ID)
}

func TestJSONStringListValueAndScan(t *testing.T) {
	value, err := JSONStringList(nil).Value()
	require.NoError(t, err)
	require.Equal(t, "[]", value)

	value, err = JSONStringList{"a", "b"}.Value()
	require.NoError(t, err)
	require.Equal(t, `["a","b"]`, value)

	var list JSONStringList
	require.NoError(t, list.Scan([]byte(`["x","y"]`)))
	require.Equal(t, JSONStringList{"x", "y"}, list)

	require.NoError(t, list.Scan(`["m","n"]`))
	require.Equal(t, JSONStringList{"m", "n"}, list)

	require.NoError(t, list.Scan(nil))
	require.Equal(t, JSONStringList{"m", "n"}, list)

	require.ErrorContains(t, list.Scan(123), "不支持的 JSON 类型")
}

func TestJSONStringMapValueAndScan(t *testing.T) {
	value, err := JSONStringMap(nil).Value()
	require.NoError(t, err)
	require.Equal(t, "{}", value)

	value, err = JSONStringMap{"k": "v"}.Value()
	require.NoError(t, err)
	require.Equal(t, `{"k":"v"}`, value)

	var m JSONStringMap
	require.NoError(t, m.Scan([]byte(`{"a":"1"}`)))
	require.Equal(t, JSONStringMap{"a": "1"}, m)

	require.NoError(t, m.Scan(`{"b":"2"}`))
	require.Equal(t, JSONStringMap{"a": "1", "b": "2"}, m)

	require.NoError(t, m.Scan(nil))
	require.Equal(t, JSONStringMap{"a": "1", "b": "2"}, m)

	require.ErrorContains(t, m.Scan(true), "不支持的 JSON 类型")
}

func TestJSONMapValueAndScan(t *testing.T) {
	value, err := JSONMap(nil).Value()
	require.NoError(t, err)
	require.Equal(t, "{}", value)

	value, err = JSONMap{"k": "v", "n": float64(1)}.Value()
	require.NoError(t, err)
	require.Contains(t, value, `"k":"v"`)

	var m JSONMap
	require.NoError(t, m.Scan([]byte(`{"a":1,"nested":{"x":"y"}}`)))
	require.Equal(t, float64(1), m["a"])
	require.Equal(t, map[string]any{"x": "y"}, m["nested"])

	require.NoError(t, m.Scan(`{"ok":true}`))
	require.Equal(t, JSONMap{"a": float64(1), "nested": map[string]any{"x": "y"}, "ok": true}, m)

	require.NoError(t, m.Scan(nil))
	require.Equal(t, JSONMap{"a": float64(1), "nested": map[string]any{"x": "y"}, "ok": true}, m)

	require.ErrorContains(t, m.Scan(struct{}{}), "不支持的 JSON 类型")
}

func TestUserRoleHelpers(t *testing.T) {
	require.True(t, User{Role: RoleAdmin}.CanModify())
	require.True(t, User{Role: RoleOperator}.CanModify())
	require.False(t, User{Role: RoleReadonly}.CanModify())
	require.True(t, User{Role: RoleAdmin}.IsAdmin())
	require.False(t, User{Role: RoleOperator}.IsAdmin())
}

func TestMCPServiceIsRemote(t *testing.T) {
	require.False(t, MCPService{TransportType: TransportTypeStdio}.IsRemote())
	require.True(t, MCPService{TransportType: TransportTypeSSE}.IsRemote())
	require.True(t, MCPService{TransportType: TransportTypeStreamableHTTP}.IsRemote())
}
