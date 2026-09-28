package validate

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTP_ADDR                  string        `env:"HTTP_ADDR"`
	LOG_LEVEL                  string        `env:"LOG_LEVEL"`
	SHUTDOWN_TIMEOUT           time.Duration `env:"SHUTDOWN_TIMEOUT"`
	DATABASE_URL               string        `env:"DATABASE_URL"`
	DATABASE_MAX_CONNS         int           `env:"DATABASE_MAX_CONNS"`
	DATABASE_MIN_CONNS         int           `env:"DATABASE_MIN_CONNS"`
	DATABASE_MAX_CONN_LIFETIME time.Duration `env:"DATABASE_MAX_CONN_LIFETIME"`
	DATABASE_CONNECT_TIMEOUT   time.Duration `env:"DATABASE_CONNECT_TIMEOUT"`
	DATABASE_QUERY_TIMEOUT     time.Duration `env:"DATABASE_QUERY_TIMEOUT"`
}

var durationType = reflect.TypeOf(time.Duration(0))

func LoadConfig() (*Config, error) {
	godotenv.Load(".env")
	conf := &Config{}

	v := reflect.ValueOf(conf).Elem()
	t := v.Type()

	// Загружаем значения из переменных окружения.
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		key := field.Tag.Get("env")

		if key == "" {
			return nil, fmt.Errorf(
				"field %s has no env tag",
				field.Name,
			)
		}

		raw, err := getEnv(key)
		if err != nil {
			return nil, fmt.Errorf(
				"config field %s: %w",
				field.Name,
				err,
			)
		}

		value := v.Field(i)

		if !value.CanSet() {
			return nil, fmt.Errorf(
				"field %s cannot be set",
				field.Name,
			)
		}

		if err := setField(value, raw); err != nil {
			return nil, fmt.Errorf(
				"config field %s (%s): %w",
				field.Name,
				key,
				err,
			)
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

		switch {
		case value.Type() == durationType:
			if value.Int() <= 0 {
				return fmt.Errorf(
					"config field %s must be greater than zero",
					field.Name,
				)
			}

		case value.Kind() == reflect.Int:
			if value.Int() <= 0 {
				return fmt.Errorf(
					"config field %s must be greater than zero",
					field.Name,
				)
			}
		}
	}

	if conf.DATABASE_MAX_CONNS < conf.DATABASE_MIN_CONNS {
		return fmt.Errorf(
			"DATABASE_MAX_CONNS must be greater than or equal to DATABASE_MIN_CONNS",
		)
	}

	return nil
}

func getEnv(key string) (string, error) {
	value, exists := os.LookupEnv(key)

	if !exists || value == "" {
		return "", fmt.Errorf(
			"environment variable %s is not set",
			key,
		)
	}

	return value, nil
}

func setField(value reflect.Value, raw string) error {
	switch value.Kind() {
	case reflect.String:
		value.SetString(raw)
		return nil

	case reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64:

		if value.Type() == durationType {
			d, err := time.ParseDuration(raw)
			if err != nil {
				return fmt.Errorf(
					"invalid duration %q: %w",
					raw,
					err,
				)
			}

			value.SetInt(int64(d))
			return nil
		}

		n, err := strconv.ParseInt(
			raw,
			10,
			value.Type().Bits(),
		)
		if err != nil {
			return fmt.Errorf(
				"invalid integer %q: %w",
				raw,
				err,
			)
		}

		value.SetInt(n)
		return nil

	default:
		return fmt.Errorf(
			"unsupported type: %s",
			value.Type(),
		)
	}
}
