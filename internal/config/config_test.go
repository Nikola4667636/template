package config_test

import (
	"strings"
	"testing"

	"github.com/Nikola4667636/template/internal/config"
)

const (
	maxConnsVariable = "DATABASE_MAX_CONNS"
	minConnsVariable = "DATABASE_MIN_CONNS"
	endpointVariable = "OTEL_EXPORTER_OTLP_ENDPOINT"
	resourceVariable = "OTEL_RESOURCE_ATTRIBUTES"
)

func configure(t *testing.T) {
	t.Helper()
	for name, value := range map[string]string{
		"HTTP_ADDR": ":8080", "HTTP_READ_TIMEOUT": "5s", "HTTP_READ_HEADER_TIMEOUT": "2s",
		"HTTP_WRITE_TIMEOUT": "10s", "HTTP_IDLE_TIMEOUT": "120s", "LOG_LEVEL": "info", "SHUTDOWN_TIMEOUT": "10s",
		"DATABASE_URL": "postgres://localhost/test", maxConnsVariable: "10", minConnsVariable: "0",
		"DATABASE_MAX_CONN_LIFETIME": "30m", "DATABASE_CONNECT_TIMEOUT": "5s", "DATABASE_QUERY_TIMEOUT": "3s",
		"OTEL_SERVICE_NAME": "trip-service", endpointVariable: "http://localhost:4318",
		"OTEL_EXPORTER_OTLP_PROTOCOL": "http/protobuf", resourceVariable: "service.namespace=tripgo,deployment.environment=local",
		"OTEL_TRACES_SAMPLER": "parentbased_always_on", "OTEL_TRACES_SAMPLER_ARG": "",
	} {
		t.Setenv(name, value)
	}
}

func TestLoadConfig(t *testing.T) {
	configure(t)
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DATABASE_MIN_CONNS != 0 || cfg.DATABASE_MAX_CONNS != 10 || cfg.OTEL_SERVICE_NAME != "trip-service" {
		t.Errorf("configuration fields were not loaded: %+v", cfg)
	}
}

func TestInvalidConfiguration(t *testing.T) {
	tests := []struct{ name, key, value string }{
		{"missing endpoint", endpointVariable, ""},
		{"relative endpoint", endpointVariable, "localhost:4318"},
		{"invalid endpoint scheme", endpointVariable, "ftp://localhost"},
		{"unsupported protocol", "OTEL_EXPORTER_OTLP_PROTOCOL", "json"},
		{"unsupported sampler", "OTEL_TRACES_SAMPLER", "random"},
		{"invalid resource attributes", resourceVariable, "broken"},
		{"missing resource namespace", resourceVariable, "deployment.environment=local"},
		{"missing resource environment", resourceVariable, "service.namespace=tripgo"},
		{"unknown log level", "LOG_LEVEL", "verbose"},
		{"negative maximum pool size", maxConnsVariable, "-1"},
		{"zero maximum pool size", maxConnsVariable, "0"},
		{"negative minimum pool size", minConnsVariable, "-1"},
		{"minimum exceeds maximum", minConnsVariable, "11"},
		{"int32 overflow", maxConnsVariable, "2147483648"},
		{"invalid duration", "DATABASE_QUERY_TIMEOUT", "invalid"},
		{"zero shutdown budget", "SHUTDOWN_TIMEOUT", "0s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configure(t)
			t.Setenv(tt.key, tt.value)
			if _, err := config.LoadConfig(); err == nil {
				t.Errorf("accepted invalid %s", tt.key)
			}
		})
	}
}

func TestSamplers(t *testing.T) {
	for _, sampler := range []string{"always_on", "always_off", "parentbased_always_on", "parentbased_always_off", "traceidratio", "parentbased_traceidratio"} {
		t.Run(sampler, func(t *testing.T) {
			configure(t)
			t.Setenv("OTEL_TRACES_SAMPLER", sampler)
			t.Setenv("OTEL_TRACES_SAMPLER_ARG", "0.25")
			cfg, err := config.LoadConfig()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.OTEL_TRACES_SAMPLER_ARG != "0.25" {
				t.Error("sampler argument was lost")
			}
		})
	}
	for _, ratio := range []string{"-0.1", "1.1", "NaN", "Inf", "invalid"} {
		t.Run(ratio, func(t *testing.T) {
			configure(t)
			t.Setenv("OTEL_TRACES_SAMPLER", "traceidratio")
			t.Setenv("OTEL_TRACES_SAMPLER_ARG", ratio)
			if _, err := config.LoadConfig(); err == nil || !strings.Contains(err.Error(), "OTEL_TRACES_SAMPLER_ARG") {
				t.Errorf("invalid ratio error = %v", err)
			}
		})
	}
}
