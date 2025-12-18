// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

const (
	randomUUIDSpecialValue = "random-uuid"
)

var buildInfo = component.BuildInfo{
	Command: "otelcol",
	Version: "1.0.0",
}

func ptr[T any](v T) *T {
	return &v
}

func TestNew(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		resourceCfg map[string]*string
		detectors   []string
		want        map[string]string
	}{
		{
			name:        "empty",
			resourceCfg: map[string]*string{},
			detectors:   nil,
			want: map[string]string{
				"service.name":        "otelcol",
				"service.version":     "1.0.0",
				"service.instance.id": randomUUIDSpecialValue,
			},
		},
		{
			name: "overwrite",
			resourceCfg: map[string]*string{
				"service.name":        ptr("my-service"),
				"service.version":     ptr("1.2.3"),
				"service.instance.id": ptr("123"),
			},
			detectors: nil,
			want: map[string]string{
				"service.name":        "my-service",
				"service.version":     "1.2.3",
				"service.instance.id": "123",
			},
		},
		{
			name: "remove",
			resourceCfg: map[string]*string{
				"service.name":        nil,
				"service.version":     nil,
				"service.instance.id": nil,
			},
			detectors: nil,
			want:      map[string]string{},
		},
		{
			name: "add",
			resourceCfg: map[string]*string{
				"host.name": ptr("my-host"),
			},
			detectors: nil,
			want: map[string]string{
				"service.name":        "otelcol",
				"service.version":     "1.0.0",
				"service.instance.id": randomUUIDSpecialValue,
				"host.name":           "my-host",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := New(ctx, buildInfo, tt.resourceCfg, tt.detectors)
			require.NoError(t, err)

			got := make(map[string]string)
			for _, attr := range res.Attributes() {
				got[string(attr.Key)] = attr.Value.Emit()
			}

			if tt.want["service.instance.id"] == randomUUIDSpecialValue {
				assert.Contains(t, got, "service.instance.id")

				// Check that the value is a valid UUID.
				_, err := uuid.Parse(got["service.instance.id"])
				require.NoError(t, err)

				// Remove so that we can compare the rest of the map.
				delete(got, "service.instance.id")
				delete(tt.want, "service.instance.id")
			}

			assert.Equal(t, tt.want, got)
		})
	}
}

func pdataFromSdk(res *sdkresource.Resource) pcommon.Resource {
	// pcommon.NewResource is the best way to generate a new resource currently and is safe to use outside of tests.
	// Because the resource is signal agnostic, and we need a net new resource, not an existing one, this is the only
	// method of creating it without exposing internal packages.
	pcommonRes := pcommon.NewResource()
	for _, keyValue := range res.Attributes() {
		pcommonRes.Attributes().PutStr(string(keyValue.Key), keyValue.Value.AsString())
	}
	return pcommonRes
}

func TestBuildResource(t *testing.T) {
	ctx := context.Background()
	buildInfo := component.NewDefaultBuildInfo()

	// Check default config
	var resMap map[string]*string
	otelRes, err := New(ctx, buildInfo, resMap, nil)
	require.NoError(t, err)
	res := pdataFromSdk(otelRes)

	assert.Equal(t, 3, res.Attributes().Len())
	value, ok := res.Attributes().Get("service.name")
	assert.True(t, ok)
	assert.Equal(t, buildInfo.Command, value.AsString())
	value, ok = res.Attributes().Get("service.version")
	assert.True(t, ok)
	assert.Equal(t, buildInfo.Version, value.AsString())

	_, ok = res.Attributes().Get("service.instance.id")
	assert.True(t, ok)

	// Check override by nil
	resMap = map[string]*string{
		"service.name":        nil,
		"service.version":     nil,
		"service.instance.id": nil,
	}
	otelRes, err = New(ctx, buildInfo, resMap, nil)
	require.NoError(t, err)
	res = pdataFromSdk(otelRes)

	// Attributes should not exist since we nil-ified all.
	assert.Equal(t, 0, res.Attributes().Len())

	// Check override values
	strPtr := func(v string) *string { return &v }
	resMap = map[string]*string{
		"service.name":        strPtr("a"),
		"service.version":     strPtr("b"),
		"service.instance.id": strPtr("c"),
	}
	otelRes, err = New(ctx, buildInfo, resMap, nil)
	require.NoError(t, err)
	res = pdataFromSdk(otelRes)

	assert.Equal(t, 3, res.Attributes().Len())
	value, ok = res.Attributes().Get("service.name")
	assert.True(t, ok)
	assert.Equal(t, "a", value.AsString())
	value, ok = res.Attributes().Get("service.version")
	assert.True(t, ok)
	assert.Equal(t, "b", value.AsString())
	value, ok = res.Attributes().Get("service.instance.id")
	assert.True(t, ok)
	assert.Equal(t, "c", value.AsString())
}

func TestNewWithDetectors(t *testing.T) {
	ctx := context.Background()
	buildInfo := component.BuildInfo{
		Command: "otelcol-test",
		Version: "1.0.0",
	}

	t.Run("with env detector", func(t *testing.T) {
		// Set environment variable
		t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "test.key=test.value")

		resMap := map[string]*string{}
		otelRes, err := New(ctx, buildInfo, resMap, []string{"env"})
		require.NoError(t, err)
		res := pdataFromSdk(otelRes)

		// Verify that env detector added the attribute
		value, ok := res.Attributes().Get("test.key")
		assert.True(t, ok, "test.key should be set from env detector")
		assert.Equal(t, "test.value", value.AsString())

		// Verify default attributes are still present
		value, ok = res.Attributes().Get("service.name")
		assert.True(t, ok)
		assert.Equal(t, "otelcol-test", value.AsString())
	})

	t.Run("with host detector", func(t *testing.T) {
		resMap := map[string]*string{}
		otelRes, err := New(ctx, buildInfo, resMap, []string{"host"})
		require.NoError(t, err)
		res := pdataFromSdk(otelRes)

		// Host detector should add host.id (if available)
		// We just verify the resource is created successfully
		assert.NotNil(t, res)
		assert.GreaterOrEqual(t, res.Attributes().Len(), 3, "should have at least service attributes")
	})

	t.Run("with multiple detectors", func(t *testing.T) {
		t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "env.attr=value")

		resMap := map[string]*string{}
		otelRes, err := New(ctx, buildInfo, resMap, []string{"env", "host"})
		require.NoError(t, err)
		res := pdataFromSdk(otelRes)

		// Verify env detector attribute
		value, ok := res.Attributes().Get("env.attr")
		assert.True(t, ok)
		assert.Equal(t, "value", value.AsString())

		// Verify default attributes
		value, ok = res.Attributes().Get("service.name")
		assert.True(t, ok)
		assert.Equal(t, "otelcol-test", value.AsString())
	})

	t.Run("detector attributes can be overridden by config", func(t *testing.T) {
		t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "service.name=from-env")

		// Config should override detector values
		resMap := map[string]*string{
			string(semconv.ServiceNameKey): ptr("from-config"),
		}
		otelRes, err := New(ctx, buildInfo, resMap, []string{"env"})
		require.NoError(t, err)
		res := pdataFromSdk(otelRes)

		value, ok := res.Attributes().Get("service.name")
		assert.True(t, ok)
		assert.Equal(t, "from-config", value.AsString(), "config should override detector value")
	})

	t.Run("invalid detector returns error", func(t *testing.T) {
		resMap := map[string]*string{}
		_, err := New(ctx, buildInfo, resMap, []string{"invalid-detector"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown detector")
	})
}
