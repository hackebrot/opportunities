package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/BurntSushi/toml"
)

// Config holds the resolved configuration after merging defaults, the
// TOML config file, and OPPS_* environment overrides. The Email,
// Compensation, Ranking, and Dashboard blocks parse for forward
// compatibility — features that consume them ship later.
type Config struct {
	Locale       string             `toml:"locale"`
	Timezone     string             `toml:"timezone"`
	LogLevel     string             `toml:"-"`
	Database     DatabaseConfig     `toml:"database"`
	HTTP         HTTPConfig         `toml:"http"`
	Email        EmailConfig        `toml:"email"`
	Compensation CompensationConfig `toml:"compensation"`
	Ranking      RankingConfig      `toml:"ranking"`
	Dashboard    DashboardConfig    `toml:"dashboard"`
}

type DatabaseConfig struct {
	URL string `toml:"url"`
}

type HTTPConfig struct {
	Port int `toml:"port"`
}

type EmailConfig struct {
	Pattern string `toml:"pattern"`
}

type CompensationConfig struct {
	ReportingCurrency string             `toml:"reporting_currency"`
	BaseMinimum       int64              `toml:"base_minimum"`
	BaseTarget        int64              `toml:"base_target"`
	WeightBase        float64            `toml:"weight_base"`
	WeightBonus       float64            `toml:"weight_bonus"`
	WeightEquity      float64            `toml:"weight_equity"`
	FXRates           map[string]float64 `toml:"fx_rates"`
}

type RankingConfig struct {
	Priority      float64            `toml:"priority"`
	Source        float64            `toml:"source"`
	CompFit       float64            `toml:"comp_fit"`
	Freshness     float64            `toml:"freshness"`
	SourceWeights map[string]float64 `toml:"source_weights"`
}

type DashboardConfig struct {
	ActiveApplicationStaleAfter string `toml:"active_application_stale_after"`
	ExploringStaleAfter         string `toml:"exploring_stale_after"`
	WatchingStaleAfter          string `toml:"watching_stale_after"`
	DormantStaleAfter           string `toml:"dormant_stale_after"`
}

// Defaults returns the baseline config used when no file exists and no
// env overrides are set. Compensation/Ranking/Dashboard values are
// uniform placeholders; users supply tuned numbers in their config.toml.
func Defaults() Config {
	return Config{
		Locale:   "en-US",
		Timezone: "UTC",
		LogLevel: "info",
		Database: DatabaseConfig{
			URL: "postgres://localhost/opportunities_dev?sslmode=disable",
		},
		HTTP: HTTPConfig{Port: 8484},
		Email: EmailConfig{
			Pattern: "hello+{company_slug}@example.com",
		},
		Compensation: CompensationConfig{
			ReportingCurrency: "EUR",
			BaseMinimum:       0,
			BaseTarget:        0,
			WeightBase:        1.0,
			WeightBonus:       1.0,
			WeightEquity:      1.0,
			FXRates:           map[string]float64{"EUR": 1.0, "USD": 1.0},
		},
		Ranking: RankingConfig{
			Priority:  1.0,
			Source:    1.0,
			CompFit:   1.0,
			Freshness: 1.0,
			SourceWeights: map[string]float64{
				"referral":                   1.0,
				"network":                    1.0,
				"inbound_founder":            1.0,
				"inbound_employee":           1.0,
				"outbound":                   1.0,
				"inbound_inhouse_recruiter":  1.0,
				"inbound_external_recruiter": 1.0,
				"other":                      1.0,
			},
		},
		Dashboard: DashboardConfig{
			ActiveApplicationStaleAfter: "30d",
			ExploringStaleAfter:         "30d",
			WatchingStaleAfter:          "30d",
			DormantStaleAfter:           "30d",
		},
	}
}

// Load resolves the config file path from XDG/home and applies process
// environment overrides on top.
func Load() (Config, error) {
	path, err := DefaultPath()
	if err != nil {
		return Config{}, err
	}
	return LoadFrom(path, os.Getenv)
}

// LoadFrom merges defaults with the TOML at path (if present) and then
// applies env overrides via getenv. A nil getenv is treated as a no-op
// lookup. A non-existent path is not an error — the resulting config is
// defaults + env.
func LoadFrom(path string, getenv func(string) string) (Config, error) {
	cfg := Defaults()

	if path != "" {
		data, err := os.ReadFile(path)
		switch {
		case err == nil:
			if _, err := toml.Decode(string(data), &cfg); err != nil {
				return Config{}, fmt.Errorf("config: decode %s: %w", path, err)
			}
		case errors.Is(err, fs.ErrNotExist):
			// fall through; defaults stand
		default:
			return Config{}, fmt.Errorf("config: read %s: %w", path, err)
		}
	}

	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	if err := applyEnv(&cfg, getenv); err != nil {
		return Config{}, err
	}
	if cfg.HTTP.Port < 1 || cfg.HTTP.Port > 65535 {
		return Config{}, fmt.Errorf("config: http.port %d out of range (1-65535)", cfg.HTTP.Port)
	}
	return cfg, nil
}

func applyEnv(cfg *Config, getenv func(string) string) error {
	if v := getenv("OPPS_LOCALE"); v != "" {
		cfg.Locale = v
	}
	if v := getenv("OPPS_TIMEZONE"); v != "" {
		cfg.Timezone = v
	}
	if v := getenv("OPPS_DATABASE_URL"); v != "" {
		cfg.Database.URL = v
	}
	if v := getenv("OPPS_HTTP_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("config: OPPS_HTTP_PORT %q: %w", v, err)
		}
		if p < 1 || p > 65535 {
			return fmt.Errorf("config: OPPS_HTTP_PORT %d out of range (1-65535)", p)
		}
		cfg.HTTP.Port = p
	}
	if v := getenv("OPPS_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	return nil
}

// DefaultPath returns the conventional config file path:
// $XDG_CONFIG_HOME/opportunities/config.toml when XDG_CONFIG_HOME is set
// to an absolute path, otherwise ~/.config/opportunities/config.toml.
// Per the XDG Base Directory spec, a non-absolute XDG_CONFIG_HOME is
// invalid and is ignored.
func DefaultPath() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(x) {
		return filepath.Join(x, "opportunities", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("config: resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", "opportunities", "config.toml"), nil
}
