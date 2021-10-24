package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	botgolang "github.com/mail-ru-im/bot-golang"

	"github.com/z0rr0/mtbot/db"
)

// Main contains base configuration parameters.
type Main struct {
	BotURL   string `toml:"bot_url"`
	BotToken string `toml:"bot_token"`
	Database string `toml:"database"`
	Period   int    `toml:"period"`
	Debug     bool   `toml:"debug"`
}

// Workers is a struct of workers settings.
type Workers struct {
	User   int `toml:"user"`
	Notify int `toml:"notify"`
}

// Config is common configuration struct.
type Config struct {
	*db.Logger
	M Main      `toml:"main"`
	L db.Limits `toml:"limits"`
	W Workers   `toml:"workers"`
	Events  []*db.Event `toml:"events"`
	B       *botgolang.Bot
	Timeout time.Duration
	Period  time.Duration
}

// New returns new configuration.
func New(fileName string) (*Config, error) {
	fullPath, err := filepath.Abs(strings.Trim(fileName, " "))
	if err != nil {
		return nil, fmt.Errorf("config file: %w", err)
	}
	_, err = os.Stat(fullPath)
	if err != nil {
		return nil, fmt.Errorf("config existing: %w", err)
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("config read: %w", err)
	}
	c := &Config{}
	if err = toml.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("config parsing: %w", err)
	}
	if err = c.isValid(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}
	c.Period = time.Duration(c.M.Period) * time.Second

	bot, err := botgolang.NewBot(c.M.BotToken, botgolang.BotDebug(c.M.Debug), botgolang.BotApiURL(c.M.BotURL))
	if err != nil {
		return nil, fmt.Errorf("can not init bot: %w", err)
	}
	c.B = bot
	c.Logger = db.NewLogger(c.M.Debug)
	return c, nil
}

func (c *Config) initEvents() error {
	for i := range c.Events {
		err := c.Events[i].Init()
		if err != nil {
			return fmt.Errorf("event [%d]: %w", i, err)
		}
	}
	return nil
}

// isValid checks the Settings are valid.
func (c *Config) isValid() error {
	err := c.initEvents()
	err = isGreaterOrEqualThan(c.L.Users, 1, "limits.max_users", err)
	err = isGreaterOrEqualThan(c.L.Delays, 1, "limits.delays", err)
	err = isGreaterOrEqualThan(c.L.MinDelay, 1, "limits.min_delay", err)
	err = isGreaterOrEqualThan(c.L.MaxDelay, c.L.MinDelay, "limits.max_delay", err)
	err = isGreaterOrEqualThan(c.M.Period, 1, "main.period", err)
	err = isGreaterOrEqualThan(c.W.User, 1, "workers.user", err)
	err = isGreaterOrEqualThan(c.W.Notify, 1, "workers.notify", err)
	if err != nil {
		return fmt.Errorf("config validation: %w", err)
	}
	return nil
}

// isGreaterOrEqualThan returns error if err is already error or x is less than y.
func isGreaterOrEqualThan(x, y int, name string, err error) error {
	if err != nil {
		return err
	}
	if x < y {
		return fmt.Errorf("%s=%d should be greater than %d", name, x, y)
	}
	return nil
}
