package config

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/kazuki-oshino/prjev/internal/model"
	"github.com/kazuki-oshino/prjev/internal/rules"
	"gopkg.in/yaml.v3"
)

type raw struct {
	Version int `yaml:"version"`
	Jev     struct {
		Model          string `yaml:"model"`
		Concurrency    int    `yaml:"concurrency"`
		RequestTimeout string `yaml:"request_timeout"`
	} `yaml:"jev"`
	ExtraChecks []model.Rule `yaml:"extra_checks"`
}
type Config struct {
	Key            string
	Model          string
	Concurrency    int
	RequestTimeout time.Duration
	Rules          []model.Rule
}

func Load(configPath, envPath string, explicitEnv bool) (Config, error) {
	c := Config{Model: "jev-latest", Concurrency: 3, RequestTimeout: 8 * time.Second, Rules: rules.Standard()}
	if configPath != "" {
		b, e := os.ReadFile(configPath)
		if e != nil {
			return c, fmt.Errorf("設定ファイル: %w", e)
		}
		var r raw
		d := yaml.NewDecoder(bytes.NewReader(b))
		d.KnownFields(true)
		if e = d.Decode(&r); e != nil {
			return c, fmt.Errorf("設定ファイル: %w", e)
		}
		if r.Version != 1 {
			return c, fmt.Errorf("設定versionは1を指定してください")
		}
		if r.Jev.Model != "" {
			c.Model = r.Jev.Model
		}
		if r.Jev.Concurrency != 0 {
			c.Concurrency = r.Jev.Concurrency
		}
		if r.Jev.RequestTimeout != "" {
			var e error
			c.RequestTimeout, e = time.ParseDuration(r.Jev.RequestTimeout)
			if e != nil {
				return c, e
			}
		}
		c.Rules = append(c.Rules, r.ExtraChecks...)
	}
	if c.Concurrency < 1 || c.Concurrency > 3 || c.RequestTimeout <= 0 || c.RequestTimeout > 30*time.Second || c.Model == "" {
		return c, fmt.Errorf("Jev設定が不正です")
	}
	if e := rules.Validate(c.Rules); e != nil {
		return c, e
	}
	key := os.Getenv("TYPESAFE_API_KEY")
	if envPath == "" {
		envPath = ".env"
	}
	m, e := godotenv.Read(envPath)
	if e != nil && (explicitEnv || !os.IsNotExist(e)) {
		return c, fmt.Errorf("envファイル: %w", e)
	}
	if key == "" {
		key = m["TYPESAFE_API_KEY"]
	}
	if key == "" {
		return c, fmt.Errorf("TYPESAFE_API_KEYが設定されていません")
	}
	c.Key = key
	return c, nil
}
