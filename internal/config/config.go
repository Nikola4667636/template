package config

import (
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTP_ADDR                string        `env:"HTTP_ADDR"`
	HTTP_READ_TIMEOUT        time.Duration `env:"HTTP_READ_TIMEOUT"`
	HTTP_READ_HEADER_TIMEOUT time.Duration `env:"HTTP_READ_HEADER_TIMEOUT"`
	HTTP_WRITE_TIMEOUT       time.Duration `env:"HTTP_WRITE_TIMEOUT"`
	HTTP_IDLE_TIMEOUT        time.Duration `env:"HTTP_IDLE_TIMEOUT"`

	LOG_LEVEL                  string        `env:"LOG_LEVEL"`
	SHUTDOWN_TIMEOUT           time.Duration `env:"SHUTDOWN_TIMEOUT"`
	DATABASE_URL               string        `env:"DATABASE_URL"`
	DATABASE_MAX_CONNS         int32         `env:"DATABASE_MAX_CONNS"`
	DATABASE_MIN_CONNS         int32         `env:"DATABASE_MIN_CONNS"`
	DATABASE_MAX_CONN_LIFETIME time.Duration `env:"DATABASE_MAX_CONN_LIFETIME"`
	DATABASE_CONNECT_TIMEOUT   time.Duration `env:"DATABASE_CONNECT_TIMEOUT"`
	DATABASE_QUERY_TIMEOUT     time.Duration `env:"DATABASE_QUERY_TIMEOUT"`

	OTEL_SERVICE_NAME           string `env:"OTEL_SERVICE_NAME"`
	OTEL_EXPORTER_OTLP_ENDPOINT string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	OTEL_EXPORTER_OTLP_PROTOCOL string `env:"OTEL_EXPORTER_OTLP_PROTOCOL"`
	OTEL_RESOURCE_ATTRIBUTES    string `env:"OTEL_RESOURCE_ATTRIBUTES"`
	OTEL_TRACES_SAMPLER         string `env:"OTEL_TRACES_SAMPLER"`
	OTEL_TRACES_SAMPLER_ARG     string `env:"OTEL_TRACES_SAMPLER_ARG" optional:"true"`
}

var durationType = reflect.TypeOf(time.Duration(0))

func LoadConfig() (*Config, error) {
	_ = godotenv.Load(".env")
	conf := &Config{}

	v := reflect.ValueOf(conf).Elem()
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		key := field.Tag.Get("env")

		if key == "" {
			return nil, fmt.Errorf("field %s has no env tag", field.Name)
		}

		raw, err := getEnv(key)
		if field.Tag.Get("optional") == "true" {
			raw, err = os.Getenv(key), nil
		}
		if err != nil {
			return nil, fmt.Errorf("config field %s: %w", field.Name, err)
		}

		value := v.Field(i)
		if !value.CanSet() {
			return nil, fmt.Errorf("field %s cannot be set", field.Name)
		}

		if err := setField(value, raw); err != nil {
			return nil, fmt.Errorf("config field %s (%s): %w", field.Name, key, err)
		}
	}

	if err := validateConfig(conf); err != nil {
		return nil, err
	}

	return conf, nil
}

func validateConfig(conf *Config) error {
	v := reflect.ValueOf(conf).Elem()
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		value := v.Field(i)
		if field.Tag.Get("optional") == "true" && value.Kind() == reflect.String && value.String() == "" {
			continue
		}

		switch {
		case value.Kind() == reflect.String:
			if strings.TrimSpace(value.String()) == "" {
				return fmt.Errorf("config field %s cannot be empty", field.Name)
			}
		case value.Type() == durationType:
			if value.Int() <= 0 {
				return fmt.Errorf("config field %s must be greater than zero", field.Name)
			}
		case value.Kind() >= reflect.Int && value.Kind() <= reflect.Int64:
			if value.Int() < 0 || (value.Int() == 0 && field.Name != "DATABASE_MIN_CONNS") {
				return fmt.Errorf("config field %s must be greater than zero", field.Name)
			}
		}
	}

	if conf.DATABASE_MAX_CONNS < conf.DATABASE_MIN_CONNS {
		return fmt.Errorf("DATABASE_MAX_CONNS must be greater than or equal to DATABASE_MIN_CONNS")
	}

	var level slog.Level
	if err := level.UnmarshalText([]byte(conf.LOG_LEVEL)); err != nil {
		return fmt.Errorf("invalid LOG_LEVEL: %w", err)
	}
	return validateTelemetry(conf)
}

func validateTelemetry(conf *Config) error {
	if conf.OTEL_EXPORTER_OTLP_PROTOCOL != "http/protobuf" && conf.OTEL_EXPORTER_OTLP_PROTOCOL != "grpc" {
		return fmt.Errorf("OTEL_EXPORTER_OTLP_PROTOCOL must be http/protobuf or grpc")
	}
	endpoint, err := url.Parse(conf.OTEL_EXPORTER_OTLP_ENDPOINT)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return fmt.Errorf("OTEL_EXPORTER_OTLP_ENDPOINT must be an http or https URL")
	}
	switch conf.OTEL_TRACES_SAMPLER {
	case "always_on", "always_off", "parentbased_always_on", "parentbased_always_off":
	case "traceidratio", "parentbased_traceidratio":
		if conf.OTEL_TRACES_SAMPLER_ARG != "" {
			ratio, err := strconv.ParseFloat(conf.OTEL_TRACES_SAMPLER_ARG, 64)
			if err != nil || math.IsNaN(ratio) || ratio < 0 || ratio > 1 {
				return fmt.Errorf("OTEL_TRACES_SAMPLER_ARG must be a number between 0 and 1")
			}
		}
	default:
		return fmt.Errorf("unsupported OTEL_TRACES_SAMPLER %q", conf.OTEL_TRACES_SAMPLER)
	}
	attributes := make(map[string]string)
	for item := range strings.SplitSeq(conf.OTEL_RESOURCE_ATTRIBUTES, ",") {
		key, value, ok := strings.Cut(item, "=")
		if !ok || strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			return fmt.Errorf("OTEL_RESOURCE_ATTRIBUTES must contain comma-separated key=value pairs")
		}
		attributes[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	for _, key := range []string{"service.namespace", "deployment.environment"} {
		if attributes[key] == "" {
			return fmt.Errorf("OTEL_RESOURCE_ATTRIBUTES must include %s", key)
		}
	}
	return nil
}

func getEnv(key string) (string, error) {
	value, exists := os.LookupEnv(key)
	if !exists || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("environment variable %s is not set or empty", key)
	}
	return value, nil
}

func setField(value reflect.Value, raw string) error {
	switch value.Kind() {
	case reflect.String:
		value.SetString(raw)
		return nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if value.Type() == durationType {
			d, err := time.ParseDuration(raw)
			if err != nil {
				return fmt.Errorf("invalid duration %q: %w", raw, err)
			}
			value.SetInt(int64(d))
			return nil
		}

		n, err := strconv.ParseInt(raw, 10, value.Type().Bits())
		if err != nil {
			return fmt.Errorf("invalid integer %q: %w", raw, err)
		}
		value.SetInt(n)
		return nil

	default:
		return fmt.Errorf("unsupported type: %s", value.Type())
	}
}
